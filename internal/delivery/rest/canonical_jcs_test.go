package rest

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

func TestWriteCanonicalJCSUsesCodex(t *testing.T) {
	t.Parallel()
	response := httptest.NewRecorder()
	writeCanonicalJCS(response, 200, map[string]any{"tag": "<x>", "line": "A\u2028B"})
	body := response.Body.Bytes()
	if bytes.HasSuffix(body, []byte("\n")) {
		t.Fatal("writer trailing newline")
	}
	if bytes.Contains(body, []byte(`\u003c`)) || bytes.Contains(body, []byte(`\u2028`)) {
		t.Fatalf("writer escaped: %s", body)
	}
	want := canonical.JCS(map[string]any{"line": "A\u2028B", "tag": "<x>"})
	if string(body) != want {
		t.Fatalf("body %s want %s", body, want)
	}
}

func TestCanonicalizeTermUsesCodexProfile(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC)
	body, err := canonicalizeTerm(taxonomy.Term{
		ID: "go", Locale: "en-US", TaxonomyID: "topic", Name: "Go", Slug: "go",
		Version: 1, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("canonicalizeTerm: %v", err)
	}
	if bytes.Contains(body, []byte(`"count"`)) {
		t.Fatalf("codex-term/v1 must not include count: %s", body)
	}
	raw := `{"id":"go","locale":"en-US","taxonomy_id":"topic","name":"Go","slug":"go","count":9,"version":1,"created_at":"2026-10-25T12:00:00Z","updated_at":"2026-10-25T12:00:00Z"}`
	want, err := canonical.CanonicalizeTerm(raw)
	if err != nil {
		t.Fatalf("CanonicalizeTerm: %v", err)
	}
	if string(body) != want {
		t.Fatalf("got %s\nwant %s", body, want)
	}
}

func TestManifestDigestV2MatchesCodexWithoutCanonicalSort(t *testing.T) {
	t.Parallel()
	manifest := schema.WithCoreResources(schema.Manifest{Name: "headless", Version: "1"})
	digest, err := manifestDigestV2(manifest)
	if err != nil {
		t.Fatalf("manifestDigestV2: %v", err)
	}
	served := ServedCodexManifest(manifest)
	want, err := served.DigestV2()
	if err != nil {
		t.Fatalf("DigestV2: %v", err)
	}
	if digest != want {
		t.Fatalf("digest %s want %s", digest, want)
	}
	sorted, err := served.Canonical().DigestV2()
	if err != nil {
		t.Fatalf("sorted DigestV2: %v", err)
	}
	if digest == sorted {
		t.Fatal("manifest_digest must not use Canonical() resource sorting")
	}
}

func TestSettingsCanonicalUsesCodexProfile(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC)
	settings := settingsFromEntry(domaincontent.Entry{
		ID: "site", Locale: "en-US", Kind: domaincontent.KindSetting,
		Title: "Example", Version: 1, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]domaincontent.MetadataValue{
			"language": {Value: "en-US"},
			"currency": {Value: "USD"},
		},
	})
	body, digest, err := canonicalizeSettings(settings)
	if err != nil {
		t.Fatalf("canonicalizeSettings: %v", err)
	}
	if !strings.HasPrefix(digest, canonical.SettingsDigestPrefix) {
		t.Fatalf("digest prefix: %s", digest)
	}
	if bytes.Contains(body, []byte(`"kind"`)) || bytes.Contains(body, []byte(`"count"`)) {
		t.Fatalf("settings JCS leaked kind/count: %s", body)
	}
}
