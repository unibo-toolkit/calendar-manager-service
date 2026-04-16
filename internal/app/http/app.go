package http

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/unibo-toolkit/calendar-manager-service/internal/config"
	"github.com/unibo-toolkit/calendar-manager-service/internal/http/calendar"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage"
)

func Start(log *slog.Logger, cfg *config.Config, st *storage.Storage) {
	srv := calendar.New(log, cfg, st)

	go func() {
		log.Info("http server listening", "addr", ":"+cfg.HTTP.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server crashed", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info("shutdown signal received", "signal", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Server.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
	} else {
		log.Info("http server shut down gracefully")
	}
}
