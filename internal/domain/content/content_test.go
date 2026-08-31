package content

import (
	"testing"
	"time"
)

func TestPublicVisibilityContract(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	cases := map[string]struct {
		mutate  func(*Entry)
		visible bool
	}{
		"published public": {mutate: func(*Entry) {}, visible: true},
		"future publish": {
			mutate: func(entry *Entry) {
				future := now.Add(time.Hour)
				entry.PublishedAt = &future
			},
		},
		"draft":     {mutate: func(entry *Entry) { entry.Status = StatusDraft }},
		"scheduled": {mutate: func(entry *Entry) { entry.Status = StatusScheduled }},
		"archived":  {mutate: func(entry *Entry) { entry.Status = StatusArchived }},
		"trashed":   {mutate: func(entry *Entry) { entry.Status = StatusTrashed }},
		"private":   {mutate: func(entry *Entry) { entry.Visibility = VisibilityPrivate }},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := validEntry(now)
			test.mutate(&entry)
			if entry.IsPublicAt(now) != test.visible {
				t.Fatalf("visibility=%v want %v", entry.IsPublicAt(now), test.visible)
			}
		})
	}
}

func TestValidationAndPublicMetadataProjection(t *testing.T) {
	t.Parallel()
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

func TestLiftLocaleMetadataAndWholeDocumentFallback(t *testing.T) {
	t.Parallel()
	entry := Entry{
		Status: StatusPublished,
		Metadata: map[string]MetadataValue{
			"payload_en": {Value: map[string]any{"title": "Hello"}},
			"sku":        {Value: "A"},
		},
	}
	entry.LiftLocaleMetadata()
	if _, exists := entry.Metadata["payload_en"]; exists {
		t.Fatal("payload_en must leave metadata")
	}
	if entry.Metadata["sku"].Value != "A" {
		t.Fatal("canon metadata lost")
	}
	if entry.Locales["en"].Data["title"] != "Hello" {
		t.Fatalf("locale lift: %#v", entry.Locales)
	}
	resolved := entry.ResolveLocale("de", "en")
	if !resolved.Fallback || resolved.Served != "en" || resolved.Requested != "de" || resolved.Data["title"] != "Hello" {
		t.Fatalf("fallback: %#v", resolved)
	}
}

func TestLocalizedTextAndUnicodeSlug(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		value string
		want  string
	}{
		"cyrillic path": {value: "  Летнее платье / 2026  ", want: "летнее-платье-2026"},
		"latin":         {value: "Hello World", want: "hello-world"},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if slug := NormalizeSlug(test.value); slug != test.want {
				t.Fatalf("unexpected slug: %q", slug)
			}
		})
	}
	text := LocalizedText{"ru": "Название", "en": "Title"}
	if value := text.Value("de", "ru"); value != "Название" {
		t.Fatalf("unexpected fallback: %q", value)
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
		CreatedAt:  now,
		UpdatedAt:  now,
		PublishedAt: &now,
	}
}
