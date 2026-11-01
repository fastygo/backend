package contentcompat

import (
	"reflect"
	"testing"
	"time"

	"github.com/fastygo/codex/content"
)

func TestLocalizedText(t *testing.T) {
	text := LocalizedText{"en": "Hello", "ru": " ", "de": "Hallo"}
	cases := []struct{ locale, fallback, want string }{
		{"en", "", "Hello"}, {"ru", "de", "Hallo"}, {"fr", "", "Hallo"}, {"", "en", "Hello"},
	}
	for _, c := range cases {
		if got := text.Value(c.locale, c.fallback); got != c.want {
			t.Errorf("Value(%q,%q) = %q want %q", c.locale, c.fallback, got, c.want)
		}
	}
	if (LocalizedText(nil)).Value("en", "") != "" || (LocalizedText{}).Value("en", "") != "" || (LocalizedText{"en": " "}).Value("x", "") != " " {
		t.Fatal("empty values")
	}
	merged := LocalizedText(nil).Merge(LocalizedText{"en": "A"})
	if merged["en"] != "A" || len(text.Merge(nil)) != 3 {
		t.Fatal("merge")
	}
	if !(LocalizedText{"en": " "}).Empty() || text.Empty() || !LocalizedText(nil).Empty() {
		t.Fatal("empty")
	}
	if single("", "x")["en"] != "x" || single("ru", " ") != nil {
		t.Fatal("single")
	}
	if LocalizedText(nil).clone() != nil {
		t.Fatal("clone nil")
	}
}

func TestLiftMovesPayloadDocuments(t *testing.T) {
	entry := content.Entry{Status: content.StatusDraft, Metadata: map[string]content.MetadataValue{
		"payload_ru":  {Value: map[string]any{"title": "Курс"}},
		"payload_":    {Value: map[string]any{"x": 1}},
		"payload_bad": {Value: "not an object"},
		"price":       {Value: 1.0},
	}}
	Lift(&entry)
	Lift(nil)
	documents := Documents(entry)
	if documents["ru"].Data["title"] != "Курс" || documents["ru"].Status != content.StatusDraft {
		t.Fatalf("documents: %#v", documents)
	}
	if _, kept := entry.Metadata["payload_ru"]; kept {
		t.Fatal("payload_ru must be lifted")
	}
	if _, kept := entry.Metadata["payload_bad"]; !kept {
		t.Fatal("non-object payload stays for validation")
	}
	if !entry.Metadata[LocalesMetaKey].Private || LocaleIndex(entry)[0] != "ru" {
		t.Fatal("sidecar must be private and indexed")
	}
	empty := content.Entry{}
	Lift(&empty)
	if empty.Metadata != nil {
		t.Fatal("entry without metadata stays untouched")
	}
}

func TestDocumentsResolveAndMerge(t *testing.T) {
	stamp := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	entry := content.Entry{}
	PutDocument(&entry, " en ", LocaleDocument{Data: map[string]any{"a": 1.0}, Status: content.StatusPublished, UpdatedAt: stamp})
	PutDocument(&entry, "", LocaleDocument{})
	PutDocument(nil, "en", LocaleDocument{})
	PutDocument(&entry, "ru", LocaleDocument{})
	got := Documents(entry)
	if got["en"].UpdatedAt != stamp || got["en"].Status != content.StatusPublished {
		t.Fatalf("round trip: %#v", got["en"])
	}
	if r := ResolveDocument(entry, "en", ""); r.Served != "en" || r.Fallback {
		t.Fatalf("exact: %#v", r)
	}
	if r := ResolveDocument(entry, "de", "en"); r.Served != "en" || !r.Fallback {
		t.Fatalf("fallback: %#v", r)
	}
	only := content.Entry{}
	PutDocument(&only, "fr", LocaleDocument{Data: map[string]any{}})
	if r := ResolveDocument(only, "", ""); r.Served != "fr" || r.Requested != content.DefaultLocale {
		t.Fatalf("first: %#v", r)
	}
	if r := ResolveDocument(content.Entry{}, "de", ""); r.Served != "de" || r.Data == nil {
		t.Fatalf("none: %#v", r)
	}
	merged := MergeDocuments(nil, map[string]LocaleDocument{
		"en": {Data: map[string]any{"b": 2.0}, Status: content.StatusDraft, UpdatedAt: stamp}, " ": {},
	})
	if merged["en"].Data["b"] != 2.0 || len(merged) != 1 || len(MergeDocuments(merged, nil)) != 1 {
		t.Fatalf("merge: %#v", merged)
	}
	SetDocuments(&entry, nil)
	SetDocuments(nil, nil)
	if _, ok := entry.Metadata[LocalesMetaKey]; ok {
		t.Fatal("empty documents delete sidecar")
	}
	legacy := decodeDocuments(map[string]any{"en": map[string]any{"title": "x"}, "bad": "x", "ru": map[string]any{"data": map[string]any{}, "updated_at": "nope"}})
	if legacy["en"].Data["title"] != "x" || len(legacy) != 2 || len(decodeDocuments(nil)) != 0 {
		t.Fatalf("decode: %#v", legacy)
	}
}

