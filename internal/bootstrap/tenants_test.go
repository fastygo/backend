package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/fastygo/backend/internal/storage/drivers"
	"github.com/fastygo/backend/internal/tenant"
	"github.com/fastygo/framework/pkg/app"
)

func TestLoadConfigImplicitTenantWhenTenantsPathUnset(t *testing.T) {
	t.Setenv("HEADLESS_TENANTS_PATH", "")
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if config.TenantsPath != "" || len(config.Tenants) != 0 {
		t.Fatalf("single mode must not load tenants.json: %#v", config.Tenants)
	}
}

func TestLoadConfigReadsTenantsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tenants.json")
	body := `[
		{"id":"shop","hosts":["shop.example.test"],"storage":{"path":"` + filepath.Join(dir, "shop.db") + `"},"media_root":"` + filepath.Join(dir, "shop-media") + `"},
		{"id":"press","hosts":["press.example.test"],"storage":{"path":"` + filepath.Join(dir, "press.db") + `"},"media_root":"` + filepath.Join(dir, "press-media") + `"}
	]`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("HEADLESS_TENANTS_PATH", path)
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(config.Tenants) != 2 || config.Tenants[0].ID != "shop" {
		t.Fatalf("tenants: %#v", config.Tenants)
	}
}

func TestMultiTenantUnknownHostIs404AndIgnoresForwardedHost(t *testing.T) {
	runtime := newMultiTenantRuntime(t)
	request := httptest.NewRequest(http.MethodGet, "http://unknown.example.test/codex/v1/", nil)
	request.Header.Set("X-Forwarded-Host", "shop.example.test")
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("unknown host %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"not_found"`) {
		t.Fatalf("unknown host body: %s", response.Body.String())
	}
	health := httptest.NewRequest(http.MethodGet, "http://unknown.example.test/healthz", nil)
	health.Header.Set("User-Agent", "headless-conformance/1.0")
	healthResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(healthResponse, health)
	if healthResponse.Code != http.StatusOK {
		t.Fatalf("healthz on unknown host %d: %s", healthResponse.Code, healthResponse.Body.String())
	}
}

func TestMultiTenantIsolationSameSlugAndSessions(t *testing.T) {
	runtime := newMultiTenantRuntime(t)
	shopToken := loginHost(t, runtime, "shop.example.test", "admin@example.com", "correct horse battery staple")
	pressToken := loginHost(t, runtime, "press.example.test", "admin@example.com", "correct horse battery staple")
	if shopToken == pressToken {
		t.Fatal("tenants must not issue identical session tokens")
	}

	createPost(t, runtime, "shop.example.test", shopToken, "about", "Shop about")
	createPost(t, runtime, "press.example.test", pressToken, "about", "Press about")

	shopBody := getBySlug(t, runtime, "shop.example.test", "about")
	pressBody := getBySlug(t, runtime, "press.example.test", "about")
	if !strings.Contains(shopBody, "Shop about") || strings.Contains(shopBody, "Press about") {
		t.Fatalf("shop slug leaked: %s", shopBody)
	}
	if !strings.Contains(pressBody, "Press about") || strings.Contains(pressBody, "Shop about") {
		t.Fatalf("press slug leaked: %s", pressBody)
	}

	cross := httptest.NewRequest(http.MethodGet, "http://press.example.test/go-json/auth/me", nil)
	cross.Header.Set("User-Agent", "headless-conformance/1.0")
	cross.Header.Set("Authorization", "Bearer "+shopToken)
	crossResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(crossResponse, cross)
	if crossResponse.Code != http.StatusUnauthorized {
		t.Fatalf("shop token on press: %d %s", crossResponse.Code, crossResponse.Body.String())
	}
}

func TestSingleModeIgnoresHost(t *testing.T) {
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en",
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "single.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest: DefaultManifest(), AllowInsecureAuth: true,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	request := httptest.NewRequest(http.MethodGet, "http://whatever.example.test/codex/v1/", nil)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("single mode host %d: %s", response.Code, response.Body.String())
	}
	if len(runtime.sites) != 1 || runtime.sites[0].spec.ID != tenant.DefaultID {
		t.Fatalf("implicit tenant: %#v", runtime.sites)
	}
}

func newMultiTenantRuntime(t *testing.T) *Runtime {
	t.Helper()
	dir := t.TempDir()
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", Manifest: DefaultManifest(),
		TokenSecret: "bootstrap-identity-secret-at-least-32-bytes",
		TokenIssuer: "bootstrap-test", AdminEmail: "admin@example.com",
		AdminPassword: "correct horse battery staple",
		MediaMaxBytes: 1 << 20,
		Tenants: []tenant.Spec{
			{
				ID: "shop", Hosts: []string{"shop.example.test"},
				Storage:   tenant.Storage{Path: filepath.Join(dir, "shop.db")},
				MediaRoot: filepath.Join(dir, "shop-media"),
			},
			{
				ID: "press", Hosts: []string{"press.example.test"},
				Storage:   tenant.Storage{Path: filepath.Join(dir, "press.db")},
				MediaRoot: filepath.Join(dir, "press-media"),
			},
		},
	})
	if err != nil {
		t.Fatalf("build multi: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime
}

func loginHost(t *testing.T, runtime *Runtime, host, email, password string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	request := httptest.NewRequest(http.MethodPost, "http://"+host+"/go-json/go/v2/auth/login", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("login %s: %d %s", host, response.Code, response.Body.String())
	}
	var login struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &login); err != nil || login.AccessToken == "" {
		t.Fatalf("login token %s: %v %s", host, err, response.Body.String())
	}
	return login.AccessToken
}

func createPost(t *testing.T, runtime *Runtime, host, token, slug, title string) {
	t.Helper()
	payload := `{
		"title":{"en":"` + title + `"},
		"slug":{"en":"` + slug + `"},
		"content":{"en":"` + title + ` body"},
		"status":"published",
		"visibility":"public"
	}`
	request := httptest.NewRequest(http.MethodPost, "http://"+host+"/go-json/go/v2/posts", strings.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusCreated && response.Code != http.StatusOK {
		t.Fatalf("create %s %s: %d %s", host, slug, response.Code, response.Body.String())
	}
}

func getBySlug(t *testing.T, runtime *Runtime, host, slug string) string {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "http://"+host+"/go-json/go/v2/posts/by-slug/"+slug, nil)
	request.Header.Set("User-Agent", "headless-conformance/1.0")
	response := httptest.NewRecorder()
	runtime.App.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("by-slug %s %s: %d %s", host, slug, response.Code, response.Body.String())
	}
	return response.Body.String()
}
