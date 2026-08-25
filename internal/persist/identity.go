package persist

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
)

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

type Role struct {
	ID           string             `json:"id"`
	Label        string             `json:"label"`
	Capabilities []authz.Capability `json:"capabilities"`
	Version      uint64             `json:"version"`
}

func UserFromDomain(user domainidentity.User) UserRecord {
	return UserRecord{
		ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		PasswordHash: user.PasswordHash, RoleIDs: user.RoleIDs, Active: user.Active,
		Version: user.Version, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func (record UserRecord) Domain() domainidentity.User {
	return domainidentity.User{
		ID: record.ID, Email: record.Email, DisplayName: record.DisplayName,
		PasswordHash: record.PasswordHash, RoleIDs: record.RoleIDs, Active: record.Active,
		Version: record.Version, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

func RoleFromDomain(role domainidentity.Role) Role {
	return Role{
		ID: role.ID, Label: role.Label, Capabilities: role.Capabilities, Version: role.Version,
	}
}

func (role Role) Domain() domainidentity.Role {
	return domainidentity.Role{
		ID: role.ID, Label: role.Label, Capabilities: role.Capabilities, Version: role.Version,
	}
}

func EncodeUser(user domainidentity.User) ([]byte, error) {
	encoded, err := json.Marshal(UserFromDomain(user))
	if err != nil {
		return nil, fmt.Errorf("failed to encode identity user: %w", err)
	}
	return encoded, nil
}

func DecodeUser(encoded []byte) (domainidentity.User, error) {
	var record UserRecord
	if err := json.Unmarshal(encoded, &record); err != nil {
		return domainidentity.User{}, fmt.Errorf("failed to decode identity user: %w", err)
	}
	return record.Domain(), nil
}

func EncodeRole(role domainidentity.Role) ([]byte, error) {
	encoded, err := json.Marshal(RoleFromDomain(role))
	if err != nil {
		return nil, fmt.Errorf("failed to encode identity role: %w", err)
	}
	return encoded, nil
}

func DecodeRole(encoded []byte) (domainidentity.Role, error) {
	var document Role
	if err := json.Unmarshal(encoded, &document); err != nil {
		return domainidentity.Role{}, fmt.Errorf("failed to decode identity role: %w", err)
	}
	return document.Domain(), nil
}
