package services

import (
	"context"
	"encoding/json"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/internal/infrastructure/buffer"
	"github.com/fastygo/backend/usecase"
)

type BufferBridge struct {
	processor *BufferProcessor
}

func NewBufferBridge(processor *BufferProcessor) *BufferBridge {
	return &BufferBridge{processor: processor}
}

func (b *BufferBridge) BufferProfile(ctx context.Context, operation string, user *domain.User) error {
	if b.processor == nil || user == nil {
		return domain.ErrInvalidPayload
	}
	payload, err := json.Marshal(user)
	if err != nil {
		return err
	}
	item := buffer.Item{
		UserID:    user.ID,
		Entity:    buffer.EntityProfile,
		Operation: operation,
		Data:      payload,
		Priority:  3,
	}
	return b.processor.BufferOperation(ctx, item)
}

func (b *BufferBridge) BufferTask(ctx context.Context, operation string, task *domain.Task) error {
	if b.processor == nil || task == nil {
		return domain.ErrInvalidPayload
	}
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	item := buffer.Item{
		ID:        task.ID,
		UserID:    task.UserID,
		Entity:    buffer.EntityTask,
		Operation: operation,
		Data:      payload,
		Priority:  4,
	}
	return b.processor.BufferOperation(ctx, item)
}

var _ usecase.OperationBuffer = (*BufferBridge)(nil)
