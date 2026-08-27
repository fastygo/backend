package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/fastygo/framework/pkg/app"
)

func TestBuildComposesFrameworkPanelRESTAndBbolt(t *testing.T) {
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "backend.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest: DefaultManifest(), AllowInsecureAuth: true,
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	for _, path := range []string{
		"/healthz",
		"/readyz",
		"/go-json",
		"/go-json/go/v2/",
		"/go-json/go/v2/posts",
		"/go-json/go/v2/types",
		"/go-json/go/v2/taxonomies",
		"/go-json/go/v2/schema",
		"/go-json/go/v2/types/post/json-schema",
		"/go-json/go/v2/types/post/form",
		"/go-json/go/v2/openapi.json",
		"/go-json/go/v2/menus",
		"/go-json/go/v2/settings",
		"/go-json/go/v2/search?q=example",
		"/go-graphql?query=%7BschemaIdentity%7Bname%20version%20digest%7D%7D",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("User-Agent", "headless-conformance/1.0")
		response := httptest.NewRecorder()
		runtime.App.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", path, response.Code, response.Body.String())
		}
	}
	if len(runtime.ControlPlane.Panel.Resources()) != 4 {
		t.Fatalf("Panel did not receive manifest resources")
	}
}

func TestBuildBootstrapsDurableAdminLogin(t *testing.T) {
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "identity.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest:    DefaultManifest(),
		TokenSecret: "bootstrap-identity-secret-at-least-32-bytes",
		TokenIssuer: "bootstrap-test", AdminEmail: "admin@example.com",
		AdminPassword: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	request := httptest.NewRequest(
		http.MethodPost,
		"/go-json/go/v2/auth/login",
		bytes.NewBufferString(`{"email":"admin@example.com","password":"correct horse battery staple"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login returned %d: %s", response.Code, response.Body.String())
	}
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil || login.AccessToken == "" {
		t.Fatalf("decode login response: %v", err)
	}
	request = httptest.NewRequest(http.MethodGet, "/go-json/go/v2/roles", nil)
	request.Header.Set("Authorization", "Bearer "+login.AccessToken)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response = httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("role list returned %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(
		http.MethodPost,
		"/go-json/go/v2/posts",
		bytes.NewBufferString(`{
			"title":{"en":"Codex post"},
			"slug":{"en":"codex-post"},
			"content":{"en":"Portable content"},
			"status":"published",
			"visibility":"public"
		}`),
	)
	request.Header.Set("Authorization", "Bearer "+login.AccessToken)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("go-codex create returned %d: %s", response.Code, response.Body.String())
	}
	request = httptest.NewRequest(http.MethodGet, "/go-json/go/v2/posts/by-slug/codex-post?locale=en", nil)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response = httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("go-codex slug lookup returned %d: %s", response.Code, response.Body.String())
	}
}

func TestLoadManifestKeepsCodexKindsOnGitCourseProfile(t *testing.T) {
	t.Parallel()
	manifest, err := loadManifest(filepath.Join("..", "..", "dev", "gitcourse.manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"post", "page", "product", "menu", "setting"} {
		if _, ok := manifest.Resource(id); !ok {
			t.Fatalf("gitcourse profile missing core or site kind %s", id)
		}
	}
}

func TestOpenStorageRejectsIncompleteExternalDatabaseConfig(t *testing.T) {
	_, err := OpenStorage(context.Background(), Config{Storage: "postgres"})
	if err == nil {
		t.Fatalf("PostgreSQL without DATABASE_URL was accepted")
	}
	_, err = OpenStorage(context.Background(), Config{Storage: "unknown"})
	if err == nil {
		t.Fatalf("unknown storage engine was accepted")
	}
}

func TestBuildRequiresAuthenticationSecretByDefault(t *testing.T) {
	_, err := Build(context.Background(), Config{Manifest: DefaultManifest()})
	if err == nil {
		t.Fatalf("runtime without authentication secret was accepted")
	}
}

func TestLoadConfigReadsProductDefinedManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{
		"name":"store",
		"version":"1",
		"resources":[{"id":"product","collection":"products","fields":[{"id":"price","type":"money","required":true}]}]
	}`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	t.Setenv("HEADLESS_MANIFEST_PATH", path)
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if _, ok := config.Manifest.Resource("product"); !ok {
		t.Fatalf("external product resource was not loaded")
	}
	if _, ok := config.Manifest.Resource("post"); !ok {
		t.Fatalf("codex post kind must be merged into a site manifest")
	}
}

func TestBuildExposesCMSCookieSession(t *testing.T) {
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "session.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest: DefaultManifest(), TokenSecret: "bootstrap-identity-secret-at-least-32-bytes",
		TokenIssuer: "bootstrap-test", AdminEmail: "admin@example.test",
		AdminPassword: "admin-local-dev",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	request := httptest.NewRequest(
		http.MethodPost, "/go-json/auth/login",
		bytes.NewBufferString(`{"email":"admin@example.test","password":"admin-local-dev"}`),
	)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Result().Cookies() == nil {
		t.Fatalf("session login returned %d: %s", response.Code, response.Body.String())
	}
	var session struct {
		Data struct {
			Email     string `json:"email"`
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil || session.Data.Email == "" || session.Data.CSRFToken == "" {
		t.Fatalf("decode session: %v %#v", err, session)
	}
	me := httptest.NewRequest(http.MethodGet, "/go-json/auth/me", nil)
	me.Header.Set("User-Agent", "headless-conformance/1.0")
	for _, cookie := range response.Result().Cookies() {
		me.AddCookie(cookie)
	}
	meResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(meResponse, me)
	if meResponse.Code != http.StatusOK {
		t.Fatalf("session me returned %d: %s", meResponse.Code, meResponse.Body.String())
	}
	graphql := httptest.NewRequest(
		http.MethodPost, "/go-graphql",
		bytes.NewBufferString(`{"query":"{ schemaIdentity { name } }"}`),
	)
	graphql.Header.Set("Content-Type", "application/json")
	graphql.Header.Set("User-Agent", "headless-conformance/1.0")
	graphql.Header.Set("X-CSRF-Token", session.Data.CSRFToken)
	for _, cookie := range response.Result().Cookies() {
		graphql.AddCookie(cookie)
	}
	graphResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(graphResponse, graphql)
	if graphResponse.Code != http.StatusOK {
		t.Fatalf("graphql alias returned %d: %s", graphResponse.Code, graphResponse.Body.String())
	}
}

func TestFrameworkConfigBoundsHTTPShutdown(t *testing.T) {
	t.Parallel()
	config, err := app.LoadConfig()
	if err != nil {
		t.Fatalf("load framework config: %v", err)
	}
	if config.HTTPShutdownTimeout <= 0 || config.HTTPReadTimeout <= 0 || config.HTTPWriteTimeout <= 0 {
		t.Fatalf("HTTP timeouts must be bounded: %#v", config)
	}
}
