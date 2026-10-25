package persist

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/codex/content"
)

// Entry is the durable JSON document for a content aggregate.
// Chrome fields accept both v0.3 maps and v0.4 strings. FormSet locale
// documents stay in `locales` so /go-json/go/v2 can keep ADR-0009 fallback.
type Entry struct {
	ID              content.ID                              `json:"id"`
	Kind            content.Kind                            `json:"kind"`
	Locale          string                                  `json:"locale,omitempty"`
	Status          content.Status                          `json:"status"`
	Visibility      content.Visibility                      `json:"visibility"`
	Slug            contentcompat.LocalizedText             `json:"slug"`
	Title           contentcompat.LocalizedText             `json:"title"`
	Content         contentcompat.LocalizedText             `json:"content"`
	Excerpt         contentcompat.LocalizedText             `json:"excerpt"`
	AuthorID        string                                  `json:"author_id"`
	ParentID        content.ID                              `json:"parent_id,omitempty"`
	FeaturedMediaID string                                  `json:"featured_media_id,omitempty"`
	Template        string                                  `json:"template,omitempty"`
	Metadata        map[string]MetadataValue                `json:"metadata,omitempty"`
	Locales         map[string]contentcompat.LocaleDocument `json:"locales,omitempty"`
	LegacyLocale    string                                  `json:"-"`
	Data            map[string]any                          `json:"data,omitempty"`
	Terms           []TermRef                               `json:"taxonomy_ids,omitempty"`
	Order           *int64                                  `json:"order,omitempty"`
	Pinned          *bool                                   `json:"pinned,omitempty"`
	Format          string                                  `json:"format,omitempty"`
	Discussion      *content.Discussion                     `json:"discussion,omitempty"`
	Origin          []content.Origin                        `json:"origin,omitempty"`
	Version         uint64                                  `json:"version"`
	CreatedAt       time.Time                               `json:"created_at"`
	UpdatedAt       time.Time                               `json:"updated_at"`
	PublishedAt     *time.Time                              `json:"published_at,omitempty"`
	DeletedAt       *time.Time                              `json:"deleted_at,omitempty"`
}

type MetadataValue struct {
	Value   any  `json:"value"`
	Private bool `json:"private,omitempty"`
}

type TermRef struct {
	Taxonomy string `json:"taxonomy"`
	TermID   string `json:"term_id"`
}

type wireEntry struct {
	ID              content.ID                              `json:"id"`
	Kind            content.Kind                            `json:"kind"`
	Locale          string                                  `json:"locale"`
	Status          content.Status                          `json:"status"`
	Visibility      content.Visibility                      `json:"visibility"`
	Slug            json.RawMessage                         `json:"slug"`
	Title           json.RawMessage                         `json:"title"`
	Content         json.RawMessage                         `json:"content"`
	Excerpt         json.RawMessage                         `json:"excerpt"`
	AuthorID        string                                  `json:"author_id"`
	ParentID        content.ID                              `json:"parent_id"`
	FeaturedMediaID string                                  `json:"featured_media_id"`
	Template        string                                  `json:"template"`
	Metadata        map[string]MetadataValue                `json:"metadata"`
	Locales         map[string]contentcompat.LocaleDocument `json:"locales"`
	Data            map[string]any                          `json:"data"`
	Terms           []TermRef                               `json:"terms"`
	TaxonomyIDs     []TermRef                               `json:"taxonomy_ids"`
	Order           *int64                                  `json:"order"`
	Pinned          *bool                                   `json:"pinned"`
	Format          string                                  `json:"format"`
	Discussion      *content.Discussion                     `json:"discussion"`
	Origin          []content.Origin                        `json:"origin"`
	Version         uint64                                  `json:"version"`
	CreatedAt       time.Time                               `json:"created_at"`
	UpdatedAt       time.Time                               `json:"updated_at"`
	PublishedAt     *time.Time                              `json:"published_at"`
	DeletedAt       *time.Time                              `json:"deleted_at"`
}

func (entry *Entry) UnmarshalJSON(data []byte) error {
	var wire wireEntry
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	locale := contentcompat.NormalizeLocale(wire.Locale)
	entry.ID = wire.ID
	entry.Kind = wire.Kind
	entry.Locale = locale
	entry.Status = wire.Status
	entry.Visibility = wire.Visibility
	entry.Slug = decodeText(wire.Slug, locale)
	entry.Title = decodeText(wire.Title, locale)
	entry.Content = decodeText(wire.Content, locale)
	entry.Excerpt = decodeText(wire.Excerpt, locale)
	entry.AuthorID = wire.AuthorID
	entry.ParentID = wire.ParentID
	entry.FeaturedMediaID = wire.FeaturedMediaID
	entry.Template = wire.Template
	entry.Metadata = wire.Metadata
	entry.Locales = wire.Locales
	entry.Data = wire.Data
	entry.Terms = wire.TaxonomyIDs
	if len(wire.Terms) > 0 {
		entry.Terms = wire.Terms
	}
	entry.Order = wire.Order
	entry.Pinned = wire.Pinned
	entry.Format = wire.Format
	entry.Discussion = wire.Discussion
	entry.Origin = wire.Origin
	entry.Version = wire.Version
	entry.CreatedAt = wire.CreatedAt
	entry.UpdatedAt = wire.UpdatedAt
	entry.PublishedAt = wire.PublishedAt
	entry.DeletedAt = wire.DeletedAt
	if entry.Locale == "" {
		entry.Locale = firstLocale(entry.Slug, entry.Title)
	}
	return nil
}

