package codeximport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/operations/codeximport"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
)

func TestImportCreatesUpdatesUnchangedDeletesAndRejects(t *testing.T) {
	service, manifest := newImportService(t)
	now := time.Date(2026, time.October, 30, 15, 0, 0, 0, time.UTC)
	published := now
	entry := domaincontent.Entry{
		ID: "post-about", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "about", Title: "About", Content: "# Hello\n\nMarkdown stays markdown.",
		Version: 1, CreatedAt: now, UpdatedAt: now, PublishedAt: &published,
	}
	raw, _ := json.Marshal(entry)

	stdout := &bytes.Buffer{}
	result, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(raw), stdout, codeximport.Options{})
	if err != nil {
		t.Fatalf("create import: %v", err)
	}
	if len(result.Reports) != 1 || result.Reports[0].Status != codeximport.StatusCreated {
		t.Fatalf("create report: %#v", result.Reports)
	}
	stored, err := service.Get(context.Background(), admin(), "post-about")
	if err != nil {
		t.Fatalf("get created: %v", err)
	}
	if stored.Content != "# Hello\n\nMarkdown stays markdown." {
		t.Fatalf("markdown converted: %q", stored.Content)
	}

	storedJSON, _ := json.Marshal(stored)
	stdout.Reset()
	result, err = codeximport.Run(context.Background(), service, manifest, bytes.NewReader(storedJSON), stdout, codeximport.Options{})
	if err != nil {
		t.Fatalf("unchanged import: %v", err)
	}
	if result.Reports[0].Status != codeximport.StatusUnchanged {
		t.Fatalf("unchanged: %#v stdout=%s", result.Reports, stdout.String())
	}

	entry.Title = "About the shop"
	entry.Version = stored.Version
	updatedJSON, _ := json.Marshal(entry)
	stdout.Reset()
	result, err = codeximport.Run(context.Background(), service, manifest, bytes.NewReader(updatedJSON), stdout, codeximport.Options{})
	if err != nil {
		t.Fatalf("update import: %v", err)
	}
	if result.Reports[0].Status != codeximport.StatusUpdated {
		t.Fatalf("update: %#v", result.Reports)
	}
	revisions, _, err := service.Revisions(context.Background(), admin(), "post-about", domaincontent.KindPost, 1, 20)
	if err != nil || len(revisions) == 0 {
		t.Fatalf("overwrite must store a revision: %v %#v", err, revisions)
	}
	if revisions[0].Snapshot.Title != "About" {
		t.Fatalf("revision snapshot: %#v", revisions[0].Snapshot)
	}

	del := []byte(`{"op":"delete","id":"post-about"}`)
	stdout.Reset()
	result, err = codeximport.Run(context.Background(), service, manifest, bytes.NewReader(del), stdout, codeximport.Options{})
	if err != nil {
		t.Fatalf("delete import: %v", err)
	}
	if result.Reports[0].Status != codeximport.StatusDeleted {
		t.Fatalf("delete: %#v", result.Reports)
	}
	trashed, err := service.Get(context.Background(), admin(), "post-about")
	if err != nil || trashed.Status != domaincontent.StatusTrashed {
		t.Fatalf("trashed: %#v %v", trashed, err)
	}
	if !strings.Contains(stdout.String(), `"name":"headless"`) && !strings.Contains(stdout.String(), `"name": "headless"`) {
		t.Fatalf("missing manifest line: %s", stdout.String())
	}
}