func TestChromeAndLocaleProjection(t *testing.T) {
	entry := content.Entry{Locale: "en", Slug: "hello", Title: "Hello", Content: "Body"}
	plain := ChromeFrom(entry)
	if plain.Title["en"] != "Hello" || plain.Excerpt != nil {
		t.Fatalf("plain chrome: %#v", plain)
	}
	AttachChrome(&entry, Chrome{Slug: LocalizedText{"en": "hello", "ru": "privet"}, Title: LocalizedText{"en": "Hello", "ru": "Привет"}})
	AttachChrome(nil, Chrome{})
	if !entry.Metadata[ChromeMetaKey].Private {
		t.Fatal("chrome sidecar must be private")
	}
	if SlugAt(entry, "ru") != "privet" || SlugAt(entry, "en") != "hello" || SlugAt(entry, "") != "hello" {
		t.Fatal("slug at")
	}
	ru, ok := ProjectExact(entry, "ru")
	if !ok || ru.Title != "Привет" || ru.Locale != "ru" {
		t.Fatalf("exact ru: %#v", ru)
	}
	if _, ok := ProjectExact(entry, "de"); ok {
		t.Fatal("de is not available")
	}
	if projected, ok := ProjectExact(entry, ""); !ok || projected.Locale != "en" {
		t.Fatal("default exact")
	}
	if ProjectLocale(entry, "de").Title != "Hello" || ProjectLocale(entry, "").Locale != "en" {
		t.Fatal("project locale fallback")
	}
	if !HasLocale(entry, "") || !HasLocale(entry, "en") || HasLocale(entry, "de") {
		t.Fatal("has locale")
	}
	entry.Locale = "ru"
	entry.Title = "Новый"
	SyncChrome(&entry)
	SyncChrome(nil)
	if ChromeFrom(entry).Title["ru"] != "Новый" || ChromeFrom(entry).Title["en"] != "Hello" {
		t.Fatalf("sync: %#v", ChromeFrom(entry))
	}
	docOnly := content.Entry{Locale: "en"}
	PutDocument(&docOnly, "fr", LocaleDocument{Data: map[string]any{}})
	titleOnly := content.Entry{Locale: "en", Metadata: map[string]content.MetadataValue{ChromeMetaKey: {Value: map[string]any{"title": map[string]string{"it": "Ciao"}}}}}
	if !HasLocale(docOnly, "fr") || !HasLocale(titleOnly, "it") {
		t.Fatal("documents and title chrome count as locales")
	}
	cleared := content.Entry{Metadata: map[string]content.MetadataValue{ChromeMetaKey: {Value: "x"}}}
	packChrome(&cleared, Chrome{})
	if _, ok := cleared.Metadata[ChromeMetaKey]; ok {
		t.Fatal("empty chrome deletes sidecar")
	}
	if _, ok := unpackChrome(content.Entry{Metadata: map[string]content.MetadataValue{ChromeMetaKey: {Value: map[string]any{"slug": map[string]any{}}}}}); ok {
		t.Fatal("empty packed chrome")
	}
	if asText(LocalizedText{"en": "a"})["en"] != "a" || asText(map[string]any{"en": 1}) == nil || asText(1) != nil || stringify(nil) != "" || mapString(nil) != nil {
		t.Fatal("helpers")
	}
}

func TestLocaleHelpersAndPublicProjection(t *testing.T) {
	if NormalizeLocale(" en-US ") != "en-US" || DefaultLocale(content.Entry{}) != content.DefaultLocale || DefaultLocale(content.Entry{Locale: "ru"}) != "ru" {
		t.Fatal("locale helpers")
	}
	entry := content.Entry{}
	EnsureLocale(&entry)
	EnsureLocale(nil)
	if entry.Locale != content.DefaultLocale {
		t.Fatal("ensure locale")
	}
	for key, want := range map[string]bool{"__chrome__": true, "payload_en": true, "price": false} {
		if IsInternalMeta(key) != want {
			t.Errorf("IsInternalMeta(%q)", key)
		}
	}
	published := time.Date(2026, 11, 1, 10, 0, 0, 500, time.FixedZone("MSK", 3*3600))
	public := PublicCodex(content.Entry{
		CreatedAt: published, UpdatedAt: published, PublishedAt: &published, DeletedAt: &published,
		Metadata: map[string]content.MetadataValue{"__chrome__": {Value: map[string]any{}}, "price": {Value: 1.0}, "secret": {Value: "x", Private: true}},
	})
	if !reflect.DeepEqual(public.Metadata, map[string]content.MetadataValue{"price": {Value: 1.0}}) {
		t.Fatalf("metadata: %#v", public.Metadata)
	}
	if public.CreatedAt.Location() != time.UTC || public.PublishedAt.Nanosecond() != 0 || public.DeletedAt.Location() != time.UTC {
		t.Fatal("times must be UTC seconds")
	}
	if PublicCodex(content.Entry{Metadata: map[string]content.MetadataValue{"__x": {}}}).Metadata != nil {
		t.Fatal("empty metadata becomes nil")
	}
}
