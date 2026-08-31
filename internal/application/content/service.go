package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/application/forms"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/schema"
	domainTaxonomy "github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/framework/pkg/core"
	"github.com/google/uuid"
)

type Service struct {
	transactor Transactor
	hooks      Hooks
	now        func() time.Time
	manifest   *schema.Manifest
}

func (service *Service) SetManifest(manifest schema.Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	service.manifest = &manifest
	return nil
}

type Transition struct {
	Kind            domaincontent.Kind
	Status          domaincontent.Status
	PublishAt       *time.Time
	ExpectedVersion uint64
	Reason          string
}

func NewService(transactor Transactor, hooks Hooks, clock Clock) (*Service, error) {
	if transactor == nil {
		return nil, errors.New("content transactor is required")
	}
	now := time.Now
	if clock != nil {
		now = clock.Now
	}
	return &Service{transactor: transactor, hooks: hooks, now: now}, nil
}

func (service *Service) Get(ctx context.Context, principal authz.Principal, id domaincontent.ID) (domaincontent.Entry, error) {
	entry, err := service.load(ctx, id)
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if !entry.IsPublicAt(service.now().UTC()) && !principal.Has(authz.CapabilityContentReadPrivate) {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
	}
	if principal.Has(authz.CapabilityContentReadPrivate) {
		return entry, nil
	}
	return entry.PublicProjection(), nil
}

func (service *Service) GetBySlug(
	ctx context.Context,
	principal authz.Principal,
	kind domaincontent.Kind,
	locale string,
	slug string,
) (domaincontent.Entry, error) {
	var entry domaincontent.Entry
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, err := transaction.Content().GetBySlug(ctx, kind, locale, slug)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "content was not found", err)
		}
		entry = resolved
		return nil
	})
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if !entry.IsPublicAt(service.now().UTC()) && !principal.Has(authz.CapabilityContentReadPrivate) {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
	}
	if !principal.Has(authz.CapabilityContentReadPrivate) {
		entry = entry.PublicProjection()
	}
	return entry, nil
}

// GetAuthorized returns an unredacted aggregate after applying a feature-specific private capability.
func (service *Service) GetAuthorized(
	ctx context.Context,
	principal authz.Principal,
	id domaincontent.ID,
	privateCapability authz.Capability,
) (domaincontent.Entry, error) {
	entry, err := service.load(ctx, id)
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if !entry.IsPublicAt(service.now().UTC()) && !principal.Has(privateCapability) {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
	}
	return entry, nil
}

func (service *Service) load(ctx context.Context, id domaincontent.ID) (domaincontent.Entry, error) {
	var entry domaincontent.Entry
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, err := transaction.Content().Get(ctx, id)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "content was not found", err)
		}
		entry = resolved
		return nil
	})
	if err != nil {
		return domaincontent.Entry{}, err
	}
	return entry, nil
}

func (service *Service) List(ctx context.Context, principal authz.Principal, query Query) (ListResult, error) {
	if query.Page < 1 || query.PerPage < 1 || query.PerPage > 100 {
		return ListResult{}, core.NewDomainError(core.ErrorCodeValidation, "invalid pagination")
	}
	private := principal.Has(authz.CapabilityContentReadPrivate)
	if !private {
		query.PublicOnly = true
		query.PublicAt = service.now().UTC()
	}
	var result ListResult
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, err := transaction.Content().List(ctx, query)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "content list failed", err)
		}
		result = resolved
		return nil
	})
	if err != nil {
		return ListResult{}, err
	}
	if private {
		return result, nil
	}
	now := service.now().UTC()
	public := result.Entries[:0]
	for _, entry := range result.Entries {
		if entry.IsPublicAt(now) {
			public = append(public, entry.PublicProjection())
		}
	}
	result.Entries = public
	return result, nil
}

func (service *Service) ListAudit(
	ctx context.Context,
	principal authz.Principal,
	query AuditQuery,
) ([]audit.Event, Page, error) {
	if !principal.Has(authz.CapabilityAuditView) {
		return nil, Page{}, core.NewDomainError(core.ErrorCodeForbidden, "audit.view is required")
	}
	if query.Page < 1 || query.PerPage < 1 || query.PerPage > 100 {
		return nil, Page{}, core.NewDomainError(core.ErrorCodeValidation, "invalid pagination")
	}
	var events []audit.Event
	var page Page
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, resolvedPage, err := transaction.Audit().List(ctx, query)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "audit list failed", err)
		}
		events = resolved
		page = resolvedPage
		return nil
	})
	if err != nil {
		return nil, Page{}, err
	}
	return events, page, nil
}

