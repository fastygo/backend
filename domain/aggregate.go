package domain

import (
	"encoding/json"
	"time"
)

// Aggregate describes a generic business entity supporting different product lines (CRM, CMS, chats, etc.).
type Aggregate struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	TenantID  string            `json:"tenant_id,omitempty"`
	OwnerID   string            `json:"owner_id,omitempty"`
	Version   int               `json:"version"`
	Payload   json.RawMessage   `json:"payload"`
	Labels    map[string]string `json:"labels,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func (a *Aggregate) Touch() {
	if a == nil {
		return
	}
	a.UpdatedAt = time.Now()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = a.UpdatedAt
	}
}

// Event represents a change applied to an aggregate instance.
type Event struct {
	ID          string            `json:"id"`
	AggregateID string            `json:"aggregate_id"`
	Name        string            `json:"name"`
	Version     int               `json:"version"`
	Payload     json.RawMessage   `json:"payload"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
}
