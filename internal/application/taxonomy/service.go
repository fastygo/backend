package taxonomy

import (
	"context"
	"errors"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/authz"
	domaintaxonomy "github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/framework/pkg/core"
)

type Service struct {
	transactor application.Transactor
	now        func() time.Time
}

func NewService(transactor application.Transactor, clock application.Clock) (*Service, error) {
	if transactor == nil {
		return nil, errors.New("taxonomy transactor is required")
	}
	now := time.Now
	if clock != nil {
		now = clock.Now
	}
	return &Service{transactor: transactor, now: now}, nil
}

func (service *Service) ListDefinitions(
	ctx context.Context,
	principal authz.Principal,
) ([]domaintaxonomy.Definition, error) {
	var definitions []domaintaxonomy.Definition
	err := service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		items, err := transaction.Taxonomies().ListDefinitions(ctx)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy list failed", err)
		}
		for _, item := range items {
			if item.Public || principal.Has(authz.CapabilityTaxonomiesManage) {
				definitions = append(definitions, item)
			}
		}
		return nil
	})
	return definitions, err
}

func (service *Service) SaveDefinition(
	ctx context.Context,
	principal authz.Principal,
	definition domaintaxonomy.Definition,
	expectedVersion uint64,
) (domaintaxonomy.Definition, error) {
	if err := requireManage(principal); err != nil {
		return domaintaxonomy.Definition{}, err
	}
	definition.ID = strings.TrimSpace(definition.ID)
	definition.Version = expectedVersion + 1
	if err := definition.Validate(); err != nil {
		return domaintaxonomy.Definition{}, core.WrapDomainError(
			core.ErrorCodeValidation,
			"taxonomy is invalid",
			err,
		)
	}
	err := service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		if expectedVersion > 0 {
			current, err := transaction.Taxonomies().GetDefinition(ctx, definition.ID)
			if err != nil {
				return core.WrapDomainError(core.ErrorCodeNotFound, "taxonomy was not found", err)
			}
			if current.Version != expectedVersion {
				return core.NewDomainError(core.ErrorCodeConflict, "taxonomy version conflict")
			}
			terms, err := transaction.Taxonomies().ListTerms(ctx, definition.ID)
			if err != nil {
				return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy terms could not be loaded", err)
			}
			if err := domaintaxonomy.ValidateHierarchy(definition, terms); err != nil {
				return core.WrapDomainError(core.ErrorCodeValidation, "taxonomy update is invalid", err)
			}
		}
		if err := transaction.Taxonomies().SaveDefinition(ctx, definition, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "taxonomy save failed", err)
		}
		return saveAudit(ctx, transaction.Audit(), principal.ID, "taxonomy.save", definition.ID, expectedVersion, definition.Version, service.now())
	})
	return definition, err
}

func (service *Service) DeleteDefinition(
	ctx context.Context,
	principal authz.Principal,
	id string,
	expectedVersion uint64,
) error {
	if err := requireManage(principal); err != nil {
		return err
	}
	return service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		terms, err := transaction.Taxonomies().ListTerms(ctx, id)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy terms could not be loaded", err)
		}
		if len(terms) > 0 {
			return core.NewDomainError(core.ErrorCodeConflict, "taxonomy still contains terms")
		}
		if err := transaction.Taxonomies().DeleteDefinition(ctx, id, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "taxonomy delete failed", err)
		}
		return saveAudit(ctx, transaction.Audit(), principal.ID, "taxonomy.delete", id, expectedVersion, 0, service.now())
	})
}

func (service *Service) ListTerms(
	ctx context.Context,
	principal authz.Principal,
	taxonomyID string,
) ([]domaintaxonomy.Term, error) {
	var terms []domaintaxonomy.Term
	err := service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		definition, err := transaction.Taxonomies().GetDefinition(ctx, taxonomyID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "taxonomy was not found", err)
		}
		if !definition.Public && !principal.Has(authz.CapabilityTaxonomiesManage) {
			return core.NewDomainError(core.ErrorCodeNotFound, "taxonomy was not found")
		}
		terms, err = transaction.Taxonomies().ListTerms(ctx, taxonomyID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy term list failed", err)
		}
		return nil
	})
	return terms, err
}