func (service *Service) Create(ctx context.Context, principal authz.Principal, entry domaincontent.Entry) (domaincontent.Entry, error) {
	createCapability := authz.CapabilityContentCreate
	if entry.Kind == "media" {
		createCapability = authz.CapabilityMediaUpload
	}
	if !principal.Has(createCapability) {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeForbidden, string(createCapability)+" is required")
	}
	if err := authorizeStatus(principal, "", entry.Status); err != nil {
		return domaincontent.Entry{}, err
	}
	now := service.now().UTC()
	if strings.TrimSpace(string(entry.ID)) == "" {
		entry.ID = domaincontent.ID(uuid.NewString())
	}
	entry.AuthorID = strings.TrimSpace(entry.AuthorID)
	if entry.AuthorID == "" {
		entry.AuthorID = principal.ID
	}
	entry.Version = 1
	entry.CreatedAt = now
	entry.UpdatedAt = now
	normalizeEntrySlugs(&entry)
	if err := entry.Validate(); err != nil {
		return domaincontent.Entry{}, core.WrapDomainError(core.ErrorCodeValidation, "content is invalid", err)
	}
	event := LifecycleEvent{Name: "content.create", Principal: principal.ID, After: &entry}
	if err := service.before(ctx, event); err != nil {
		return domaincontent.Entry{}, err
	}
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		if err := validateTaxonomyAssignments(ctx, principal, transaction.Taxonomies(), entry); err != nil {
			return err
		}
		if err := service.validateManifest(ctx, transaction.Content(), &entry); err != nil {
			return err
		}
		if err := ensureUniqueSlugs(ctx, transaction.Content(), entry, ""); err != nil {
			return err
		}
		if err := transaction.Content().Create(ctx, entry); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "content create failed", err)
		}
		if err := saveAudit(ctx, transaction.Audit(), audit.Event{
			OccurredAt: entry.CreatedAt, ActorID: principal.ID, Action: event.Name,
			Resource: string(entry.Kind), ResourceID: string(entry.ID), AfterVersion: entry.Version,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if err := service.after(ctx, event); err != nil {
		return domaincontent.Entry{}, err
	}
	return entry, nil
}

func (service *Service) Update(
	ctx context.Context,
	principal authz.Principal,
	entry domaincontent.Entry,
	expectedVersion uint64,
	reason string,
) (domaincontent.Entry, error) {
	var before domaincontent.Entry
	var event LifecycleEvent
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		current, err := transaction.Content().Get(ctx, entry.ID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "content was not found", err)
		}
		if !principal.CanEdit(current.AuthorID) {
			return core.NewDomainError(core.ErrorCodeForbidden, "content edit capability is required")
		}
		if current.Version != expectedVersion {
			return core.NewDomainError(core.ErrorCodeConflict, "content version conflict")
		}
		if current.Kind != entry.Kind {
			return core.NewDomainError(core.ErrorCodeValidation, "content kind cannot change")
		}
		if current.Status != domaincontent.StatusTrashed && entry.Status == domaincontent.StatusTrashed {
			if err := service.applyDeletePolicies(
				ctx,
				transaction,
				principal,
				current,
				map[domaincontent.ID]struct{}{current.ID: {}},
			); err != nil {
				return err
			}
		}
		if err := authorizeStatus(principal, current.Status, entry.Status); err != nil {
			return err
		}
		before = current
		entry.AuthorID = current.AuthorID
		entry.CreatedAt = current.CreatedAt
		entry.UpdatedAt = service.now().UTC()
		entry.Version = current.Version + 1
		normalizeEntrySlugs(&entry)
		if err := entry.Validate(); err != nil {
			return core.WrapDomainError(core.ErrorCodeValidation, "content is invalid", err)
		}
		if err := validateTaxonomyAssignments(ctx, principal, transaction.Taxonomies(), entry); err != nil {
			return err
		}
		if err := service.validateManifest(ctx, transaction.Content(), &entry); err != nil {
			return err
		}
		if err := ensureUniqueSlugs(ctx, transaction.Content(), entry, entry.ID); err != nil {
			return err
		}
		event = LifecycleEvent{Name: lifecycleName(current.Status, entry.Status), Principal: principal.ID, Before: &before, After: &entry}
		if err := service.before(ctx, event); err != nil {
			return err
		}
		revisionID, err := transaction.Revisions().NextID(ctx)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "revision id allocation failed", err)
		}
		snapshot := revision.Revision{
			ID: revisionID, EntryID: current.ID, Version: current.Version, Snapshot: current,
			AuthorID: principal.ID, Reason: strings.TrimSpace(reason), CreatedAt: service.now().UTC(),
		}
		if err := snapshot.Validate(); err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "revision is invalid", err)
		}
		if err := transaction.Revisions().Save(ctx, snapshot); err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "revision save failed", err)
		}
		if err := transaction.Content().Update(ctx, entry, expectedVersion); err != nil {
			return core.WrapDomainError(core.ErrorCodeConflict, "content update failed", err)
		}
		if err := saveAudit(ctx, transaction.Audit(), audit.Event{
			OccurredAt: entry.UpdatedAt, ActorID: principal.ID, Action: event.Name,
			Resource: string(entry.Kind), ResourceID: string(entry.ID),
			BeforeVersion: current.Version, AfterVersion: entry.Version,
		}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if err := service.after(ctx, event); err != nil {
		return domaincontent.Entry{}, err
	}
	return entry, nil
}

