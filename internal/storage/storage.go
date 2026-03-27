package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/unibo-toolkit/calendar-manager-service/internal/config"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage/db"
)

type Storage struct {
	Shutdown func()
	*db.Queries
	Pool *pgxpool.Pool
}

func New(cfg *config.DBConfig) *Storage {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port, cfg.Name)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		panic("failed to parse db config: " + err.Error())
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = time.Duration(cfg.MaxConnLifetime)
	poolCfg.MaxConnIdleTime = time.Duration(cfg.MaxConnIdleTime)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		panic("failed to connect to db: " + err.Error())
	}

	if err = pool.Ping(context.Background()); err != nil {
		panic("failed to ping db: " + err.Error())
	}

	return &Storage{
		Shutdown: pool.Close,
		Queries:  db.New(pool),
		Pool:     pool,
	}
}
