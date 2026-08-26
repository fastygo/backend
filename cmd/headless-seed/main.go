package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/bootstrap"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/operations/seed"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("path", "", "fastygo.data.seed/v1 JSON file")
	flag.Parse()
	if *path == "" {
		return fmt.Errorf("-path is required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	config, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}
	storage, err := bootstrap.OpenStorage(ctx, config)
	if err != nil {
		return err
	}
	defer storage.Close()
	service, err := content.NewService(storage, nil, nil)
	if err != nil {
		return err
	}
	if err := service.SetManifest(config.Manifest); err != nil {
		return err
	}
	file, err := os.Open(*path)
	if err != nil {
		return fmt.Errorf("failed to open seed: %w", err)
	}
	defer file.Close()
	result, err := seed.Apply(ctx, service, authz.AdministratorRole().Principal("seed"), file)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(os.Stdout, "seeded %d records, skipped %d existing\n", result.Created, result.Skipped)
	return err
}
