package taos

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"github.com/klauspost/pgzip"
	"github.com/panjf2000/ants/v2"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	fetchDataBatchSize = 50000
	batchSize          = 1000
	rowPoolSize        = 32
	channelBuffer      = 2000
	blockSize          = 1024 * 1024
	maxBatchBytes      = 800 * 1024
	maxRetries         = 1
)

type RowBatch struct {
	Data []RowPackage
}

type RowPackage struct {
	TableName string
	Data      []string
}

// Global object pool, reusing memory
var (
	batchPool = sync.Pool{
		New: func() interface{} { return make([]RowPackage, 0, batchSize) },
	}
	rowPool = sync.Pool{
		New: func() interface{} { return make([]string, 0, rowPoolSize) },
	}

	bufferPool = sync.Pool{
		New: func() interface{} {
			return bytes.NewBuffer(make([]byte, 0, maxBatchBytes+1024))
		},
	}
)

var (
	quotedJSONRegex = regexp.MustCompile(`"{.*?}"`)
	signedNumRegex  = regexp.MustCompile(`^-?[0-9]+$`)
	tagNumRegex     = regexp.MustCompile(`(?i)TAGS\s*\((.*)\)`)
)

// ExportAllTables Export all data
func ExportAllTables(db *sql.DB, dbName string, limitCores int, workPath string, isFull bool, toCsvRows int, specifiedTables map[string]int8, log *zap.SugaredLogger) error {
	eg, ctx := errgroup.WithContext(context.Background())
	start := time.Now()
	globalQueryPool, _ := ants.NewPool(limitCores*4, ants.WithPreAlloc(true))
	defer globalQueryPool.Release()
	stableWorkerPool, _ := ants.NewPool(limitCores, ants.WithPreAlloc(true))
	defer stableWorkerPool.Release()
	otableWorkerPool, _ := ants.NewPool(limitCores, ants.WithPreAlloc(true))
	defer otableWorkerPool.Release()
	var stableWg sync.WaitGroup
	var otableWg sync.WaitGroup
	switch {
	case len(specifiedTables) != 0:
		for tb, ty := range specifiedTables {
			stableWg.Add(1)
			err := stableWorkerPool.Submit(func() {
				defer stableWg.Done()
				if ctx.Err() != nil {
					return
				}
				err := ExportDataByTable(ctx, db, globalQueryPool, tb, ty, workPath, isFull, toCsvRows, log)
				if err != nil {
					log.Errorw("ExportAllTables export error", "error", err, "stables", tb)
					return
				}
			})
			if err != nil {
				stableWg.Done()
				log.Errorw("ExportAllTables submit task failed", "error", err, "taskid", tb)
				continue
			}
			log.Infow("ExportAllTables tasks submit successfully", "taskid", tb)
		}
		stableWg.Wait()
	default:
		for i := 0; i < 2; i++ {
			eg.Go(func() error {
				if i == 0 {
					rows, err := db.QueryContext(ctx, "show STABLES")
					if err != nil {
						return err
					}
					defer rows.Close()
					for rows.Next() {
						var tableName string
						if err := rows.Scan(&tableName); err != nil {
							log.Errorw("ExportAllTables row scan error", "error", err, "stables", tableName)
							continue
						}
						stableWg.Add(1)
						err := stableWorkerPool.Submit(func() {
							defer stableWg.Done()
							if ctx.Err() != nil {
								return
							}
							err := ExportDataByTable(ctx, db, globalQueryPool, tableName, int8(0), workPath, isFull, toCsvRows, log)
							if err != nil {
								log.Errorw("ExportAllTables export error", "error", err, "stables", tableName)
							}
						})

						if err != nil {
							stableWg.Done()
							log.Errorw("ExportAllTables submit task failed", "error", err, "taskid", tableName)
							continue
						}
						log.Infow("ExportAllTables tasks submit successfully", "taskid", tableName)

					}
					stableWg.Wait()
				} else {
					rows, err := db.QueryContext(ctx, "SELECT table_name FROM INFORMATION_SCHEMA.INS_TABLES  WHERE DB_NAME = '?' AND type = 'NORMAL_TABLE'", dbName)
					if err != nil {
						return err
					}
					defer rows.Close()
					for rows.Next() {
						var tableName string
						if err := rows.Scan(&tableName); err != nil {
							log.Errorw("ExportAllTables row scan error", "error", err, "stables", tableName)
							continue
						}
						otableWg.Add(1)
						err := otableWorkerPool.Submit(func() {
							defer otableWg.Done()
							if ctx.Err() != nil {
								return
							}
							err := ExportDataByTable(ctx, db, globalQueryPool, tableName, int8(1), workPath, isFull, toCsvRows, log)
							if err != nil {
								log.Errorw("ExportAllTables export error", "error", err, "stables", tableName)
							}
						})

						if err != nil {
							otableWg.Done()
							log.Errorw("ExportAllTables submit task failed", "error", err, "taskid", tableName)
							continue
						}
						log.Infow("ExportAllTables tasks submit successfully", "taskid", tableName)

					}
					otableWg.Wait()
				}

				return nil
			})

		}
		if err := eg.Wait(); err != nil {
			log.Errorw("ExportAllTables failed!", "error", err, "cost", time.Since(start))
			return err
		}

	}
	log.Infof("ExportAllTables completed, total time elapsed: %v", time.Since(start))
	return nil

}