func (service *Service) applyDeletePolicies(
	ctx context.Context,
	transaction Transaction,
	principal authz.Principal,
	target domaincontent.Entry,
	visiting map[domaincontent.ID]struct{},
) error {
	if service.manifest == nil {
		return nil
	}
	for _, resource := range service.manifest.Resources {
		for _, field := range resource.Fields {
			if field.Type != schema.FieldRelation || field.Relation == nil ||
				field.Relation.Resource != string(target.Kind) {
				continue
			}
			dependents, err := listRelatedContent(
				ctx,
				transaction.Content(),
				domaincontent.Kind(resource.ID),
				field.ID,
				target.ID,
			)
			if err != nil {
				return core.WrapDomainError(core.ErrorCodeInternal, "relation dependencies could not be loaded", err)
			}
			for _, dependent := range dependents {
				if dependent.ID == target.ID || dependent.Status == domaincontent.StatusTrashed {
					continue
				}
				original := dependent
				switch field.Relation.OnDelete {
				case schema.DeleteRestrict:
					return core.NewDomainError(
						core.ErrorCodeConflict,
						fmt.Sprintf("content is referenced by %s through field %q", dependent.ID, field.ID),
					)
				case schema.DeleteNullify:
					nullifyRelation(&dependent, field, string(target.ID))
				case schema.DeleteCascade:
					if _, cycle := visiting[dependent.ID]; cycle {
						return core.NewDomainError(core.ErrorCodeConflict, "cascade relation contains a cycle")
					}
					visiting[dependent.ID] = struct{}{}
					if err := service.applyDeletePolicies(ctx, transaction, principal, dependent, visiting); err != nil {
						return err
					}
					delete(visiting, dependent.ID)
					dependent.Status = domaincontent.StatusTrashed
					dependent.DeletedAt = timePointer(service.now().UTC())
				default:
					continue
				}
				before := original
				dependent.Version++
				dependent.UpdatedAt = service.now().UTC()
				if err := service.validateManifest(ctx, transaction.Content(), &dependent); err != nil {
					return err
				}
				revisionID, err := transaction.Revisions().NextID(ctx)
				if err != nil {
					return core.WrapDomainError(core.ErrorCodeInternal, "relation revision id allocation failed", err)
				}
				snapshot := revision.Revision{
					ID: revisionID, EntryID: before.ID, Version: before.Version, Snapshot: before,
					AuthorID: principal.ID, Reason: "relation on_delete " + string(field.Relation.OnDelete),
					CreatedAt: service.now().UTC(),
				}
				if err := transaction.Revisions().Save(ctx, snapshot); err != nil {
					return core.WrapDomainError(core.ErrorCodeInternal, "relation revision save failed", err)
				}
				if err := transaction.Content().Update(ctx, dependent, before.Version); err != nil {
					return core.WrapDomainError(core.ErrorCodeConflict, "relation policy update failed", err)
				}
				if err := saveAudit(ctx, transaction.Audit(), audit.Event{
					OccurredAt: dependent.UpdatedAt, ActorID: principal.ID,
					Action: "content.relation_" + string(field.Relation.OnDelete), Resource: string(dependent.Kind),
					ResourceID: string(dependent.ID), BeforeVersion: before.Version, AfterVersion: dependent.Version,
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func listRelatedContent(
	ctx context.Context,
	repository Repository,
	kind domaincontent.Kind,
	fieldID string,
	targetID domaincontent.ID,
) ([]domaincontent.Entry, error) {
	var entries []domaincontent.Entry
	for page := 1; ; page++ {
		result, err := repository.List(ctx, Query{
			Kinds: []domaincontent.Kind{kind}, RelationField: fieldID, RelatedID: targetID,
			Page: page, PerPage: 100,
		})
		if err != nil {
			return nil, err
		}
		entries = append(entries, result.Entries...)
		if result.Page.TotalPages == 0 || page >= result.Page.TotalPages {
			return entries, nil
		}
	}
}

func relationContains(field schema.Field, value any, targetID string) bool {
	if field.Relation == nil {
		return false
	}
	if field.Relation.Cardinality == schema.CardinalityOne {
		id := asString(value)
		return id.ok && id.value == targetID
	}
	switch values := value.(type) {
	case []string:
		for _, id := range values {
			if strings.TrimSpace(id) == targetID {
				return true
			}
		}
	case []any:
		for _, item := range values {
			id := asString(item)
			if id.ok && id.value == targetID {
				return true
			}
		}
	}
	return false
}

func nullifyRelation(entry *domaincontent.Entry, field schema.Field, targetID string) {
	if field.Relation == nil || entry.Metadata == nil {
		return
	}
	if field.Relation.Cardinality == schema.CardinalityOne {
		delete(entry.Metadata, field.ID)
		return
	}
	metadata := entry.Metadata[field.ID]
	switch values := metadata.Value.(type) {
	case []string:
		filtered := make([]string, 0, len(values))
		for _, id := range values {
			if strings.TrimSpace(id) != targetID {
				filtered = append(filtered, id)
			}
		}
		metadata.Value = filtered
	case []any:
		filtered := make([]any, 0, len(values))
		for _, item := range values {
			id := asString(item)
			if !id.ok || id.value != targetID {
				filtered = append(filtered, item)
			}
		}
		metadata.Value = filtered
	}
	entry.Metadata[field.ID] = metadata
}

func (service *Service) validateManifest(
	ctx context.Context,
	repository Repository,
	entry *domaincontent.Entry,
) error {
	if service.manifest == nil {
		return nil
	}
	entry.LiftLocaleMetadata()
	var resource *schema.Resource
	for index := range service.manifest.Resources {
		if service.manifest.Resources[index].ID == string(entry.Kind) {
			resource = &service.manifest.Resources[index]
			break
		}
	}
	if resource == nil {
		if entry.Kind == "media" {
			return nil
		}
		return core.NewDomainError(core.ErrorCodeValidation, "content kind is not declared in the manifest")
	}
	allowedTaxonomies := make(map[string]struct{}, len(resource.Taxonomies))
	for _, id := range resource.Taxonomies {
		allowedTaxonomies[id] = struct{}{}
	}
	for _, reference := range entry.Terms {
		if _, allowed := allowedTaxonomies[reference.Taxonomy]; !allowed {
			return core.NewDomainError(core.ErrorCodeValidation, "taxonomy is not declared for this resource")
		}
	}
	for _, field := range resource.Fields {
		value, present := entry.Metadata[field.ID]
		if field.Required && (!present || value.Value == nil) {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("required field %q is missing", field.ID),
			)
		}
		if !present {
			continue
		}
		if value.Value == nil {
			if field.Nullable {
				continue
			}
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("field %q cannot be null", field.ID),
			)
		}
		if field.ReadOnly {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("read-only field %q cannot be written", field.ID),
			)
		}
		if field.Sensitive && !value.Private {
			value.Private = true
			entry.Metadata[field.ID] = value
		}
		if field.Type == schema.FieldRelation {
			if err := validateRelationValue(ctx, repository, field, value.Value); err != nil {
				return err
			}
		}
		if field.Type == schema.FieldMedia {
			if err := validateRelationIDs(ctx, repository, field.ID, "media", []stringValue{asString(value.Value)}); err != nil {
				return err
			}
		}
		if field.Type != schema.FieldRelation && field.Type != schema.FieldMedia {
			if err := validateFieldValue(field, value.Value); err != nil {
				return err
			}
		}
	}
	declared := make(map[string]struct{}, len(resource.Fields))
	for _, field := range resource.Fields {
		declared[field.ID] = struct{}{}
	}
	for id := range entry.Metadata {
		if strings.HasPrefix(id, "payload_") {
			continue
		}
		if _, exists := declared[id]; !exists {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("field %q is not declared in the manifest", id),
			)
		}
	}
	return forms.ValidateEntry(*resource, *entry)
}

type stringValue struct {
	value string
	ok    bool
}

func asString(value any) stringValue {
	resolved, ok := value.(string)
	return stringValue{value: strings.TrimSpace(resolved), ok: ok && strings.TrimSpace(resolved) != ""}
}

func metadataReferences(value any, targetID string) bool {
	field := schema.Field{Relation: &schema.Relation{Cardinality: schema.CardinalityOne}}
	if relationContains(field, value, targetID) {
		return true
	}
	field.Relation.Cardinality = schema.CardinalityMany
	return relationContains(field, value, targetID)
}

func validateRelationValue(
	ctx context.Context,
	repository Repository,
	field schema.Field,
	value any,
) error {
	if field.Relation == nil {
		return core.NewDomainError(core.ErrorCodeInternal, "relation metadata is unavailable")
	}
	var ids []stringValue
	switch field.Relation.Cardinality {
	case schema.CardinalityOne:
		ids = []stringValue{asString(value)}
	case schema.CardinalityMany:
		switch values := value.(type) {
		case []string:
			ids = make([]stringValue, 0, len(values))
			for _, item := range values {
				ids = append(ids, asString(item))
			}
		case []any:
			ids = make([]stringValue, 0, len(values))
			for _, item := range values {
				ids = append(ids, asString(item))
			}
		default:
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("relation field %q must be an array of ids", field.ID),
			)
		}
	}
	return validateRelationIDs(ctx, repository, field.ID, field.Relation.Resource, ids)
}

func validateFieldValue(field schema.Field, value any) error {
	if field.Localized {
		values, ok := value.(map[string]any)
		if !ok || len(values) == 0 {
			return invalidField(field.ID, "must be a non-empty locale map")
		}
		copyField := field
		copyField.Localized = false
		for locale, localized := range values {
			if strings.TrimSpace(locale) == "" {
				return invalidField(field.ID, "contains an invalid locale")
			}
			if err := validateFieldValue(copyField, localized); err != nil {
				return err
			}
		}
		return nil
	}
	switch field.Type {
	case schema.FieldString, schema.FieldText, schema.FieldRichText, schema.FieldMarkdown:
		if _, ok := value.(string); !ok {
			return invalidField(field.ID, "must be a string")
		}
	case schema.FieldBoolean:
		if _, ok := value.(bool); !ok {
			return invalidField(field.ID, "must be a boolean")
		}
	case schema.FieldInteger:
		number, ok := numericValue(value)
		if !ok || math.Trunc(number) != number {
			return invalidField(field.ID, "must be an integer")
		}
	case schema.FieldNumber:
		if _, ok := numericValue(value); !ok {
			return invalidField(field.ID, "must be a number")
		}
	case schema.FieldDecimal, schema.FieldMoney:
		if _, ok := decimalValue(value); !ok {
			return invalidField(field.ID, "must be a decimal number")
		}
	case schema.FieldDate:
		resolved, ok := value.(string)
		if !ok {
			return invalidField(field.ID, "must be an ISO date")
		}
		if _, err := time.Parse("2006-01-02", resolved); err != nil {
			return invalidField(field.ID, "must be an ISO date")
		}
	case schema.FieldDateTime:
		resolved, ok := value.(string)
		if !ok {
			return invalidField(field.ID, "must be an RFC3339 timestamp")
		}
		if _, err := time.Parse(time.RFC3339, resolved); err != nil {
			return invalidField(field.ID, "must be an RFC3339 timestamp")
		}
	case schema.FieldURI:
		resolved, ok := value.(string)
		if !ok {
			return invalidField(field.ID, "must be a URI")
		}
		parsed, err := url.ParseRequestURI(resolved)
		if err != nil || parsed.String() == "" {
			return invalidField(field.ID, "must be a URI")
		}
	case schema.FieldUUID:
		resolved, ok := value.(string)
		if !ok {
			return invalidField(field.ID, "must be a UUID")
		}
		if _, err := uuid.Parse(resolved); err != nil {
			return invalidField(field.ID, "must be a UUID")
		}
	case schema.FieldEnum:
		resolved, ok := value.(string)
		if !ok {
			return invalidField(field.ID, "must be an enum string")
		}
		found := false
		for _, allowed := range field.Enum {
			if resolved == allowed {
				found = true
				break
			}
		}
		if !found {
			return invalidField(field.ID, "contains an unsupported enum value")
		}
	case schema.FieldCollection:
		if field.Items == nil {
			return core.NewDomainError(core.ErrorCodeInternal, "collection item schema is unavailable")
		}
		switch values := value.(type) {
		case []any:
			for _, item := range values {
				if err := validateFieldValue(*field.Items, item); err != nil {
					return err
				}
			}
		case []string:
			for _, item := range values {
				if err := validateFieldValue(*field.Items, item); err != nil {
					return err
				}
			}
		default:
			return invalidField(field.ID, "must be an array")
		}
	case schema.FieldObject:
		document, ok := value.(map[string]any)
		if !ok {
			return invalidField(field.ID, "must be an object")
		}
		for _, nested := range field.Fields {
			item, exists := document[nested.ID]
			if !exists || item == nil {
				if nested.Required {
					return invalidField(nested.ID, "is required")
				}
				continue
			}
			if err := validateFieldValue(nested, item); err != nil {
				return err
			}
		}
	case schema.FieldJSON:
		// JSON values are already decoded into Go scalar, array, or object values.
	default:
		return invalidField(field.ID, "has unsupported runtime type")
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch resolved := value.(type) {
	case float64:
		return resolved, !math.IsInf(resolved, 0) && !math.IsNaN(resolved)
	case float32:
		return float64(resolved), true
	case int:
		return float64(resolved), true
	case int64:
		return float64(resolved), true
	case int32:
		return float64(resolved), true
	case json.Number:
		number, err := resolved.Float64()
		return number, err == nil
	default:
		return 0, false
	}
}

func decimalValue(value any) (float64, bool) {
	if number, ok := numericValue(value); ok {
		return number, true
	}
	resolved, ok := value.(string)
	if !ok || strings.TrimSpace(resolved) == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(resolved, 64)
	return number, err == nil && !math.IsInf(number, 0) && !math.IsNaN(number)
}

func invalidField(id string, reason string) error {
	return core.NewDomainError(
		core.ErrorCodeValidation,
		fmt.Sprintf("field %q %s", id, reason),
	)
}

func validateRelationIDs(
	ctx context.Context,
	repository Repository,
	fieldID string,
	targetKind string,
	ids []stringValue,
) error {
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !id.ok {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("relation field %q contains an invalid id", fieldID),
			)
		}
		if _, duplicate := seen[id.value]; duplicate {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("relation field %q contains a duplicate id", fieldID),
			)
		}
		seen[id.value] = struct{}{}
		target, err := repository.Get(ctx, domaincontent.ID(id.value))
		if err != nil {
			return core.WrapDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("relation field %q references missing content", fieldID),
				err,
			)
		}
		if string(target.Kind) != targetKind || target.Status == domaincontent.StatusTrashed {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("relation field %q references an invalid resource", fieldID),
			)
		}
	}
	return nil
}

