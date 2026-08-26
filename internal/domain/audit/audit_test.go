package audit

import (
	"testing"
	"time"
)

func TestEventValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	valid := Event{
		ID: "evt_1", OccurredAt: now, ActorID: "editor",
		Action: "content.update", Resource: "content", ResourceID: "post_1",
	}
	cases := map[string]struct {
		mutate    func(*Event)
		wantError bool
	}{
		"valid": {},
		"missing id":          {mutate: func(event *Event) { event.ID = " " }, wantError: true},
		"missing timestamp":   {mutate: func(event *Event) { event.OccurredAt = time.Time{} }, wantError: true},
		"missing actor":       {mutate: func(event *Event) { event.ActorID = "" }, wantError: true},
		"missing action":      {mutate: func(event *Event) { event.Action = "" }, wantError: true},
		"missing resource":    {mutate: func(event *Event) { event.Resource = "" }, wantError: true},
		"missing resource id": {mutate: func(event *Event) { event.ResourceID = "" }, wantError: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			event := valid
			if test.mutate != nil {
				test.mutate(&event)
			}
			err := event.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid event rejected: %v", err)
			}
		})
	}
}
