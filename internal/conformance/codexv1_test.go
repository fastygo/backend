package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/backend/internal/delivery/rest"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

// TestCodexV1Conform is the stand-in for ui8kit/contract's codex-conform HTTP CLI
// (not shipped at commit 8299fee). Replace this test with the official CLI once
// it is published: GET {base}/codex/v1 against the same fixtures.
func TestCodexV1Conform(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC)
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "codex-v1.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, fixedClock{now})
	if err != nil {
		t.Fatalf("content service: %v", err)
	}
	taxonomies, err := applicationtaxonomy.NewService(adapter, nil)
	if err != nil {
		t.Fatalf("taxonomy service: %v", err)
	}
	manifest := schema.WithCoreResources(schema.Manifest{Name: "headless", Version: "1"})
	if err := service.SetManifest(manifest); err != nil {
		t.Fatalf("set manifest: %v", err)
	}
	editor := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
		authz.CapabilityTaxonomiesManage,
		authz.CapabilitySettingsManage,
		authz.CapabilityMediaUpload,
	)
	handler, err := rest.NewCodexHandler(service, taxonomies, nil, manifest)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	handler.SetAvailableLocales([]string{"en-US", "de"})
	contentHandler, err := rest.NewContentHandler(service, nil, manifest)
	if err != nil {
		t.Fatalf("content handler: %v", err)
	}
	collectionRouter, err := rest.NewCollectionRouter(handler, contentHandler)
	if err != nil {
		t.Fatalf("collection router: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)
	contentHandler.Routes(mux)
	collectionRouter.Routes(mux)

	published := now
	if _, err := service.Create(context.Background(), editor, domaincontent.Entry{
		ID: "site", Kind: domaincontent.KindSetting, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "site", Title: "Example Site", Version: 1,
		PublishedAt: &published, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]domaincontent.MetadataValue{
			"value": {Value: map[string]any{"language": "en-US", "currency": "USD"}},
		},
	}); err != nil {
		t.Fatalf("create settings: %v", err)
	}
	alpha, err := service.Create(context.Background(), editor, domaincontent.Entry{
		ID: "post_alpha", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "alpha", Title: "Alpha <x>", Content: "First\u2028line", Version: 1,
		PublishedAt: &published, CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := service.Create(context.Background(), editor, domaincontent.Entry{
		ID: "post_beta", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "beta", Title: "Beta", Content: "Second", Version: 1,
		PublishedAt: &published, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	if _, err := service.Create(context.Background(), editor, domaincontent.Entry{
		ID: "post_draft", Kind: domaincontent.KindPost, Locale: "en-US",
		Status: domaincontent.StatusDraft, Visibility: domaincontent.VisibilityPublic,
		Slug: "draft", Title: "Draft", Version: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if _, err := service.Create(context.Background(), editor, domaincontent.Entry{
		ID: "media_1", Kind: "media", Locale: "en-US",
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: "cover", Title: "Cover", Version: 1,
		PublishedAt: &published, CreatedAt: now, UpdatedAt: now,
		Metadata: map[string]domaincontent.MetadataValue{
			"media_filename": {Value: "cover.png"},
			"post":           {Value: "post-1"},
		},
	}); err != nil {
		t.Fatalf("create media: %v", err)
	}
	if _, err := taxonomies.SaveDefinition(context.Background(), editor, taxonomy.Definition{
		ID: "topic", Locale: "en-US", Label: "Topics", Mode: taxonomy.ModeFlat,
		AssignedToKinds: []domaincontent.Kind{domaincontent.KindPost}, Public: true,
	}, 0); err != nil {
		t.Fatalf("save taxonomy: %v", err)
	}
	if _, err := taxonomies.SaveTerm(context.Background(), editor, taxonomy.Term{
		ID: "go", Locale: "en-US", TaxonomyID: "topic", Name: "Go", Slug: "go",
	}, 0); err != nil {
		t.Fatalf("save term: %v", err)
	}

	t.Run("discovery", func(t *testing.T) {
		response := get(mux, "/codex/v1/")
		if response.Code != http.StatusOK {
			t.Fatalf("discovery %d: %s", response.Code, response.Body.String())
		}
		assertCodexV1Body(t, response.Body.Bytes())
		if ct := response.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("content-type: %s", ct)
		}
		var document struct {
			CodexVersion   string `json:"codex_version"`
			Profile        string `json:"profile"`
			ManifestDigest string `json:"manifest_digest"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatalf("decode discovery: %v", err)
		}
		if document.CodexVersion != "0.4.1" || document.Profile != canonical.EntryProfile {
			t.Fatalf("discovery: %#v", document)
		}
		wantDigest, err := rest.ServedCodexManifest(manifest).DigestV2()
		if err != nil {
			t.Fatalf("DigestV2: %v", err)
		}
		if document.ManifestDigest != wantDigest {
			t.Fatalf("manifest_digest %s want DigestV2 %s", document.ManifestDigest, wantDigest)
		}
	})

	t.Run("list_order_pagination", func(t *testing.T) {
		response := get(mux, "/codex/v1/entries/post?locale=en-US")
		if response.Code != http.StatusOK {
			t.Fatalf("list %d: %s", response.Code, response.Body.String())
		}
		assertCodexV1Body(t, response.Body.Bytes())
		var document struct {
			Data       []map[string]any `json:"data"`
			Pagination struct {
				Page       int `json:"page"`
				PerPage    int `json:"per_page"`
				Total      int `json:"total"`
				TotalPages int `json:"total_pages"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatalf("decode list: %v", err)
		}
		if document.Pagination.Page != 1 || document.Pagination.PerPage != 20 || document.Pagination.Total != 2 {
			t.Fatalf("pagination: %#v", document.Pagination)
		}
		if len(document.Data) != 2 {
			t.Fatalf("data len %d", len(document.Data))
		}
		if document.Data[0]["id"] != "post_alpha" || document.Data[1]["id"] != "post_beta" {
			t.Fatalf("order: %#v %#v", document.Data[0]["id"], document.Data[1]["id"])
		}
		page := get(mux, "/codex/v1/entries/post?locale=en-US&page=1&per_page=1")
		assertCodexV1Body(t, page.Body.Bytes())
		if page.Code != http.StatusOK {
			t.Fatalf("page1 %d: %s", page.Code, page.Body.String())
		}
		var first struct {
			Data       []map[string]any `json:"data"`
			Pagination struct {
				PerPage int `json:"per_page"`
				Total   int `json:"total"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(page.Body.Bytes(), &first); err != nil {
			t.Fatalf("decode page1: %v", err)
		}
		if first.Pagination.PerPage != 1 || first.Pagination.Total != 2 || first.Data[0]["id"] != "post_alpha" {
			t.Fatalf("page1: %#v", first)
		}
		clamped := get(mux, "/codex/v1/entries/post?locale=en-US&per_page=1000")
		assertCodexV1Body(t, clamped.Body.Bytes())
		var clampedDoc struct {
			Pagination struct {
				PerPage int `json:"per_page"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(clamped.Body.Bytes(), &clampedDoc); err != nil {
			t.Fatalf("decode clamp: %v", err)
		}
		if clampedDoc.Pagination.PerPage != 100 {
			t.Fatalf("per_page clamp: %d", clampedDoc.Pagination.PerPage)
		}
		past := get(mux, "/codex/v1/entries/post?locale=en-US&page=9&per_page=1")
		assertCodexV1Body(t, past.Body.Bytes())
		if past.Code != http.StatusOK {
			t.Fatalf("page past end %d: %s", past.Code, past.Body.String())
		}
		var empty struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(past.Body.Bytes(), &empty); err != nil {
			t.Fatalf("decode empty: %v", err)
		}
		if empty.Data == nil || len(empty.Data) != 0 {
			t.Fatalf("page past end must be empty array: %s", past.Body.String())
		}
	})

	t.Run("single_entry_canonical_bytes", func(t *testing.T) {
		stored, err := service.Get(context.Background(), editor, alpha.ID)
		if err != nil {
			t.Fatalf("load stored: %v", err)
		}
		projected, ok := contentcompat.ProjectExact(stored, "en-US")
		if !ok {
			t.Fatal("project stored alpha")
		}
		raw, err := json.Marshal(contentcompat.PublicCodex(projected))
		if err != nil {
			t.Fatalf("marshal stored: %v", err)
		}
		want, err := canonical.Canonicalize(string(raw))
		if err != nil {
			t.Fatalf("canonicalize stored: %v", err)
		}
		digest, err := canonical.Digest(string(raw))
		if err != nil {
			t.Fatalf("digest stored: %v", err)
		}
		response := get(mux, "/codex/v1/entries/post/post_alpha?locale=en-US")
		if response.Code != http.StatusOK {
			t.Fatalf("get %d: %s", response.Code, response.Body.String())
		}
		body := response.Body.Bytes()
		assertCodexV1Body(t, body)
		if bytes.Contains(body, []byte(`\u003c`)) || bytes.Contains(body, []byte(`\u2028`)) {
			t.Fatalf("entry escaped < or U+2028: %s", body)
		}
		if !bytes.Contains(body, []byte(`Alpha <x>`)) || !bytes.Contains(body, []byte("First\u2028line")) {
			t.Fatalf("entry missing raw < / U+2028: %s", body)
		}
		if bytes.Equal(body, append([]byte(want), '\n')) {
			t.Fatal("single-entry body must not have a trailing newline")
		}
		if !bytes.Equal(body, []byte(want)) {
			t.Fatalf("body != Canonicalize(stored)\n got: %s\nwant: %s", body, want)
		}
		if got := response.Header().Get("Codex-Digest"); got != digest {
			t.Fatalf("Codex-Digest %s want %s", got, digest)
		}
		hexID := digest[strings.LastIndex(digest, ":")+1:]
		if got := response.Header().Get("ETag"); got != `"`+hexID+`"` {
			t.Fatalf("ETag %s want %q", got, hexID)
		}
		if strings.Contains(string(body), "__chrome__") || strings.Contains(string(body), "__locales__") {
			t.Fatalf("sidecar metadata leaked: %s", body)
		}
	})

	t.Run("errors", func(t *testing.T) {
		cases := []struct {
			path   string
			status int
			code   string
		}{
			{"/codex/v1/entries/unknownkind", http.StatusNotFound, "not_found"},
			{"/codex/v1/entries/post/missing", http.StatusNotFound, "not_found"},
			{"/codex/v1/entries/post/post_draft", http.StatusNotFound, "not_found"},
			{"/codex/v1/entries/post?locale=fr", http.StatusNotFound, "not_found"},
			{"/codex/v1/terms/missing", http.StatusNotFound, "not_found"},
			{"/codex/v1/entries/post?locale=ENGLISH", http.StatusBadRequest, "validation_error"},
			{"/codex/v1/entries/post?page=0", http.StatusBadRequest, "validation_error"},
			{"/codex/v1/entries/post?per_page=-1", http.StatusBadRequest, "validation_error"},
			{"/codex/v1/entries/post?foo=1", http.StatusBadRequest, "validation_error"},
		}
		for _, test := range cases {
			response := get(mux, test.path)
			if response.Code != test.status {
				t.Fatalf("%s: status %d want %d body %s", test.path, response.Code, test.status, response.Body.String())
			}
			assertCodexV1Body(t, response.Body.Bytes())
			got := errorBodyIgnoringRequestID(t, response.Body.Bytes())
			if got["code"] != test.code {
				t.Fatalf("%s error code %#v", test.path, got)
			}
			status, _ := got["status"].(float64)
			if int(status) != test.status {
				t.Fatalf("%s error status %#v", test.path, got)
			}
			if _, hasRequestID := got["request_id"]; hasRequestID {
				t.Fatalf("%s comparison must ignore request_id: %#v", test.path, got)
			}
		}
	})

	t.Run("media_attached_to", func(t *testing.T) {
		response := get(mux, "/codex/v1/entries/media/media_1?locale=en-US")
		if response.Code != http.StatusOK {
			t.Fatalf("media %d: %s", response.Code, response.Body.String())
		}
		assertCodexV1Body(t, response.Body.Bytes())
		var document map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatalf("decode media: %v", err)
		}
		metadata, _ := document["metadata"].(map[string]any)
		if metadata == nil {
			t.Fatalf("media metadata missing: %s", response.Body.String())
		}
		if _, old := metadata["post"]; old {
			t.Fatalf("codex/v1 media must not emit metadata.post: %#v", metadata)
		}
		attached, _ := metadata["attached_to"].(map[string]any)
		if attached["value"] != "post-1" {
			t.Fatalf("attached_to: %#v", metadata["attached_to"])
		}
		gov2 := get(mux, "/go-json/go/v2/media/media_1?locale=en-US")
		if gov2.Code != http.StatusOK {
			t.Fatalf("go/v2 media %d: %s", gov2.Code, gov2.Body.String())
		}
		if !strings.Contains(gov2.Body.String(), `"post"`) || strings.Contains(gov2.Body.String(), "attached_to") {
			t.Fatalf("go/v2 media must keep post and omit attached_to: %s", gov2.Body.String())
		}
	})

	t.Run("taxonomies_terms_settings", func(t *testing.T) {
		tax := get(mux, "/codex/v1/taxonomies?locale=en-US")
		if tax.Code != http.StatusOK {
			t.Fatalf("taxonomies %d: %s", tax.Code, tax.Body.String())
		}
		assertCodexV1Body(t, tax.Body.Bytes())
		var taxDoc struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(tax.Body.Bytes(), &taxDoc); err != nil {
			t.Fatalf("decode taxonomies: %v", err)
		}
		if len(taxDoc.Data) != 1 || taxDoc.Data[0]["id"] != "topic" {
			t.Fatalf("taxonomies: %#v", taxDoc.Data)
		}
		terms := get(mux, "/codex/v1/terms/topic?locale=en-US")
		if terms.Code != http.StatusOK {
			t.Fatalf("terms %d: %s", terms.Code, terms.Body.String())
		}
		assertCodexV1Body(t, terms.Body.Bytes())
		var termDoc struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(terms.Body.Bytes(), &termDoc); err != nil {
			t.Fatalf("decode terms: %v", err)
		}
		if len(termDoc.Data) != 1 || termDoc.Data[0]["id"] != "go" || termDoc.Data[0]["name"] != "Go" {
			t.Fatalf("terms: %#v", termDoc.Data)
		}
		if _, hasCount := termDoc.Data[0]["count"]; hasCount {
			t.Fatalf("codex-term/v1 must not include count: %#v", termDoc.Data[0])
		}
		settings := get(mux, "/codex/v1/settings?locale=en-US")
		if settings.Code != http.StatusOK {
			t.Fatalf("settings %d: %s", settings.Code, settings.Body.String())
		}
		assertCodexV1Body(t, settings.Body.Bytes())
		var settingsDoc struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(settings.Body.Bytes(), &settingsDoc); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		if len(settingsDoc.Data) != 1 || settingsDoc.Data[0]["id"] != "site" {
			t.Fatalf("settings list: %#v", settingsDoc.Data)
		}
		if settingsDoc.Data[0]["language"] != "en-US" || settingsDoc.Data[0]["currency"] != "USD" {
			t.Fatalf("SettingsV04 fields: %#v", settingsDoc.Data[0])
		}
		if _, hasKind := settingsDoc.Data[0]["kind"]; hasKind {
			t.Fatalf("settings must not be entries kind=setting: %#v", settingsDoc.Data[0])
		}
		one := get(mux, "/codex/v1/settings/site?locale=en-US")
		if one.Code != http.StatusOK {
			t.Fatalf("settings get %d: %s", one.Code, one.Body.String())
		}
		assertCodexV1Body(t, one.Body.Bytes())
		var oneDoc map[string]any
		if err := json.Unmarshal(one.Body.Bytes(), &oneDoc); err != nil {
			t.Fatalf("decode settings get: %v", err)
		}
		if _, wrapped := oneDoc["data"]; wrapped {
			t.Fatalf("single settings must be raw JCS, not a list envelope: %s", one.Body.String())
		}
		if oneDoc["id"] != "site" || oneDoc["language"] != "en-US" {
			t.Fatalf("settings get: %#v", oneDoc)
		}
		if one.Header().Get("Codex-Digest") == "" || one.Header().Get("ETag") == "" {
			t.Fatalf("settings get missing digest/etag: %v", one.Header())
		}
		implicit := get(mux, "/codex/v1/entries/post")
		if implicit.Code != http.StatusOK {
			t.Fatalf("default locale list %d: %s", implicit.Code, implicit.Body.String())
		}
		assertCodexV1Body(t, implicit.Body.Bytes())
		var implicitDoc struct {
			Data []map[string]any `json:"data"`
		}
		if err := json.Unmarshal(implicit.Body.Bytes(), &implicitDoc); err != nil {
			t.Fatalf("decode default locale list: %v", err)
		}
		if len(implicitDoc.Data) != 2 || implicitDoc.Data[0]["locale"] != "en-US" {
			t.Fatalf("default locale must come from settings.language: %#v", implicitDoc.Data)
		}
	})
}

func assertCodexV1Body(t *testing.T, body []byte) {
	t.Helper()
	if len(body) > 0 && body[len(body)-1] == '\n' {
		t.Fatalf("trailing newline: %s", body)
	}
	if bytes.Contains(body, []byte(`\u003c`)) || bytes.Contains(body, []byte(`\u2028`)) {
		t.Fatalf("HTML/unicode escape leaked: %s", body)
	}
	got, err := canonical.CanonicalJSON(string(body))
	if err != nil {
		t.Fatalf("CanonicalJSON: %v body %s", err, body)
	}
	if got != string(body) {
		t.Fatalf("body != JCS(re-parse(body))\n got: %s\nwant: %s", body, got)
	}
}

func errorBodyIgnoringRequestID(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatalf("decode error: %v %s", err, body)
	}
	errorBody, _ := document["error"].(map[string]any)
	if errorBody == nil {
		t.Fatalf("error object missing: %s", body)
	}
	delete(errorBody, "request_id")
	return errorBody
}

func get(handler http.Handler, path string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

type fixedClock struct{ now time.Time }

func (clock fixedClock) Now() time.Time { return clock.now }
