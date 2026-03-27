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

	st := storage.New(&cfg.DB)
	defer st.Shutdown()

	httpapp.Start(log, cfg, st)
}
