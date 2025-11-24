package buffer

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	EntityProfile = "profile"
	EntityTask    = "task"

	OperationCreate = "create"
	OperationUpdate = "update"
	OperationDelete = "delete"
)

// Item represents an operation that should be retried when primary storage is unavailable.
type Item struct {
	ID        string          `json:"id"`
	UserID    string          `json:"user_id"`
	Entity    string          `json:"entity"`
	Operation string          `json:"operation"`
	Data      json.RawMessage `json:"data"`
	Priority  int             `json:"priority"`
	Retries   int             `json:"retries"`
	Timestamp time.Time       `json:"timestamp"`

	bucketKey []byte
}

func (i *Item) normalize() {
	if i.ID == "" {
		i.ID = uuid.NewString()
	}
	if i.Priority <= 0 || i.Priority > 5 {
		i.Priority = 3
	}
	if i.Timestamp.IsZero() {
		i.Timestamp = time.Now()
	}
}
