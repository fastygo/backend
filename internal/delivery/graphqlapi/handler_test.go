package graphqlapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
)

func TestGraphQLSupportsAdminCreateUpdateListContract(t *testing.T) {
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "graphql.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, graphQLClock{time.Now().UTC()})
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)
	manifest := schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{{ID: "product", Collection: "products"}},
	}
	handler, err := New(service, manifest, fixedPrincipal{principal})
	if err != nil {
		t.Fatalf("create GraphQL handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)

	create := execute(t, mux, `
		mutation CreateProduct($input: ProductInput!, $idempotencyKey: String!) {
			createProduct(input: $input, idempotencyKey: $idempotencyKey) {
				id version title slug status visibility payloadEn
			}
		}`,
		map[string]any{
			"input": map[string]any{
				"title": "Course", "slug": "course", "status": "published", "visibility": "public",
				"payloadEn": map[string]any{"price": 49},
			},
			"idempotencyKey": "create-product-1",
		},
	)
	created := create["data"].(map[string]any)["createProduct"].(map[string]any)
	if created["slug"] != "course" || created["version"].(float64) != 1 {
		t.Fatalf("unexpected GraphQL create result: %#v", created)
	}
	retried := execute(t, mux, `
		mutation CreateProduct($input: ProductInput!, $idempotencyKey: String!) {
			createProduct(input: $input, idempotencyKey: $idempotencyKey) { id version }
		}`,
		map[string]any{
			"input": map[string]any{
				"title": "Course", "slug": "course", "status": "published", "visibility": "public",
			},
			"idempotencyKey": "create-product-1",
		},
	)["data"].(map[string]any)["createProduct"].(map[string]any)
	if retried["id"] != created["id"] {
		t.Fatalf("idempotent retry created a different record")
	}

	list := execute(t, mux, `
		query ProductList($page: Int!, $perPage: Int!) {
			products(page: $page, perPage: $perPage) {
				items { id title payloadEn }
				page perPage total totalPages
			}
		}`,
		map[string]any{"page": 1, "perPage": 20},
	)
	page := list["data"].(map[string]any)["products"].(map[string]any)
	if page["total"].(float64) != 1 || len(page["items"].([]any)) != 1 {
		t.Fatalf("unexpected GraphQL list result: %#v", page)
	}
}

type graphQLClock struct {
	value time.Time
}

func TestGraphQLKeepsPagedListWhenResourceAndCollectionShareAName(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "settings.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, graphQLClock{time.Now().UTC()})
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)
	manifest := schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{{
			ID: "site_settings", Collection: "site-settings", Public: true, GraphQLVisible: true,
			Fields: []schema.Field{{ID: "sku", Type: schema.FieldString}},
		}},
	}
	handler, err := New(service, manifest, fixedPrincipal{principal})
	if err != nil {
		t.Fatalf("create GraphQL handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)
	created := execute(t, mux, `
		mutation CreateSiteSettings($input: SiteSettingsInput!) {
			createSiteSettings(input: $input) { id slug }
		}`,
		map[string]any{"input": map[string]any{"title": "Brand", "slug": "brand", "status": "published", "visibility": "public"}},
	)
	id := created["data"].(map[string]any)["createSiteSettings"].(map[string]any)["id"]
	listed := execute(t, mux, `
		query SiteSettingsList($page: Int!, $perPage: Int!) {
			siteSettings(page: $page, perPage: $perPage) { total items { id slug } }
		}`,
		map[string]any{"page": 1, "perPage": 10},
	)
	page := listed["data"].(map[string]any)["siteSettings"].(map[string]any)
	if page["total"].(float64) != 1 {
		t.Fatalf("paged siteSettings was overwritten: %#v", listed)
	}
	got := execute(t, mux, `query ($id: ID!) { siteSettingsById(id: $id) { id slug } }`, map[string]any{"id": id})
	if got["data"].(map[string]any)["siteSettingsById"].(map[string]any)["slug"] != "brand" {
		t.Fatalf("singular lookup failed: %#v", got)
	}
}

func TestGraphQLFormsetBindsLocaleDocuments(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "formset.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, graphQLClock{time.Now().UTC()})
	_ = service.SetManifest(schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products", Public: true, GraphQLVisible: true,
			Fields: []schema.Field{{ID: "sku", Type: schema.FieldString}},
			Form:   []schema.Field{{ID: "title", Type: schema.FieldString}, {ID: "price", Type: schema.FieldMoney}},
		}},
	})
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)
	handler, err := New(service, schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products", Public: true, GraphQLVisible: true,
			Fields: []schema.Field{{ID: "sku", Type: schema.FieldString}},
			Form:   []schema.Field{{ID: "title", Type: schema.FieldString}, {ID: "price", Type: schema.FieldMoney}},
		}},
	}, fixedPrincipal{principal})
	if err != nil {
		t.Fatalf("create GraphQL handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)
	created := execute(t, mux, `
		mutation CreateProduct($input: ProductInput!) {
			createProduct(input: $input) { id }
		}`,
		map[string]any{"input": map[string]any{
			"title": "Course", "slug": "course", "status": "published", "visibility": "public",
			"payloadRu": map[string]any{"title": "Курс", "price": 39900.0, "kicker": "Backend"},
			"payloadEn": map[string]any{"title": "Course", "price": 39900.0},
		}},
	)
	id := created["data"].(map[string]any)["createProduct"].(map[string]any)["id"]
	got := execute(t, mux, `
		query ($id: ID!) {
			formset(resource: "product", id: $id) {
				record
				values
				extra
				schema
			}
		}`, map[string]any{"id": id})
	form := got["data"].(map[string]any)["formset"].(map[string]any)
	if form["record"] != "product" {
		t.Fatalf("formset: %#v", form)
	}
	extra, _ := form["extra"].(map[string]any)
	ru, _ := extra["ru"].(map[string]any)
	if ru["kicker"] != "Backend" {
		t.Fatalf("extra kicker lost: %#v", form)
	}
}

func TestGraphQLIsMutation(t *testing.T) {
	t.Parallel()
	cases := map[string]bool{
		"query ProductList { products { total } }":       false,
		"{ schemaIdentity { name } }":                    false,
		"# comment\nquery Q { schemaIdentity { name } }": false,
		"mutation CreateProduct { createProduct }":       true,
		"# note\nmutation UpdateProduct { x }":           true,
	}
	for query, want := range cases {
		if got := graphQLIsMutation(query); got != want {
			t.Fatalf("graphQLIsMutation(%q)=%v want %v", query, got, want)
		}
	}
}

func (clock graphQLClock) Now() time.Time {
	return clock.value
}

type fixedPrincipal struct {
	value authz.Principal
}

func (resolver fixedPrincipal) Resolve(*http.Request) (authz.Principal, error) {
	return resolver.value, nil
}

func execute(t *testing.T, handler http.Handler, query string, variables map[string]any) map[string]any {
	t.Helper()
	encoded, _ := json.Marshal(map[string]any{"query": query, "variables": variables})
	request := httptest.NewRequest(http.MethodPost, "/go-graphql", bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GraphQL status %d: %s", response.Code, response.Body.String())
	}
	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode GraphQL response: %v", err)
	}
	if graphQLErrors, exists := document["errors"]; exists {
		t.Fatalf("GraphQL errors: %#v", graphQLErrors)
	}
	return document
}
