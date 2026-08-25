package identity_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	"github.com/fastygo/backend/internal/domain/authz"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	tokenidentity "github.com/fastygo/backend/internal/identity"
	"github.com/fastygo/backend/internal/storage/bbolt"
)

func TestBootstrapLoginAndUserRoleManagement(t *testing.T) {
	ctx := context.Background()
	storage, err := bbolt.Open(filepath.Join(t.TempDir(), "identity.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	defer storage.Close()
	tokens, err := tokenidentity.NewTokenManager(
		"identity-service-test-secret-at-least-32-bytes",
		"identity-test",
	)
	if err != nil {
		t.Fatalf("new token manager: %v", err)
	}
	service, err := applicationidentity.NewService(storage, tokens, nil)
	if err != nil {
		t.Fatalf("new identity service: %v", err)
	}
	if err := service.Initialize(ctx, "admin@example.com", "correct horse battery staple"); err != nil {
		t.Fatalf("initialize identity: %v", err)
	}
	if err := service.Initialize(ctx, "admin@example.com", ""); err != nil {
		t.Fatalf("reinitialize existing identity without bootstrap password: %v", err)
	}
	token, err := service.Authenticate(ctx, "ADMIN@example.com", "correct horse battery staple", time.Hour)
	if err != nil || token == "" {
		t.Fatalf("authenticate bootstrap admin: %v", err)
	}
	if _, err := service.Authenticate(ctx, "admin@example.com", "wrong", time.Hour); err == nil {
		t.Fatalf("invalid password was accepted")
	}
	adminRole := authz.AdministratorRole()
	admin := adminRole.Principal("bootstrap-admin")
	role, err := service.SaveRole(ctx, admin, domainidentity.Role{
		ID: "reviewer", Label: "Reviewer",
		Capabilities: []authz.Capability{authz.CapabilityContentReadPrivate},
	}, 0)
	if err != nil {
		t.Fatalf("save role: %v", err)
	}
	user, err := service.SaveUser(ctx, admin, applicationidentity.UserInput{
		Email: "reviewer@example.com", DisplayName: "Reviewer",
		Password: "another correct horse battery staple",
		RoleIDs:  []string{role.ID}, Active: true,
	}, 0)
	if err != nil {
		t.Fatalf("save user: %v", err)
	}
	encoded, err := json.Marshal(user)
	if err != nil {
		t.Fatalf("marshal user: %v", err)
	}
	if strings.Contains(string(encoded), "PasswordHash") ||
		strings.Contains(string(encoded), "password_hash") ||
		strings.Contains(string(encoded), "$2") {
		t.Fatalf("password hash leaked through user JSON: %s", encoded)
	}
}
