package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/repository"
)

type userRepository struct {
	pool *pgxpool.Pool
}

// NewUserRepository instantiates a Postgres-backed user repository.
func NewUserRepository(pool *pgxpool.Pool) repository.UserRepository {
	return &userRepository{pool: pool}
}

func (r *userRepository) GetByID(ctx context.Context, id string) (*domain.User, error) {
	const query = `
		SELECT id, email, role, status, metadata, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)

	var user domain.User
	var metadata []byte

	if err := row.Scan(&user.ID, &user.Email, &user.Role, &user.Status, &metadata, &user.CreatedAt, &user.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, err
	}

	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &user.Metadata)
	}

	return &user, nil
}

func (r *userRepository) Upsert(ctx context.Context, user *domain.User) error {
	if user == nil {
		return domain.ErrInvalidPayload
	}

	const query = `
	INSERT INTO users (id, email, role, status, metadata, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, COALESCE($6, NOW()), NOW())
	ON CONFLICT (id) DO UPDATE
	SET email = EXCLUDED.email,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		metadata = EXCLUDED.metadata,
		updated_at = NOW()
	RETURNING created_at, updated_at;
	`

	metadata := marshalMap(user.Metadata)
	var createdAt, updatedAt time.Time

	if err := r.pool.QueryRow(ctx, query,
		user.ID,
		user.Email,
		user.Role,
		user.Status,
		metadata,
		nullTime(user.CreatedAt),
	).Scan(&createdAt, &updatedAt); err != nil {
		return err
	}

	user.CreatedAt = createdAt
	user.UpdatedAt = updatedAt
	return nil
}
