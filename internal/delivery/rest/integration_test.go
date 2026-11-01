package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/schema"
	tokenidentity "github.com/fastygo/backend/internal/identity"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
)

type restStack struct {
	mux     *http.ServeMux
	tokens  *tokenidentity.TokenManager
	content *application.Service
}

func catalogManifest() schema.Manifest {
	return schema.WithCoreResources(schema.Manifest{
		Name: "catalog", Version: "1",
		Resources: []schema.Resource{
			{ID: "brand", Collection: "brands", Public: true, RESTVisible: true, GraphQLVisible: true,
				Fields: []schema.Field{{ID: "country", Type: schema.FieldString}}},
			{
				ID: "product", Collection: "products", Public: true, RESTVisible: true, GraphQLVisible: true,
				Taxonomies: []string{"product_cat"},
				Fields: []schema.Field{
					{ID: "sku", Type: schema.FieldString, Indexed: true},
					{ID: "price", Type: schema.FieldNumber, Rules: schema.Rules(schema.RuleMoney)},
					{ID: "currency", Type: schema.FieldString},
					{ID: "state", Type: schema.FieldSelect, Options: schema.Options("active", "archived")},
					{ID: "brand", Type: schema.FieldRelation},
				},
				Relations: []schema.Relation{{ID: "brand", Target: "brand", Cardinality: schema.OneToOne, DeleteBehavior: schema.DeleteNullify}},
				Form:      []schema.Field{{ID: "title", Type: schema.FieldString, Localized: true}},
			},
		},
	})
}

