package persist

import (
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
)

func TestEncodeDecodeRevisionRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	cases := map[string]revision.Revision{
		"content snapshot": {
			ID: "revision_1", EntryID: "post_1", Version: 1, AuthorID: "editor", CreatedAt: now,
			Snapshot: content.Entry{
				ID: "post_1", Kind: content.KindPost, Status: content.StatusDraft,
				Visibility: content.VisibilityPrivate,
				Slug:       content.LocalizedText{"en": "draft"}, Title: content.LocalizedText{"en": "Draft"},
				Version: 1, CreatedAt: now, UpdatedAt: now,
			},
		},
	}
	for name, item := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := EncodeRevision(item)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := DecodeRevision(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded.ID != item.ID || decoded.Snapshot.Title["en"] != "Draft" {
				t.Fatalf("revision round-trip diverged: %#v", decoded)
			}
		})
	}
}
