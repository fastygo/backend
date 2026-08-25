package audit

import (
	"errors"
	"strings"
	"time"
)

type ID string

type Event struct {
	ID            ID
	OccurredAt    time.Time
	ActorID       string
	Action        string
	Resource      string
	ResourceID    string
	BeforeVersion uint64
	AfterVersion  uint64
	Metadata      map[string]any
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
