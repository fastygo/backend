package persist

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/content"
)

// Entry is the durable JSON document for a content aggregate.
type Entry struct {
	ID              content.ID               `json:"id"`
	Kind            content.Kind             `json:"kind"`
	Status          content.Status           `json:"status"`
	Visibility      content.Visibility       `json:"visibility"`
	Slug            content.LocalizedText    `json:"slug"`
	Title           content.LocalizedText    `json:"title"`
	Content         content.LocalizedText    `json:"content"`
	Excerpt         content.LocalizedText    `json:"excerpt"`
	AuthorID        string                   `json:"author_id"`
	ParentID        content.ID               `json:"parent_id,omitempty"`
	FeaturedMediaID string                   `json:"featured_media_id,omitempty"`
	Template        string                   `json:"template,omitempty"`
	Metadata        map[string]MetadataValue `json:"metadata,omitempty"`
	Locales         map[string]LocaleDocument `json:"locales,omitempty"`
	Locale          string                   `json:"locale,omitempty"`
	Data            map[string]any           `json:"data,omitempty"`
	Terms           []TermRef                `json:"taxonomy_ids,omitempty"`
	Version         uint64                   `json:"version"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	PublishedAt     *time.Time               `json:"published_at,omitempty"`
	DeletedAt       *time.Time               `json:"deleted_at,omitempty"`
}

type LocaleDocument struct {
	Data      map[string]any  `json:"data,omitempty"`
	Status    content.Status  `json:"status,omitempty"`
	UpdatedAt time.Time       `json:"updated_at,omitempty"`
}

type MetadataValue struct {
	Value   any  `json:"value"`
	Private bool `json:"private,omitempty"`
}

type TermRef struct {
	Taxonomy string `json:"taxonomy"`
	TermID   string `json:"term_id"`
}

func EntryFromDomain(entry content.Entry) Entry {
	entry.LiftLocaleMetadata()
	metadata := make(map[string]MetadataValue, len(entry.Metadata))
	for key, value := range entry.Metadata {
		metadata[key] = MetadataValue{Value: value.Value, Private: value.Private}
	}
	terms := make([]TermRef, 0, len(entry.Terms))
	for _, term := range entry.Terms {
		terms = append(terms, TermRef{Taxonomy: term.Taxonomy, TermID: term.TermID})
	}
	locales := make(map[string]LocaleDocument, len(entry.Locales))
	for locale, document := range entry.Locales {
		locales[locale] = LocaleDocument{Data: document.Data, Status: document.Status, UpdatedAt: document.UpdatedAt}
	}
	for key := range metadata {
		if _, ok := strings.CutPrefix(key, "payload_"); ok {
			delete(metadata, key)
		}
	}
	return Entry{
		ID: entry.ID, Kind: entry.Kind, Status: entry.Status, Visibility: entry.Visibility,
		Slug: entry.Slug, Title: entry.Title, Content: entry.Content, Excerpt: entry.Excerpt,
		AuthorID: entry.AuthorID, ParentID: entry.ParentID, FeaturedMediaID: entry.FeaturedMediaID,
		Template: entry.Template, Metadata: metadata, Locales: locales, Terms: terms, Version: entry.Version,
		CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
		PublishedAt: entry.PublishedAt, DeletedAt: entry.DeletedAt,
	}
}

func (entry Entry) Domain() content.Entry {
	metadata := make(map[string]content.MetadataValue, len(entry.Metadata))
	for key, value := range entry.Metadata {
		metadata[key] = content.MetadataValue{Value: value.Value, Private: value.Private}
	}
	terms := make([]content.TermRef, 0, len(entry.Terms))
	for _, term := range entry.Terms {
		terms = append(terms, content.TermRef{Taxonomy: term.Taxonomy, TermID: term.TermID})
	}
	locales := make(map[string]content.LocaleDocument, len(entry.Locales))
	for locale, document := range entry.Locales {
		locales[content.NormalizeLocale(locale)] = content.LocaleDocument{
			Data: document.Data, Status: document.Status, UpdatedAt: document.UpdatedAt,
		}
	}
	if loc := content.NormalizeLocale(entry.Locale); loc != "" && len(entry.Data) > 0 {
		if _, exists := locales[loc]; !exists {
			locales[loc] = content.LocaleDocument{Data: entry.Data, Status: entry.Status}
		}
	}
	resolved := content.Entry{
		ID: entry.ID, Kind: entry.Kind, Status: entry.Status, Visibility: entry.Visibility,
		Slug: entry.Slug, Title: entry.Title, Content: entry.Content, Excerpt: entry.Excerpt,
		AuthorID: entry.AuthorID, ParentID: entry.ParentID, FeaturedMediaID: entry.FeaturedMediaID,
		Template: entry.Template, Metadata: metadata, Locales: locales, Terms: terms, Version: entry.Version,
		CreatedAt: entry.CreatedAt, UpdatedAt: entry.UpdatedAt,
		PublishedAt: entry.PublishedAt, DeletedAt: entry.DeletedAt,
	}
	resolved.LiftLocaleMetadata()
	return resolved
}

func EncodeEntry(entry content.Entry) ([]byte, error) {
	entry.LiftLocaleMetadata()
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
