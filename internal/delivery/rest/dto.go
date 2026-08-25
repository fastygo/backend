package rest

import (
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	"github.com/fastygo/backend/internal/domain/audit"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/backend/internal/persist"
)

type paginationDocument struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type userWriteDocument struct {
	ID          string   `json:"id"`
	Email       string   `json:"email"`
	DisplayName string   `json:"display_name"`
	Password    string   `json:"password"`
	RoleIDs     []string `json:"role_ids"`
	Active      bool     `json:"active"`
}

func (document userWriteDocument) input() applicationidentity.UserInput {
	return applicationidentity.UserInput{
		ID: document.ID, Email: document.Email, DisplayName: document.DisplayName,
		Password: document.Password, RoleIDs: document.RoleIDs, Active: document.Active,
	}
}

type publicUser struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	RoleIDs     []string  `json:"role_ids"`
	Active      bool      `json:"active"`
	Version     uint64    `json:"version"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func projectPage(page application.Page) paginationDocument {
	return paginationDocument{
		Page: page.Number, PerPage: page.PerPage, Total: page.Total, TotalPages: page.TotalPages,
	}
}

func projectUser(user domainidentity.User) publicUser {
	return publicUser{
		ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		RoleIDs: user.RoleIDs, Active: user.Active, Version: user.Version,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func projectUsers(users []domainidentity.User) []publicUser {
	projected := make([]publicUser, 0, len(users))
	for _, user := range users {
		projected = append(projected, projectUser(user))
	}
	return projected
}

func projectRoles(roles []domainidentity.Role) []persist.Role {
	projected := make([]persist.Role, 0, len(roles))
	for _, role := range roles {
		projected = append(projected, persist.RoleFromDomain(role))
	}
	return projected
}

func projectRevisions(items []revision.Revision) []persist.Revision {
	projected := make([]persist.Revision, 0, len(items))
	for _, item := range items {
		projected = append(projected, persist.RevisionFromDomain(item))
	}
	return projected
}

func projectDefinitions(items []taxonomy.Definition) []persist.Definition {
	projected := make([]persist.Definition, 0, len(items))
	for _, item := range items {
		projected = append(projected, persist.DefinitionFromDomain(item))
	}
	return projected
}

func projectTerms(items []taxonomy.Term) []persist.Term {
	projected := make([]persist.Term, 0, len(items))
	for _, item := range items {
		projected = append(projected, persist.TermFromDomain(item))
	}
	return projected
}

func projectEvents(events []audit.Event) []persist.Event {
	projected := make([]persist.Event, 0, len(events))
	for _, event := range events {
		projected = append(projected, persist.EventFromDomain(event))
	}
	return projected
}
