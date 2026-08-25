package taxonomy_test

import (
	"context"
	"path/filepath"
	"testing"

	applicationcontent "github.com/fastygo/backend/internal/application/content"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/backend/internal/storage/bbolt"
)

func TestTaxonomyLifecycleAndContentAssignment(t *testing.T) {
	ctx := context.Background()
	storage, err := bbolt.Open(filepath.Join(t.TempDir(), "taxonomy.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer storage.Close()
	service, err := applicationtaxonomy.NewService(storage, nil)
	if err != nil {
		t.Fatalf("new taxonomy service: %v", err)
	}
	admin := authz.NewPrincipal(
		"admin",
		authz.CapabilityTaxonomiesManage,
		authz.CapabilityTaxonomiesAssign,
		authz.CapabilityContentCreate,
	)
	definition, err := service.SaveDefinition(ctx, admin, taxonomy.Definition{
		ID: "category", Label: content.LocalizedText{"en": "Categories"},
		Mode: taxonomy.ModeHierarchical, AssignedToKinds: []content.Kind{content.KindPost},
		Public: true,
	}, 0)
	if err != nil {
		t.Fatalf("save taxonomy: %v", err)
	}
	parent, err := service.SaveTerm(ctx, admin, taxonomy.Term{
		ID: "technology", TaxonomyID: definition.ID,
		Name: content.LocalizedText{"en": "Technology"},
		Slug: content.LocalizedText{"en": "technology"},
	}, 0)
	if err != nil {
		t.Fatalf("save parent term: %v", err)
	}
	child, err := service.SaveTerm(ctx, admin, taxonomy.Term{
		ID: "go", TaxonomyID: definition.ID, ParentID: parent.ID,
		Name: content.LocalizedText{"en": "Go"},
		Slug: content.LocalizedText{"en": "go"},
	}, 0)
	if err != nil {
		t.Fatalf("save child term: %v", err)
	}
	terms, err := service.ListTerms(ctx, authz.Anonymous(), definition.ID)
	if err != nil || len(terms) != 2 {
		t.Fatalf("list public terms: items=%d err=%v", len(terms), err)
	}

	contentService, err := applicationcontent.NewService(storage, nil, nil)
	if err != nil {
		t.Fatalf("new content service: %v", err)
	}
	_, err = contentService.Create(ctx, admin, content.Entry{
		Kind: content.KindPost, Status: content.StatusDraft, Visibility: content.VisibilityPublic,
		Title: content.LocalizedText{"en": "Typed taxonomies"},
		Slug:  content.LocalizedText{"en": "typed-taxonomies"},
		Terms: []content.TermRef{{Taxonomy: definition.ID, TermID: string(child.ID)}},
	})
	if err != nil {
		t.Fatalf("create assigned content: %v", err)
	}
	if err := service.DeleteTerm(ctx, admin, child.ID, child.Version); err == nil {
		t.Fatalf("assigned term deletion must be rejected")
	}
}

func TestTaxonomyRejectsUnauthorizedManagement(t *testing.T) {
	ctx := context.Background()
	storage, err := bbolt.Open(filepath.Join(t.TempDir(), "taxonomy.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer storage.Close()
	service, _ := applicationtaxonomy.NewService(storage, nil)
	if _, err := service.SaveDefinition(ctx, authz.Anonymous(), taxonomy.Definition{
		ID: "tag", Mode: taxonomy.ModeFlat, AssignedToKinds: []content.Kind{content.KindPost},
	}, 0); err == nil {
		t.Fatalf("anonymous taxonomy management must be rejected")
	}
}
