package repository

import (
	"context"

	"github.com/fastygo/backend/domain"
)

type UserRepository interface {
	GetByID(ctx context.Context, id string) (*domain.User, error)
	Upsert(ctx context.Context, user *domain.User) error
}
