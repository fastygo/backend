package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fastygo/backend/internal/domain/authz"
)

func TestCollectionRouterPrefersLiteralBySlugAndKeepsCanonicalExtras(t *testing.T) {
	t.Parallel()
	mux, _ := newShopMux(t, authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
		authz.CapabilityContentManageRevisions,
	))
	created := performJSON(mux, http.MethodPost, "/go-json/go/v2/products", `{
		"status":"published",
		"visibility":"public",
		"slug":{"en":"revisions"},
		"title":{"en":"Revision named course"}
	}`, "")
	if created.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", created.Code, created.Body.String())
	}
	var body struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode created entry: %v", err)
	}

	bySlug := httptest.NewRecorder()
	mux.ServeHTTP(bySlug, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/products/by-slug/revisions?locale=en", nil))
	if bySlug.Code != http.StatusOK {
		t.Fatalf("by-slug status %d: %s", bySlug.Code, bySlug.Body.String())
	}
	var slugResult struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(bySlug.Body.Bytes(), &slugResult); err != nil {
		t.Fatalf("decode by-slug response: %v", err)
	}
	if slugResult.Data.ID != body.Data.ID {
		t.Fatalf("by-slug returned %q, want %q", slugResult.Data.ID, body.Data.ID)
	}

	for name, path := range map[string]string{
		"revisions": "/go-json/go/v2/products/" + body.Data.ID + "/revisions",
		"form":      "/go-json/go/v2/products/" + body.Data.ID + "/form",
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusOK {
				t.Fatalf("%s status %d: %s", name, response.Code, response.Body.String())
			}
		})
	}
}

func TestCollectionRouterReportsMethodsAndMediaCapabilities(t *testing.T) {
	t.Parallel()
	mux, _ := newShopMux(t, authz.NewPrincipal("editor"))

	method := httptest.NewRecorder()
	mux.ServeHTTP(method, httptest.NewRequest(http.MethodPost, "/go-json/go/v2/products/example/revisions", nil))
	if method.Code != http.StatusMethodNotAllowed {
		t.Fatalf("revisions method status %d", method.Code)
	}
	if allow := method.Header().Get("Allow"); allow != http.MethodGet {
		t.Fatalf("revisions Allow %q", allow)
	}

	unsupported := httptest.NewRecorder()
	mux.ServeHTTP(unsupported, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/media/by-slug/example", nil))
	if unsupported.Code != http.StatusNotFound {
		t.Fatalf("media by-slug status %d", unsupported.Code)
	}

	revisions := httptest.NewRecorder()
	mux.ServeHTTP(revisions, httptest.NewRequest(http.MethodGet, "/go-json/go/v2/media/example/revisions", nil))
	if revisions.Code != http.StatusNotFound {
		t.Fatalf("media revisions status %d", revisions.Code)
	}
}
