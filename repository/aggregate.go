package repository

import (
	"context"

	"github.com/e-go/backend/domain"
)

type AggregateFilter struct {
	Kind     string
	TenantID string
	OwnerID  string
	Limit    int
	Offset   int
}

type AggregateRepository interface {
	Get(ctx context.Context, id string) (*domain.Aggregate, error)
	List(ctx context.Context, filter AggregateFilter) ([]domain.Aggregate, error)
	Save(ctx context.Context, aggregate *domain.Aggregate) error
	AppendEvent(ctx context.Context, event domain.Event) error
}
