package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/fastygo/framework/pkg/app"
)

func TestCookieMutationsRequireCSRFAndBearerIsExempt(t *testing.T) {
	runtime, err := Build(context.Background(), Config{
		App: app.Config{
			AppBind: "127.0.0.1:0", DefaultLocale: "en", AvailableLocales: []string{"en"},
			HealthLivePath: "/healthz", HealthReadyPath: "/readyz",
		},
		Storage: "bbolt", BboltPath: filepath.Join(t.TempDir(), "csrf.db"),
		MediaRoot: filepath.Join(t.TempDir(), "media"), MediaMaxBytes: 1 << 20,
		Manifest: DefaultManifest(), TokenSecret: "csrf-canary-secret-at-least-32-bytes",
		TokenIssuer: "csrf-test", AdminEmail: "admin@example.test",
		AdminPassword: "admin-local-dev",
	})
	if err != nil {
		t.Fatalf("build runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close() })

	login := httptest.NewRequest(
		http.MethodPost, "/go-json/auth/login",
		bytes.NewBufferString(`{"email":"admin@example.test","password":"admin-local-dev"}`),
	)
	login.Header.Set("Content-Type", "application/json")
	login.Header.Set("User-Agent", "headless-conformance/1.0")
	loginResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(loginResponse, login)
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("session login returned %d: %s", loginResponse.Code, loginResponse.Body.String())
	}
	var session struct {
		Data struct {
			CSRFToken string `json:"csrfToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal(loginResponse.Body.Bytes(), &session); err != nil || session.Data.CSRFToken == "" {
		t.Fatalf("decode session: %v %#v", err, session)
	}
	cookies := loginResponse.Result().Cookies()

	bearerLogin := httptest.NewRequest(
		http.MethodPost, "/go-json/go/v2/auth/login",
		bytes.NewBufferString(`{"email":"admin@example.test","password":"admin-local-dev"}`),
	)
	bearerLogin.Header.Set("Content-Type", "application/json")
	bearerLogin.Header.Set("User-Agent", "headless-conformance/1.0")
	bearerResponse := httptest.NewRecorder()
	runtime.App.ServeHTTP(bearerResponse, bearerLogin)
	if bearerResponse.Code != http.StatusOK {
		t.Fatalf("bearer login returned %d: %s", bearerResponse.Code, bearerResponse.Body.String())
	}
	var credentials struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(bearerResponse.Body.Bytes(), &credentials); err != nil || credentials.AccessToken == "" {
		t.Fatalf("decode bearer login: %v", err)
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "taxonomy create", method: http.MethodPost, path: "/go-json/go/v2/taxonomies", body: `{"id":"topic","label":{"en":"Topic"}}`},
		{name: "identity user create", method: http.MethodPost, path: "/go-json/go/v2/users", body: `{"email":"editor@example.test","password":"editor-local-dev","role_ids":["editor"],"active":true}`},
		{name: "content create", method: http.MethodPost, path: "/go-json/go/v2/posts", body: `{"title":{"en":"CSRF"},"slug":{"en":"csrf"},"status":"draft","visibility":"private"}`},
	}
	for _, test := range cases {
		t.Run(test.name+" without csrf", func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("User-Agent", "headless-conformance/1.0")
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			runtime.App.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("cookie mutation without CSRF returned %d: %s", response.Code, response.Body.String())
			}
		})
		t.Run(test.name+" with csrf", func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("User-Agent", "headless-conformance/1.0")
			request.Header.Set("X-CSRF-Token", session.Data.CSRFToken)
			for _, cookie := range cookies {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			runtime.App.ServeHTTP(response, request)
			if response.Code == http.StatusForbidden {
				t.Fatalf("valid CSRF was rejected: %s", response.Body.String())
			}
		})
		t.Run(test.name+" bearer exempt", func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, bytes.NewBufferString(test.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("User-Agent", "headless-conformance/1.0")
			request.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
			response := httptest.NewRecorder()
			runtime.App.ServeHTTP(response, request)
			if response.Code == http.StatusForbidden {
				t.Fatalf("bearer mutation was CSRF-blocked: %s", response.Body.String())
			}
		})
	}

	t.Run("transition without csrf", func(t *testing.T) {
		create := httptest.NewRequest(
			http.MethodPost, "/go-json/go/v2/posts",
			bytes.NewBufferString(`{"title":{"en":"Due"},"slug":{"en":"due"},"status":"draft","visibility":"private"}`),
		)
		create.Header.Set("Content-Type", "application/json")
		create.Header.Set("User-Agent", "headless-conformance/1.0")
		create.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
		created := httptest.NewRecorder()
		runtime.App.ServeHTTP(created, create)
		if created.Code != http.StatusCreated {
			t.Fatalf("create post returned %d: %s", created.Code, created.Body.String())
		}
		var document struct {
			Data struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := json.Unmarshal(created.Body.Bytes(), &document); err != nil || document.Data.ID == "" {
			t.Fatalf("decode created post: %v", err)
		}
		request := httptest.NewRequest(
			http.MethodPost, "/go-json/go/v2/posts/"+document.Data.ID+"/transitions",
			bytes.NewBufferString(`{"status":"published","expected_version":1}`),
		)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("User-Agent", "headless-conformance/1.0")
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		runtime.App.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("transition without CSRF returned %d: %s", response.Code, response.Body.String())
		}
	})

	t.Run("media upload without csrf", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		file, err := writer.CreateFormFile("file", "manual.txt")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := io.WriteString(file, "documentation"); err != nil {
			t.Fatalf("write form file: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("close multipart: %v", err)
		}
		request := httptest.NewRequest(http.MethodPost, "/go-json/go/v2/media", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("User-Agent", "headless-conformance/1.0")
		for _, cookie := range cookies {
			request.AddCookie(cookie)
		}
		response := httptest.NewRecorder()
		runtime.App.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("media upload without CSRF returned %d: %s", response.Code, response.Body.String())
		}
	})
}
