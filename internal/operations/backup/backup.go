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
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/backend/internal/persist"
)

const FormatVersion = 3

type Document struct {
	FormatVersion  int                  `json:"format_version"`
	CreatedAt      time.Time            `json:"created_at"`
	Manifest       persist.Manifest     `json:"manifest"`
	ManifestDigest string               `json:"manifest_digest"`
	Entries        []persist.Entry      `json:"entries"`
	Revisions      []persist.Revision   `json:"revisions"`
	Audit          []persist.Event      `json:"audit"`
	Taxonomies     []persist.Definition `json:"taxonomies"`
	TaxonomyTerms  []persist.Term       `json:"taxonomy_terms"`
	Users          []persist.UserRecord `json:"users"`
	Roles          []persist.Role       `json:"roles"`
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
		Manifest: persist.ManifestFromDomain(service.manifest), ManifestDigest: digest,
	}
	err = service.storage.WithinBackupTransaction(ctx, func(transaction Transaction) error {
		entries, err := allEntries(ctx, transaction.Content())
		if err != nil {
			return err
		}
		for _, entry := range entries {
			document.Entries = append(document.Entries, persist.EntryFromDomain(entry))
			items, err := allRevisions(ctx, transaction.Revisions(), entry.ID)
			if err != nil {
				return err
			}
			for _, item := range items {
				document.Revisions = append(document.Revisions, persist.RevisionFromDomain(item))
			}
		}
		events, err := allAudit(ctx, transaction.Audit())
		if err != nil {
			return err
		}
		for _, event := range events {
			document.Audit = append(document.Audit, persist.EventFromDomain(event))
		}
		definitions, err := transaction.Taxonomies().ListDefinitions(ctx)
		if err != nil {
			return err
		}
		for _, definition := range definitions {
			document.Taxonomies = append(document.Taxonomies, persist.DefinitionFromDomain(definition))
			terms, err := transaction.Taxonomies().ListTerms(ctx, definition.ID)
			if err != nil {
				return err
			}
			for _, term := range terms {
				document.TaxonomyTerms = append(document.TaxonomyTerms, persist.TermFromDomain(term))
			}
		}
		users, err := transaction.Identity().ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, user := range users {
			document.Users = append(document.Users, persist.UserFromDomain(user))
		}
		roles, err := transaction.Identity().ListRoles(ctx)
		if err != nil {
			return err
		}
		for _, role := range roles {
			document.Roles = append(document.Roles, persist.RoleFromDomain(role))
		}
		return nil
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
		return fmt.Errorf("failed to decode backup: %w", err)
	}
	if document.FormatVersion != FormatVersion {
		return errors.New("backup format version is unsupported")
	}
	digest, err := document.Manifest.Domain().Digest()
	if err != nil || digest != document.ManifestDigest {
		return errors.New("backup manifest digest is invalid")
	}
	currentDigest, err := service.manifest.Digest()
	if err != nil || currentDigest != digest {
		return errors.New("backup manifest does not match runtime manifest")
	}
	for _, encoded := range document.Entries {
		if err := encoded.Domain().Validate(); err != nil {
			return fmt.Errorf("failed to validate backup content: %w", err)
		}
	}
	for _, encoded := range document.Revisions {
		if err := encoded.Domain().Validate(); err != nil {
			return fmt.Errorf("failed to validate backup revision: %w", err)
		}
	}
	for _, encoded := range document.Audit {
		if err := encoded.Domain().Validate(); err != nil {
			return fmt.Errorf("failed to validate backup audit: %w", err)
		}
	}
	termsByTaxonomy := make(map[string][]taxonomy.Term)
	for _, encoded := range document.TaxonomyTerms {
		term := encoded.Domain()
		termsByTaxonomy[term.TaxonomyID] = append(termsByTaxonomy[term.TaxonomyID], term)
	}
	for _, encoded := range document.Taxonomies {
		definition := encoded.Domain()
		if err := taxonomy.ValidateHierarchy(definition, termsByTaxonomy[definition.ID]); err != nil {
			return fmt.Errorf("failed to validate backup taxonomy: %w", err)
		}
		delete(termsByTaxonomy, definition.ID)
	}
	if len(termsByTaxonomy) != 0 {
		return errors.New("backup contains terms without taxonomy definitions")
	}
	roleIDs := make(map[string]struct{}, len(document.Roles))
	for _, encoded := range document.Roles {
		role := encoded.Domain()
		if err := role.Validate(); err != nil {
			return fmt.Errorf("failed to validate backup role: %w", err)
		}
		roleIDs[role.ID] = struct{}{}
	}
	for _, encoded := range document.Users {
		user := encoded.Domain()
		if err := user.Validate(); err != nil {
			return fmt.Errorf("failed to validate backup user: %w", err)
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
		for _, encoded := range document.Roles {
			if err := transaction.Identity().SaveRole(ctx, encoded.Domain(), 0); err != nil {
				return err
			}
		}
		for _, encoded := range document.Users {
			if err := transaction.Identity().SaveUser(ctx, encoded.Domain(), 0); err != nil {
				return err
			}
		}
		for _, encoded := range document.Taxonomies {
			if err := transaction.Taxonomies().SaveDefinition(ctx, encoded.Domain(), 0); err != nil {
				return err
			}
		}
		for _, encoded := range document.TaxonomyTerms {
			if err := transaction.Taxonomies().SaveTerm(ctx, encoded.Domain(), 0); err != nil {
				return err
			}
		}
		for _, encoded := range document.Entries {
			if err := transaction.Content().Create(ctx, encoded.Domain()); err != nil {
				return err
			}
		}
		for _, encoded := range document.Revisions {
			if err := transaction.Revisions().Save(ctx, encoded.Domain()); err != nil {
				return err
			}
		}
		for _, encoded := range document.Audit {
			if err := transaction.Audit().Save(ctx, encoded.Domain()); err != nil {
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