func ExportDataByTable(ctx context.Context, db *sql.DB, queryPool *ants.Pool, tableName string, tableType int8, workPath string, isFull bool, toCsvRows int, log *zap.SugaredLogger) error {
	batchChan := make(chan RowBatch, batchSize/10)
	eg, _ := errgroup.WithContext(context.Background())

	// Start concurrent file writer (Consumer)
	eg.Go(func() error {
		return sequentialFileWriterRoutine(ctx, batchChan, workPath, tableName, toCsvRows, log)
	})

	eg.Go(func() error {
		defer close(batchChan)
		var producerWg sync.WaitGroup
		if tableType == 0 {
			//Retrieve all sub-tables containing data
			rows, err := db.QueryContext(ctx, "SELECT tbname FROM ?", tableName)
			if err != nil {
				return fmt.Errorf("ExportDataByTable query sub-tables failed: %w", err)
			}
			defer rows.Close()
			for rows.Next() {
				var subTableName string
				if err := rows.Scan(&subTableName); err != nil {
					log.Errorw("ExportDataByTable row scan error", "error", err)
					continue
				}
				tasks, err := buildFetchTasks(ctx, db, subTableName, tableName, tableType, isFull, log)
				if err != nil {
					log.Errorw("ExportDataByTable build tasks failed: %w", "error", err)
					continue
				}
				for _, taskRange := range tasks {
					tr := taskRange
					tb := subTableName
					producerWg.Add(1)
					err := queryPool.Submit(func() {
						defer producerWg.Done()
						if ctx.Err() != nil {
							return
						}
						if err := fetchDataInBatches(ctx, db, tb, tableName, tableType, tr.Start, tr.End, batchChan); err != nil {
							log.Errorw("fetch data failed", "tb", tb, "error", err)
						}
					})
					if err != nil {
						producerWg.Done()
						log.Errorw("ExportDataByTable submit task failed", "error", err, "taskid", tableName+"_"+tb+"_"+strconv.FormatInt(tr.Start, 10)+"-"+strconv.FormatInt(tr.End, 10))

					}

				}

			}

		} else {
			tasks, err := buildFetchTasks(ctx, db, tableName, tableName, tableType, isFull, log)
			if err != nil {
				log.Errorw("ExportDataByTable build tasks failed: %w", "error", err)
			}
			for _, taskRange := range tasks {
				tr := taskRange
				tb := tableName
				producerWg.Add(1)
				err := queryPool.Submit(func() {
					defer producerWg.Done()
					if ctx.Err() != nil {
						return
					}
					if err := fetchDataInBatches(ctx, db, tb, tb, tableType, tr.Start, tr.End, batchChan); err != nil {
						log.Errorw("fetch data failed", "tb", tb, "error", err)
					}
				})
				if err != nil {
					producerWg.Done()
					log.Errorw("ExportDataByTable submit task failed", "error", err, "taskid", tb+"_"+strconv.FormatInt(tr.Start, 10)+"-"+strconv.FormatInt(tr.End, 10))

				}

			}

		}

		producerWg.Wait()
		return nil
	})

	if err := eg.Wait(); err != nil {
		log.Errorw("ExportTablesByStable Export process terminated with error", "error", err, "taskid", tableName)
		return err
	}

	log.Infow("ExportTablesByStable All export tasks completed successfully", "taskid", tableName)
	return nil
}

