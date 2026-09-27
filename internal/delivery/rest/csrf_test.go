package rest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/identity"
)

type denyingCSRF struct{}

func (denyingCSRF) Resolve(*http.Request) (authz.Principal, error) {
	return authz.NewPrincipal("editor"), nil
}

func (denyingCSRF) ValidateCookieCSRF(*http.Request) error {
	return errors.New("csrf token is invalid")
}

func TestCookieMutationsRejectMissingCSRF(t *testing.T) {
	t.Parallel()
	content := &ContentHandler{principal: denyingCSRF{}}
	taxonomy := &TaxonomyHandler{principal: denyingCSRF{}}
	media := &MediaHandler{principal: denyingCSRF{}}
	users := &IdentityHandler{principal: denyingCSRF{}}
	cases := []struct {
		name string
		call func(http.ResponseWriter, *http.Request)
	}{
		{"transition", content.transition},
		{"restore", content.restoreRevision},
		{"bind", content.bindForm},
		{"taxonomy", taxonomy.createDefinition},
		{"media", media.upload},
		{"user", users.createUser},
		{"role", users.deleteRole},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{}`))
			recorder := httptest.NewRecorder()
			test.call(recorder, request)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status %d body %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSessionLoginRejectsCrossSiteAndExistingCookieWithoutToken(t *testing.T) {
	t.Parallel()
	tokens, err := identity.NewTokenManager(strings.Repeat("s", 32), "test")
	if err != nil {
		t.Fatal(err)
	}
	handler := &SessionHandler{tokens: tokens}
	cross := httptest.NewRequest(http.MethodPost, "/go-json/auth/login", strings.NewReader(`{"email":"a@b.c","password":"x"}`))
	cross.Header.Set("Content-Type", "application/json")
	cross.Header.Set("Sec-Fetch-Site", "cross-site")
	crossRecorder := httptest.NewRecorder()
	handler.login(crossRecorder, cross)
	if crossRecorder.Code != http.StatusForbidden {
		t.Fatalf("cross-site status %d", crossRecorder.Code)
	}

	cookie := httptest.NewRequest(http.MethodPost, "/go-json/auth/login", strings.NewReader(`{"email":"a@b.c","password":"x"}`))
	cookie.Header.Set("Content-Type", "application/json")
	cookie.AddCookie(&http.Cookie{Name: identity.SessionCookieName, Value: "session-token"})
	cookieRecorder := httptest.NewRecorder()
	handler.login(cookieRecorder, cookie)
	if cookieRecorder.Code != http.StatusForbidden {
		t.Fatalf("cookie status %d", cookieRecorder.Code)
	}
}