func TestImportRejectsBlockedKindsAndIsPlanAtomic(t *testing.T) {
	service, manifest := newImportService(t)
	now := time.Date(2026, time.October, 30, 15, 0, 0, 0, time.UTC)
	good := domaincontent.Entry{
		ID: "post-keep", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "keep", Title: "Keep", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	goodJSON, _ := json.Marshal(good)
	media := []byte(`{"op":"upsert","entry":{"id":"media-1","locale":"en-US","kind":"media","status":"published","visibility":"public","slug":"cover","title":"Cover","version":1,"created_at":"2026-10-30T15:00:00Z","updated_at":"2026-10-30T15:00:00Z"}}`)
	input := append(append(goodJSON, '\n'), media...)
	stdout := &bytes.Buffer{}
	result, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(input), stdout, codeximport.Options{})
	if err == nil || result.Rejected == 0 {
		t.Fatalf("want reject: %v %#v", err, result)
	}
	if _, getErr := service.Get(context.Background(), admin(), "post-keep"); getErr == nil {
		t.Fatal("rejected batch must not write the good record")
	}
	if !strings.Contains(stdout.String(), "not an importable long read") {
		t.Fatalf("reason: %s", stdout.String())
	}
}

func TestImportDryRunAndSlugMatch(t *testing.T) {
	service, manifest := newImportService(t)
	now := time.Date(2026, time.October, 30, 15, 0, 0, 0, time.UTC)
	existing := domaincontent.Entry{
		ID: "post-old", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "about", Title: "Old", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if _, err := service.Create(context.Background(), admin(), existing); err != nil {
		t.Fatalf("seed: %v", err)
	}
	incoming := existing
	incoming.ID = "post-new"
	incoming.Title = "New title"
	raw, _ := json.Marshal(incoming)
	stdout := &bytes.Buffer{}
	result, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(raw), stdout, codeximport.Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if result.Reports[0].Status != codeximport.StatusUpdated || result.Reports[0].ID != "post-old" {
		t.Fatalf("slug match plan: %#v", result.Reports)
	}
	stored, err := service.Get(context.Background(), admin(), "post-old")
	if err != nil || stored.Title != "Old" {
		t.Fatalf("dry-run wrote: %#v %v", stored, err)
	}
	if _, err := service.Get(context.Background(), admin(), "post-new"); err == nil {
		t.Fatal("dry-run created post-new")
	}

	stdout.Reset()
	if _, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(raw), stdout, codeximport.Options{}); err != nil {
		t.Fatalf("apply slug match: %v", err)
	}
	stored, err = service.Get(context.Background(), admin(), "post-old")
	if err != nil || stored.Title != "New title" {
		t.Fatalf("slug overwrite: %#v %v", stored, err)
	}
}

func TestImportVerifiesProvidedDigest(t *testing.T) {
	service, manifest := newImportService(t)
	now := time.Date(2026, time.October, 30, 15, 0, 0, 0, time.UTC)
	entry := domaincontent.Entry{
		ID: "post-d", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "d", Title: "D", Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	raw, _ := json.Marshal(entry)
	digest, err := canonical.Digest(string(raw))
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	line, _ := json.Marshal(map[string]any{"op": "upsert", "digest": digest, "entry": json.RawMessage(raw)})
	if _, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(line), &bytes.Buffer{}, codeximport.Options{}); err != nil {
		t.Fatalf("matching digest: %v", err)
	}
	bad, _ := json.Marshal(map[string]any{"op": "upsert", "digest": "codex-entry/v1:sha256:" + strings.Repeat("0", 64), "entry": json.RawMessage(raw)})
	result, err := codeximport.Run(context.Background(), service, manifest, bytes.NewReader(bad), &bytes.Buffer{}, codeximport.Options{})
	if err == nil || result.Reports[0].Reason != "digest mismatch" {
		t.Fatalf("mismatch: %v %#v", err, result.Reports)
	}
}

func newImportService(t *testing.T) (*application.Service, schema.Manifest) {
	t.Helper()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "import.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, nil)
	if err != nil {
		t.Fatalf("service: %v", err)
	}
	manifest := schema.WithCoreResources(schema.Manifest{Name: "headless", Version: "1"})
	if err := service.SetManifest(manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	return service, manifest
}

func admin() authz.Principal {
	return authz.AdministratorRole().Principal("codex-import")
}
