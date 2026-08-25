package content

import (
	"testing"
	"time"
)

func TestPublicVisibilityContract(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	entry := validEntry(now)

	if !entry.IsPublicAt(now) {
		t.Fatalf("published public content must be visible")
	}

	future := now.Add(time.Hour)
	entry.PublishedAt = &future
	if entry.IsPublicAt(now) {
		t.Fatalf("future content must not be visible")
	}

	entry.PublishedAt = nil
	for _, status := range []Status{StatusDraft, StatusScheduled, StatusArchived, StatusTrashed} {
		entry.Status = status
		if entry.IsPublicAt(now) {
			t.Fatalf("%s content must not be visible", status)
		}
	}

	entry.Status = StatusPublished
	entry.Visibility = VisibilityPrivate
	if entry.IsPublicAt(now) {
		t.Fatalf("private content must not be visible")
	}
}

func TestValidationAndPublicMetadataProjection(t *testing.T) {
	now := time.Now().UTC()
	entry := validEntry(now)
	entry.Metadata = map[string]MetadataValue{
		"seo_title":   {Value: "Public"},
		"internal_id": {Value: "secret", Private: true},
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("valid entry rejected: %v", err)
	}

	projected := entry.PublicProjection()
	if _, ok := projected.Metadata["internal_id"]; ok {
		t.Fatalf("private metadata leaked")
	}
	if projected.Metadata["seo_title"].Value != "Public" {
		t.Fatalf("public metadata missing")
	}

	entry.Status = StatusScheduled
	entry.PublishedAt = nil
	if err := entry.Validate(); err == nil {
		t.Fatalf("scheduled content without published_at must fail")
	}
}

func TestLocalizedTextAndUnicodeSlug(t *testing.T) {
	text := LocalizedText{"ru": "Название", "en": "Title"}
	if value := text.Value("de", "ru"); value != "Название" {
		t.Fatalf("unexpected fallback: %q", value)
	}

	if slug := NormalizeSlug("  Летнее платье / 2026  "); slug != "летнее-платье-2026" {
		t.Fatalf("unexpected slug: %q", slug)
	}
	if !ValidKind("product_variant") || ValidKind("Product Variant") {
		t.Fatalf("kind validation is inconsistent")
	}
}

func validEntry(now time.Time) Entry {
	return Entry{
		ID:         "post_1",
		Kind:       KindPost,
		Status:     StatusPublished,
		Visibility: VisibilityPublic,
		Slug:       LocalizedText{"en": "hello"},
		Title:      LocalizedText{"en": "Hello"},
		Version:    1,
		CreatedAt:  now.Add(-time.Hour),
		UpdatedAt:  now,
	}
}
