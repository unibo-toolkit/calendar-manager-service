package app

import (
	"log/slog"
	"os"

	httpapp "github.com/unibo-toolkit/calendar-manager-service/internal/app/http"
	"github.com/unibo-toolkit/calendar-manager-service/internal/config"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage"
)

func Run() {
	cfg := config.MustLoad()

	level := slog.LevelInfo
	if cfg.HTTP.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level}))

	log.Info("starting calendar manager",
		"environment", cfg.HTTP.Environment,
		"log_level", cfg.HTTP.LogLevel,
		"port", cfg.HTTP.Port,
	)

	log.Info("connecting to database", "host", cfg.DB.Host, "port", cfg.DB.Port, "db", cfg.DB.Name)
	st := storage.New(log, &cfg.DB)
	log.Info("database connected")
	defer func() {
		log.Info("closing database connection")
		st.Shutdown()
		log.Info("database connection closed")
	}()

	httpapp.Start(log, cfg, st)
	log.Info("calendar-manager-service stopped")
}
