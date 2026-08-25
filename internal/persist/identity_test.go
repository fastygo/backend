package persist

import (
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
)

func TestEncodeDecodeIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	cases := map[string]domainidentity.User{
		"active editor": {
			ID: "user_1", Email: "editor@example.com", DisplayName: "Editor",
			PasswordHash: "stored-hash", RoleIDs: []string{"editor"}, Active: true,
			Version: 2, CreatedAt: now, UpdatedAt: now,
		},
	}
	for name, user := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			encoded, err := EncodeUser(user)
			if err != nil {
				t.Fatalf("encode user: %v", err)
			}
			decoded, err := DecodeUser(encoded)
			if err != nil {
				t.Fatalf("decode user: %v", err)
			}
			if decoded.PasswordHash != user.PasswordHash || decoded.Email != user.Email {
				t.Fatalf("user round-trip diverged: %#v", decoded)
			}
			role := domainidentity.Role{
				ID: "editor", Label: "Editor",
				Capabilities: []authz.Capability{authz.CapabilityContentEdit}, Version: 1,
			}
			encodedRole, err := EncodeRole(role)
			if err != nil {
				t.Fatalf("encode role: %v", err)
			}
			decodedRole, err := DecodeRole(encodedRole)
			if err != nil || decodedRole.ID != role.ID {
				t.Fatalf("role round-trip diverged: %#v %v", decodedRole, err)
			}
		})
	}
}
