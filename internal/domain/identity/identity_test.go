package identity

import (
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
)

func TestUserValidation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	valid := User{
		ID: "user_1", Email: "editor@example.com", DisplayName: "Editor",
		PasswordHash: "hash", RoleIDs: []string{"editor"}, Active: true,
		Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	cases := map[string]struct {
		mutate    func(*User)
		wantError bool
	}{
		"valid": {},
		"missing email": {mutate: func(user *User) { user.Email = "not-an-email" }, wantError: true},
		"no roles":      {mutate: func(user *User) { user.RoleIDs = nil }, wantError: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			user := valid
			if test.mutate != nil {
				test.mutate(&user)
			}
			err := user.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid user rejected: %v", err)
			}
		})
	}
}

func TestRoleValidation(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		role      Role
		wantError bool
	}{
		"valid": {
			role: Role{ID: "editor", Label: "Editor", Capabilities: []authz.Capability{authz.CapabilityContentEdit}, Version: 1},
		},
		"empty": {role: Role{}, wantError: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := test.role.Validate()
			if test.wantError && err == nil {
				t.Fatalf("expected role validation error")
			}
			if !test.wantError && err != nil {
				t.Fatalf("valid role rejected: %v", err)
			}
		})
	}
}
