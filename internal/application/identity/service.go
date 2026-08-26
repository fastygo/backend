package identity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/authz"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/framework/pkg/core"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type TokenIssuer interface {
	Issue(authz.Principal, time.Duration) (string, error)
}

type Service struct {
	transactor Transactor
	tokens     TokenIssuer
	now        func() time.Time
}

type UserInput struct {
	ID          string
	Email       string
	DisplayName string
	Password    string
	RoleIDs     []string
	Active      bool
}

func NewService(
	transactor Transactor,
	tokens TokenIssuer,
	clock Clock,
) (*Service, error) {
	if transactor == nil {
		return nil, errors.New("identity transactor is required")
	}
	now := time.Now
	if clock != nil {
		now = clock.Now
	}
	return &Service{transactor: transactor, tokens: tokens, now: now}, nil
}

func (service *Service) Initialize(
	ctx context.Context,
	adminEmail string,
	adminPassword string,
) error {
	var hasUsers bool
	if err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		users, err := transaction.Identity().ListUsers(ctx)
		hasUsers = len(users) > 0
		return err
	}); err != nil {
		return err
	}
	var bootstrapHash string
	if !hasUsers && strings.TrimSpace(adminEmail) != "" {
		if strings.TrimSpace(adminPassword) == "" {
			return errors.New("bootstrap admin password is required when admin email is configured")
		}
		if len(adminPassword) < 12 {
			return errors.New("bootstrap admin password must contain at least 12 characters")
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		bootstrapHash = string(hash)
	}
	return service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		repository := transaction.Identity()
		for _, builtin := range []authz.Role{authz.AdministratorRole(), authz.EditorRole()} {
			if _, err := repository.GetRole(ctx, builtin.ID); err == nil {
				continue
			}
			role := domainidentity.Role{
				ID: builtin.ID, Label: builtin.Label,
				Capabilities: builtin.Capabilities, Version: 1,
			}
			if err := repository.SaveRole(ctx, role, 0); err != nil {
				return err
			}
		}
		users, err := repository.ListUsers(ctx)
		if err != nil || len(users) > 0 || strings.TrimSpace(adminEmail) == "" {
			return err
		}
		now := service.now().UTC()
		user := domainidentity.User{
			ID: uuid.NewString(), Email: adminEmail, DisplayName: "Administrator",
			PasswordHash: bootstrapHash, RoleIDs: []string{"administrator"}, Active: true,
			Version: 1, CreatedAt: now, UpdatedAt: now,
		}
		user.Normalize()
		if err := user.Validate(); err != nil {
			return err
		}
		return repository.SaveUser(ctx, user, 0)
	})
}

func (service *Service) Authenticate(
	ctx context.Context,
	email string,
	password string,
	ttl time.Duration,
) (string, error) {
	session, err := service.SignIn(ctx, email, password, ttl)
	if err != nil {
		return "", err
	}
	return session.Token, nil
}

type Session struct {
	User  domainidentity.User
	Token string
}

func (service *Service) SignIn(
	ctx context.Context,
	email string,
	password string,
	ttl time.Duration,
) (Session, error) {
	if service.tokens == nil {
		return Session{}, core.NewDomainError(core.ErrorCodeInternal, "token issuer is unavailable")
	}
	var user domainidentity.User
	var capabilities []authz.Capability
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		var err error
		user, err = transaction.Identity().GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(email)))
		if err != nil || !user.Active {
			return core.NewDomainError(core.ErrorCodeUnauthorized, "invalid credentials")
		}
		for _, roleID := range user.RoleIDs {
			role, err := transaction.Identity().GetRole(ctx, roleID)
			if err != nil {
				return core.WrapDomainError(core.ErrorCodeInternal, "user role could not be resolved", err)
			}
			capabilities = append(capabilities, role.Capabilities...)
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)) != nil {
		return Session{}, core.NewDomainError(core.ErrorCodeUnauthorized, "invalid credentials")
	}
	principal := authz.NewPrincipal(user.ID, capabilities...)
	token, err := service.tokens.Issue(principal, ttl)
	if err != nil {
		return Session{}, core.WrapDomainError(core.ErrorCodeInternal, "token issue failed", err)
	}
	return Session{User: user, Token: token}, nil
}

func (service *Service) CurrentUser(ctx context.Context, principal authz.Principal) (domainidentity.User, error) {
	if principal.Anonymous || strings.TrimSpace(principal.ID) == "" {
		return domainidentity.User{}, core.NewDomainError(core.ErrorCodeUnauthorized, "authentication is required")
	}
	var user domainidentity.User
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		resolved, err := transaction.Identity().GetUser(ctx, principal.ID)
		if err != nil || !resolved.Active {
			return core.NewDomainError(core.ErrorCodeUnauthorized, "authentication is required")
		}
		user = resolved
		return nil
	})
	return user, err
}

func (service *Service) ListUsers(
	ctx context.Context,
	principal authz.Principal,
) ([]domainidentity.User, error) {
	if !principal.Has(authz.CapabilityUsersView) {
		return nil, core.NewDomainError(core.ErrorCodeForbidden, "users.view is required")
	}
	var users []domainidentity.User
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		var err error
		users, err = transaction.Identity().ListUsers(ctx)
		return err
	})
	return users, err
}