func validateTaxonomyAssignments(
	ctx context.Context,
	principal authz.Principal,
	repository TaxonomyReader,
	entry domaincontent.Entry,
) error {
	if len(entry.Terms) == 0 {
		return nil
	}
	if !principal.Has(authz.CapabilityTaxonomiesAssign) {
		return core.NewDomainError(core.ErrorCodeForbidden, "taxonomies.assign is required")
	}
	if repository == nil {
		return core.NewDomainError(core.ErrorCodeInternal, "taxonomy repository is unavailable")
	}
	seen := make(map[string]struct{}, len(entry.Terms))
	for _, reference := range entry.Terms {
		key := strings.TrimSpace(reference.Taxonomy) + "\x00" + strings.TrimSpace(reference.TermID)
		if _, duplicate := seen[key]; duplicate {
			return core.NewDomainError(core.ErrorCodeValidation, "taxonomy assignment is duplicated")
		}
		seen[key] = struct{}{}
		definition, err := repository.GetDefinition(ctx, strings.TrimSpace(reference.Taxonomy))
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeValidation, "taxonomy does not exist", err)
		}
		if !definition.Allows(entry.Kind) {
			return core.NewDomainError(core.ErrorCodeValidation, "taxonomy does not allow this content kind")
		}
		term, err := repository.GetTerm(ctx, domainTaxonomy.ID(strings.TrimSpace(reference.TermID)))
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeValidation, "taxonomy term does not exist", err)
		}
		if term.TaxonomyID != definition.ID {
			return core.NewDomainError(core.ErrorCodeValidation, "term belongs to another taxonomy")
		}
	}
	return nil
}

