package taxonomy

import (
	"errors"
	"strings"

	"github.com/fastygo/backend/internal/domain/content"
)

type ID string
type Mode string

const (
	ModeFlat         Mode = "flat"
	ModeHierarchical Mode = "hierarchical"
)

type Definition struct {
	ID              string
	Label           content.LocalizedText
	Mode            Mode
	AssignedToKinds []content.Kind
	Public          bool
	Version         uint64
}

type Term struct {
	ID          ID
	TaxonomyID  string
	Name        content.LocalizedText
	Slug        content.LocalizedText
	Description content.LocalizedText
	ParentID    ID
	Version     uint64
}

type Assignment struct {
	ResourceKind content.Kind
	ResourceID   string
	TaxonomyID   string
	TermID       ID
}

func (definition Definition) Validate() error {
	if strings.TrimSpace(definition.ID) == "" {
		return errors.New("taxonomy id is required")
	}
	if definition.Version == 0 {
		return errors.New("taxonomy version is required")
	}
	if definition.Mode != ModeFlat && definition.Mode != ModeHierarchical {
		return errors.New("taxonomy mode is invalid")
	}
	if len(definition.AssignedToKinds) == 0 {
		return errors.New("taxonomy requires at least one resource kind")
	}
	for _, kind := range definition.AssignedToKinds {
		if !content.ValidKind(kind) {
			return errors.New("taxonomy resource kind is invalid")
		}
	}
	return nil
}

func (term Term) Validate(definition Definition) error {
	switch {
	case strings.TrimSpace(string(term.ID)) == "":
		return errors.New("term id is required")
	case term.TaxonomyID != definition.ID:
		return errors.New("term taxonomy does not match definition")
	case term.Version == 0:
		return errors.New("term version is required")
	case term.ParentID == term.ID:
		return errors.New("term cannot be its own parent")
	case definition.Mode == ModeFlat && term.ParentID != "":
		return errors.New("flat taxonomy cannot contain parent terms")
	case !hasLocalizedValue(term.Name):
		return errors.New("term name is required")
	case !hasLocalizedValue(term.Slug):
		return errors.New("term slug is required")
	default:
		return nil
	}
}

func (definition Definition) Allows(kind content.Kind) bool {
	for _, allowed := range definition.AssignedToKinds {
		if allowed == kind {
			return true
		}
	}
	return false
}

// ValidateHierarchy rejects missing parents and parent cycles.
func ValidateHierarchy(definition Definition, terms []Term) error {
	if err := definition.Validate(); err != nil {
		return err
	}
	byID := make(map[ID]Term, len(terms))
	for _, term := range terms {
		if err := term.Validate(definition); err != nil {
			return err
		}
		if _, exists := byID[term.ID]; exists {
			return errors.New("term id is duplicated")
		}
		byID[term.ID] = term
	}
	for _, term := range terms {
		visited := map[ID]struct{}{term.ID: {}}
		parentID := term.ParentID
		for parentID != "" {
			if _, cycle := visited[parentID]; cycle {
				return errors.New("taxonomy hierarchy contains a cycle")
			}
			visited[parentID] = struct{}{}
			parent, exists := byID[parentID]
			if !exists {
				return errors.New("taxonomy parent does not exist")
			}
			parentID = parent.ParentID
		}
	}
	return nil
}

func hasLocalizedValue(values content.LocalizedText) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}