func fetchDataInBatches(ctx context.Context, db *sql.DB, tbName string, stableName string, tableType int8, start int64, end int64, batchChan chan<- RowBatch) error {
	var (
		rows *sql.Rows
	)
	if tableType == 0 {
		dbRows, err := db.QueryContext(ctx, "SELECT * FROM ? WHERE tbname = '?' AND ts between ? and ?", stableName, tbName, start, end)
		if err != nil {
			return fmt.Errorf("query failed for table %s: %v", tbName, err)
		}
		rows = dbRows
	} else {
		dbRows, err := db.QueryContext(ctx, "SELECT * FROM ? WHERE ts between ? and ?", tbName, start, end)
		if err != nil {
			return fmt.Errorf("query failed for table %s: %v", tbName, err)
		}
		rows = dbRows
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	colCount := len(cols)
	currentBatch := batchPool.Get().([]RowPackage)[:0]
	values := make([]sql.RawBytes, colCount)
	scanArgs := make([]interface{}, colCount)
	for i := range values {
		scanArgs[i] = &values[i]
	}

	for rows.Next() {
		select {
		case <-ctx.Done():
			return fmt.Errorf("ctx Done")
		default:
		}

		if err := rows.Scan(scanArgs...); err != nil {
			continue
		}

		rowData := rowPool.Get().([]string)
		if cap(rowData) < colCount+1 {
			rowData = make([]string, colCount+1)
		} else {
			rowData = rowData[:colCount+1]
		}

		rowData[0] = tbName
		for i, col := range values {
			if col == nil {
				rowData[i+1] = "NULL"
			} else {
				rowData[i+1] = string(col)
			}
		}

		currentBatch = append(currentBatch, RowPackage{TableName: tbName, Data: rowData})

		if len(currentBatch) >= fetchDataBatchSize {
			select {
			case batchChan <- RowBatch{Data: currentBatch}:
				currentBatch = batchPool.Get().([]RowPackage)[:0]
			case <-ctx.Done():
				return nil
			}
		}
	}

	if len(currentBatch) > 0 {
		select {
		case batchChan <- RowBatch{Data: currentBatch}:
		case <-ctx.Done():
		}
	} else {
		batchPool.Put(currentBatch)
	}
	return nil
}

func buildFetchTasks(ctx context.Context, db *sql.DB, tbName string, stableName string, tableType int8, isFull bool, log *zap.SugaredLogger) ([]*TimeRange, error) {
	var (
		startMs      int64
		endMs        int64
		intervalDays int
		tr           []*TimeRange
	)
	if isFull {
		var t time.Time
		if tableType == 0 {
			err := db.QueryRowContext(ctx, "SELECT ts FROM ? WHERE tbname = '?'  ORDER BY ts limit 1", stableName, tbName).Scan(&t)
			if err != nil {
				log.Errorw("query tbName ts failed", "error", err, "tbName", tbName)
				return tr, err
			}
		} else {
			err := db.QueryRowContext(ctx, "SELECT ts FROM ? ORDER BY ts limit 1", tbName).Scan(&t)
			if err != nil {
				log.Errorw("query tbName ts failed", "error", err, "tbName", tbName)
				return tr, err
			}
		}
		startMs = t.UnixMilli()
		endMs = 0
		intervalDays = 3
	} else {
		now := time.Now().UnixMilli()
		t := time.UnixMilli(now)
		lastDay := time.Date(t.Year(), t.Month(), t.Day()-1, 0, 0, 0, 0, t.Location())
		startMs = lastDay.UnixMilli()
		endMs = now
		intervalDays = 1
	}
	tr = SplitTimeRangeByDay(startMs, endMs, intervalDays)
	return tr, nil
}

// sequentialFileWriterRoutine Write files serially, utilizing pgzip's internal concurrent compression
func sequentialFileWriterRoutine(ctx context.Context, batchChan <-chan RowBatch, workPath, stableName string, maxRows int, log *zap.SugaredLogger) error {
	var (
		fileIdx     = 1
		currentRows = 0
		file        *os.File
		gzWriter    *pgzip.Writer
		csvWriter   *csv.Writer
		dateStr     = time.Now().Format("200601021504")
		dirName     = filepath.Join(workPath, fmt.Sprintf("%s", stableName))
	)

	if err := os.MkdirAll(dirName, 0755); err != nil {
		log.Errorw("mkdir failed: %w", "error", err)
		return err
	}

	rotate := func() error {
		if csvWriter != nil {
			csvWriter.Flush()
			gzWriter.Close()
			file.Close()
		}
		f, err := os.Create(filepath.Join(dirName, fmt.Sprintf("%s-%s_part%d.csv.gz", stableName, dateStr, fileIdx)))
		if err != nil {
			return err
		}
		file = f
		gzWriter, _ = pgzip.NewWriterLevel(file, pgzip.BestSpeed)
		// Set block size and number of concurrent workers
		_ = gzWriter.SetConcurrency(blockSize, 4)
		csvWriter = csv.NewWriter(gzWriter)
		fileIdx++
		currentRows = 0
		return nil
	}

	defer func() {
		if csvWriter != nil {
			csvWriter.Flush()
			gzWriter.Close()
			file.Close()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batch, ok := <-batchChan:
			if !ok {
				return nil
			}

			if file == nil || currentRows >= maxRows {
				if err := rotate(); err != nil {
					log.Errorw("file rotate failed", "error", err)
					return err
				}
			}
			rowsToWrite := make([][]string, 0, len(batch.Data))
			for _, p := range batch.Data {
				rowsToWrite = append(rowsToWrite, p.Data)
			}
			if err := csvWriter.WriteAll(rowsToWrite); err != nil {
				log.Errorw("csv batch write failed", "error", err)
				return err
			}
			for _, p := range batch.Data {
				rowPool.Put(p.Data)
			}
			currentRows += len(batch.Data)
			batchPool.Put(batch.Data[:0])
		}
	}
}

func BatchImport(db *sql.DB, limitCores int, workPath string, consoleLog *zap.SugaredLogger, fileLog *zap.SugaredLogger) error {
	eg, ctx := errgroup.WithContext(context.Background())
	start := time.Now()
	fileChan := make(chan string, channelBuffer)
	var WorkerCount = limitCores * 4

	// Start consumer
	for i := 0; i < WorkerCount; i++ {
		eg.Go(func() error {
			err := importWorker(ctx, i, db, fileChan, consoleLog, fileLog)
			if err != nil {
				consoleLog.Errorw("import worker failed", "error", err, "id", i)
				return nil
			}
			return nil
		})
	}

	// Start the producer (catalog scan)
	eg.Go(func() error {
		count := 0
		err := filepath.WalkDir(workPath, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".gz") {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case fileChan <- path:
				count++
			}
			return nil
		})
		if err != nil {
			consoleLog.Errorw("Error scanning directory", "error", err)
			return err
		}
		consoleLog.Infof("Scan complete: Number of tasks submitted: %d", count)
		defer close(fileChan)
		return nil

	})

	if err := eg.Wait(); err != nil {
		consoleLog.Errorw("Import task interrupted", "error", err, "cost", time.Since(start))
		return err
	}

	consoleLog.Infof("All tasks completed, total time elapsed: %v", time.Since(start))
	return nil
}