func (service *Service) Transition(
	ctx context.Context,
	principal authz.Principal,
	id domaincontent.ID,
	transition Transition,
) (domaincontent.Entry, error) {
	entry, err := service.load(ctx, id)
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if transition.Kind != "" && entry.Kind != transition.Kind {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
	}
	now := service.now().UTC()
	entry.Status = transition.Status
	switch transition.Status {
	case domaincontent.StatusDraft:
		entry.DeletedAt = nil
		entry.PublishedAt = nil
	case domaincontent.StatusScheduled:
		entry.DeletedAt = nil
		entry.PublishedAt = transition.PublishAt
	case domaincontent.StatusPublished:
		entry.DeletedAt = nil
		if transition.PublishAt != nil {
			entry.PublishedAt = transition.PublishAt
		} else if entry.PublishedAt == nil || entry.PublishedAt.After(now) {
			entry.PublishedAt = &now
		}
	case domaincontent.StatusArchived:
		entry.DeletedAt = nil
	case domaincontent.StatusTrashed:
		entry.DeletedAt = &now
	default:
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeValidation, "unsupported content status")
	}
	return service.Update(ctx, principal, entry, transition.ExpectedVersion, transition.Reason)
}

func (service *Service) Revisions(
	ctx context.Context,
	principal authz.Principal,
	entryID domaincontent.ID,
	expectedKind domaincontent.Kind,
	page int,
	perPage int,
) ([]revision.Revision, Page, error) {
	if !principal.Has(authz.CapabilityContentManageRevisions) {
		return nil, Page{}, core.NewDomainError(core.ErrorCodeForbidden, "content.manage_revisions is required")
	}
	if page < 1 || perPage < 1 || perPage > 100 {
		return nil, Page{}, core.NewDomainError(core.ErrorCodeValidation, "invalid pagination")
	}
	entry, err := service.load(ctx, entryID)
	if err != nil {
		return nil, Page{}, err
	}
	if expectedKind != "" && entry.Kind != expectedKind {
		return nil, Page{}, core.NewDomainError(core.ErrorCodeNotFound, "content was not found")
	}
	var items []revision.Revision
	var pagination Page
	err = service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, resolvedPage, err := transaction.Revisions().List(ctx, entryID, page, perPage)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "revision list failed", err)
		}
		items = resolved
		pagination = resolvedPage
		return nil
	})
	if err != nil {
		return nil, Page{}, err
	}
	return items, pagination, nil
}