func (service *Service) SaveUser(
	ctx context.Context,
	principal authz.Principal,
	input UserInput,
	expectedVersion uint64,
) (domainidentity.User, error) {
	required := authz.CapabilityUsersCreate
	if expectedVersion > 0 {
		required = authz.CapabilityUsersEdit
	}
	if !principal.Has(required) {
		return domainidentity.User{}, core.NewDomainError(core.ErrorCodeForbidden, string(required)+" is required")
	}
	passwordHash := ""
	if input.Password != "" {
		if len(input.Password) < 12 {
			return domainidentity.User{}, core.NewDomainError(
				core.ErrorCodeValidation,
				"password must contain at least 12 characters",
			)
		}
		encoded, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return domainidentity.User{}, err
		}
		passwordHash = string(encoded)
	}
	var saved domainidentity.User
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		repository := transaction.Identity()
		now := service.now().UTC()
		hash := ""
		createdAt := now
		id := strings.TrimSpace(input.ID)
		if expectedVersion > 0 {
			current, err := repository.GetUser(ctx, id)
			if err != nil {
				return core.WrapDomainError(core.ErrorCodeNotFound, "user was not found", err)
			}
			if current.Version != expectedVersion {
				return core.NewDomainError(core.ErrorCodeConflict, "user version conflict")
			}
			hash, createdAt = current.PasswordHash, current.CreatedAt
		} else if id == "" {
			id = uuid.NewString()
		}
		if passwordHash != "" {
			hash = passwordHash
		}
		for _, roleID := range input.RoleIDs {
			if _, err := repository.GetRole(ctx, roleID); err != nil {
				return core.WrapDomainError(core.ErrorCodeValidation, "user role does not exist", err)
			}
		}
		saved = domainidentity.User{
			ID: id, Email: input.Email, DisplayName: input.DisplayName,
			PasswordHash: hash, RoleIDs: input.RoleIDs, Active: input.Active,
			Version: expectedVersion + 1, CreatedAt: createdAt, UpdatedAt: now,
		}
		saved.Normalize()
		if err := saved.Validate(); err != nil {
			return core.WrapDomainError(core.ErrorCodeValidation, "user is invalid", err)
		}
		if err := repository.SaveUser(ctx, saved, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "user save failed", err)
		}
		return service.saveAudit(ctx, transaction.Audit(), principal.ID, "identity.user.save", saved.ID, expectedVersion, saved.Version)
	})
	return saved, err
}

func (service *Service) DeleteUser(
	ctx context.Context,
	principal authz.Principal,
	id string,
	expectedVersion uint64,
) error {
	if !principal.Has(authz.CapabilityUsersDelete) {
		return core.NewDomainError(core.ErrorCodeForbidden, "users.delete is required")
	}
	if principal.ID == id {
		return core.NewDomainError(core.ErrorCodeConflict, "current user cannot delete itself")
	}
	return service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		if err := transaction.Identity().DeleteUser(ctx, id, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "user delete failed", err)
		}
		return service.saveAudit(ctx, transaction.Audit(), principal.ID, "identity.user.delete", id, expectedVersion, 0)
	})
}

func (service *Service) ListRoles(
	ctx context.Context,
	principal authz.Principal,
) ([]domainidentity.Role, error) {
	if !principal.Has(authz.CapabilityRolesView) && !principal.Has(authz.CapabilityRolesManage) {
		return nil, core.NewDomainError(core.ErrorCodeForbidden, "roles.view is required")
	}
	var roles []domainidentity.Role
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		var err error
		roles, err = transaction.Identity().ListRoles(ctx)
		return err
	})
	return roles, err
}

func (service *Service) SaveRole(
	ctx context.Context,
	principal authz.Principal,
	role domainidentity.Role,
	expectedVersion uint64,
) (domainidentity.Role, error) {
	if !principal.Has(authz.CapabilityRolesManage) {
		return domainidentity.Role{}, core.NewDomainError(core.ErrorCodeForbidden, "roles.manage is required")
	}
	role.Version = expectedVersion + 1
	role.Normalize()
	if err := role.Validate(); err != nil {
		return domainidentity.Role{}, core.WrapDomainError(core.ErrorCodeValidation, "role is invalid", err)
	}
	err := service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		if err := transaction.Identity().SaveRole(ctx, role, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "role save failed", err)
		}
		return service.saveAudit(ctx, transaction.Audit(), principal.ID, "identity.role.save", role.ID, expectedVersion, role.Version)
	})
	return role, err
}

func (service *Service) DeleteRole(
	ctx context.Context,
	principal authz.Principal,
	id string,
	expectedVersion uint64,
) error {
	if !principal.Has(authz.CapabilityRolesManage) {
		return core.NewDomainError(core.ErrorCodeForbidden, "roles.manage is required")
	}
	if id == "administrator" || id == "editor" {
		return core.NewDomainError(core.ErrorCodeConflict, "built-in role cannot be deleted")
	}
	return service.transactor.WithinIdentityTransaction(ctx, func(transaction Transaction) error {
		users, err := transaction.Identity().ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, user := range users {
			for _, roleID := range user.RoleIDs {
				if roleID == id {
					return core.NewDomainError(core.ErrorCodeConflict, "role is assigned to a user")
				}
			}
		}
		if err := transaction.Identity().DeleteRole(ctx, id, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "role delete failed", err)
		}
		return service.saveAudit(ctx, transaction.Audit(), principal.ID, "identity.role.delete", id, expectedVersion, 0)
	})
}

func (service *Service) saveAudit(
	ctx context.Context,
	repository AuditRepository,
	actorID string,
	action string,
	resourceID string,
	before uint64,
	after uint64,
) error {
	id, err := repository.NextID(ctx)
	if err != nil {
		return err
	}
	event := audit.Event{
		ID: id, OccurredAt: service.now().UTC(), ActorID: actorID, Action: action,
		Resource: "identity", ResourceID: resourceID, BeforeVersion: before, AfterVersion: after,
	}
	return repository.Save(ctx, event)
}
