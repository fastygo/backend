package contentcompat

import (
	"sort"
	"strings"
	"time"

	"github.com/fastygo/codex/content"
)

const (
	// ChromeMetaKey stores v0.3 chrome maps on a v0.4 entry so go/v2 can
	// still project LocalizedText objects. It is never part of the public
	// Codex projection.
	ChromeMetaKey = "__chrome__"
	// LocalesMetaKey stores FormSet locale documents (ADR-0009). Codex v0.4
	// dropped Entry.Locales; the backend keeps them as private sidecar data.
	LocalesMetaKey = "__locales__"
)

// LocaleDocument is a FormSet document for one locale.
type LocaleDocument struct {
	Data      map[string]any `json:"data,omitempty"`
	Status    content.Status `json:"status,omitempty"`
	UpdatedAt time.Time      `json:"updated_at,omitempty"`
}

// LocaleResolution is the go/v2 locale envelope (served / requested / fallback).
type LocaleResolution struct {
	Requested string         `json:"requested"`
	Served    string         `json:"served"`
	Fallback  bool           `json:"fallback"`
	Data      map[string]any `json:"data"`
	Status    content.Status `json:"status,omitempty"`
}

// Chrome is the per-locale chrome maps for one stored entry id.
type Chrome struct {
	Slug    LocalizedText `json:"slug,omitempty"`
	Title   LocalizedText `json:"title,omitempty"`
	Content LocalizedText `json:"content,omitempty"`
	Excerpt LocalizedText `json:"excerpt,omitempty"`
}

// NormalizeLocale trims a locale tag. Region case is preserved (Codex v0.4).
func NormalizeLocale(locale string) string {
	return strings.TrimSpace(locale)
}

// IsInternalMeta reports metadata keys that must not leak through REST flattening
// or Codex canonicalization.
func IsInternalMeta(key string) bool {
	return strings.HasPrefix(key, "__") || strings.HasPrefix(key, "payload_")
}

// DefaultLocale returns entry.Locale or content.DefaultLocale.
func DefaultLocale(entry content.Entry) string {
	if entry.Locale != "" {
		return entry.Locale
	}
	return content.DefaultLocale
}

// EnsureLocale sets Locale when empty so v0.4 Validate can succeed.
func EnsureLocale(entry *content.Entry) {
	if entry == nil {
		return
	}
	entry.Locale = NormalizeLocale(entry.Locale)
	if entry.Locale == "" {
		entry.Locale = content.DefaultLocale
	}
}

// ChromeFrom reconstructs chrome maps packed on the entry, or wraps the v0.4 strings.
func ChromeFrom(entry content.Entry) Chrome {
	if packed, ok := unpackChrome(entry); ok {
		return packed
	}
	locale := DefaultLocale(entry)
	return Chrome{
		Slug:    single(locale, entry.Slug),
		Title:   single(locale, entry.Title),
		Content: single(locale, entry.Content),
		Excerpt: single(locale, entry.Excerpt),
	}
}

// AttachChrome writes chrome maps into sidecar metadata and syncs v0.4 strings
// for entry.Locale.
func AttachChrome(entry *content.Entry, chrome Chrome) {
	if entry == nil {
		return
	}
	EnsureLocale(entry)
	locale := entry.Locale
	if value := chrome.Slug.Value(locale, content.DefaultLocale); value != "" {
		entry.Slug = value
	}
	if value := chrome.Title.Value(locale, content.DefaultLocale); value != "" {
		entry.Title = value
	}
	entry.Content = chrome.Content.Value(locale, content.DefaultLocale)
	entry.Excerpt = chrome.Excerpt.Value(locale, content.DefaultLocale)
	packChrome(entry, chrome)
}

// SyncChrome writes the v0.4 strings for entry.Locale into the chrome sidecar
// without dropping other locales.
func SyncChrome(entry *content.Entry) {
	if entry == nil {
		return
	}
	EnsureLocale(entry)
	chrome := ChromeFrom(*entry)
	if chrome.Slug == nil {
		chrome.Slug = LocalizedText{}
	}
	if chrome.Title == nil {
		chrome.Title = LocalizedText{}
	}
	if chrome.Content == nil {
		chrome.Content = LocalizedText{}
	}
	if chrome.Excerpt == nil {
		chrome.Excerpt = LocalizedText{}
	}
	locale := entry.Locale
	if entry.Slug != "" {
		chrome.Slug[locale] = entry.Slug
	}
	if entry.Title != "" {
		chrome.Title[locale] = entry.Title
	}
	chrome.Content[locale] = entry.Content
	chrome.Excerpt[locale] = entry.Excerpt
	packChrome(entry, chrome)
}

// PutDocument upserts one FormSet locale document on the sidecar.
func PutDocument(entry *content.Entry, locale string, document LocaleDocument) {
	if entry == nil {
		return
	}
	locale = NormalizeLocale(locale)
	if locale == "" {
		return
	}
	documents := Documents(*entry)
	documents[locale] = document
	SetDocuments(entry, documents)
}

