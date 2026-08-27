package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
)

func TestCodexRegistersManifestPostTypes(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "codex-cpt.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, restClock{time.Now().UTC()})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	taxonomies, err := applicationtaxonomy.NewService(adapter, nil)
	if err != nil {
		t.Fatalf("create taxonomy service: %v", err)
	}
	manifest := schema.WithCoreResources(schema.Manifest{
		Name: "crm", Version: "1",
		Resources: []schema.Resource{
			{ID: "lead", Collection: "leads", Public: false, RESTVisible: true},
			{ID: "note", Collection: "notes", Public: false, RESTVisible: false},
		},
	})
	if err := service.SetManifest(manifest); err != nil {
		t.Fatalf("set manifest: %v", err)
	}
	editor := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)
	handler, err := NewCodexHandler(service, taxonomies, fixedPrincipal{principal: editor}, manifest)
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)

	discovery := httptest.NewRecorder()
	mux.ServeHTTP(discovery, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/", nil))
	if discovery.Code != http.StatusOK {
		t.Fatalf("discovery %d: %s", discovery.Code, discovery.Body.String())
	}
	var document struct {
		Routes map[string]string `json:"routes"`
	}
	if err := json.Unmarshal(discovery.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if document.Routes["leads"] != "/go-json/go/v2/leads" || document.Routes["types"] != "/go-json/go/v2/types" {
		t.Fatalf("discovery routes: %#v", document.Routes)
	}
	if _, exists := document.Routes["notes"]; exists {
		t.Fatal("hidden CPT was advertised")
	}

	created := performJSON(mux, http.MethodPost, "/go-json/go/v2/leads", `{
		"title":{"en":"Acme"},"slug":{"en":"acme"},"status":"published","visibility":"private"
	}`, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("create lead %d: %s", created.Code, created.Body.String())
	}
	var body struct {
		Data struct {
			Kind  string            `json:"kind"`
			Links map[string]string `json:"links"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if body.Data.Kind != "lead" || body.Data.Links["self"] == "" {
		t.Fatalf("lead payload: %#v", body.Data)
	}
	if want := "/go-json/go/v2/leads/"; len(body.Data.Links["self"]) < len(want) || body.Data.Links["self"][:len(want)] != want {
		t.Fatalf("self link: %s", body.Data.Links["self"])
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/leads", nil))
	if list.Code != http.StatusOK {
		t.Fatalf("list leads %d: %s", list.Code, list.Body.String())
	}
	types := httptest.NewRecorder()
	mux.ServeHTTP(types, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/types", nil))
	if types.Code != http.StatusOK {
		t.Fatalf("types %d: %s", types.Code, types.Body.String())
	}
	var typeDocument struct {
		Data []struct {
			ID          string `json:"id"`
			Collection  string `json:"collection"`
			RESTVisible bool   `json:"rest_visible"`
			Form        []any  `json:"form"`
			Taxonomies  []any  `json:"taxonomies"`
		} `json:"data"`
	}
	if err := json.Unmarshal(types.Body.Bytes(), &typeDocument); err != nil {
		t.Fatalf("decode types: %v", err)
	}
	var leadType bool
	for _, item := range typeDocument.Data {
		if item.Form == nil || item.Taxonomies == nil {
			t.Fatalf("types DTO must include form and taxonomies arrays: %#v", item)
		}
		if item.ID == "lead" {
			leadType = item.Collection == "leads" && item.RESTVisible
		}
	}
	if !leadType {
		t.Fatalf("lead type missing: %#v", typeDocument.Data)
	}
	hidden := httptest.NewRecorder()
	mux.ServeHTTP(hidden, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/notes", nil))
	if hidden.Code != http.StatusNotFound {
		t.Fatalf("hidden CPT route %d", hidden.Code)
	}

	posts := httptest.NewRecorder()
	mux.ServeHTTP(posts, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/posts", nil))
	if posts.Code != http.StatusOK {
		t.Fatalf("core posts %d: %s", posts.Code, posts.Body.String())
	}
}
