package rest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

func TestEncodeJCSNoHTMLEscapeOrNewline(t *testing.T) {
	t.Parallel()
	text, err := encodeJCS(map[string]any{
		"message": "A < B\u2028C",
		"status":  200,
	})
	if err != nil {
		t.Fatalf("encodeJCS: %v", err)
	}
	if strings.HasSuffix(text, "\n") {
		t.Fatalf("trailing newline: %q", text)
	}
	if strings.Contains(text, `\u003c`) || strings.Contains(text, `\u2028`) {
		t.Fatalf("HTML/unicode escape leaked: %s", text)
	}
	if !strings.Contains(text, "<") || !strings.Contains(text, "\u2028") {
		t.Fatalf("raw characters missing: %s", text)
	}
	roundTrip, err := canonical.CanonicalJSON(text)
	if err != nil {
		t.Fatalf("CanonicalJSON: %v", err)
	}
	if roundTrip != text {
		t.Fatalf("not JCS\n got: %s\nwant: %s", text, roundTrip)
	}
	response := httptest.NewRecorder()
	writeCanonicalJCS(response, 200, map[string]any{"ok": true, "tag": "<x>"})
	body := response.Body.Bytes()
	if bytes.HasSuffix(body, []byte("\n")) {
		t.Fatal("writer trailing newline")
	}
	if bytes.Contains(body, []byte(`\u003c`)) {
		t.Fatalf("writer escaped <: %s", body)
	}
}

func TestCanonicalizeTermOmitsCount(t *testing.T) {
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
}

func TestManifestDigestUsesFullCanonical(t *testing.T) {
	t.Parallel()
	digest, err := manifestDigestV2(schema.WithCoreResources(schema.Manifest{Name: "headless", Version: "1"}))
	if err != nil {
		t.Fatalf("manifestDigestV2: %v", err)
	}
	if !strings.HasPrefix(digest, canonical.ManifestDigestV2Prefix) {
		t.Fatalf("prefix: %s", digest)
	}
	reduced, err := encodeJCS(map[string]any{
		"name": "headless", "version": "1",
		"resources": []any{
			map[string]any{"collection": "menus", "id": "menu"},
			map[string]any{"collection": "pages", "id": "page"},
			map[string]any{"collection": "posts", "id": "post"},
			map[string]any{"collection": "settings", "id": "setting"},
		},
	})
	if err != nil {
		t.Fatalf("reduced: %v", err)
	}
	sum := sha256.Sum256([]byte(reduced))
	if digest == canonical.ManifestDigestV2Prefix+hex.EncodeToString(sum[:]) {
		t.Fatal("digest must not use the reduced {id,collection} formula")
	}
}

func TestSettingsDocumentShape(t *testing.T) {
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
	if settings.Language != "en-US" || settings.Currency != "USD" {
		t.Fatalf("settingsFromEntry: %#v", settings)
	}
	document := settingsDocument(settings)
	if _, ok := document["kind"]; ok {
		t.Fatalf("SettingsV04 must not include kind: %#v", document)
	}
	body, digest, err := canonicalizeSettings(settings)
	if err != nil {
		t.Fatalf("canonicalizeSettings: %v", err)
	}
	if !strings.HasPrefix(digest, settingsDigestPrefix) {
		t.Fatalf("digest prefix: %s", digest)
	}
	if bytes.Contains(body, []byte(`"kind"`)) {
		t.Fatalf("settings JCS leaked kind: %s", body)
	}
}
