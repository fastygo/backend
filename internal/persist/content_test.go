package persist

import (
	"testing"
	"time"

	"github.com/fastygo/codex/content"
)

func TestEncodeDecodeEntryRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	published := now.Add(-time.Hour)
	cases := map[string]content.Entry{
		"public product": {
			ID: "product_1", Kind: "product", Status: content.StatusPublished,
			Visibility: content.VisibilityPublic, AuthorID: "author-1",
			Slug: "course", Title: "Course", Locale: "en",
			Metadata: map[string]content.MetadataValue{
				"sku": {Value: "SKU-1"}, "secret": {Value: "internal", Private: true},
				"payload_en": {Value: map[string]any{"title": "Course"}},
			},
			Terms:   []content.TermRef{{Taxonomy: "brand", TermID: "acme"}},
			Version: 2, CreatedAt: now, UpdatedAt: now, PublishedAt: &published,
		},
		"trashed page": {
			ID: "page_1", Kind: content.KindPage, Status: content.StatusTrashed,
			Visibility: content.VisibilityPrivate,
			Slug:       "old", Title: "Old", Locale: "en",
			Version: 1, CreatedAt: now, UpdatedAt: now, DeletedAt: &now,
		},
	}
	for name, entry := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := EncodeEntry(entry)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := DecodeEntry(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if decoded.ID != entry.ID || decoded.Kind != entry.Kind ||
				decoded.Metadata["secret"].Private != entry.Metadata["secret"].Private ||
				len(decoded.Terms) != len(entry.Terms) {
				t.Fatalf("round-trip diverged: %#v", decoded)
			}
		})
	}
}
