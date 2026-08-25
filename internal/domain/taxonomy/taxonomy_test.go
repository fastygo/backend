package taxonomy

import (
	"testing"

	"github.com/fastygo/backend/internal/domain/content"
)

func TestHierarchyValidation(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mutate    func(*Definition, []Term)
		wantError bool
	}{
		"valid tree": {mutate: func(*Definition, []Term) {}},
		"cycle": {
			mutate: func(_ *Definition, terms []Term) {
				terms[0].ParentID = "dresses"
			},
			wantError: true,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			definition := Definition{
				ID: "category", Mode: ModeHierarchical,
				AssignedToKinds: []content.Kind{"product"}, Version: 1,
			}
			terms := []Term{
				{ID: "women", TaxonomyID: "category", Name: localized("Women"), Slug: localized("women"), Version: 1},
				{ID: "dresses", TaxonomyID: "category", Name: localized("Dresses"), Slug: localized("dresses"), ParentID: "women", Version: 1},
			}
			test.mutate(&definition, terms)
			err := ValidateHierarchy(definition, terms)
			if test.wantError && err == nil {
				t.Fatalf("expected hierarchy error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid hierarchy rejected: %v", err)
			}
		})
	}
}

func TestTermAndAssignmentRules(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mode      Mode
		parent    ID
		kind      content.Kind
		wantTerm  bool
		wantAllow bool
	}{
		"flat rejects parent": {mode: ModeFlat, parent: "programming", kind: content.KindPost, wantAllow: true},
		"post is assigned":    {mode: ModeFlat, kind: content.KindPost, wantTerm: true, wantAllow: true},
		"product is denied":   {mode: ModeFlat, kind: "product", wantTerm: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			definition := Definition{
				ID: "topic", Mode: test.mode,
				AssignedToKinds: []content.Kind{content.KindPost}, Version: 1,
			}
			term := Term{
				ID: "go", TaxonomyID: "topic", Name: localized("Go"), Slug: localized("go"),
				ParentID: test.parent, Version: 1,
			}
			err := term.Validate(definition)
			if test.wantTerm && err != nil {
				t.Fatalf("term rejected: %v", err)
			}
			if !test.wantTerm && err == nil {
				t.Fatalf("invalid term accepted")
			}
			if definition.Allows(test.kind) != test.wantAllow {
				t.Fatalf("kind allow=%v want %v", definition.Allows(test.kind), test.wantAllow)
			}
		})
	}
}

func localized(value string) content.LocalizedText {
	return content.LocalizedText{"en": value}
}
