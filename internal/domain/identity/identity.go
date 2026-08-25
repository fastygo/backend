package identity

import (
	"errors"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
)

type User struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	RoleIDs      []string
	Active       bool
	Version      uint64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Role struct {
	ID           string
	Label        string
	Capabilities []authz.Capability
	Version      uint64
}

func (user *User) Normalize() {
	user.ID = strings.TrimSpace(user.ID)
	user.Email = strings.ToLower(strings.TrimSpace(user.Email))
	user.DisplayName = strings.TrimSpace(user.DisplayName)
	for index := range user.RoleIDs {
		user.RoleIDs[index] = strings.TrimSpace(user.RoleIDs[index])
	}
	slices.Sort(user.RoleIDs)
	user.RoleIDs = slices.Compact(user.RoleIDs)
}

func (user User) Validate() error {
	if user.ID == "" || user.DisplayName == "" || user.PasswordHash == "" {
		return errors.New("user id, display name, and password hash are required")
	}
	address, err := mail.ParseAddress(user.Email)
	if err != nil || !strings.EqualFold(address.Address, user.Email) {
		return errors.New("user email is invalid")
	}
	if user.Version == 0 || user.CreatedAt.IsZero() || user.UpdatedAt.IsZero() {
		return errors.New("user version and timestamps are required")
	}
	if len(user.RoleIDs) == 0 {
		return errors.New("user requires at least one role")
	}
	for _, roleID := range user.RoleIDs {
		if roleID == "" {
			return errors.New("user role id is invalid")
		}
	}
	return nil
}

func (role *Role) Normalize() {
	role.ID = strings.TrimSpace(role.ID)
	role.Label = strings.TrimSpace(role.Label)
	slices.Sort(role.Capabilities)
	role.Capabilities = slices.Compact(role.Capabilities)
}

func (role Role) Validate() error {
	if role.ID == "" || role.Label == "" || role.Version == 0 {
		return errors.New("role id, label, and version are required")
	}
	projection := authz.Role{ID: role.ID, Label: role.Label, Capabilities: role.Capabilities}
	return projection.Validate()
}
