package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/internal/infrastructure/buffer"
	"github.com/e-go/backend/repository"
)

// ConnectionHealth abstracts the connection monitor functionality.
type ConnectionHealth interface {
	IsOnline() bool
}

// ProcessorConfig controls how frequently the buffer is drained.
type ProcessorConfig struct {
	Interval   time.Duration
	BatchSize  int
	MaxRetries int
}

// BufferProcessor synchronizes buffered operations with primary datastores.
type BufferProcessor struct {
	store    *buffer.Store
	monitor  ConnectionHealth
	userRepo repository.UserRepository
	taskRepo repository.TaskRepository
	logger   *zap.Logger
	cron     *cron.Cron
	cfg      ProcessorConfig
}

func NewBufferProcessor(
	store *buffer.Store,
	monitor ConnectionHealth,
	userRepo repository.UserRepository,
	taskRepo repository.TaskRepository,
	logger *zap.Logger,
	cfg ProcessorConfig,
) *BufferProcessor {
	if cfg.Interval <= 0 {
		cfg.Interval = 30 * time.Second
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 50
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 3
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	bp := &BufferProcessor{
		store:    store,
		monitor:  monitor,
		userRepo: userRepo,
		taskRepo: taskRepo,
		logger:   logger,
		cfg:      cfg,
		cron:     cron.New(cron.WithSeconds()),
	}

	schedule := fmt.Sprintf("@every %ds", int(cfg.Interval.Seconds()))
	_, _ = bp.cron.AddFunc(schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Interval)
		defer cancel()
		if err := bp.Drain(ctx); err != nil {
			bp.logger.Error("buffer drain failed", zap.Error(err))
		}
	})

	return bp
}

// Start launches the cron scheduler.
func (bp *BufferProcessor) Start() {
	if bp == nil || bp.cron == nil {
		return
	}
	bp.cron.Start()
	bp.logger.Info("buffer processor started")
}

// Stop gracefully stops the scheduler.
func (bp *BufferProcessor) Stop(ctx context.Context) {
	if bp == nil || bp.cron == nil {
		return
	}
	stopCtx := bp.cron.Stop()
	select {
	case <-stopCtx.Done():
	case <-ctx.Done():
	}
	bp.logger.Info("buffer processor stopped")
}

// Drain processes buffered items synchronously.
func (bp *BufferProcessor) Drain(ctx context.Context) error {
	if bp == nil || bp.store == nil {
		return nil
	}
	if bp.monitor != nil && !bp.monitor.IsOnline() {
		bp.logger.Debug("skipping buffer drain (offline)")
		return nil
	}

	items, err := bp.store.GetBatch(bp.cfg.BatchSize)
	if err != nil {
		return err
	}

	for _, item := range items {
		if err := bp.processItem(ctx, item); err != nil {
			bp.logger.Error("failed to process buffer item",
				zap.String("item_id", item.ID),
				zap.String("entity", item.Entity),
				zap.Error(err))

			item.Retries++
			if item.Retries >= bp.cfg.MaxRetries {
				bp.logger.Warn("dropping buffer item (max retries reached)", zap.String("item_id", item.ID))
				_ = bp.store.Remove(item)
				continue
			}

			if err := bp.store.Remove(item); err != nil {
				bp.logger.Warn("failed to remove buffer item", zap.Error(err))
			}
			if err := bp.store.Requeue(item); err != nil {
				bp.logger.Error("failed to requeue buffer item", zap.Error(err))
			}
			continue
		}

		if err := bp.store.Remove(item); err != nil {
			bp.logger.Warn("failed to purge processed buffer item", zap.Error(err))
		}
	}
	return nil
}

// BufferOperation attempts to run the operation immediately and falls back to persisting it.
func (bp *BufferProcessor) BufferOperation(ctx context.Context, item buffer.Item) error {
	if bp == nil || bp.store == nil {
		return fmt.Errorf("buffer processor not configured")
	}

	if bp.monitor == nil || bp.monitor.IsOnline() {
		if err := bp.processItem(ctx, item); err == nil {
			return nil
		} else {
			bp.logger.Warn("immediate processing failed, buffering", zap.Error(err))
		}
	}
	return bp.store.Enqueue(item)
}

// Size returns the number of buffered items.
func (bp *BufferProcessor) Size() int {
	if bp == nil || bp.store == nil {
		return 0
	}
	size, err := bp.store.Size()
	if err != nil {
		return 0
	}
	return size
}

func (bp *BufferProcessor) processItem(ctx context.Context, item buffer.Item) error {
	if ctx == nil {
		ctx = context.Background()
	}

	switch item.Entity {
	case buffer.EntityProfile:
		var user domain.User
		if err := json.Unmarshal(item.Data, &user); err != nil {
			return err
		}
		return bp.userRepo.Upsert(ctx, &user)

	case buffer.EntityTask:
		var task domain.Task
		if err := json.Unmarshal(item.Data, &task); err != nil {
			return err
		}
		switch item.Operation {
		case buffer.OperationCreate:
			_, err := bp.taskRepo.Create(ctx, &task)
			return err
		case buffer.OperationUpdate:
			return bp.taskRepo.Update(ctx, &task)
		case buffer.OperationDelete:
			return bp.taskRepo.Delete(ctx, task.ID)
		default:
			return fmt.Errorf("unsupported operation %s", item.Operation)
		}
	default:
		return fmt.Errorf("unsupported entity %s", item.Entity)
	}
}
