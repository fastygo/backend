package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/repository"
)

type aggregateRepository struct {
	pool *pgxpool.Pool
}

// NewAggregateRepository creates a Postgres-backed AggregateRepository implementation.
func NewAggregateRepository(pool *pgxpool.Pool) repository.AggregateRepository {
	return &aggregateRepository{pool: pool}
}

func (r *aggregateRepository) Get(ctx context.Context, id string) (*domain.Aggregate, error) {
	const query = `
	SELECT id, kind, tenant_id, owner_id, version, payload, labels, created_at, updated_at
	FROM aggregates
	WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)
	return scanAggregate(row)
}

func (r *aggregateRepository) List(ctx context.Context, filter repository.AggregateFilter) ([]domain.Aggregate, error) {
	const query = `
	SELECT id, kind, tenant_id, owner_id, version, payload, labels, created_at, updated_at
	FROM aggregates
	WHERE ($1 = '' OR kind = $1)
	  AND ($2 = '' OR tenant_id = $2)
	  AND ($3 = '' OR owner_id = $3)
	ORDER BY updated_at DESC
	LIMIT $4 OFFSET $5
	`
	rows, err := r.pool.Query(ctx, query, filter.Kind, filter.TenantID, filter.OwnerID, clampLimit(filter.Limit), filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aggregates []domain.Aggregate
	for rows.Next() {
		entity, err := scanAggregate(rows)
		if err != nil {
			return nil, err
		}
		aggregates = append(aggregates, *entity)
	}
	return aggregates, rows.Err()
}

func (r *aggregateRepository) Save(ctx context.Context, aggregate *domain.Aggregate) error {
	if aggregate == nil {
		return domain.ErrInvalidPayload
	}

	const query = `
	INSERT INTO aggregates (id, kind, tenant_id, owner_id, version, payload, labels, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, NOW()), NOW())
	ON CONFLICT (id) DO UPDATE
	SET kind = EXCLUDED.kind,
		tenant_id = EXCLUDED.tenant_id,
		owner_id = EXCLUDED.owner_id,
		version = EXCLUDED.version,
		payload = EXCLUDED.payload,
		labels = EXCLUDED.labels,
		updated_at = NOW()
	RETURNING created_at, updated_at
	`

	if aggregate.ID == "" {
		return domain.ErrInvalidPayload
	}

	labels := marshalMap(aggregate.Labels)

	if err := r.pool.QueryRow(ctx, query,
		aggregate.ID,
		aggregate.Kind,
		aggregate.TenantID,
		aggregate.OwnerID,
		aggregate.Version,
		[]byte(aggregate.Payload),
		labels,
		nullTime(aggregate.CreatedAt),
	).Scan(&aggregate.CreatedAt, &aggregate.UpdatedAt); err != nil {
		return err
	}

	return nil
}

func (r *aggregateRepository) AppendEvent(ctx context.Context, event domain.Event) error {
	const query = `
	INSERT INTO aggregate_events (id, aggregate_id, name, version, payload, metadata, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, NOW()))
	`

	metadata := marshalMap(event.Metadata)

	_, err := r.pool.Exec(ctx, query,
		event.ID,
		event.AggregateID,
		event.Name,
		event.Version,
		[]byte(event.Payload),
		metadata,
		nullTime(event.CreatedAt),
	)

	return err
}

func scanAggregate(row interface {
	Scan(dest ...interface{}) error
}) (*domain.Aggregate, error) {
	var entity domain.Aggregate
	var (
		payload []byte
		labels  []byte
	)

	if err := row.Scan(
		&entity.ID,
		&entity.Kind,
		&entity.TenantID,
		&entity.OwnerID,
		&entity.Version,
		&payload,
		&labels,
		&entity.CreatedAt,
		&entity.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrAggregateNotFound
		}
		return nil, err
	}

	entity.Payload = make([]byte, len(payload))
	copy(entity.Payload, payload)
	if len(labels) > 0 {
		_ = json.Unmarshal(labels, &entity.Labels)
	}

	return &entity, nil
}
