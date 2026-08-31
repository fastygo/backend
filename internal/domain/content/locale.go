package content

import (
	"strings"
	"time"
)

// LocaleDocument is one FormSet data document for a locale.
type LocaleDocument struct {
	Data      map[string]any
	Status    Status
	UpdatedAt time.Time
}

// LocaleResolution is a whole-document read with explicit fallback.
type LocaleResolution struct {
	Requested string
	Served    string
	Fallback  bool
	Data      map[string]any
	Status    Status
}

const defaultLocale = "en"

// NormalizeLocale lowercases and trims a locale tag.
func NormalizeLocale(locale string) string {
	return strings.ToLower(strings.TrimSpace(locale))
}

// LiftLocaleMetadata moves payload_<locale> metadata into Locales.
func (entry *Entry) LiftLocaleMetadata() {
	if entry.Locales == nil {
		entry.Locales = map[string]LocaleDocument{}
	}
	if entry.Metadata == nil {
		return
	}
	for key, value := range entry.Metadata {
		locale, ok := strings.CutPrefix(key, "payload_")
		if !ok {
			continue
		}
		locale = NormalizeLocale(locale)
		if locale == "" {
			continue
		}
		document, _ := value.Value.(map[string]any)
		if document == nil {
			continue
		}
		if _, exists := entry.Locales[locale]; !exists {
			entry.Locales[locale] = LocaleDocument{
				Data:   document,
				Status: entry.Status,
			}
		}
		delete(entry.Metadata, key)
	}
}

// ResolveLocale returns one locale document. Fallback is the whole default
// document, never a field mix. Empty requested uses defaultLocale.
func (entry Entry) ResolveLocale(requested, fallback string) LocaleResolution {
	requested = NormalizeLocale(requested)
	if requested == "" {
		requested = defaultLocale
	}
	fallback = NormalizeLocale(fallback)
	if fallback == "" {
		fallback = defaultLocale
	}
	if document, ok := entry.Locales[requested]; ok && len(document.Data) > 0 {
		return LocaleResolution{
			Requested: requested, Served: requested, Data: document.Data, Status: document.Status,
		}
	}
	if document, ok := entry.Locales[fallback]; ok && len(document.Data) > 0 {
		return LocaleResolution{
			Requested: requested, Served: fallback, Fallback: true, Data: document.Data, Status: document.Status,
		}
	}
	for locale, document := range entry.Locales {
		if len(document.Data) == 0 {
			continue
		}
		return LocaleResolution{
			Requested: requested, Served: locale, Fallback: locale != requested,
			Data: document.Data, Status: document.Status,
		}
	}
	return LocaleResolution{Requested: requested, Served: requested, Data: map[string]any{}}
}

// MergeLocales writes only the locales present in patch.
func MergeLocales(target map[string]LocaleDocument, patch map[string]LocaleDocument) map[string]LocaleDocument {
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
		target[locale] = document
	}
	return target
}

// LocaleIndex lists stored locales without their documents.
func (entry Entry) LocaleIndex() []map[string]any {
	index := make([]map[string]any, 0, len(entry.Locales))
	for locale, document := range entry.Locales {
		index = append(index, map[string]any{
			"locale": locale, "status": document.Status,
		})
	}
	return index
}
