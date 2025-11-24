package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redislib "github.com/redis/go-redis/v9"

	"github.com/e-go/backend/domain"
	"github.com/e-go/backend/repository"
)

type sessionRepository struct {
	client *redislib.Client
	prefix string
	ttl    time.Duration
}

// NewSessionRepository creates a Redis-backed session repository.
func NewSessionRepository(client *redislib.Client, ttl time.Duration) repository.SessionRepository {
	if ttl <= 0 {
		ttl = time.Hour
	}
	return &sessionRepository{
		client: client,
		prefix: "session:",
		ttl:    ttl,
	}
}

func (r *sessionRepository) Get(ctx context.Context, id string) (*domain.Session, error) {
	result, err := r.client.Get(ctx, r.key(id)).Result()
	if err != nil {
		if err == redislib.Nil {
			return nil, domain.ErrSessionNotFound
		}
		return nil, err
	}

	var session domain.Session
	if err := json.Unmarshal([]byte(result), &session); err != nil {
		return nil, err
	}
	return &session, nil
}

func (r *sessionRepository) Save(ctx context.Context, session *domain.Session) error {
	if session == nil || session.ID == "" {
		return domain.ErrInvalidPayload
	}

	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	if session.ExpiresAt.Before(session.CreatedAt) {
		session.ExpiresAt = session.CreatedAt.Add(r.ttl)
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}

	ttl := time.Until(session.ExpiresAt)
	if ttl <= 0 {
		ttl = r.ttl
	}

	return r.client.Set(ctx, r.key(session.ID), payload, ttl).Err()
}

func (r *sessionRepository) Delete(ctx context.Context, id string) error {
	return r.client.Del(ctx, r.key(id)).Err()
}

func (r *sessionRepository) Extend(ctx context.Context, id string, ttlSeconds int) error {
	duration := time.Duration(ttlSeconds) * time.Second
	if duration <= 0 {
		duration = r.ttl
	}
	return r.client.Expire(ctx, r.key(id), duration).Err()
}

func (r *sessionRepository) key(id string) string {
	return fmt.Sprintf("%s%s", r.prefix, id)
}