func (service *Service) RestoreRevision(
	ctx context.Context,
	principal authz.Principal,
	entryID domaincontent.ID,
	expectedKind domaincontent.Kind,
	revisionID revision.ID,
	expectedVersion uint64,
) (domaincontent.Entry, error) {
	if !principal.Has(authz.CapabilityContentManageRevisions) {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeForbidden, "content.manage_revisions is required")
	}
	var item revision.Revision
	err := service.transactor.WithinContentTransaction(ctx, func(transaction Transaction) error {
		resolved, err := transaction.Revisions().Get(ctx, revisionID)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeNotFound, "revision was not found", err)
		}
		item = resolved
		return nil
	})
	if err != nil {
		return domaincontent.Entry{}, err
	}
	if item.EntryID != entryID {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "revision was not found")
	}
	if expectedKind != "" && item.Snapshot.Kind != expectedKind {
		return domaincontent.Entry{}, core.NewDomainError(core.ErrorCodeNotFound, "revision was not found")
	}
	desired := item.Snapshot
	desired.ID = entryID
	return service.Update(ctx, principal, desired, expectedVersion, "restore revision "+string(revisionID))
}

func (service *Service) PublishDue(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 100 {
		limit = 100
	}
	now := service.now().UTC()
	system := authz.NewPrincipal(
		"system:scheduler",
		authz.CapabilityContentReadPrivate,
		authz.CapabilityContentEditOthers,
		authz.CapabilityContentPublish,
	)
	result, err := service.List(ctx, system, Query{
		Statuses:      []domaincontent.Status{domaincontent.StatusScheduled},
		PublishBefore: &now, Page: 1, PerPage: limit,
	})
	if err != nil {
		return 0, err
	}
	var failures []error
	published := 0
	for _, entry := range result.Entries {
		if _, err := service.Transition(ctx, system, entry.ID, Transition{
			Status: domaincontent.StatusPublished, ExpectedVersion: entry.Version,
			PublishAt: entry.PublishedAt, Reason: "scheduled publication",
		}); err != nil {
			failures = append(failures, err)
			continue
		}
		published++
	}
	return published, errors.Join(failures...)
}

