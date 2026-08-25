package content

import (
	"errors"
	"strings"
	"time"
	"unicode"
)

type ID string
type Kind string
type Status string
type Visibility string

const (
	KindPost Kind = "post"
	KindPage Kind = "page"
)

const (
	StatusDraft     Status = "draft"
	StatusScheduled Status = "scheduled"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
	StatusTrashed   Status = "trashed"
)

const (
	VisibilityPublic  Visibility = "public"
	VisibilityPrivate Visibility = "private"
)

// LocalizedText stores values by normalized locale identifier.
type LocalizedText map[string]string

// Value resolves a locale and then a deterministic fallback.
func (text LocalizedText) Value(locale, fallback string) string {
	if value := strings.TrimSpace(text[normalizeLocale(locale)]); value != "" {
		return value
	}
	return strings.TrimSpace(text[normalizeLocale(fallback)])
}

// MetadataValue carries an explicit visibility policy.
type MetadataValue struct {
	Value   any  `json:"value"`
	Private bool `json:"private,omitempty"`
}

type TermRef struct {
	Taxonomy string `json:"taxonomy"`
	TermID   string `json:"term_id"`
}

// Entry is the protocol-neutral content aggregate shared by all adapters.
type Entry struct {
	ID              ID                       `json:"id"`
	Kind            Kind                     `json:"kind"`
	Status          Status                   `json:"status"`
	Visibility      Visibility               `json:"visibility"`
	Slug            LocalizedText            `json:"slug"`
	Title           LocalizedText            `json:"title"`
	Content         LocalizedText            `json:"content"`
	Excerpt         LocalizedText            `json:"excerpt"`
	AuthorID        string                   `json:"author_id"`
	ParentID        ID                       `json:"parent_id,omitempty"`
	FeaturedMediaID string                   `json:"featured_media_id,omitempty"`
	Template        string                   `json:"template,omitempty"`
	Metadata        map[string]MetadataValue `json:"metadata,omitempty"`
	Terms           []TermRef                `json:"taxonomy_ids,omitempty"`
	Version         uint64                   `json:"version"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	PublishedAt     *time.Time               `json:"published_at,omitempty"`
	DeletedAt       *time.Time               `json:"deleted_at,omitempty"`
}

// Validate verifies invariants that are independent from storage and delivery.
func (entry Entry) Validate() error {
	switch {
	case strings.TrimSpace(string(entry.ID)) == "":
		return errors.New("content id is required")
	case !ValidKind(entry.Kind):
		return errors.New("content kind is invalid")
	case !entry.Status.Valid():
		return errors.New("content status is invalid")
	case !entry.Visibility.Valid():
		return errors.New("content visibility is invalid")
	case entry.Version == 0:
		return errors.New("content version is required")
	case entry.CreatedAt.IsZero() || entry.UpdatedAt.IsZero():
		return errors.New("content timestamps are required")
	case entry.UpdatedAt.Before(entry.CreatedAt):
		return errors.New("content updated_at precedes created_at")
	case entry.ParentID != "" && entry.ParentID == entry.ID:
		return errors.New("content cannot be its own parent")
	case entry.Status == StatusScheduled && entry.PublishedAt == nil:
		return errors.New("scheduled content requires published_at")
	case entry.Status == StatusScheduled && !entry.PublishedAt.After(entry.UpdatedAt):
		return errors.New("scheduled content requires a future published_at")
	case entry.Status == StatusTrashed && entry.DeletedAt == nil:
		return errors.New("trashed content requires deleted_at")
	case !hasLocalizedValue(entry.Slug):
		return errors.New("content slug is required")
	case !hasLocalizedValue(entry.Title):
		return errors.New("content title is required")
	default:
		return nil
	}
}

// IsPublicAt applies the public visibility contract at a point in time.
func (entry Entry) IsPublicAt(now time.Time) bool {
	if entry.Visibility != VisibilityPublic || entry.DeletedAt != nil {
		return false
	}
	if entry.Status != StatusPublished {
		return false
	}
	return entry.PublishedAt == nil || !entry.PublishedAt.After(now)
}

// PublicProjection removes metadata that is not public.
func (entry Entry) PublicProjection() Entry {
	projected := entry
	projected.Metadata = make(map[string]MetadataValue, len(entry.Metadata))
	for key, value := range entry.Metadata {
		if !value.Private {
			projected.Metadata[key] = value
		}
	}
	return projected
}

// NormalizeSlug provides one stable Unicode-aware slug algorithm.
func NormalizeSlug(value string) string {
	var result strings.Builder
	pendingSeparator := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case unicode.IsLetter(character) || unicode.IsNumber(character):
			if pendingSeparator && result.Len() > 0 {
				result.WriteByte('-')
			}
			result.WriteRune(character)
			pendingSeparator = false
		default:
			pendingSeparator = result.Len() > 0
		}
	}
	return result.String()
}

func (status Status) Valid() bool {
	switch status {
	case StatusDraft, StatusScheduled, StatusPublished, StatusArchived, StatusTrashed:
		return true
	default:
		return false
	}
}

func (visibility Visibility) Valid() bool {
	return visibility == VisibilityPublic || visibility == VisibilityPrivate
}

func ValidKind(kind Kind) bool {
	value := strings.TrimSpace(string(kind))
	if value == "" {
		return false
	}
	for index, character := range value {
		if unicode.IsLower(character) || unicode.IsDigit(character) || (index > 0 && (character == '_' || character == '-')) {
			continue
		}
		return false
	}
	return true
}

func hasLocalizedValue(values LocalizedText) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func normalizeLocale(locale string) string {
	return strings.ToLower(strings.TrimSpace(locale))
}