// getTagsNum Get the number of tags in the super table
func getTagsNum(ctx context.Context, db *sql.DB, stableName string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var resultSql string
	var tableName string
	err := db.QueryRowContext(ctx, "SHOW CREATE TABLE ?", stableName).Scan(&tableName, &resultSql)
	if err != nil {
		return 0, fmt.Errorf("query failed for table %s: %v", stableName, err)
	}
	match := tagNumRegex.FindStringSubmatch(resultSql)
	if len(match) == 0 {
		return 0, nil
	}
	tagsContent := match[1]
	fields := strings.Split(tagsContent, ",")
	return len(fields), nil
}

func formatValueInCsv(values []string) []string {
	for i := 0; i < len(values); i++ {
		if i == 0 {
			values[0] = fmt.Sprintf("'%s'", values[0])
		} else if quotedJSONRegex.MatchString(values[i]) {
			values[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(values[i], `""`, `"`))
		} else if !signedNumRegex.MatchString(values[i]) && values[i] != "NULL" {
			values[i] = fmt.Sprintf("'%s'", values[i])
		}
	}

	return values
}

func importWorker(ctx context.Context, id int, db *sql.DB, paths <-chan string, consoleLog *zap.SugaredLogger, fileLog *zap.SugaredLogger) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case path, ok := <-paths:
			if !ok {
				return nil
			}
			stableName := filepath.Base(filepath.Dir(path))
			tagNum, err := getTagsNum(ctx, db, stableName)
			if err != nil {
				return err
			}
			if err := processSingleGzFile(ctx, path, stableName, tagNum, db, consoleLog, fileLog); err != nil {
				consoleLog.Errorf("[Worker %d] faild %s: %v", id, path, err)
				return err
			}
		}
	}
}

