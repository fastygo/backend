package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fastygo/backend/domain"
	"github.com/fastygo/backend/repository"
)

type taskRepository struct {
	pool *pgxpool.Pool
}

// NewTaskRepository returns a Postgres-backed implementation of TaskRepository.
func NewTaskRepository(pool *pgxpool.Pool) repository.TaskRepository {
	return &taskRepository{pool: pool}
}

func (r *taskRepository) GetByID(ctx context.Context, id string) (*domain.Task, error) {
	const query = `
	SELECT id, user_id, title, description, status, priority, due_date, metadata, created_at, updated_at
	FROM tasks
	WHERE id = $1
	`
	row := r.pool.QueryRow(ctx, query, id)
	return scanTask(row)
}

func (r *taskRepository) List(ctx context.Context, filter repository.TaskFilter) ([]domain.Task, error) {
	const query = `
	SELECT id, user_id, title, description, status, priority, due_date, metadata, created_at, updated_at
	FROM tasks
	WHERE ($1 = '' OR user_id = $1)
	  AND ($2 = '' OR status = $2)
	ORDER BY created_at DESC
	LIMIT $3 OFFSET $4
	`
	rows, err := r.pool.Query(ctx, query, filter.UserID, filter.Status, clampLimit(filter.Limit), filter.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, *task)
	}
	return tasks, rows.Err()
}

func (r *taskRepository) Create(ctx context.Context, task *domain.Task) (*domain.Task, error) {
	if task == nil {
		return nil, domain.ErrInvalidPayload
	}
	if task.ID == "" {
		task.ID = uuid.NewString()
	}

	const query = `
	INSERT INTO tasks (id, user_id, title, description, status, priority, due_date, metadata)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	RETURNING created_at, updated_at
	`

	var due interface{}
	if task.DueDate != nil {
		due = *task.DueDate
	}

	metadata := marshalMap(task.Metadata)

	if err := r.pool.QueryRow(ctx, query,
		task.ID,
		task.UserID,
		task.Title,
		task.Description,
		task.Status,
		task.Priority,
		due,
		metadata,
	).Scan(&task.CreatedAt, &task.UpdatedAt); err != nil {
		return nil, err
	}

	return task, nil
}

func (r *taskRepository) Update(ctx context.Context, task *domain.Task) error {
	if task == nil {
		return domain.ErrInvalidPayload
	}

	const query = `
	UPDATE tasks
	SET title = $2,
		description = $3,
		status = $4,
		priority = $5,
		due_date = $6,
		metadata = $7,
		updated_at = NOW()
	WHERE id = $1
	RETURNING updated_at
	`

	var due interface{}
	if task.DueDate != nil {
		due = *task.DueDate
	}

	metadata := marshalMap(task.Metadata)

	if err := r.pool.QueryRow(ctx, query,
		task.ID,
		task.Title,
		task.Description,
		task.Status,
		task.Priority,
		due,
		metadata,
	).Scan(&task.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.ErrTaskNotFound
		}
		return err
	}

	return nil
}

func (r *taskRepository) Delete(ctx context.Context, id string) error {
	const query = `DELETE FROM tasks WHERE id = $1`
	tag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrTaskNotFound
	}
	return nil
}

func scanTask(row interface {
	Scan(dest ...interface{}) error
}) (*domain.Task, error) {
	var task domain.Task
	var (
		due      *time.Time
		metadata []byte
	)

	if err := row.Scan(
		&task.ID,
		&task.UserID,
		&task.Title,
		&task.Description,
		&task.Status,
		&task.Priority,
		&due,
		&metadata,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrTaskNotFound
		}
		return nil, err
	}

	task.DueDate = due
	if len(metadata) > 0 {
		_ = json.Unmarshal(metadata, &task.Metadata)
	}

	return &task, nil
}

func clampLimit(limit int) int {
	if limit <= 0 || limit > 100 {
		return 100
	}
	return limit
}