func EntryFromDomain(entry content.Entry) Entry {
	contentcompat.Lift(&entry)
	contentcompat.EnsureLocale(&entry)
	contentcompat.SyncChrome(&entry)
	chrome := contentcompat.ChromeFrom(entry)
	if chrome.Slug.Empty() {
		chrome.Slug = contentcompat.LocalizedText{entry.Locale: entry.Slug}
	}
	if chrome.Title.Empty() {
		chrome.Title = contentcompat.LocalizedText{entry.Locale: entry.Title}
	}
	if chrome.Content.Empty() && entry.Content != "" {
		chrome.Content = contentcompat.LocalizedText{entry.Locale: entry.Content}
	}
	if chrome.Excerpt.Empty() && entry.Excerpt != "" {
		chrome.Excerpt = contentcompat.LocalizedText{entry.Locale: entry.Excerpt}
	}
	metadata := make(map[string]MetadataValue, len(entry.Metadata))
	for key, value := range entry.Metadata {
		if contentcompat.IsInternalMeta(key) {
			continue
		}
		metadata[key] = MetadataValue{Value: value.Value, Private: value.Private}
	}
	terms := make([]TermRef, 0, len(entry.Terms))
	for _, term := range entry.Terms {
		terms = append(terms, TermRef{Taxonomy: term.Taxonomy, TermID: term.TermID})
	}
	return Entry{
		ID: entry.ID, Kind: entry.Kind, Locale: entry.Locale, Status: entry.Status,
		Visibility: entry.Visibility, Slug: chrome.Slug, Title: chrome.Title,
		Content: chrome.Content, Excerpt: chrome.Excerpt, AuthorID: entry.AuthorID,
		ParentID: entry.ParentID, FeaturedMediaID: entry.FeaturedMediaID,
		Template: entry.Template, Metadata: metadata, Locales: contentcompat.Documents(entry),
		Terms: terms, Order: entry.Order, Pinned: entry.Pinned, Format: entry.Format,
		Discussion: entry.Discussion, Origin: entry.Origin, Version: entry.Version,
		CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
		PublishedAt: entry.PublishedAt, DeletedAt: entry.DeletedAt,
	}
}

func (entry Entry) Domain() content.Entry {
	locale := contentcompat.NormalizeLocale(entry.Locale)
	if locale == "" {
		locale = firstLocale(entry.Slug, entry.Title)
	}
	metadata := make(map[string]content.MetadataValue, len(entry.Metadata))
	for key, value := range entry.Metadata {
		if contentcompat.IsInternalMeta(key) {
			continue
		}
		metadata[key] = content.MetadataValue{Value: value.Value, Private: value.Private}
	}
	terms := make([]content.TermRef, 0, len(entry.Terms))
	for _, term := range entry.Terms {
		terms = append(terms, content.TermRef{Taxonomy: term.Taxonomy, TermID: term.TermID})
	}
	resolved := content.Entry{
		ID: entry.ID, Locale: locale, Kind: entry.Kind, Status: entry.Status,
		Visibility: entry.Visibility, Slug: entry.Slug.Value(locale, content.DefaultLocale),
		Title:    entry.Title.Value(locale, content.DefaultLocale),
		Content:  entry.Content.Value(locale, content.DefaultLocale),
		Excerpt:  entry.Excerpt.Value(locale, content.DefaultLocale),
		AuthorID: entry.AuthorID, ParentID: entry.ParentID, FeaturedMediaID: entry.FeaturedMediaID,
		Template: entry.Template, Metadata: metadata, Terms: terms, Order: entry.Order,
		Pinned: entry.Pinned, Format: entry.Format, Discussion: entry.Discussion,
		Origin: entry.Origin, Version: entry.Version, CreatedAt: entry.CreatedAt,
		UpdatedAt: entry.UpdatedAt, PublishedAt: entry.PublishedAt, DeletedAt: entry.DeletedAt,
	}
	contentcompat.AttachChrome(&resolved, contentcompat.Chrome{
		Slug: entry.Slug, Title: entry.Title, Content: entry.Content, Excerpt: entry.Excerpt,
	})
	documents := entry.Locales
	if loc := contentcompat.NormalizeLocale(entry.Locale); loc != "" && len(entry.Data) > 0 {
		if documents == nil {
			documents = map[string]contentcompat.LocaleDocument{}
		}
		if _, exists := documents[loc]; !exists {
			documents[loc] = contentcompat.LocaleDocument{Data: entry.Data, Status: entry.Status}
		}
	}
	contentcompat.SetDocuments(&resolved, documents)
	contentcompat.Lift(&resolved)
	return resolved
}

func EncodeEntry(entry content.Entry) ([]byte, error) {
	contentcompat.Lift(&entry)
	encoded, err := json.Marshal(EntryFromDomain(entry))
	if err != nil {
		return nil, fmt.Errorf("failed to encode content entry: %w", err)
	}
	return encoded, nil
}

func DecodeEntry(encoded []byte) (content.Entry, error) {
	var document Entry
	if err := json.Unmarshal(encoded, &document); err != nil {
		return content.Entry{}, fmt.Errorf("failed to decode content entry: %w", err)
	}
	return document.Domain(), nil
}

func decodeText(raw json.RawMessage, locale string) contentcompat.LocalizedText {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil
		}
		if locale == "" {
			locale = content.DefaultLocale
		}
		return contentcompat.LocalizedText{locale: value}
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	return values
}

func firstLocale(values ...contentcompat.LocalizedText) string {
	for _, text := range values {
		if len(text) == 0 {
			continue
		}
		if _, ok := text[content.DefaultLocale]; ok {
			return content.DefaultLocale
		}
		for key := range text {
			return key
		}
	}
	return content.DefaultLocale
}
