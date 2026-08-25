package audit

import (
	"errors"
	"strings"
	"time"
)

type ID string

type Event struct {
	ID            ID             `json:"id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	ActorID       string         `json:"actor_id"`
	Action        string         `json:"action"`
	Resource      string         `json:"resource"`
	ResourceID    string         `json:"resource_id"`
	BeforeVersion uint64         `json:"before_version,omitempty"`
	AfterVersion  uint64         `json:"after_version,omitempty"`
	Metadata      map[string]any `json:"metadata,omitempty"`
}

func (event Event) Validate() error {
	switch {
	case strings.TrimSpace(string(event.ID)) == "":
		return errors.New("audit event id is required")
	case event.OccurredAt.IsZero():
		return errors.New("audit event timestamp is required")
	case strings.TrimSpace(event.ActorID) == "":
		return errors.New("audit actor is required")
	case strings.TrimSpace(event.Action) == "":
		return errors.New("audit action is required")
	case strings.TrimSpace(event.Resource) == "":
		return errors.New("audit resource is required")
	case strings.TrimSpace(event.ResourceID) == "":
		return errors.New("audit resource id is required")
	default:
		return nil
	}
}
