package taxonomy

import (
	"testing"

	"github.com/fastygo/backend/internal/domain/content"
)

func TestHierarchyValidation(t *testing.T) {
	definition := Definition{
		ID: "category", Mode: ModeHierarchical,
		AssignedToKinds: []content.Kind{"product"}, Version: 1,
	}
	terms := []Term{
		{ID: "women", TaxonomyID: "category", Name: localized("Women"), Slug: localized("women"), Version: 1},
		{ID: "dresses", TaxonomyID: "category", Name: localized("Dresses"), Slug: localized("dresses"), ParentID: "women", Version: 1},
	}
	if err := ValidateHierarchy(definition, terms); err != nil {
		t.Fatalf("valid hierarchy rejected: %v", err)
	}

	terms[0].ParentID = "dresses"
	if err := ValidateHierarchy(definition, terms); err == nil {
		t.Fatalf("taxonomy cycle must fail")
	}
}

func TestFlatTaxonomyRejectsParents(t *testing.T) {
	definition := Definition{
		ID: "tag", Mode: ModeFlat,
		AssignedToKinds: []content.Kind{content.KindPost}, Version: 1,
	}
	term := Term{
		ID: "go", TaxonomyID: "tag", Name: localized("Go"), Slug: localized("go"),
		ParentID: "programming", Version: 1,
	}
	if err := term.Validate(definition); err == nil {
		t.Fatalf("flat taxonomy accepted a parent")
	}
}

func TestDefinitionRestrictsAssignedKinds(t *testing.T) {
	definition := Definition{
		ID: "topic", Mode: ModeFlat,
		AssignedToKinds: []content.Kind{content.KindPost}, Version: 1,
	}
	if !definition.Allows(content.KindPost) || definition.Allows("product") {
		t.Fatalf("taxonomy kind restriction is inconsistent")
	}
}

func localized(value string) content.LocalizedText {
	return content.LocalizedText{"en": value}
}
