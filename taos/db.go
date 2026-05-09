package taos

import (
	"context"
	"database/sql"
	_ "github.com/taosdata/driver-go/v3/taosWS"
	"go.uber.org/zap"
	"time"
)

type DBConfig struct {
	TaosUri     string
	MaxOpen     int
	MaxIdle     int
	MaxLifetime time.Duration
}

// InitMySQL  Initialize the database connection pool
func (dbc *DBConfig) InitMySQL(log *zap.SugaredLogger) (*sql.DB, error) {
	db, err := sql.Open("taosWS", dbc.TaosUri)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(dbc.MaxOpen)
	db.SetMaxIdleConns(dbc.MaxIdle)
	db.SetConnMaxLifetime(dbc.MaxLifetime)
	db.SetConnMaxIdleTime(1 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}
	log.Infow("Taos pool initialized", "max_open", dbc.MaxOpen, "max_idle", dbc.MaxIdle)
	return db, nil
}
