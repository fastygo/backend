package persist

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

type Definition struct {
	ID              string                      `json:"id"`
	Locale          string                      `json:"locale,omitempty"`
	Label           contentcompat.LocalizedText `json:"label"`
	Mode            taxonomy.Mode               `json:"mode"`
	AssignedToKinds []content.Kind              `json:"assigned_to_kinds"`
	Public          bool                        `json:"public"`
	Version         uint64                      `json:"version"`
}

type Term struct {
	ID          taxonomy.ID                 `json:"id"`
	Locale      string                      `json:"locale,omitempty"`
	TaxonomyID  string                      `json:"taxonomy_id"`
	Name        contentcompat.LocalizedText `json:"name"`
	Slug        contentcompat.LocalizedText `json:"slug"`
	Description contentcompat.LocalizedText `json:"description,omitempty"`
	ParentID    taxonomy.ID                 `json:"parent_id,omitempty"`
	Origin      []content.Origin            `json:"origin,omitempty"`
	Metadata    map[string]MetadataValue    `json:"metadata,omitempty"`
	Version     uint64                      `json:"version"`
	CreatedAt   time.Time                   `json:"created_at,omitempty"`
	UpdatedAt   time.Time                   `json:"updated_at,omitempty"`
}

type wireDefinition struct {
	ID              string          `json:"id"`
	Locale          string          `json:"locale"`
	Label           json.RawMessage `json:"label"`
	Mode            taxonomy.Mode   `json:"mode"`
	AssignedToKinds []content.Kind  `json:"assigned_to_kinds"`
	Public          bool            `json:"public"`
	Version         uint64          `json:"version"`
}

type wireTerm struct {
	ID          taxonomy.ID              `json:"id"`
	Locale      string                   `json:"locale"`
	TaxonomyID  string                   `json:"taxonomy_id"`
	Name        json.RawMessage          `json:"name"`
	Slug        json.RawMessage          `json:"slug"`
	Description json.RawMessage          `json:"description"`
	ParentID    taxonomy.ID              `json:"parent_id"`
	Origin      []content.Origin         `json:"origin"`
	Metadata    map[string]MetadataValue `json:"metadata"`
	Version     uint64                   `json:"version"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
}

func (item *Definition) UnmarshalJSON(data []byte) error {
	var wire wireDefinition
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	locale := contentcompat.NormalizeLocale(wire.Locale)
	item.ID = wire.ID
	item.Locale = locale
	item.Label = decodeText(wire.Label, locale)
	item.Mode = wire.Mode
	item.AssignedToKinds = wire.AssignedToKinds
	item.Public = wire.Public
	item.Version = wire.Version
	if item.Locale == "" {
		item.Locale = firstLocale(item.Label)
	}
	return nil
}

func (item *Term) UnmarshalJSON(data []byte) error {
	var wire wireTerm
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	locale := contentcompat.NormalizeLocale(wire.Locale)
	item.ID = wire.ID
	item.Locale = locale
	item.TaxonomyID = wire.TaxonomyID
	item.Name = decodeText(wire.Name, locale)
	item.Slug = decodeText(wire.Slug, locale)
	item.Description = decodeText(wire.Description, locale)
	item.ParentID = wire.ParentID
	item.Origin = wire.Origin
	item.Metadata = wire.Metadata
	item.Version = wire.Version
	item.CreatedAt = wire.CreatedAt
	item.UpdatedAt = wire.UpdatedAt
	if item.Locale == "" {
		item.Locale = firstLocale(item.Name, item.Slug)
	}
	return nil
}

func DefinitionFromDomain(item taxonomy.Definition) Definition {
	locale := contentcompat.NormalizeLocale(item.Locale)
	if locale == "" {
		locale = content.DefaultLocale
	}
	return Definition{
		ID: item.ID, Locale: locale, Label: contentcompat.LocalizedText{locale: item.Label},
		Mode: item.Mode, AssignedToKinds: item.AssignedToKinds, Public: item.Public, Version: item.Version,
	}
}

func (item Definition) Domain() taxonomy.Definition {
	locale := contentcompat.NormalizeLocale(item.Locale)
	if locale == "" {
		locale = firstLocale(item.Label)
	}
	return taxonomy.Definition{
		ID: item.ID, Locale: locale, Label: item.Label.Value(locale, content.DefaultLocale),
		Mode: item.Mode, AssignedToKinds: item.AssignedToKinds, Public: item.Public, Version: item.Version,
	}
}

func TermFromDomain(item taxonomy.Term) Term {
	locale := contentcompat.NormalizeLocale(item.Locale)
	if locale == "" {
		locale = content.DefaultLocale
	}
	metadata := make(map[string]MetadataValue, len(item.Metadata))
	for key, value := range item.Metadata {
		metadata[key] = MetadataValue{Value: value.Value, Private: value.Private}
	}
	return Term{
		ID: item.ID, Locale: locale, TaxonomyID: item.TaxonomyID,
		Name:        contentcompat.LocalizedText{locale: item.Name},
		Slug:        contentcompat.LocalizedText{locale: item.Slug},
		Description: contentcompat.LocalizedText{locale: item.Description},
		ParentID:    item.ParentID, Origin: item.Origin, Metadata: metadata, Version: item.Version,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func (item Term) Domain() taxonomy.Term {
	locale := contentcompat.NormalizeLocale(item.Locale)
	if locale == "" {
		locale = firstLocale(item.Name, item.Slug)
	}
	metadata := make(map[string]content.MetadataValue, len(item.Metadata))
	for key, value := range item.Metadata {
		metadata[key] = content.MetadataValue{Value: value.Value, Private: value.Private}
	}
	created, updated := item.CreatedAt, item.UpdatedAt
	if created.IsZero() {
		created = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	if updated.IsZero() {
		updated = created
	}
	return taxonomy.Term{
		ID: item.ID, Locale: locale, TaxonomyID: item.TaxonomyID,
		Name:        item.Name.Value(locale, content.DefaultLocale),
		Slug:        item.Slug.Value(locale, content.DefaultLocale),
		Description: item.Description.Value(locale, content.DefaultLocale),
		ParentID:    item.ParentID, Origin: item.Origin, Metadata: metadata, Version: item.Version,
		CreatedAt: created, UpdatedAt: updated,
	}
}

func EncodeDefinition(item taxonomy.Definition) ([]byte, error) {
	encoded, err := json.Marshal(DefinitionFromDomain(item))
	if err != nil {
		return nil, fmt.Errorf("failed to encode taxonomy definition: %w", err)
	}
	return encoded, nil
}

func DecodeDefinition(encoded []byte) (taxonomy.Definition, error) {
	var document Definition
	if err := json.Unmarshal(encoded, &document); err != nil {
		return taxonomy.Definition{}, fmt.Errorf("failed to decode taxonomy definition: %w", err)
	}
	return document.Domain(), nil
}

func EncodeTerm(item taxonomy.Term) ([]byte, error) {
	encoded, err := json.Marshal(TermFromDomain(item))
	if err != nil {
		return nil, fmt.Errorf("failed to encode taxonomy term: %w", err)
	}
	return encoded, nil
}

func DecodeTerm(encoded []byte) (taxonomy.Term, error) {
	var document Term
	if err := json.Unmarshal(encoded, &document); err != nil {
		return taxonomy.Term{}, fmt.Errorf("failed to decode taxonomy term: %w", err)
	}
	return document.Domain(), nil
}
