package bbolt

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type auditRepository struct {
	transaction *bolt.Tx
}

func (repository auditRepository) NextID(_ context.Context) (audit.ID, error) {
	return audit.ID("audit_" + uuid.NewString()), nil
}

func (repository auditRepository) Save(_ context.Context, event audit.Event) error {
	bucket := repository.transaction.Bucket(auditBucket)
	if bucket.Get([]byte(event.ID)) != nil {
		return ErrConflict
	}
	return putJSON(bucket, []byte(event.ID), event)
}

func (repository auditRepository) List(
	ctx context.Context,
	query application.AuditQuery,
) ([]audit.Event, application.Page, error) {
	if query.Page < 1 || query.PerPage < 1 {
		return nil, application.Page{}, errors.New("invalid pagination")
	}
	events := make([]audit.Event, 0)
	err := repository.transaction.Bucket(auditBucket).ForEach(func(_, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var event audit.Event
		if err := json.Unmarshal(value, &event); err != nil {
			return err
		}
		if matchesAudit(event, query) {
			events = append(events, event)
		}
		return nil
	})
	if err != nil {
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