func newRestStack(t *testing.T) restStack {
	t.Helper()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "stack.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	tokens, err := tokenidentity.NewTokenManager(strings.Repeat("k", 48), "test")
	if err != nil {
		t.Fatal(err)
	}
	identity, err := applicationidentity.NewService(adapter, tokens, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.Initialize(context.Background(), "admin@example.test", "correct-horse-battery"); err != nil {
		t.Fatalf("initialize identity: %v", err)
	}
	content, err := application.NewService(adapter, nil, restClock{time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	manifest := catalogManifest()
	if err := content.SetManifest(manifest); err != nil {
		t.Fatalf("manifest: %v", err)
	}
	taxonomies, err := applicationtaxonomy.NewService(adapter, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	identityHandler, err := NewIdentityHandler(identity, tokens)
	if err != nil {
		t.Fatal(err)
	}
	sessionHandler, err := NewSessionHandler(identity, tokens, false)
	if err != nil {
		t.Fatal(err)
	}
	taxonomyHandler, err := NewTaxonomyHandler(taxonomies, tokens)
	if err != nil {
		t.Fatal(err)
	}
	contentHandler, err := NewContentHandler(content, tokens, manifest)
	if err != nil {
		t.Fatal(err)
	}
	codexHandler, err := NewCodexHandler(content, taxonomies, tokens, manifest)
	if err != nil {
		t.Fatal(err)
	}
	router, err := NewCollectionRouter(codexHandler, contentHandler)
	if err != nil {
		t.Fatal(err)
	}
	identityHandler.Routes(mux)
	sessionHandler.Routes(mux)
	taxonomyHandler.Routes(mux)
	contentHandler.Routes(mux)
	codexHandler.Routes(mux)
	router.Routes(mux)
	return restStack{mux: mux, tokens: tokens, content: content}
}

func (stack restStack) do(t *testing.T, method, path, token, body, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response := httptest.NewRecorder()
	stack.mux.ServeHTTP(response, request)
	return response
}

func expectStatus(t *testing.T, response *httptest.ResponseRecorder, want int, what string) map[string]any {
	t.Helper()
	if response.Code != want {
		t.Fatalf("%s: status %d want %d: %s", what, response.Code, want, response.Body.String())
	}
	var document map[string]any
	if response.Body.Len() > 0 && strings.HasPrefix(strings.TrimSpace(response.Body.String()), "{") {
		if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
			t.Fatalf("%s: decode: %v", what, err)
		}
	}
	return document
}

func dataID(t *testing.T, document map[string]any) string {
	t.Helper()
	data, _ := document["data"].(map[string]any)
	id, _ := data["id"].(string)
	if id == "" {
		t.Fatalf("missing data.id: %#v", document)
	}
	return id
}

func TestRESTIdentitySessionAndRoles(t *testing.T) {
	stack := newRestStack(t)
	expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/auth/login", "", `{"email":"admin@example.test","password":"wrong-password!"}`, ""), http.StatusUnauthorized, "bad login")
	login := expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/auth/login", "", `{"email":"admin@example.test","password":"correct-horse-battery"}`, ""), http.StatusOK, "login")
	token, _ := login["access_token"].(string)
	if token == "" {
		t.Fatalf("token: %#v", login)
	}
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/users", "", "", ""), http.StatusUnauthorized, "anonymous users")
	users := expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/users", token, "", ""), http.StatusOK, "list users")
	if list, _ := users["data"].([]any); len(list) != 1 {
		t.Fatalf("users: %#v", users)
	}
	created := stack.do(t, http.MethodPost, "/go-json/go/v2/users", token, `{"email":"ed@example.test","display_name":"Ed","password":"another-long-pass","role_ids":["editor"],"active":true}`, "")
	userID := dataID(t, expectStatus(t, created, http.StatusCreated, "create user"))
	etag := created.Header().Get("ETag")
	expectStatus(t, stack.do(t, http.MethodPut, "/go-json/go/v2/users/"+userID, token, `{"email":"ed@example.test","display_name":"Edward","role_ids":["editor"],"active":true}`, ""), http.StatusBadRequest, "update without version")
	updated := stack.do(t, http.MethodPut, "/go-json/go/v2/users/"+userID, token, `{"email":"ed@example.test","display_name":"Edward","role_ids":["editor"],"active":true}`, etag)
	expectStatus(t, updated, http.StatusOK, "update user")
	expectStatus(t, stack.do(t, http.MethodDelete, "/go-json/go/v2/users/"+userID, token, "", updated.Header().Get("ETag")), http.StatusNoContent, "delete user")

	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/roles", token, "", ""), http.StatusOK, "list roles")
	role := stack.do(t, http.MethodPost, "/go-json/go/v2/roles", token, `{"id":"reviewer","label":"Reviewer","capabilities":["content.read_private"]}`, "")
	expectStatus(t, role, http.StatusCreated, "create role")
	roleUpdate := stack.do(t, http.MethodPut, "/go-json/go/v2/roles/reviewer", token, `{"label":"Reviewers","capabilities":["content.read_private","rest.access"]}`, role.Header().Get("ETag"))
	expectStatus(t, roleUpdate, http.StatusOK, "update role")
	expectStatus(t, stack.do(t, http.MethodDelete, "/go-json/go/v2/roles/reviewer", token, "", roleUpdate.Header().Get("ETag")), http.StatusNoContent, "delete role")
	expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/roles", token, `{"id":`, ""), http.StatusBadRequest, "broken json")

	session := stack.do(t, http.MethodPost, "/go-json/auth/login", "", `{"email":"admin@example.test","password":"correct-horse-battery"}`, "")
	expectStatus(t, session, http.StatusOK, "session login")
	cookie := session.Result().Cookies()[0]
	me := httptest.NewRequest(http.MethodGet, "/go-json/auth/me", nil)
	me.AddCookie(cookie)
	meResponse := httptest.NewRecorder()
	stack.mux.ServeHTTP(meResponse, me)
	meDoc := expectStatus(t, meResponse, http.StatusOK, "me")
	csrf, _ := meDoc["data"].(map[string]any)["csrfToken"].(string)
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/auth/me", token, "", ""), http.StatusOK, "me bearer")
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/auth/me", "", "", ""), http.StatusUnauthorized, "me anonymous")
	logout := httptest.NewRequest(http.MethodPost, "/go-json/auth/logout", nil)
	logout.AddCookie(cookie)
	logout.Header.Set("X-CSRF-Token", csrf)
	logoutResponse := httptest.NewRecorder()
	stack.mux.ServeHTTP(logoutResponse, logout)
	expectStatus(t, logoutResponse, http.StatusNoContent, "logout")
}

func TestRESTTaxonomiesContentAndCanonical(t *testing.T) {
	stack := newRestStack(t)
	login := expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/auth/login", "", `{"email":"admin@example.test","password":"correct-horse-battery"}`, ""), http.StatusOK, "login")
	token := login["access_token"].(string)

	definition := stack.do(t, http.MethodPost, "/go-json/go/v2/taxonomies", token, `{"id":"product_cat","label":{"en":"Categories"},"mode":"hierarchical","assigned_to_kinds":["product"],"public":true}`, "")
	expectStatus(t, definition, http.StatusCreated, "create taxonomy")
	definitionUpdate := stack.do(t, http.MethodPut, "/go-json/go/v2/taxonomies/product_cat", token, `{"label":{"en":"Product categories"},"mode":"hierarchical","assigned_to_kinds":["product"],"public":true}`, definition.Header().Get("ETag"))
	expectStatus(t, definitionUpdate, http.StatusOK, "update taxonomy")
	term := stack.do(t, http.MethodPost, "/go-json/go/v2/taxonomies/product_cat/terms", token, `{"id":"coffee","name":{"en":"Coffee"},"slug":{"en":"coffee"}}`, "")
	expectStatus(t, term, http.StatusCreated, "create term")
	termUpdate := stack.do(t, http.MethodPut, "/go-json/go/v2/taxonomies/product_cat/terms/coffee", token, `{"name":{"en":"Coffee beans"},"slug":{"en":"coffee"}}`, term.Header().Get("ETag"))
	expectStatus(t, termUpdate, http.StatusOK, "update term")
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/taxonomies", "", "", ""), http.StatusOK, "list taxonomies")
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/taxonomies/product_cat", "", "", ""), http.StatusOK, "list terms")
	expectStatus(t, stack.do(t, http.MethodGet, "/codex/v1/taxonomies", "", "", ""), http.StatusOK, "canonical taxonomies")
	expectStatus(t, stack.do(t, http.MethodGet, "/codex/v1/terms/product_cat", "", "", ""), http.StatusOK, "canonical terms")

	brand := expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/brands", token, `{"status":"published","visibility":"public","slug":{"en":"acme"},"title":{"en":"Acme"},"metadata":{"country":{"value":"DE"}}}`, ""), http.StatusCreated, "create brand")
	brandID := dataID(t, brand)
	product := `{"status":"published","visibility":"public","slug":{"en":"espresso"},"title":{"en":"Espresso"},
		"terms":[{"taxonomy":"product_cat","term_id":"coffee"}],
		"metadata":{"sku":{"value":"ESP-1"},"price":{"value":1250},"currency":{"value":"EUR"},"state":{"value":"active"},"brand":{"value":"` + brandID + `"}}}`
	created := expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/products", token, product, ""), http.StatusCreated, "create product")
	productID := dataID(t, created)
	expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/products", token, strings.Replace(strings.Replace(product, "1250", "12.5", 1), "espresso", "bad", 2), ""), http.StatusBadRequest, "money must be minor units")

	for _, path := range []string{
		"/go-json/go/v2/products", "/go-json/go/v2/products?search=Espresso", "/go-json/go/v2/products/" + productID,
		"/go-json/go/v2/products/by-slug/espresso", "/go-json/go/v2/search?search=Espresso",
		"/go-json/go/v2/types", "/go-json/go/v2/content-types", "/go-json/go/v2/", "/go-json", "/",
		"/go-json/go/v2/types/product/json-schema", "/go-json/go/v2/types/product/form", "/go-json/go/v2/products/form",
		"/go-json/go/v2/schema", "/go-json/go/v2/openapi.json", "/go-json/go/v2/graphql.sdl",
		"/codex/v1/", "/codex/v1/entries/product", "/codex/v1/entries/product/" + productID, "/codex/v1/settings",
	} {
		expectStatus(t, stack.do(t, http.MethodGet, path, "", "", ""), http.StatusOK, "GET "+path)
	}
	authed := []string{"/go-json/go/v2/audit", "/go-json/go/v2/products/" + productID + "/revisions", "/go-json/go/v2/products/" + productID + "/form"}
	for _, path := range authed {
		expectStatus(t, stack.do(t, http.MethodGet, path, token, "", ""), http.StatusOK, "GET "+path)
	}
	if !strings.Contains(stack.do(t, http.MethodGet, "/go-json/go/v2/types/product/json-schema", "", "", "").Body.String(), "fastygo.codex/money") {
		t.Fatal("json schema must publish codex rules")
	}
	sdl := stack.do(t, http.MethodGet, "/go-json/go/v2/graphql.sdl", "", "", "").Body.String()
	if !strings.Contains(sdl, "price: Int") {
		t.Fatalf("money is Int in SDL: %s", sdl)
	}
	expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/types/product/form/bind", token, `{"locales":{"en":{"title":"Espresso","extra":"x"}}}`, ""), http.StatusOK, "bind form")
	expectStatus(t, stack.do(t, http.MethodGet, "/go-json/go/v2/types/missing/json-schema", "", "", ""), http.StatusNotFound, "missing type")
	expectStatus(t, stack.do(t, http.MethodGet, "/codex/v1/entries/product/missing", "", "", ""), http.StatusNotFound, "missing canonical")
	expectStatus(t, stack.do(t, http.MethodGet, "/codex/v1/entries/nope", "", "", ""), http.StatusNotFound, "unknown kind")

	trash := stack.do(t, http.MethodDelete, "/go-json/go/v2/brands/"+brandID, token, "", `"1"`)
	if trash.Code >= 500 {
		t.Fatalf("delete brand: %d %s", trash.Code, trash.Body.String())
	}
	expectStatus(t, stack.do(t, http.MethodDelete, "/go-json/go/v2/taxonomies/product_cat/terms/coffee", token, "", termUpdate.Header().Get("ETag")), http.StatusConflict, "assigned term cannot be deleted")
	if response := stack.do(t, http.MethodDelete, "/go-json/go/v2/taxonomies/product_cat", token, "", definitionUpdate.Header().Get("ETag")); response.Code >= 500 {
		t.Fatalf("delete taxonomy: %d %s", response.Code, response.Body.String())
	}
	expectStatus(t, stack.do(t, http.MethodPost, "/go-json/go/v2/taxonomies", "", `{"id":"x"}`, ""), http.StatusForbidden, "anonymous taxonomy")
}
