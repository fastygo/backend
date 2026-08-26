package revision

import (
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/content"
)

func TestRevisionValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 9, 0, 0, 0, time.UTC)
	snapshot := content.Entry{ID: "post_1", Version: 2}
	valid := Revision{
		ID: "rev_1", EntryID: "post_1", Version: 2,
		Snapshot: snapshot, AuthorID: "editor", CreatedAt: now,
	}
	cases := map[string]struct {
		mutate    func(*Revision)
		wantError bool
	}{
		"valid": {},
		"missing id": {mutate: func(item *Revision) { item.ID = " " }, wantError: true},
		"entry mismatch": {
			mutate:    func(item *Revision) { item.Snapshot.ID = "post_2" },
			wantError: true,
		},
		"version mismatch": {
			mutate:    func(item *Revision) { item.Snapshot.Version = 1 },
			wantError: true,
		},
		"missing author": {mutate: func(item *Revision) { item.AuthorID = "" }, wantError: true},
		"missing created_at": {
			mutate:    func(item *Revision) { item.CreatedAt = time.Time{} },
			wantError: true,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			item := valid
			if test.mutate != nil {
				test.mutate(&item)
			}
			err := item.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid revision rejected: %v", err)
			}
		})
	}
}