func (service *Service) SaveTerm(
	ctx context.Context,
	principal authz.Principal,
	term domaintaxonomy.Term,
	expectedVersion uint64,
) (domaintaxonomy.Term, error) {
	if err := requireManage(principal); err != nil {
		return domaintaxonomy.Term{}, err
	}
	term.ID = domaintaxonomy.ID(strings.TrimSpace(string(term.ID)))
	term.TaxonomyID = strings.TrimSpace(term.TaxonomyID)
	term.Version = expectedVersion + 1
	err := service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		definition, err := transaction.Taxonomies().GetDefinition(ctx, term.TaxonomyID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "taxonomy was not found", err)
		}
		terms, err := transaction.Taxonomies().ListTerms(ctx, term.TaxonomyID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy terms could not be loaded", err)
		}
		replaced := false
		for index := range terms {
			if terms[index].ID == term.ID {
				if terms[index].Version != expectedVersion {
					return core.NewDomainError(core.ErrorCodeConflict, "taxonomy term version conflict")
				}
				terms[index] = term
				replaced = true
				break
			}
		}
		if expectedVersion > 0 && !replaced {
			return core.NewDomainError(core.ErrorCodeNotFound, "taxonomy term was not found")
		}
		if !replaced {
			terms = append(terms, term)
		}
		if err := domaintaxonomy.ValidateHierarchy(definition, terms); err != nil {
			return core.WrapDomainError(core.ErrorCodeValidation, "taxonomy term is invalid", err)
		}
		if err := transaction.Taxonomies().SaveTerm(ctx, term, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "taxonomy term save failed", err)
		}
		return saveAudit(ctx, transaction.Audit(), principal.ID, "taxonomy.term.save", string(term.ID), expectedVersion, term.Version, service.now())
	})
	return term, err
}

func (service *Service) DeleteTerm(
	ctx context.Context,
	principal authz.Principal,
	id domaintaxonomy.ID,
	expectedVersion uint64,
) error {
	if err := requireManage(principal); err != nil {
		return err
	}
	return service.transactor.WithinTransaction(ctx, func(transaction application.Transaction) error {
		term, err := transaction.Taxonomies().GetTerm(ctx, id)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "taxonomy term was not found", err)
		}
		terms, err := transaction.Taxonomies().ListTerms(ctx, term.TaxonomyID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy terms could not be loaded", err)
		}
		for _, candidate := range terms {
			if candidate.ParentID == id {
				return core.NewDomainError(core.ErrorCodeConflict, "taxonomy term still has children")
			}
		}
		used, err := termIsAssigned(ctx, transaction.Content(), term)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "taxonomy assignments could not be checked", err)
		}
		if used {
			return core.NewDomainError(core.ErrorCodeConflict, "taxonomy term is still assigned")
		}
		if err := transaction.Taxonomies().DeleteTerm(ctx, id, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "taxonomy term delete failed", err)
		}
		return saveAudit(ctx, transaction.Audit(), principal.ID, "taxonomy.term.delete", string(id), expectedVersion, 0, service.now())
	})
}

func requireManage(principal authz.Principal) error {
	if !principal.Has(authz.CapabilityTaxonomiesManage) {
		return core.NewDomainError(core.ErrorCodeForbidden, "taxonomies.manage is required")
	}
	return nil
}

func termIsAssigned(
	ctx context.Context,
	repository application.Repository,
	term domaintaxonomy.Term,
) (bool, error) {
	for page := 1; ; page++ {
		result, err := repository.List(ctx, application.Query{Page: page, PerPage: 100})
		if err != nil {
			return false, err
		}
		for _, entry := range result.Entries {
			for _, reference := range entry.Terms {
				if reference.Taxonomy == term.TaxonomyID && reference.TermID == string(term.ID) {
					return true, nil
				}
			}
		}
		if result.Page.TotalPages == 0 || page >= result.Page.TotalPages {
			return false, nil
		}
	}
}

func saveAudit(
	ctx context.Context,
	repository application.AuditRepository,
	actorID string,
	action string,
	resourceID string,
	beforeVersion uint64,
	afterVersion uint64,
	now time.Time,
) error {
	id, err := repository.NextID(ctx)
	if err != nil {
		return err
	}
	event := audit.Event{
		ID: id, OccurredAt: now.UTC(), ActorID: actorID, Action: action,
		Resource: "taxonomy", ResourceID: resourceID,
		BeforeVersion: beforeVersion, AfterVersion: afterVersion,
	}
	if err := event.Validate(); err != nil {
		return err
	}
	return repository.Save(ctx, event)
}
