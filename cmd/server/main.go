package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/fastygo/backend/internal/bootstrap"
)

func main() {
	if err := run(); err != nil {
		slog.Error("headless backend stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	config, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	configureProcessLogger(config.App.LogLevel, config.App.LogFormat, os.Stdout)
	logBuildInfo()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := bootstrap.Build(ctx, config)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := runtime.Close(); closeErr != nil {
			slog.Error("headless backend storage close failed", "error", closeErr)
		}
	}()
	if err := runtime.App.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