func (service *Service) before(ctx context.Context, event LifecycleEvent) error {
	if service.hooks == nil {
		return nil
	}
	if err := service.hooks.Before(ctx, event); err != nil {
		return core.WrapDomainError(core.ErrorCodeValidation, "content lifecycle hook rejected the operation", err)
	}
	return nil
}

func (service *Service) after(ctx context.Context, event LifecycleEvent) error {
	if service.hooks == nil {
		return nil
	}
	if err := service.hooks.After(ctx, event); err != nil {
		return core.WrapDomainError(core.ErrorCodeInternal, "content lifecycle hook failed", err)
	}
	return nil
}

func authorizeStatus(principal authz.Principal, before, after domaincontent.Status) error {
	if before == domaincontent.StatusTrashed && after != domaincontent.StatusTrashed {
		if !principal.Has(authz.CapabilityContentRestore) {
			return core.NewDomainError(core.ErrorCodeForbidden, string(authz.CapabilityContentRestore)+" is required")
		}
	}
	if before == after || after == domaincontent.StatusDraft {
		return nil
	}
	var capability authz.Capability
	switch after {
	case domaincontent.StatusPublished:
		capability = authz.CapabilityContentPublish
	case domaincontent.StatusScheduled:
		capability = authz.CapabilityContentSchedule
	case domaincontent.StatusArchived:
		capability = authz.CapabilityContentArchive
	case domaincontent.StatusTrashed:
		capability = authz.CapabilityContentDelete
	default:
		return core.NewDomainError(core.ErrorCodeValidation, "unsupported content status")
	}
	if !principal.Has(capability) {
		return core.NewDomainError(core.ErrorCodeForbidden, string(capability)+" is required")
	}
	return nil
}

