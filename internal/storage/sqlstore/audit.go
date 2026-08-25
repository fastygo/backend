package sqlstore

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"math"
	"slices"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/persist"
	"github.com/google/uuid"
)

type auditRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository auditRepository) NextID(context.Context) (audit.ID, error) {
	return audit.ID("audit_" + uuid.NewString()), nil
}

func (repository auditRepository) Save(ctx context.Context, event audit.Event) error {
	encoded, err := persist.EncodeEvent(event)
	if err != nil {
		return err
	}
	_, err = repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			`INSERT INTO audit_events
			 (id, occurred_at, actor_id, action, resource, resource_id, payload)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		event.ID, event.OccurredAt.UnixNano(), event.ActorID, event.Action,
		event.Resource, event.ResourceID, encoded,
	)
	return err
}

func (repository auditRepository) List(
	ctx context.Context,
	query application.AuditQuery,
) ([]audit.Event, application.Page, error) {
	if query.Page < 1 || query.PerPage < 1 {
		return nil, application.Page{}, errors.New("invalid pagination")
	}
	rows, err := repository.transaction.QueryContext(ctx, "SELECT payload FROM audit_events")
	if err != nil {
		return nil, application.Page{}, err
	}
	defer rows.Close()
	events := make([]audit.Event, 0)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, application.Page{}, err
		}
		event, err := persist.DecodeEvent(encoded)
		if err != nil {
			return nil, application.Page{}, err
		}
		if matchesAudit(event, query) {
			events = append(events, event)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, application.Page{}, err
	}
	slices.SortFunc(events, func(left, right audit.Event) int {
		comparison := right.OccurredAt.Compare(left.OccurredAt)
		if comparison == 0 {
			return cmp.Compare(right.AfterVersion, left.AfterVersion)
		}
		return comparison
	})
	total := len(events)
	start := min((query.Page-1)*query.PerPage, total)
	end := min(start+query.PerPage, total)
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(query.PerPage)))
	}
	return append([]audit.Event(nil), events[start:end]...), application.Page{
		Number: query.Page, PerPage: query.PerPage, Total: total, TotalPages: totalPages,
	}, nil
}

func matchesAudit(event audit.Event, query application.AuditQuery) bool {
	switch {
	case query.ActorID != "" && event.ActorID != query.ActorID:
		return false
	case query.Action != "" && event.Action != query.Action:
		return false
	case query.Resource != "" && event.Resource != query.Resource:
		return false
	case query.ResourceID != "" && event.ResourceID != query.ResourceID:
		return false
	case query.After != nil && event.OccurredAt.Before(*query.After):
		return false
	case query.Before != nil && event.OccurredAt.After(*query.Before):
		return false
	default:
		return true
	}
}
