package storage

import (
	"context"
	"fmt"
	"log/slog"
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

func New(log *slog.Logger, cfg *config.DBConfig) *Storage {
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
		cfg.User, cfg.Pass, cfg.Host, cfg.Port, cfg.Name)

	poolCfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		log.Error("failed to parse db config", "error", err)
		panic("failed to parse db config: " + err.Error())
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = time.Duration(cfg.MaxConnLifetime)
	poolCfg.MaxConnIdleTime = time.Duration(cfg.MaxConnIdleTime)

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		log.Error("failed to create db pool", "error", err)
		panic("failed to connect to db: " + err.Error())
	}

	if err = pool.Ping(context.Background()); err != nil {
		log.Error("failed to ping db", "error", err)
		panic("failed to ping db: " + err.Error())
	}

	log.Info("db pool initialized", "max_conns", cfg.MaxConns, "min_conns", cfg.MinConns)

	return &Storage{
		Shutdown: pool.Close,
		Queries:  db.New(pool),
		Pool:     pool,
	}
}