func processSingleGzFile(ctx context.Context, filePath string, stableName string, tagNum int, db *sql.DB, consoleLog *zap.SugaredLogger, fileLog *zap.SugaredLogger) error {
	eg, _ := errgroup.WithContext(context.Background())
	batchChan := make(chan string, batchSize)
	errChan := make(chan error, 1)
	if err := ctx.Err(); err != nil {
		close(batchChan)
		close(errChan)
		return err
	}
	eg.Go(func() error {
		defer close(batchChan)
		f, err := os.Open(filePath)
		if err != nil {
			return err
		}
		defer f.Close()

		gzReader, err := pgzip.NewReaderN(f, blockSize, 4)
		if err != nil {
			return err
		}
		defer gzReader.Close()
		sqlBuf := bufferPool.Get().(*bytes.Buffer)
		sqlBuf.Reset()
		defer bufferPool.Put(sqlBuf)

		rowCount := 0
		var (
			rowData string
			tagData string
		)
		reader := csv.NewReader(gzReader)
		reader.LazyQuotes = true
		for {
			record, err := reader.Read()
			if err != nil {
				if err.Error() == "EOF" {
					if sqlBuf.Len() > 0 {
						batchChan <- sqlBuf.String()
					}
					return nil
				}
				consoleLog.Warnf("read csv err, error: %v, filePath: %v, record: %v", err, filePath, record)
				continue
			}
			for i, field := range record {
				record[i] = strings.TrimSpace(field)
			}
			subTableName := record[0]
			if sqlBuf.Len() == 0 {
				sqlBuf.WriteString("INSERT INTO ")
			} else {
				sqlBuf.WriteString(" ")
			}
			formatFields := formatValueInCsv(record[1:])
			if tagNum > 0 {
				totalFields := len(formatFields)
				if tagNum > totalFields {
					consoleLog.Warnf("Error: tagNum (%d) is outside the valid field range (1-%d)", tagNum, totalFields-1)
					continue
				}
				splitIdx := totalFields - tagNum
				rowData = strings.Join(formatFields[:splitIdx], ",")
				tagData = strings.Join(formatFields[splitIdx:], ",")
				sqlBuf.WriteString(subTableName)
				sqlBuf.WriteString(" USING ")
				sqlBuf.WriteString(stableName)
				sqlBuf.WriteString(" TAGS (")
				sqlBuf.WriteString(tagData)
				sqlBuf.WriteString(") VALUES (")
				sqlBuf.WriteString(rowData)
				sqlBuf.WriteString(")")
			} else {
				rowData = strings.Join(formatFields, ",")
				sqlBuf.WriteString(subTableName)
				sqlBuf.WriteString(" VALUES (")
				sqlBuf.WriteString(rowData)
				sqlBuf.WriteString(")")
			}

			rowCount++
			//Double restrictions: line count limit + byte count limit to prevent data loss
			if rowCount >= batchSize || sqlBuf.Len() >= maxBatchBytes {
				batchChan <- sqlBuf.String()
				sqlBuf.Reset()
				rowCount = 0
			}
		}
	})

	eg.Go(func() error {
		defer close(errChan)
		for batch := range batchChan {
			if err := executeBatchWithRetry(ctx, db, batch, filePath, consoleLog, fileLog); err != nil {
				errChan <- err
			}
		}
		errChan <- nil
		return <-errChan
	})

	if err := eg.Wait(); err != nil {
		consoleLog.Errorw("processSingleGzFile process terminated with error", "error", err, "filePath", filePath)
		return err
	}

	consoleLog.Infow("processSingleGzFile tasks completed successfully", "filePath", filePath)
	return nil
}

func executeBatchWithRetry(ctx context.Context, db *sql.DB, query string, filePath string, consoleLog *zap.SugaredLogger, fileLog *zap.SugaredLogger) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		if err := ctx.Err(); err != nil {
			return err
		}

		_, err = db.ExecContext(ctx, query)
		if err == nil {
			return nil
		}

		consoleLog.Warnw("Writing to the database failed,preparing to retry", "retry", i+1, "error", err)
		time.Sleep(time.Duration(100<<i) * time.Millisecond)
	}
	fileLog.Warnw("Failed to write to database", "error", err, "filePath", filePath, "sql", query)
	return err
}
