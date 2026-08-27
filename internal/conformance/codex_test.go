package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fastygo/backend/internal/bootstrap"
	"github.com/fastygo/framework/pkg/app"
)

func TestGoCodexLevel0Discovery(t *testing.T) {
	runtime := newRuntime(t)
	for _, path := range []string{"/go-json", "/go-json/go/v2/"} {
		response := request(runtime.App, http.MethodGet, path, "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
		var discovery struct {
			Version string         `json:"version"`
			Routes  map[string]any `json:"routes"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &discovery); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if discovery.Version != "2" || len(discovery.Routes) == 0 {
			t.Fatalf("%s is not a valid Level 0 discovery document", path)
		}
	}
}

func TestGoCodexLevel1ContentAndMetadata(t *testing.T) {
	runtime := newRuntime(t)
	login := request(
		runtime.App,
		http.MethodPost,
		"/go-json/go/v2/auth/login",
		`{"email":"admin@example.com","password":"correct horse battery staple"}`,
		"",
	)
	if login.Code != http.StatusOK {
		t.Fatalf("login returned %d: %s", login.Code, login.Body.String())
	}
	var credentials struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &credentials); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	created := request(
		runtime.App,
		http.MethodPost,
		"/go-json/go/v2/posts",
		`{"title":{"en":"Conformance"},"slug":{"en":"conformance"},"content":{"en":"Body"},"status":"published","visibility":"public"}`,
		credentials.AccessToken,
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("create returned %d: %s", created.Code, created.Body.String())
	}
	for _, path := range []string{
		"/go-json/go/v2/posts",
		"/go-json/go/v2/posts/by-slug/conformance?locale=en",
		"/go-json/go/v2/content-types",
		"/go-json/go/v2/types",
		"/go-json/go/v2/taxonomies",
		"/go-json/go/v2/menus",
		"/go-json/go/v2/settings",
		"/go-json/go/v2/search?q=Conformance",
	} {
		response := request(runtime.App, http.MethodGet, path, "", "")
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
}

func newRuntime(t *testing.T) *bootstrap.Runtime {
	t.Helper()
	runtime, err := bootstrap.Build(context.Background(), bootstrap.Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en", "ru"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "codex.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest:    bootstrap.DefaultManifest(),
		TokenSecret: "codex-conformance-secret-at-least-32-bytes",
		TokenIssuer: "codex-conformance", AdminEmail: "admin@example.com",
		AdminPassword: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func request(
	handler http.Handler,
	method string,
	path string,
	body string,
	token string,
) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
