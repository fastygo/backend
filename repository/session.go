package repository

import (
	"context"

	"github.com/fastygo/backend/domain"
)

type SessionRepository interface {
	Get(ctx context.Context, id string) (*domain.Session, error)
	Save(ctx context.Context, session *domain.Session) error
	Delete(ctx context.Context, id string) error
	Extend(ctx context.Context, id string, ttlSeconds int) error
}
