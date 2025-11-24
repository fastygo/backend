package repository

import (
	"context"

	"github.com/fastygo/backend/domain"
)

type TaskFilter struct {
	UserID string
	Status string
	Limit  int
	Offset int
}

type TaskRepository interface {
	GetByID(ctx context.Context, id string) (*domain.Task, error)
	List(ctx context.Context, filter TaskFilter) ([]domain.Task, error)
	Create(ctx context.Context, task *domain.Task) (*domain.Task, error)
	Update(ctx context.Context, task *domain.Task) error
	Delete(ctx context.Context, id string) error
}
