package main

import (
	"context"
	"errors"
	"log/slog"
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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := bootstrap.Build(ctx, config)
	if err != nil {
		return err
	}
	if err := runtime.App.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
