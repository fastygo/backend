package backup_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	identityapplication "github.com/fastygo/backend/internal/application/identity"
	taxonomyapplication "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/domain/taxonomy"
	tokenidentity "github.com/fastygo/backend/internal/identity"
	"github.com/fastygo/backend/internal/operations/backup"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/backend/internal/storage/sqlstore"
)

func TestBackupRestoresAcrossBboltAndSQLiteAdapters(t *testing.T) {
	ctx := context.Background()
	manifest := schema.Manifest{
		Name: "test", Version: "1",
		Resources: []schema.Resource{{ID: "post", Collection: "posts"}},
	}
	source, _ := bboltstorage.Open(filepath.Join(t.TempDir(), "source.db"), 0o600, nil)
	t.Cleanup(func() { _ = source.Close() })
	contentService, _ := contentapplication.NewService(source, nil, nil)
	principal := authz.NewPrincipal(
		"editor", authz.CapabilityContentCreate, authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish, authz.CapabilityContentReadPrivate,
	)
	now := time.Now().UTC()
	created, err := contentService.Create(ctx, principal, content.Entry{
		ID: "post_1", Kind: "post", Status: content.StatusPublished, Visibility: content.VisibilityPublic,
		Slug: content.LocalizedText{"en": "backup"}, Title: content.LocalizedText{"en": "Backup"},
		PublishedAt: &now,
	})
	if err != nil {
		t.Fatalf("create source content: %v", err)
	}
	created.Title = content.LocalizedText{"en": "Restored"}
	if _, err := contentService.Update(ctx, principal, created, 1, "backup test"); err != nil {
		t.Fatalf("update source content: %v", err)
	}
	taxonomyService, _ := taxonomyapplication.NewService(source, nil)
	taxonomyPrincipal := authz.NewPrincipal("admin", authz.CapabilityTaxonomiesManage)
	definition, err := taxonomyService.SaveDefinition(ctx, taxonomyPrincipal, taxonomy.Definition{
		ID: "topic", Mode: taxonomy.ModeFlat, Public: true,
		Label:           content.LocalizedText{"en": "Topics"},
		AssignedToKinds: []content.Kind{content.KindPost},
	}, 0)
	if err != nil {
		t.Fatalf("create source taxonomy: %v", err)
	}
	if _, err := taxonomyService.SaveTerm(ctx, taxonomyPrincipal, taxonomy.Term{
		ID: "go", TaxonomyID: definition.ID,
		Name: content.LocalizedText{"en": "Go"}, Slug: content.LocalizedText{"en": "go"},
	}, 0); err != nil {
		t.Fatalf("create source taxonomy term: %v", err)
	}
	tokens, _ := tokenidentity.NewTokenManager(
		"backup-identity-secret-at-least-32-bytes",
		"backup-test",
	)
	identityService, _ := identityapplication.NewService(source, tokens, nil)
	if err := identityService.Initialize(ctx, "admin@example.com", "correct horse battery staple"); err != nil {
		t.Fatalf("create source identity: %v", err)
	}
	exporter, _ := backup.New(source, manifest)
	var encoded bytes.Buffer
	if err := exporter.Export(ctx, &encoded); err != nil {
		t.Fatalf("export backup: %v", err)
	}

	target, err := sqlstore.Open(ctx, "sqlite", filepath.Join(t.TempDir(), "target.db"), sqlstore.DialectSQLite)
	if err != nil {
		t.Fatalf("open target SQLite: %v", err)
	}
	t.Cleanup(func() { _ = target.Close() })
	importer, _ := backup.New(target, manifest)
	if err := importer.Restore(ctx, bytes.NewReader(encoded.Bytes())); err != nil {
		t.Fatalf("restore backup: %v", err)
	}
	targetService, _ := contentapplication.NewService(target, nil, nil)
	restored, err := targetService.Get(ctx, principal, "post_1")
	if err != nil {
		t.Fatalf("read restored content: %v", err)
	}
	if restored.Version != 2 || restored.Title["en"] != "Restored" {
		t.Fatalf("restored content mismatch: %#v", restored)
	}
	targetTaxonomies, _ := taxonomyapplication.NewService(target, nil)
	terms, err := targetTaxonomies.ListTerms(ctx, authz.Anonymous(), definition.ID)
	if err != nil || len(terms) != 1 || terms[0].ID != "go" {
		t.Fatalf("restored taxonomy mismatch: %#v, %v", terms, err)
	}
	targetIdentity, _ := identityapplication.NewService(target, tokens, nil)
	if token, err := targetIdentity.Authenticate(
		ctx,
		"admin@example.com",
		"correct horse battery staple",
		time.Hour,
	); err != nil || token == "" {
		t.Fatalf("restored identity cannot authenticate: %v", err)
	}
	if err := importer.Restore(ctx, bytes.NewReader(encoded.Bytes())); err == nil {
		t.Fatalf("restore into non-empty storage was accepted")
	}
}
