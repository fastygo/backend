package domain

import "time"

// Task represents a user-owned activity item.
type Task struct {
	ID          string            `json:"id"`
	UserID      string            `json:"user_id"`
	Title       string            `json:"title"`
	Description string            `json:"description,omitempty"`
	Status      string            `json:"status"`
	Priority    int               `json:"priority"`
	DueDate     *time.Time        `json:"due_date,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

func (t *Task) IsCompleted() bool {
	return t != nil && t.Status == "completed"
}
