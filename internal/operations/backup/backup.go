package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/content"
	domainidentity "github.com/fastygo/backend/internal/domain/identity"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/domain/taxonomy"
)

const FormatVersion = 3

type IdentityUser struct {
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

func identityUser(user domainidentity.User) IdentityUser {
	return IdentityUser{
		ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		PasswordHash: user.PasswordHash, RoleIDs: user.RoleIDs, Active: user.Active,
		Version: user.Version, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func (user IdentityUser) domain() domainidentity.User {
	return domainidentity.User{
		ID: user.ID, Email: user.Email, DisplayName: user.DisplayName,
		PasswordHash: user.PasswordHash, RoleIDs: user.RoleIDs, Active: user.Active,
		Version: user.Version, CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

type Document struct {
	FormatVersion  int                   `json:"format_version"`
	CreatedAt      time.Time             `json:"created_at"`
	Manifest       schema.Manifest       `json:"manifest"`
	ManifestDigest string                `json:"manifest_digest"`
	Entries        []content.Entry       `json:"entries"`
	Revisions      []revision.Revision   `json:"revisions"`
	Audit          []audit.Event         `json:"audit"`
	Taxonomies     []taxonomy.Definition `json:"taxonomies"`
	TaxonomyTerms  []taxonomy.Term       `json:"taxonomy_terms"`
	Users          []IdentityUser        `json:"users"`
	Roles          []domainidentity.Role `json:"roles"`
}

type Service struct {
	storage  Transactor
	manifest schema.Manifest
	now      func() time.Time
}

func New(storage Transactor, manifest schema.Manifest) (*Service, error) {
	if storage == nil {
		return nil, errors.New("backup storage is required")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &Service{storage: storage, manifest: manifest, now: time.Now}, nil
}

func (service *Service) Export(ctx context.Context, destination io.Writer) error {
	if destination == nil {
		return errors.New("backup destination is required")
	}
	digest, err := service.manifest.Digest()
	if err != nil {
		return err
	}
	document := Document{
		FormatVersion: FormatVersion, CreatedAt: service.now().UTC(),
		Manifest: service.manifest, ManifestDigest: digest,
	}
	err = service.storage.WithinBackupTransaction(ctx, func(transaction Transaction) error {
		entries, err := allEntries(ctx, transaction.Content())
		if err != nil {
			return err
		}
		document.Entries = entries
		for _, entry := range entries {
			items, err := allRevisions(ctx, transaction.Revisions(), entry.ID)
			if err != nil {
				return err
			}
			document.Revisions = append(document.Revisions, items...)
		}
		document.Audit, err = allAudit(ctx, transaction.Audit())
		if err != nil {
			return err
		}
		document.Taxonomies, err = transaction.Taxonomies().ListDefinitions(ctx)
		if err != nil {
			return err
		}
		for _, definition := range document.Taxonomies {
			terms, err := transaction.Taxonomies().ListTerms(ctx, definition.ID)
			if err != nil {
				return err
			}
			document.TaxonomyTerms = append(document.TaxonomyTerms, terms...)
		}
		users, err := transaction.Identity().ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, user := range users {
			document.Users = append(document.Users, identityUser(user))
		}
		document.Roles, err = transaction.Identity().ListRoles(ctx)
		return err
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(document)
}

func (service *Service) Restore(ctx context.Context, source io.Reader) error {
	if source == nil {
		return errors.New("backup source is required")
	}
	var document Document
	decoder := json.NewDecoder(source)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode backup: %w", err)
	}
	if document.FormatVersion != FormatVersion {
		return errors.New("backup format version is unsupported")
	}
	digest, err := document.Manifest.Digest()
	if err != nil || digest != document.ManifestDigest {
		return errors.New("backup manifest digest is invalid")
	}
	currentDigest, err := service.manifest.Digest()
	if err != nil || currentDigest != digest {
		return errors.New("backup manifest does not match runtime manifest")
	}
	for _, entry := range document.Entries {
		if err := entry.Validate(); err != nil {
			return fmt.Errorf("backup content is invalid: %w", err)
		}
	}
	for _, item := range document.Revisions {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("backup revision is invalid: %w", err)
		}
	}
	for _, event := range document.Audit {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("backup audit is invalid: %w", err)
		}
	}
	termsByTaxonomy := make(map[string][]taxonomy.Term)
	for _, term := range document.TaxonomyTerms {
		termsByTaxonomy[term.TaxonomyID] = append(termsByTaxonomy[term.TaxonomyID], term)
	}
	for _, definition := range document.Taxonomies {
		if err := taxonomy.ValidateHierarchy(definition, termsByTaxonomy[definition.ID]); err != nil {
			return fmt.Errorf("backup taxonomy is invalid: %w", err)
		}
		delete(termsByTaxonomy, definition.ID)
	}
	if len(termsByTaxonomy) != 0 {
		return errors.New("backup contains terms without taxonomy definitions")
	}
	roleIDs := make(map[string]struct{}, len(document.Roles))
	for _, role := range document.Roles {
		if err := role.Validate(); err != nil {
			return fmt.Errorf("backup role is invalid: %w", err)
		}
		roleIDs[role.ID] = struct{}{}
	}
	for _, encoded := range document.Users {
		user := encoded.domain()
		if err := user.Validate(); err != nil {
			return fmt.Errorf("backup user is invalid: %w", err)
		}
		for _, roleID := range user.RoleIDs {
			if _, exists := roleIDs[roleID]; !exists {
				return errors.New("backup user references a missing role")
			}
		}
	}
	return service.storage.WithinBackupTransaction(ctx, func(transaction Transaction) error {
		existing, err := transaction.Content().List(ctx, application.Query{Page: 1, PerPage: 1})
		if err != nil {
			return err
		}
		if existing.Page.Total != 0 {
			return errors.New("restore requires an empty content store")
		}
		existingTaxonomies, err := transaction.Taxonomies().ListDefinitions(ctx)
		if err != nil {
			return err
		}
		if len(existingTaxonomies) != 0 {
			return errors.New("restore requires an empty taxonomy store")
		}
		existingUsers, err := transaction.Identity().ListUsers(ctx)
		if err != nil {
			return err
		}
		existingRoles, err := transaction.Identity().ListRoles(ctx)
		if err != nil {
			return err
		}
		if len(existingUsers) != 0 || len(existingRoles) != 0 {
			return errors.New("restore requires an empty identity store")
		}
		for _, role := range document.Roles {
			if err := transaction.Identity().SaveRole(ctx, role, 0); err != nil {
				return err
			}
		}
		for _, encoded := range document.Users {
			if err := transaction.Identity().SaveUser(ctx, encoded.domain(), 0); err != nil {
				return err
			}
		}
		for _, definition := range document.Taxonomies {
			if err := transaction.Taxonomies().SaveDefinition(ctx, definition, 0); err != nil {
				return err
			}
		}
		for _, term := range document.TaxonomyTerms {
			if err := transaction.Taxonomies().SaveTerm(ctx, term, 0); err != nil {
				return err
			}
		}
		for _, entry := range document.Entries {
			if err := transaction.Content().Create(ctx, entry); err != nil {
				return err
			}
		}
		for _, item := range document.Revisions {
			if err := transaction.Revisions().Save(ctx, item); err != nil {
				return err
			}
		}
		for _, event := range document.Audit {
			if err := transaction.Audit().Save(ctx, event); err != nil {
				return err
			}
		}
		return nil
	})
}

func allEntries(ctx context.Context, repository ContentRepository) ([]content.Entry, error) {
	var entries []content.Entry
	for page := 1; ; page++ {
		result, err := repository.List(ctx, application.Query{Page: page, PerPage: 100})
		if err != nil {
			return nil, err
		}
		entries = append(entries, result.Entries...)
		if page >= result.Page.TotalPages {
			return entries, nil
		}
	}
}

func allRevisions(
	ctx context.Context,
	repository RevisionRepository,
	entryID content.ID,
) ([]revision.Revision, error) {
	var revisions []revision.Revision
	for page := 1; ; page++ {
		items, pagination, err := repository.List(ctx, entryID, page, 100)
		if err != nil {
			return nil, err
		}
		revisions = append(revisions, items...)
		if page >= pagination.TotalPages {
			return revisions, nil
		}
	}
}

func allAudit(ctx context.Context, repository AuditRepository) ([]audit.Event, error) {
	var events []audit.Event
	for page := 1; ; page++ {
		items, pagination, err := repository.List(ctx, application.AuditQuery{Page: page, PerPage: 100})
		if err != nil {
			return nil, err
		}
		events = append(events, items...)
		if page >= pagination.TotalPages {
			return events, nil
		}
	}
}
