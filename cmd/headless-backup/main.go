package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/fastygo/backend/internal/bootstrap"
	"github.com/fastygo/backend/internal/operations/backup"
	"github.com/fastygo/backend/internal/storage/localmedia"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	mode := flag.String("mode", "export", "operation: export or restore")
	path := flag.String("path", "", "backup JSON file")
	flag.Parse()
	if *path == "" {
		return errors.New("-path is required")
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
	service, err := backup.New(storage, config.Manifest)
	if err != nil {
		return err
	}
	media, err := localmedia.Open(config.MediaRoot)
	if err != nil {
		return err
	}
	switch *mode {
	case "export":
		if err := exportAtomic(ctx, service, *path); err != nil {
			return err
		}
		return exportMediaAtomic(ctx, media, *path+".media.tar")
	case "restore":
		source, err := os.Open(*path)
		if err != nil {
			return err
		}
		if err := service.Restore(ctx, source); err != nil {
			_ = source.Close()
			return err
		}
		if err := source.Close(); err != nil {
			return err
		}
		mediaSource, err := os.Open(*path + ".media.tar")
		if err != nil {
			return err
		}
		defer mediaSource.Close()
		return media.Restore(ctx, mediaSource)
	default:
		return errors.New("-mode must be export or restore")
	}
}

func exportMediaAtomic(ctx context.Context, store *localmedia.Store, path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".media-backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := store.Export(ctx, temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func exportAtomic(ctx context.Context, service *backup.Service, path string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".backup-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := service.Export(ctx, temporary); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
