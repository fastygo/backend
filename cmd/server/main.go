package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/fastygo/backend/internal/bootstrap"
	_ "github.com/fastygo/backend/internal/storage/drivers"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "codex" {
		if err := runCodex(os.Args[2:]); err != nil {
			_, _ = fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := runServer(); err != nil {
		slog.Error("headless backend stopped", "error", err)
		os.Exit(1)
	}
}

func runServer() error {
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