// SlugAt returns the slug for locale from the record or chrome maps.
func SlugAt(entry content.Entry, locale string) string {
	locale = NormalizeLocale(locale)
	if locale == "" || entry.Locale == locale {
		if entry.Slug != "" {
			return entry.Slug
		}
	}
	return ChromeFrom(entry).Slug[locale]
}

// ProjectExact copies chrome strings for locale onto the v0.4 fields without fallback.
func ProjectExact(entry content.Entry, locale string) (content.Entry, bool) {
	locale = NormalizeLocale(locale)
	if locale == "" {
		locale = DefaultLocale(entry)
	}
	if !HasLocale(entry, locale) {
		return content.Entry{}, false
	}
	cloned := ProjectLocale(entry, locale)
	cloned.Locale = locale
	chrome := ChromeFrom(entry)
	if value, ok := chrome.Slug[locale]; ok {
		cloned.Slug = value
	}
	if value, ok := chrome.Title[locale]; ok {
		cloned.Title = value
	}
	if value, ok := chrome.Content[locale]; ok {
		cloned.Content = value
	}
	if value, ok := chrome.Excerpt[locale]; ok {
		cloned.Excerpt = value
	}
	return cloned, true
}

// Documents returns FormSet locale documents packed on the entry.
func Documents(entry content.Entry) map[string]LocaleDocument {
	raw, ok := entry.Metadata[LocalesMetaKey]
	if !ok {
		return map[string]LocaleDocument{}
	}
	return decodeDocuments(raw.Value)
}

// SetDocuments stores FormSet locale documents on the entry sidecar.
func SetDocuments(entry *content.Entry, documents map[string]LocaleDocument) {
	if entry == nil {
		return
	}
	if entry.Metadata == nil {
		entry.Metadata = map[string]content.MetadataValue{}
	}
	if len(documents) == 0 {
		delete(entry.Metadata, LocalesMetaKey)
		return
	}
	entry.Metadata[LocalesMetaKey] = content.MetadataValue{Value: encodeDocuments(documents), Private: true}
}

// LocaleIndex returns sorted locale tags that have a FormSet document.
func LocaleIndex(entry content.Entry) []string {
	documents := Documents(entry)
	locales := make([]string, 0, len(documents))
	for locale := range documents {
		locales = append(locales, locale)
	}
	sort.Strings(locales)
	return locales
}

// ResolveDocument returns one FormSet document with whole-document fallback.
func ResolveDocument(entry content.Entry, requested, fallback string) LocaleResolution {
	requested = NormalizeLocale(requested)
	fallback = NormalizeLocale(fallback)
	if requested == "" {
		requested = fallback
	}
	if requested == "" {
		requested = content.DefaultLocale
	}
	if fallback == "" {
		fallback = content.DefaultLocale
	}
	documents := Documents(entry)
	if document, ok := documents[requested]; ok && document.Data != nil {
		return LocaleResolution{Requested: requested, Served: requested, Data: document.Data, Status: document.Status}
	}
	if document, ok := documents[fallback]; ok && document.Data != nil {
		return LocaleResolution{Requested: requested, Served: fallback, Fallback: true, Data: document.Data, Status: document.Status}
	}
	locales := LocaleIndex(entry)
	for _, locale := range locales {
		document := documents[locale]
		if document.Data == nil {
			continue
		}
		return LocaleResolution{
			Requested: requested, Served: locale, Fallback: locale != requested,
			Data: document.Data, Status: document.Status,
		}
	}
	return LocaleResolution{Requested: requested, Served: requested, Data: map[string]any{}}
}

// MergeDocuments overlays patch onto target.
func MergeDocuments(target, patch map[string]LocaleDocument) map[string]LocaleDocument {
	if len(patch) == 0 {
		return target
	}
	if target == nil {
		target = map[string]LocaleDocument{}
	}
	for locale, document := range patch {
		locale = NormalizeLocale(locale)
		if locale == "" {
			continue
		}
		current := target[locale]
		if document.Data != nil {
			current.Data = document.Data
		}
		if document.Status.Valid() {
			current.Status = document.Status
		}
		if !document.UpdatedAt.IsZero() {
			current.UpdatedAt = document.UpdatedAt
		}
		target[locale] = current
	}
	return target
}

// PublicCodex strips private metadata and backend sidecar keys before canonicalization.
func PublicCodex(entry content.Entry) content.Entry {
	projected := entry.PublicProjection()
	for key := range projected.Metadata {
		if IsInternalMeta(key) {
			delete(projected.Metadata, key)
		}
	}
	if len(projected.Metadata) == 0 {
		projected.Metadata = nil
	}
	projected.CreatedAt = projected.CreatedAt.UTC().Truncate(time.Second)
	projected.UpdatedAt = projected.UpdatedAt.UTC().Truncate(time.Second)
	if projected.PublishedAt != nil {
		value := projected.PublishedAt.UTC().Truncate(time.Second)
		projected.PublishedAt = &value
	}
	if projected.DeletedAt != nil {
		value := projected.DeletedAt.UTC().Truncate(time.Second)
		projected.DeletedAt = &value
	}
	return projected
}

