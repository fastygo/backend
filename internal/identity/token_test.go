package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
)

func TestSignedBearerTokenRoundTripAndTamperDetection(t *testing.T) {
	t.Parallel()
	manager, err := NewTokenManager("0123456789abcdef0123456789abcdef", "headless")
	if err != nil {
		t.Fatalf("create token manager: %v", err)
	}
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return now }
	token, err := manager.Issue(
		authz.NewPrincipal("editor", authz.CapabilityContentCreate, authz.CapabilityContentEditOwn),
		time.Hour,
	)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	principal, err := manager.Resolve(request)
	if err != nil {
		t.Fatalf("resolve token: %v", err)
	}
	if principal.ID != "editor" || !principal.Has(authz.CapabilityContentCreate) {
		t.Fatalf("token lost principal capabilities")
	}

	request.Header.Set("Authorization", "Bearer "+token+"tampered")
	if _, err := manager.Resolve(request); err == nil {
		t.Fatalf("tampered token was accepted")
	}
}

func TestSignedBearerTokenExpiryAndAnonymousAccess(t *testing.T) {
	t.Parallel()
	manager, _ := NewTokenManager("0123456789abcdef0123456789abcdef", "headless")
	now := time.Now().UTC()
	manager.now = func() time.Time { return now }
	token, err := manager.Issue(authz.EditorRole().Principal("editor"), time.Minute)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	manager.now = func() time.Time { return now.Add(time.Minute) }
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	if _, err := manager.Resolve(request); err == nil {
		t.Fatalf("expired token was accepted")
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	principal, err := manager.Resolve(request)
	if err != nil || !principal.Anonymous {
		t.Fatalf("missing bearer token must resolve as anonymous")
	}
}

func TestTokenManagerRejectsWeakSecretsAndUnknownCapabilities(t *testing.T) {
	t.Parallel()
	if _, err := NewTokenManager("weak", "headless"); err == nil {
		t.Fatalf("weak secret was accepted")
	}
	manager, _ := NewTokenManager("0123456789abcdef0123456789abcdef", "headless")
	_, err := manager.Issue(authz.NewPrincipal("user", "unknown.capability"), time.Hour)
	if err == nil {
		t.Fatalf("unknown capability was accepted")
	}
}
