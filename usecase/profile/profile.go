package profile

import (
	"context"

	"go.uber.org/zap"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/repository"
	"github.com/fastygo/backend/usecase"
)

type UseCase struct {
	users  repository.UserRepository
	buffer usecase.OperationBuffer
	logger *zap.Logger
}

func New(users repository.UserRepository, buffer usecase.OperationBuffer, logger *zap.Logger) *UseCase {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UseCase{
		users:  users,
		buffer: buffer,
		logger: logger,
	}
}

func (uc *UseCase) GetProfile(ctx context.Context, userID string) (*domain.User, error) {
	return uc.users.GetByID(ctx, userID)
}

func (uc *UseCase) UpdateProfile(ctx context.Context, user *domain.User) (*domain.User, error) {
	if err := uc.users.Upsert(ctx, user); err != nil {
		if uc.buffer != nil {
			if bufErr := uc.buffer.BufferProfile(ctx, usecase.OperationUpdate, user); bufErr != nil {
				uc.logger.Error("failed to buffer profile update", zap.Error(bufErr))
				return nil, err
			}
			uc.logger.Warn("profile update buffered due to repository error", zap.Error(err))
			return user, nil
		}
		return nil, err
	}
	return user, nil
}
