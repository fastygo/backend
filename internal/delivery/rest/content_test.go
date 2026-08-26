package rest

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

func TestContentRESTCreateReadAndOptimisticUpdate(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "rest.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, restClock{time.Now().UTC()})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	editor := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
		authz.CapabilityContentDelete,
		authz.CapabilityContentRestore,
		authz.CapabilityContentManageRevisions,
	)
	handler, err := NewContentHandler(service, fixedPrincipal{principal: editor})
	if err != nil {
		t.Fatalf("create handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)

	create := performJSON(mux, http.MethodPost, "/go-json/data/v1/resources/product", `{
		"status":"published",
		"visibility":"public",
		"slug":{"en":"Digital Course"},
		"title":{"en":"Digital Course"},
		"metadata":{"sku":{"value":"COURSE-1"},"secret":{"value":"internal","private":true}}
	}`, "")
	if create.Code != http.StatusCreated {
		t.Fatalf("create status %d: %s", create.Code, create.Body.String())
	}
	var created struct {
		Data resourceRecord `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Data.ID == "" || created.Data.Resource != "product" || created.Data.Values["slug_en"] != "digital-course" {
		t.Fatalf("unexpected created resource: %#v", created.Data)
	}

	publicHandler, _ := NewContentHandler(service, nil)
	publicMux := http.NewServeMux()
	publicHandler.Routes(publicMux)
	read := httptest.NewRecorder()
	publicMux.ServeHTTP(read, httptest.NewRequest(http.MethodGet, create.Header().Get("Location"), nil))
	if read.Code != http.StatusOK {
		t.Fatalf("read status %d: %s", read.Code, read.Body.String())
	}
	var public struct {
		Data resourceRecord `json:"data"`
	}
	if err := json.Unmarshal(read.Body.Bytes(), &public); err != nil {
		t.Fatalf("decode read response: %v", err)
	}
	if _, leaked := public.Data.Values["secret"]; leaked {
		t.Fatalf("private metadata leaked through REST")
	}

	updatePath := "/go-json/data/v1/resources/product/" + string(created.Data.ID)
	update := performJSON(mux, http.MethodPut, updatePath, `{
		"status":"published",
		"visibility":"public",
		"slug":{"en":"Digital Course"},
		"title":{"en":"Updated Course"}
	}`, `"v1"`)
	if update.Code != http.StatusOK || update.Header().Get("ETag") != `"v2"` {
		t.Fatalf("update failed: status=%d etag=%s body=%s", update.Code, update.Header().Get("ETag"), update.Body.String())
	}
	trashed := performJSON(mux, http.MethodPost, updatePath+"/transitions", `{
		"status":"trashed","expected_version":2,"reason":"test trash"
	}`, "")
	if trashed.Code != http.StatusOK || trashed.Header().Get("ETag") != `"v3"` {
		t.Fatalf("trash transition failed: %d %s", trashed.Code, trashed.Body.String())
	}
	restored := performJSON(mux, http.MethodPost, updatePath+"/transitions", `{
		"status":"draft","expected_version":3,"reason":"test restore"
	}`, "")
	if restored.Code != http.StatusOK || restored.Header().Get("ETag") != `"v4"` {
		t.Fatalf("restore transition failed: %d %s", restored.Code, restored.Body.String())
	}
	revisions := httptest.NewRecorder()
	mux.ServeHTTP(revisions, httptest.NewRequest(http.MethodGet, updatePath+"/revisions", nil))
	if revisions.Code != http.StatusOK {
		t.Fatalf("revision list failed: %d %s", revisions.Code, revisions.Body.String())
	}

	auditDenied := httptest.NewRecorder()
	mux.ServeHTTP(auditDenied, httptest.NewRequest(http.MethodGet, "/go-json/data/v1/audit", nil))
	if auditDenied.Code != http.StatusForbidden {
		t.Fatalf("audit endpoint ignored capability: %d", auditDenied.Code)
	}
	editor.Capabilities[authz.CapabilityAuditView] = struct{}{}
	auditResponse := httptest.NewRecorder()
	mux.ServeHTTP(auditResponse, httptest.NewRequest(http.MethodGet, "/go-json/data/v1/audit", nil))
	if auditResponse.Code != http.StatusOK {
		t.Fatalf("audit endpoint failed: %d %s", auditResponse.Code, auditResponse.Body.String())
	}
}

func TestContentRESTAcceptsFastyDataValuesContract(t *testing.T) {
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "values.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, restClock{time.Now().UTC()})
	principal := authz.NewPrincipal("editor", authz.CapabilityContentCreate, authz.CapabilityContentPublish)
	handler, _ := NewContentHandler(service, fixedPrincipal{principal: principal})
	mux := http.NewServeMux()
	handler.Routes(mux)

	response := performJSON(mux, http.MethodPost, "/go-json/data/v1/resources/product", `{
		"values": {
			"status": "published",
			"visibility": "public",
			"slug": "course",
			"title_en": "Course",
			"payload_en": {"slug":"course","price":49}
		}
	}`, "")
	if response.Code != http.StatusCreated {
		t.Fatalf("values create status %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Data resourceRecord `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode values response: %v", err)
	}
	payload, ok := body.Data.Values["payload_en"].(map[string]any)
	if !ok || payload["slug"] != "course" {
		t.Fatalf("fastygo.data payload was not preserved: %#v", body.Data.Values)
	}
}

func TestContentRESTFormBindRoundTripsExtraKeys(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "form.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, restClock{time.Now().UTC()})
	manifest := schema.Manifest{
		Name: "shop", Version: "1",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products", Public: true, RESTVisible: true,
			Fields: []schema.Field{{ID: "payload_ru", Type: schema.FieldJSON}, {ID: "payload_en", Type: schema.FieldJSON}},
			Form:   []schema.Field{{ID: "title", Type: schema.FieldString}},
		}},
	}
	handler, err := NewContentHandler(service, nil, manifest)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	mux := http.NewServeMux()
	handler.Routes(mux)
	schemaResponse := httptest.NewRecorder()
	mux.ServeHTTP(schemaResponse, httptest.NewRequest(http.MethodGet, "/go-json/data/v1/schema/product/form", nil))
	if schemaResponse.Code != http.StatusOK {
		t.Fatalf("form schema %d: %s", schemaResponse.Code, schemaResponse.Body.String())
	}
	bind := performJSON(mux, http.MethodPost, "/go-json/data/v1/schema/product/form/bind", `{
		"payload_ru":{"title":"Курс","kicker":"Backend"},
		"payload_en":{"title":"Course"}
	}`, "")
	if bind.Code != http.StatusOK {
		t.Fatalf("bind %d: %s", bind.Code, bind.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(bind.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	payloads, _ := body["payloads"].(map[string]any)
	ru, _ := payloads["payload_ru"].(map[string]any)
	if ru["kicker"] != "Backend" || ru["title"] != "Курс" {
		t.Fatalf("payloads: %#v", payloads)
	}
}

func TestContentRESTRejectsUnknownFieldsAndInvalidPagination(t *testing.T) {
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "invalid.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, _ := application.NewService(adapter, nil, restClock{time.Now().UTC()})
	handler, _ := NewContentHandler(service, fixedPrincipal{principal: authz.NewPrincipal("editor", authz.CapabilityContentCreate)})
	mux := http.NewServeMux()
	handler.Routes(mux)

	response := performJSON(mux, http.MethodPost, "/go-json/data/v1/resources/post", `{"unknown":true}`, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("unknown JSON field accepted: %d", response.Code)
	}

	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/go-json/data/v1/resources/post?per_page=101", nil))
	if list.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination accepted: %d", list.Code)
	}
}

type fixedPrincipal struct {
	principal authz.Principal
}

func (resolver fixedPrincipal) Resolve(*http.Request) (authz.Principal, error) {
	return resolver.principal, nil
}

type restClock struct {
	value time.Time
}

func (clock restClock) Now() time.Time {
	return clock.value
}

func performJSON(handler http.Handler, method, path, body, ifMatch string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
