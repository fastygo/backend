package persist

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fastygo/backend/internal/domain/audit"
)

type Event struct {
	ID            audit.ID       `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ActorID       string         `json:"actor_id"`
	Action        string         `json:"action"`
	Resource      string         `json:"resource"`
	ResourceID    string         `json:"resource_id"`
	BeforeVersion uint64         `json:"before_version,omitempty"`
	AfterVersion  uint64         `json:"after_version,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func EventFromDomain(event audit.Event) Event {
	return Event{
		ID: event.ID, OccurredAt: event.OccurredAt, ActorID: event.ActorID,
		Action: event.Action, Resource: event.Resource, ResourceID: event.ResourceID,
		BeforeVersion: event.BeforeVersion, AfterVersion: event.AfterVersion, Metadata: event.Metadata,
	}
}

func (event Event) Domain() audit.Event {
	return audit.Event{
		ID: event.ID, OccurredAt: event.OccurredAt, ActorID: event.ActorID,
		Action: event.Action, Resource: event.Resource, ResourceID: event.ResourceID,
		BeforeVersion: event.BeforeVersion, AfterVersion: event.AfterVersion, Metadata: event.Metadata,
	}
}

func EncodeEvent(event audit.Event) ([]byte, error) {
	encoded, err := json.Marshal(EventFromDomain(event))
	if err != nil {
		return nil, fmt.Errorf("failed to encode audit event: %w", err)
	}
	return encoded, nil
}

func DecodeEvent(encoded []byte) (audit.Event, error) {
	var document Event
	if err := json.Unmarshal(encoded, &document); err != nil {
		return audit.Event{}, fmt.Errorf("failed to decode audit event: %w", err)
	}
	return document.Domain(), nil
}
