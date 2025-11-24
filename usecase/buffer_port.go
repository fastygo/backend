package usecase

import (
	"context"

	"github.com/fastygo/backend/domain"
)

// OperationBuffer abstracts the buffer processor so use cases stay storage-agnostic.
type OperationBuffer interface {
	BufferProfile(ctx context.Context, operation string, user *domain.User) error
	BufferTask(ctx context.Context, operation string, task *domain.Task) error
}