// HasLocale reports whether the entry can be served for locale (exact chrome or record locale).
func HasLocale(entry content.Entry, locale string) bool {
	locale = NormalizeLocale(locale)
	if locale == "" {
		return true
	}
	if entry.Locale == locale {
		return true
	}
	chrome := ChromeFrom(entry)
	if _, ok := chrome.Slug[locale]; ok {
		return true
	}
	if _, ok := chrome.Title[locale]; ok {
		return true
	}
	if _, ok := Documents(entry)[locale]; ok {
		return true
	}
	return false
}

// ProjectLocale copies chrome strings for locale onto the v0.4 fields.
func ProjectLocale(entry content.Entry, locale string) content.Entry {
	cloned := entry.Clone()
	locale = NormalizeLocale(locale)
	if locale == "" {
		locale = DefaultLocale(cloned)
	}
	cloned.Locale = locale
	chrome := ChromeFrom(cloned)
	cloned.Slug = chrome.Slug.Value(locale, DefaultLocale(entry))
	cloned.Title = chrome.Title.Value(locale, DefaultLocale(entry))
	cloned.Content = chrome.Content.Value(locale, DefaultLocale(entry))
	cloned.Excerpt = chrome.Excerpt.Value(locale, DefaultLocale(entry))
	return cloned
}

func packChrome(entry *content.Entry, chrome Chrome) {
	if entry.Metadata == nil {
		entry.Metadata = map[string]content.MetadataValue{}
	}
	if chrome.Slug.Empty() && chrome.Title.Empty() && chrome.Content.Empty() && chrome.Excerpt.Empty() {
		delete(entry.Metadata, ChromeMetaKey)
		return
	}
	entry.Metadata[ChromeMetaKey] = content.MetadataValue{
		Value: map[string]any{
			"slug":    mapString(chrome.Slug),
			"title":   mapString(chrome.Title),
			"content": mapString(chrome.Content),
			"excerpt": mapString(chrome.Excerpt),
		},
		Private: true,
	}
}

func unpackChrome(entry content.Entry) (Chrome, bool) {
	raw, ok := entry.Metadata[ChromeMetaKey]
	if !ok {
		return Chrome{}, false
	}
	root, _ := raw.Value.(map[string]any)
	if root == nil {
		return Chrome{}, false
	}
	chrome := Chrome{
		Slug:    asText(root["slug"]),
		Title:   asText(root["title"]),
		Content: asText(root["content"]),
		Excerpt: asText(root["excerpt"]),
	}
	if chrome.Slug.Empty() && chrome.Title.Empty() && chrome.Content.Empty() && chrome.Excerpt.Empty() {
		return Chrome{}, false
	}
	return chrome, true
}

func asText(value any) LocalizedText {
	switch typed := value.(type) {
	case LocalizedText:
		return typed.clone()
	case map[string]string:
		text := make(LocalizedText, len(typed))
		for locale, item := range typed {
			text[locale] = item
		}
		return text
	case map[string]any:
		text := make(LocalizedText, len(typed))
		for locale, item := range typed {
			text[locale] = stringify(item)
		}
		return text
	default:
		return nil
	}
}

func mapString(text LocalizedText) map[string]string {
	if text == nil {
		return nil
	}
	out := make(map[string]string, len(text))
	for locale, value := range text {
		out[locale] = value
	}
	return out
}

func stringify(value any) string {
	if value == nil {
		return ""
	}
	text, _ := value.(string)
	return text
}

func encodeDocuments(documents map[string]LocaleDocument) map[string]any {
	out := make(map[string]any, len(documents))
	for locale, document := range documents {
		item := map[string]any{"data": document.Data}
		if document.Status != "" {
			item["status"] = string(document.Status)
		}
		if !document.UpdatedAt.IsZero() {
			item["updated_at"] = document.UpdatedAt.UTC().Format(time.RFC3339)
		}
		out[locale] = item
	}
	return out
}

func decodeDocuments(value any) map[string]LocaleDocument {
	root, _ := value.(map[string]any)
	if root == nil {
		return map[string]LocaleDocument{}
	}
	documents := make(map[string]LocaleDocument, len(root))
	for locale, raw := range root {
		item, _ := raw.(map[string]any)
		if item == nil {
			continue
		}
		document := LocaleDocument{Data: asObject(item["data"])}
		if document.Data == nil {
			if data := asObject(item); data != nil && item["data"] == nil && item["status"] == nil {
				document.Data = data
			}
		}
		if status, _ := item["status"].(string); status != "" {
			document.Status = content.Status(status)
		}
		if stamp, _ := item["updated_at"].(string); stamp != "" {
			if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
				document.UpdatedAt = parsed
			}
		}
		documents[NormalizeLocale(locale)] = document
	}
	return documents
}

func asObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	default:
		return nil
	}
}
