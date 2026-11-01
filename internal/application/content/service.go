package content

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/application/forms"
	"github.com/fastygo/backend/internal/contentcompat"
	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/revision"
	codexschema "github.com/fastygo/codex/schema"
	domainTaxonomy "github.com/fastygo/codex/taxonomy"
	"github.com/fastygo/framework/pkg/core"
	"github.com/google/uuid"
)

type Service struct {
	transactor Transactor
	hooks      Hooks
	now        func() time.Time
	manifest   *schema.Manifest
	// defaultCurrency is the ISO 4217 code used when a money entry carries no
	// currency field (Codex CheckMoneyCurrency).
	defaultCurrency string
}

// SetDefaultCurrency sets the ISO 4217 fallback for Codex money fields.
func (service *Service) SetDefaultCurrency(code string) {
	service.defaultCurrency = strings.ToUpper(strings.TrimSpace(code))
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
	contentcompat.EnsureLocale(&entry)
	contentcompat.Lift(&entry)
	contentcompat.SyncChrome(&entry)
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
		if entry.Locale == "" {
			entry.Locale = current.Locale
		}
		contentcompat.EnsureLocale(&entry)
		contentcompat.Lift(&entry)
		contentcompat.SyncChrome(&entry)
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
			ID: revisionID, EntryID: current.ID, Locale: current.Locale, Version: current.Version, Snapshot: current,
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
			relation, bound := resource.RelationFor(field.ID)
			if field.Type != schema.FieldRelation || !bound || string(relation.Target) != string(target.Kind) {
				continue
			}
			onDelete := schema.OnDelete(relation)
			dependents, err := listRelatedContent(
				ctx,
				transaction.Content(),
				domaincontent.Kind(resource.ID),
				string(field.ID),
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
				switch onDelete {
				case schema.DeleteRestrict:
					return core.NewDomainError(
						core.ErrorCodeConflict,
						fmt.Sprintf("content is referenced by %s through field %q", dependent.ID, string(field.ID)),
					)
				case schema.DeleteNullify:
					nullifyRelation(&dependent, string(field.ID), relation, string(target.ID))
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
					ID: revisionID, EntryID: before.ID, Locale: before.Locale, Version: before.Version, Snapshot: before,
					AuthorID: principal.ID, Reason: "relation on_delete " + string(onDelete),
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
					Action: "content.relation_" + string(onDelete), Resource: string(dependent.Kind),
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

func relationContains(many bool, value any, targetID string) bool {
	if !many {
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

func nullifyRelation(entry *domaincontent.Entry, fieldID string, relation schema.Relation, targetID string) {
	if entry.Metadata == nil {
		return
	}
	if !schema.Many(relation) {
		delete(entry.Metadata, fieldID)
		return
	}
	metadata := entry.Metadata[fieldID]
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
	entry.Metadata[fieldID] = metadata
}

func (service *Service) validateManifest(
	ctx context.Context,
	repository Repository,
	entry *domaincontent.Entry,
) error {
	if service.manifest == nil {
		return nil
	}
	contentcompat.Lift(entry)
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
		id := string(field.ID)
		value, present := entry.Metadata[id]
		if field.Required && (!present || value.Value == nil) {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("required field %q is missing", id),
			)
		}
		if !present {
			continue
		}
		if value.Value == nil {
			if schema.Nullable(field) {
				continue
			}
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("field %q cannot be null", id),
			)
		}
		if schema.ReadOnly(field) {
			return core.NewDomainError(
				core.ErrorCodeValidation,
				fmt.Sprintf("read-only field %q cannot be written", id),
			)
		}
		if field.Sensitive && !value.Private {
			value.Private = true
			entry.Metadata[id] = value
		}
		switch schema.SemanticOf(field) {
		case schema.SemanticRelation:
			relation, bound := resource.RelationFor(field.ID)
			if !bound {
				return core.NewDomainError(core.ErrorCodeInternal, "relation metadata is unavailable")
			}
			if err := validateRelationValue(ctx, repository, id, relation, value.Value); err != nil {
				return err
			}
		case schema.SemanticMedia:
			relation, bound := resource.RelationFor(field.ID)
			if !bound {
				relation = schema.Relation{Target: schema.MediaTarget, Cardinality: schema.OneToOne}
			}
			if err := validateRelationValue(ctx, repository, id, relation, value.Value); err != nil {
				return err
			}
		default:
			if err := validateFieldValue(*resource, field, value.Value); err != nil {
				return err
			}
		}
	}
	if issue := codexschema.CheckMoneyCurrency(resource.Fields, entry.Metadata, service.defaultCurrency, ""); issue.Code != "" {
		return core.NewDomainError(core.ErrorCodeValidation, issue.Message)
	}
	declared := make(map[string]struct{}, len(resource.Fields))
	for _, field := range resource.Fields {
		declared[string(field.ID)] = struct{}{}
	}
	for id := range entry.Metadata {
		if contentcompat.IsInternalMeta(id) {
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
	return relationContains(false, value, targetID) || relationContains(true, value, targetID)
}

func validateRelationValue(
	ctx context.Context,
	repository Repository,
	fieldID string,
	relation schema.Relation,
	value any,
) error {
	var ids []stringValue
	if !schema.Many(relation) {
		ids = []stringValue{asString(value)}
	} else {
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
				fmt.Sprintf("relation field %q must be an array of ids", fieldID),
			)
		}
	}
	return validateRelationIDs(ctx, repository, fieldID, string(relation.Target), ids)
}

// validateFieldValue checks one stored value with Codex value rules
// (codexschema.CheckMetadataFields). Localized fields hold a locale map whose
// values are each checked as the non-localized field.
func validateFieldValue(resource schema.Resource, field schema.Field, value any) error {
	id := string(field.ID)
	if field.Localized {
		values, ok := value.(map[string]any)
		if !ok || len(values) == 0 {
			return invalidField(id, "must be a non-empty locale map")
		}
		copyField := field
		copyField.Localized = false
		for locale, localized := range values {
			if strings.TrimSpace(locale) == "" {
				return invalidField(id, "contains an invalid locale")
			}
			if err := validateFieldValue(resource, copyField, localized); err != nil {
				return err
			}
		}
		return nil
	}
	if values, ok := value.([]string); ok {
		converted := make([]any, len(values))
		for index, item := range values {
			converted[index] = item
		}
		value = converted
	}
	metadata := map[string]domaincontent.MetadataValue{id: {Value: normalizeNumbers(value)}}
	issues := codexschema.CheckMetadataFields(codexschema.Resource{Record: resource.Record()}, []schema.Field{field}, metadata, "metadata", nil)
	if len(issues) > 0 {
		return core.NewDomainError(core.ErrorCodeValidation, issues[0].Message)
	}
	return nil
}

// normalizeNumbers converts json.Number and Go integer kinds to float64 so
// Codex numeric checks see decoded JSON numbers.
func normalizeNumbers(value any) any {
	switch typed := value.(type) {
	case json.Number:
		if number, err := typed.Float64(); err == nil {
			return number
		}
		return typed
	case int:
		return float64(typed)
	case int32:
		return float64(typed)
	case int64:
		return float64(typed)
	case float32:
		return float64(typed)
	case []any:
		out := make([]any, len(typed))
		for index, item := range typed {
			out[index] = normalizeNumbers(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[key] = normalizeNumbers(item)
		}
		return out
	default:
		return value
	}
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
	chrome := contentcompat.ChromeFrom(entry)
	if len(chrome.Slug) == 0 && entry.Slug != "" {
		chrome.Slug = contentcompat.LocalizedText{contentcompat.DefaultLocale(entry): entry.Slug}
	}
	for locale, slug := range chrome.Slug {
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
	if entry == nil {
		return
	}
	chrome := contentcompat.ChromeFrom(*entry)
	if len(chrome.Slug) == 0 && entry.Slug != "" {
		chrome.Slug = contentcompat.LocalizedText{contentcompat.DefaultLocale(*entry): entry.Slug}
	}
	for locale, slug := range chrome.Slug {
		chrome.Slug[locale] = domaincontent.NormalizeSlug(slug)
	}
	contentcompat.AttachChrome(entry, chrome)
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
