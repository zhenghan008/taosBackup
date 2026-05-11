package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"taosBackup/taos"
	"time"
)

func maxNum(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func MergeStringsToMap(stables, otables string) map[string]int8 {
	var partsA, partsB []string

	if strings.Contains(stables, ",") {
		partsA = strings.Split(stables, ",")
	} else if strings.TrimSpace(stables) != "" {
		partsA = []string{stables}
	}

	if strings.Contains(otables, ",") {
		partsB = strings.Split(otables, ",")
	} else if strings.TrimSpace(otables) != "" {
		partsB = []string{otables}
	}
	res := make(map[string]int8, len(partsA)+len(partsB))
	fillMap(res, partsA, 0)
	fillMap(res, partsB, 1)
	return res
}

func fillMap(m map[string]int8, parts []string, val int8) {
	for _, s := range parts {
		key := strings.TrimSpace(s)
		if key != "" {
			m[key] = val
		}
	}
}

func main() {
	consoleLogger := taos.Logger(false, true)
	fileLogger := taos.Logger(true, false)
	defer consoleLogger.Sync()
	defer fileLogger.Sync()
	consoleSugarLog := consoleLogger.Sugar()
	fileSugarLog := fileLogger.Sugar()
	defaultWorker := maxNum(1, runtime.NumCPU()/3)
	taosAddrs := flag.String("h", "localhost:6041", "taos addresses, e.g., taos:6041.")
	taosUser := flag.String("u", "root", "taos username.")
	taosPass := flag.String("p", "taosdata", "taos password.")
	taosDatabase := flag.String("d", "", "taos database.")
	limitWorker := flag.Int("w", defaultWorker, "limit workers, it is recommended not to exceed the number of CPU cores of the server.the default is one-third of the server CPU.")
	backupFull := flag.Bool("f", false, "whether to back up all or not, if false, is an incremental backup, and the incremental time is from the zero point of the previous day to the current backup time.")
	maxRowsCvs := flag.Int("r", 100000, "the maximum number of lines written to each csv file.")
	backupPath := flag.String("b", "/data/taos_Backup", "the backup path of the taos database.")
	model := flag.String("m", "e", "backup mode or recovery mode, where e stands for backup mode and i represents recovery mode, you must create a super table structure before using recovery mode.")
	stables := flag.String("s", "", "specifies the name of the super tables,Multiple table names are separated by commas, e.g., stableNameA,stableNameB,... or stableNameA.")
	otables := flag.String("o", "", "specifies the name of the Ordinary tables,Multiple table names are separated by commas,  e.g., otableNameA,otableNameB,... or otableNameA.")
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
	specifiedTables := MergeStringsToMap(*stables, *otables)
	if *model == "e" {
		err = taos.ExportAllTables(taosDb, *taosDatabase, *limitWorker, *backupPath, *backupFull, *maxRowsCvs, specifiedTables, consoleSugarLog)
		if err != nil {
			consoleSugarLog.Errorw("taos get stable table failed", "error", err)
		}
	} else if *model == "i" {
		err = taos.BatchImport(taosDb, *limitWorker, *backupPath, consoleSugarLog, fileSugarLog)
		if err != nil {
			consoleSugarLog.Errorw("taos import failed", "error", err)
		}
	} else {
		consoleSugarLog.Errorw("model is invalid, only e or i", "model", *model)
	}

	defer taosDb.Close()
}