func ensureUniqueSlugs(ctx context.Context, repository Repository, entry domaincontent.Entry, exclude domaincontent.ID) error {
	for locale, slug := range entry.Slug {
		exists, err := repository.SlugExists(ctx, entry.Kind, locale, slug, exclude)
		if err != nil {
			return core.WrapDomainError(core.ErrorCodeInternal, "slug uniqueness check failed", err)
		}
		if exists {
			return core.NewDomainError(core.ErrorCodeConflict, "content slug already exists")
		}
	}
	return nil
}

func normalizeEntrySlugs(entry *domaincontent.Entry) {
	for locale, slug := range entry.Slug {
		entry.Slug[locale] = domaincontent.NormalizeSlug(slug)
	}
}

func lifecycleName(before, after domaincontent.Status) string {
	if before != after {
		return "content.status_transition"
	}
	return "content.update"
}

func saveAudit(ctx context.Context, repository AuditRepository, event audit.Event) error {
	if repository == nil {
		return core.NewDomainError(core.ErrorCodeInternal, "audit repository is required")
	}
	id, err := repository.NextID(ctx)
	if err != nil {
		return core.WrapDomainError(core.ErrorCodeInternal, "audit id allocation failed", err)
	}
	event.ID = id
	if err := event.Validate(); err != nil {
		return core.WrapDomainError(core.ErrorCodeInternal, "audit event is invalid", err)
	}
	if err := repository.Save(ctx, event); err != nil {
		return core.WrapDomainError(core.ErrorCodeInternal, "audit event save failed", err)
	}
	return nil
}
