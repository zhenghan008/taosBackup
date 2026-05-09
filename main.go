package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/panjf2000/ants/v2"
	"golang.org/x/sync/errgroup"
	"os"
	"runtime"
	"taosBackup/taos"
	"time"
)

func maxNum(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func main() {
	consoleLogger := taos.Logger(false, true)
	fileLogger := taos.Logger(true, false)
	defer consoleLogger.Sync()
	defer fileLogger.Sync()
	consoleSugarLog := consoleLogger.Sugar()
	fileSugarLog := fileLogger.Sugar()
	defaultWorker := maxNum(1, runtime.NumCPU()/3)
	taosAddrs := flag.String("h", "localhost:6041", "taos addresses, e.g., taso:6041.")
	taosUser := flag.String("u", "root", "taos username.")
	taosPass := flag.String("p", "taosdata", "taos password.")
	taosDatabase := flag.String("d", "", "taos database.")
	limitWorker := flag.Int("w", defaultWorker, "limit workers, it is recommended not to exceed the number of CPU cores of the server.the default is one-third of the server CPU.")
	backupFull := flag.Bool("f", false, "whether to back up all or not, if false, is an incremental backup, and the incremental time is from the zero point of the previous day to the current backup time.")
	maxRowsCvs := flag.Int("r", 100000, "the maximum number of lines written to each csv file.")
	backupPath := flag.String("b", "/data/taosBackup", "the backup path of the taos database.")
	model := flag.String("m", "e", "backup mode or recovery mode, where e stands for backup mode and i represents recovery mode, you must create a super table structure before using recovery mode.")
	stable := flag.String("s", "", "specifies the name of the supertable.")
	flag.Parse()
	runtime.GOMAXPROCS(*limitWorker)
	if err := os.MkdirAll(*backupPath, 0755); err != nil {
		consoleSugarLog.Errorw("create backup path failed!", "error", err)
		return
	}
	consoleSugarLog.Infow("CPU resources restricted",
		"total_cores", runtime.NumCPU(),
		"limit_cores", *limitWorker,
		"backup_path", *backupPath,
	)
	taosCfg := &taos.DBConfig{
		TaosUri:     fmt.Sprintf("%s:%s@ws(%s)/%s", *taosUser, *taosPass, *taosAddrs, *taosDatabase),
		MaxOpen:     *limitWorker * 10,
		MaxIdle:     *limitWorker * 5,
		MaxLifetime: 3 * time.Minute,
	}
	taosDb, err := taosCfg.InitMySQL(consoleSugarLog)
	if err != nil {
		consoleSugarLog.Errorw("taos init failed", "error", err)
		return
	}
	if *model != "e" && *model != "i" {
		consoleSugarLog.Errorw("model is invalid, only e or i", "model", *model)
		return
	}

	if *model == "e" && *stable == "" {
		err = taos.ExportAllTables(taosDb, *limitWorker, *backupPath, *backupFull, *maxRowsCvs, consoleSugarLog)
		if err != nil {
			consoleSugarLog.Errorw("taos get stable table failed", "error", err)
		}
	} else if *model == "i" {
		err = taos.BatchImport(taosDb, *limitWorker, *backupPath, consoleSugarLog, fileSugarLog)
		if err != nil {
			consoleSugarLog.Errorw("taos import failed", "error", err)
		}
	}
	if *model == "e" && *stable != "" {
		_, ctx := errgroup.WithContext(context.Background())
		globalQueryPool, _ := ants.NewPool(*limitWorker*4, ants.WithPreAlloc(true))
		defer globalQueryPool.Release()
		err = taos.ExportTablesByStable(ctx, taosDb, globalQueryPool, *stable, *backupPath, *backupFull, *maxRowsCvs, consoleSugarLog)
		if err != nil {
			consoleSugarLog.Errorw("taos export failed", "error", err)
		}
	}

	defer taosDb.Close()
}
