package task

import (
	"context"

	"go.uber.org/zap"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/repository"
	"github.com/fastygo/backend/usecase"
)

type UseCase struct {
	tasks  repository.TaskRepository
	buffer usecase.OperationBuffer
	logger *zap.Logger
}

func New(tasks repository.TaskRepository, buffer usecase.OperationBuffer, logger *zap.Logger) *UseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UseCase{
		tasks:  tasks,
		buffer: buffer,
		logger: logger,
	}
}

func (uc *UseCase) ListTasks(ctx context.Context, filter repository.TaskFilter) ([]domain.Task, error) {
	return uc.tasks.List(ctx, filter)
}

func (uc *UseCase) GetTask(ctx context.Context, id string) (*domain.Task, error) {
	return uc.tasks.GetByID(ctx, id)
}

func (uc *UseCase) CreateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	created, err := uc.tasks.Create(ctx, task)
	if err != nil {
		if uc.shouldBuffer(ctx, usecase.OperationCreate, task) {
			return task, nil
		}
		return nil, err
	}
	return created, nil
}

func (uc *UseCase) UpdateTask(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if err := uc.tasks.Update(ctx, task); err != nil {
		if uc.shouldBuffer(ctx, usecase.OperationUpdate, task) {
			return task, nil
		}
		return nil, err
	}
	return task, nil
}

func (uc *UseCase) DeleteTask(ctx context.Context, id string) error {
	if err := uc.tasks.Delete(ctx, id); err != nil {
		if err == domain.ErrTaskNotFound {
			return err
		}
		task := &domain.Task{ID: id}
		if uc.shouldBuffer(ctx, usecase.OperationDelete, task) {
			return nil
		}
		return err
	}
	return nil
}

func (uc *UseCase) shouldBuffer(ctx context.Context, operation string, task *domain.Task) bool {
	if uc.buffer == nil {
		return false
	}
	if err := uc.buffer.BufferTask(ctx, operation, task); err != nil {
		uc.logger.Error("failed to buffer task operation", zap.String("operation", operation), zap.Error(err))
		return false
	}
	uc.logger.Warn("task operation buffered", zap.String("operation", operation))
	return true
}
