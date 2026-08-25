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
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"-"`
	RoleIDs      []string  `json:"role_ids"`
	Active       bool      `json:"active"`
	Version      uint64    `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UserRecord is the durable representation. Delivery code must expose User instead.
type UserRecord struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	DisplayName  string    `json:"display_name"`
	PasswordHash string    `json:"password_hash"`
	RoleIDs      []string  `json:"role_ids"`
	Active       bool      `json:"active"`
	Version      uint64    `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func RecordFromUser(user User) UserRecord {
	return UserRecord(user)
}

func (record UserRecord) User() User {
	return User(record)
}

type Role struct {
	ID           string             `json:"id"`
	Label        string             `json:"label"`
	Capabilities []authz.Capability `json:"capabilities"`
	Version      uint64             `json:"version"`
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
