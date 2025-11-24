package auth

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/repository"
)

type UseCase struct {
	users    repository.UserRepository
	sessions repository.SessionRepository
	logger   *zap.Logger
}

func New(users repository.UserRepository, sessions repository.SessionRepository, logger *zap.Logger) *UseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UseCase{
		users:    users,
		sessions: sessions,
		logger:   logger,
	}
}

func (uc *UseCase) CreateSession(ctx context.Context, userID string, ttl time.Duration) (*domain.Session, error) {
	if _, err := uc.users.GetByID(ctx, userID); err != nil {
		return nil, err
	}

	session := &domain.Session{
		ID:        uuid.NewString(),
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}

	if err := uc.sessions.Save(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

func (uc *UseCase) GetSession(ctx context.Context, sessionID string) (*domain.Session, error) {
	session, err := uc.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if session.IsExpired(time.Now()) {
		_ = uc.sessions.Delete(ctx, sessionID)
		return nil, domain.ErrSessionNotFound
	}
	return session, nil
}

func (uc *UseCase) RefreshSession(ctx context.Context, sessionID string, ttl time.Duration) (*domain.Session, error) {
	session, err := uc.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := uc.sessions.Extend(ctx, sessionID, int(ttl.Seconds())); err != nil {
		return nil, err
	}
	session.ExpiresAt = time.Now().Add(ttl)
	return session, nil
}

func (uc *UseCase) RevokeSession(ctx context.Context, sessionID string) error {
	return uc.sessions.Delete(ctx, sessionID)
}
