package persist

import (
	"encoding/json"
	"fmt"

	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/taxonomy"
)

type Definition struct {
	ID              string                `json:"id"`
	Label           content.LocalizedText `json:"label"`
	Mode            taxonomy.Mode         `json:"mode"`
	AssignedToKinds []content.Kind        `json:"assigned_to_kinds"`
	Public          bool                  `json:"public"`
	Version         uint64                `json:"version"`
}

type Term struct {
	ID          taxonomy.ID           `json:"id"`
	TaxonomyID  string                `json:"taxonomy_id"`
	Name        content.LocalizedText `json:"name"`
	Slug        content.LocalizedText `json:"slug"`
	Description content.LocalizedText `json:"description,omitempty"`
	ParentID    taxonomy.ID           `json:"parent_id,omitempty"`
	Version     uint64                `json:"version"`
}

func DefinitionFromDomain(item taxonomy.Definition) Definition {
	return Definition{
		ID: item.ID, Label: item.Label, Mode: item.Mode,
		AssignedToKinds: item.AssignedToKinds, Public: item.Public, Version: item.Version,
	}
}

func (item Definition) Domain() taxonomy.Definition {
	return taxonomy.Definition{
		ID: item.ID, Label: item.Label, Mode: item.Mode,
		AssignedToKinds: item.AssignedToKinds, Public: item.Public, Version: item.Version,
	}
}

func TermFromDomain(item taxonomy.Term) Term {
	return Term{
		ID: item.ID, TaxonomyID: item.TaxonomyID, Name: item.Name, Slug: item.Slug,
		Description: item.Description, ParentID: item.ParentID, Version: item.Version,
	}
}

func (item Term) Domain() taxonomy.Term {
	return taxonomy.Term{
		ID: item.ID, TaxonomyID: item.TaxonomyID, Name: item.Name, Slug: item.Slug,
		Description: item.Description, ParentID: item.ParentID, Version: item.Version,
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
