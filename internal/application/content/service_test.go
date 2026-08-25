package content

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fastygo/backend/internal/domain/audit"
	"github.com/fastygo/backend/internal/domain/authz"
	domaincontent "github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/domain/schema"
	domaintaxonomy "github.com/fastygo/backend/internal/domain/taxonomy"
	"github.com/fastygo/framework/pkg/core"
)

func TestAnonymousCannotReadDraftOrPrivateMetadata(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.entries["post_1"] = entryAt(now, domaincontent.StatusDraft)
	service := newTestService(t, repository, now)

	_, err := service.Get(context.Background(), authz.Anonymous(), "post_1")
	assertDomainCode(t, err, core.ErrorCodeNotFound)

	entry := repository.entries["post_1"]
	entry.Status = domaincontent.StatusPublished
	entry.Metadata = map[string]domaincontent.MetadataValue{
		"public": {Value: "yes"},
		"secret": {Value: "no", Private: true},
	}
	repository.entries["post_1"] = entry

	resolved, err := service.Get(context.Background(), authz.Anonymous(), "post_1")
	if err != nil {
		t.Fatalf("get public entry: %v", err)
	}
	if _, leaked := resolved.Metadata["secret"]; leaked {
		t.Fatalf("private metadata leaked")
	}
}

func TestCreateAndUpdateEnforceLifecycleCapabilitiesAndRevision(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	revisions := &memoryRevisions{}
	transactor := memoryTransactor{repository: repository, revisions: revisions}
	service, err := NewService(transactor, nil, fixedClock{now: now})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	editor := authz.NewPrincipal("editor", authz.CapabilityContentCreate, authz.CapabilityContentEditOwn)
	entry := entryAt(now, domaincontent.StatusPublished)

	_, err = service.Create(context.Background(), editor, entry)
	assertDomainCode(t, err, core.ErrorCodeForbidden)

	editor.Capabilities[authz.CapabilityContentPublish] = struct{}{}
	created, err := service.Create(context.Background(), editor, entry)
	if err != nil {
		t.Fatalf("create content: %v", err)
	}
	if created.Version != 1 || created.AuthorID != "editor" {
		t.Fatalf("create invariants were not applied")
	}

	created.Title = domaincontent.LocalizedText{"en": "Updated"}
	updated, err := service.Update(context.Background(), editor, created, 1, "editor update")
	if err != nil {
		t.Fatalf("update content: %v", err)
	}
	if updated.Version != 2 || len(revisions.items) != 1 || revisions.items[0].Snapshot.Version != 1 {
		t.Fatalf("update did not preserve a revision")
	}

	_, err = service.Update(context.Background(), editor, updated, 1, "stale update")
	assertDomainCode(t, err, core.ErrorCodeConflict)
}

func TestCreateRejectsDuplicateLocalizedSlug(t *testing.T) {
	now := time.Now().UTC()
	repository := newMemoryRepository()
	repository.entries["post_existing"] = entryAt(now, domaincontent.StatusPublished)
	service := newTestService(t, repository, now)
	principal := authz.NewPrincipal(
		"admin",
		authz.CapabilityContentCreate,
		authz.CapabilityContentPublish,
	)
	entry := entryAt(now, domaincontent.StatusPublished)
	entry.ID = "post_new"

	_, err := service.Create(context.Background(), principal, entry)
	assertDomainCode(t, err, core.ErrorCodeConflict)
}

func TestLifecycleSchedulingTrashRestoreAndRevisionRestore(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	repository := newMemoryRepository()
	revisions := &memoryRevisions{}
	audits := &memoryAudit{}
	service, _ := NewService(
		memoryTransactor{repository: repository, revisions: revisions, audit: audits}, nil, clock,
	)
	editor := authz.NewPrincipal(
		"editor", authz.CapabilityContentCreate, authz.CapabilityContentEditOwn,
		authz.CapabilityContentSchedule, authz.CapabilityContentPublish,
		authz.CapabilityContentDelete, authz.CapabilityContentRestore,
		authz.CapabilityContentManageRevisions, authz.CapabilityContentReadPrivate,
	)
	entry := entryAt(now, domaincontent.StatusDraft)
	created, err := service.Create(context.Background(), editor, entry)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	publishAt := now.Add(time.Hour)
	_, err = service.Transition(context.Background(), editor, created.ID, Transition{
		Status: domaincontent.StatusScheduled, PublishAt: &publishAt, ExpectedVersion: 1,
	})
	if err != nil {
		t.Fatalf("schedule content: %v", err)
	}
	clock.now = now.Add(2 * time.Hour)
	published, err := service.PublishDue(context.Background(), 100)
	if err != nil || published != 1 {
		t.Fatalf("publish due content: count=%d err=%v", published, err)
	}
	current := repository.entries[created.ID]
	trashed, err := service.Transition(context.Background(), editor, created.ID, Transition{
		Status: domaincontent.StatusTrashed, ExpectedVersion: current.Version,
	})
	if err != nil || trashed.DeletedAt == nil {
		t.Fatalf("trash content: %v", err)
	}
	restored, err := service.Transition(context.Background(), editor, created.ID, Transition{
		Status: domaincontent.StatusDraft, ExpectedVersion: trashed.Version,
	})
	if err != nil || restored.DeletedAt != nil {
		t.Fatalf("restore content: %v", err)
	}
	items, _, err := service.Revisions(context.Background(), editor, created.ID, created.Kind, 1, 20)
	if err != nil || len(items) < 1 {
		t.Fatalf("list revisions: %v", err)
	}
	restoredRevision, err := service.RestoreRevision(
		context.Background(), editor, created.ID, created.Kind, items[len(items)-1].ID, restored.Version,
	)
	if err != nil || restoredRevision.Version != restored.Version+1 {
		t.Fatalf("restore revision: %v", err)
	}
}

type fixedClock struct {
	now time.Time
}

type mutableClock struct {
	now time.Time
}

func (clock *mutableClock) Now() time.Time {
	return clock.now
}

func (clock fixedClock) Now() time.Time {
	return clock.now
}

type memoryTransactor struct {
	repository *memoryRepository
	revisions  *memoryRevisions
	audit      *memoryAudit
	taxonomies *memoryTaxonomies
}

func (transactor memoryTransactor) WithinContentTransaction(ctx context.Context, operation func(Transaction) error) error {
	return operation(memoryTransaction(transactor))
}

type memoryTransaction memoryTransactor

func (transaction memoryTransaction) Content() Repository {
	return transaction.repository
}

func (transaction memoryTransaction) Revisions() RevisionRepository {
	if transaction.revisions == nil {
		return &memoryRevisions{}
	}
	return transaction.revisions
}

func (transaction memoryTransaction) Audit() AuditRepository {
	if transaction.audit == nil {
		return &memoryAudit{}
	}
	return transaction.audit
}

func (transaction memoryTransaction) Taxonomies() TaxonomyReader {
	if transaction.taxonomies == nil {
		return newMemoryTaxonomies()
	}
	return transaction.taxonomies
}

type memoryRepository struct {
	entries map[domaincontent.ID]domaincontent.Entry
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{entries: map[domaincontent.ID]domaincontent.Entry{}}
}

func (repository *memoryRepository) Get(_ context.Context, id domaincontent.ID) (domaincontent.Entry, error) {
	entry, exists := repository.entries[id]
	if !exists {
		return domaincontent.Entry{}, errors.New("not found")
	}
	return entry, nil
}

func (repository *memoryRepository) GetBySlug(_ context.Context, kind domaincontent.Kind, locale, slug string) (domaincontent.Entry, error) {
	for _, entry := range repository.entries {
		if entry.Kind == kind && entry.Slug[locale] == slug {
			return entry, nil
		}
	}
	return domaincontent.Entry{}, errors.New("not found")
}

func (repository *memoryRepository) List(_ context.Context, query Query) (ListResult, error) {
	entries := make([]domaincontent.Entry, 0, len(repository.entries))
	for _, entry := range repository.entries {
		if len(query.Kinds) > 0 {
			found := false
			for _, kind := range query.Kinds {
				if entry.Kind == kind {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if query.RelationField != "" && query.RelatedID != "" {
			metadata, exists := entry.Metadata[query.RelationField]
			if !exists || !metadataReferences(metadata.Value, string(query.RelatedID)) {
				continue
			}
		}
		entries = append(entries, entry)
	}
	return ListResult{Entries: entries, Page: Page{Number: query.Page, PerPage: query.PerPage, Total: len(entries), TotalPages: 1}}, nil
}

func (repository *memoryRepository) Create(_ context.Context, entry domaincontent.Entry) error {
	if _, exists := repository.entries[entry.ID]; exists {
		return errors.New("duplicate")
	}
	repository.entries[entry.ID] = entry
	return nil
}

func (repository *memoryRepository) Update(_ context.Context, entry domaincontent.Entry, expected uint64) error {
	current, exists := repository.entries[entry.ID]
	if !exists {
		return errors.New("not found")
	}
	if current.Version != expected {
		return errors.New("conflict")
	}
	repository.entries[entry.ID] = entry
	return nil
}

func (repository *memoryRepository) Delete(_ context.Context, id domaincontent.ID, expected uint64) error {
	current, exists := repository.entries[id]
	if !exists || current.Version != expected {
		return errors.New("conflict")
	}
	delete(repository.entries, id)
	return nil
}

func (repository *memoryRepository) SlugExists(_ context.Context, kind domaincontent.Kind, locale, slug string, exclude domaincontent.ID) (bool, error) {
	for id, entry := range repository.entries {
		if id != exclude && entry.Kind == kind && entry.Slug[locale] == slug {
			return true, nil
		}
	}
	return false, nil
}

type memoryRevisions struct {
	items []revision.Revision
}

type memoryAudit struct {
	items []audit.Event
}

type memoryTaxonomies struct {
	definitions map[string]domaintaxonomy.Definition
	terms       map[domaintaxonomy.ID]domaintaxonomy.Term
}

func newMemoryTaxonomies() *memoryTaxonomies {
	return &memoryTaxonomies{
		definitions: map[string]domaintaxonomy.Definition{},
		terms:       map[domaintaxonomy.ID]domaintaxonomy.Term{},
	}
}

func (repository *memoryTaxonomies) GetDefinition(_ context.Context, id string) (domaintaxonomy.Definition, error) {
	item, exists := repository.definitions[id]
	if !exists {
		return domaintaxonomy.Definition{}, errors.New("not found")
	}
	return item, nil
}

func (repository *memoryTaxonomies) ListDefinitions(context.Context) ([]domaintaxonomy.Definition, error) {
	items := make([]domaintaxonomy.Definition, 0, len(repository.definitions))
	for _, item := range repository.definitions {
		items = append(items, item)
	}
	return items, nil
}

func (repository *memoryTaxonomies) SaveDefinition(_ context.Context, item domaintaxonomy.Definition, expected uint64) error {
	current, exists := repository.definitions[item.ID]
	if (expected == 0 && exists) || (expected > 0 && (!exists || current.Version != expected)) {
		return errors.New("conflict")
	}
	repository.definitions[item.ID] = item
	return nil
}

func (repository *memoryTaxonomies) DeleteDefinition(_ context.Context, id string, expected uint64) error {
	current, exists := repository.definitions[id]
	if !exists || current.Version != expected {
		return errors.New("conflict")
	}
	delete(repository.definitions, id)
	return nil
}

func (repository *memoryTaxonomies) GetTerm(_ context.Context, id domaintaxonomy.ID) (domaintaxonomy.Term, error) {
	item, exists := repository.terms[id]
	if !exists {
		return domaintaxonomy.Term{}, errors.New("not found")
	}
	return item, nil
}

func (repository *memoryTaxonomies) ListTerms(_ context.Context, taxonomyID string) ([]domaintaxonomy.Term, error) {
	var items []domaintaxonomy.Term
	for _, item := range repository.terms {
		if item.TaxonomyID == taxonomyID {
			items = append(items, item)
		}
	}
	return items, nil
}

func (repository *memoryTaxonomies) SaveTerm(_ context.Context, item domaintaxonomy.Term, expected uint64) error {
	current, exists := repository.terms[item.ID]
	if (expected == 0 && exists) || (expected > 0 && (!exists || current.Version != expected)) {
		return errors.New("conflict")
	}
	repository.terms[item.ID] = item
	return nil
}

func (repository *memoryTaxonomies) DeleteTerm(_ context.Context, id domaintaxonomy.ID, expected uint64) error {
	current, exists := repository.terms[id]
	if !exists || current.Version != expected {
		return errors.New("conflict")
	}
	delete(repository.terms, id)
	return nil
}

func TestManifestRelationsRequireExistingTypedTargets(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	service := newTestService(t, repository, now)
	err := service.SetManifest(schema.Manifest{
		Name: "relations", Version: "1",
		Resources: []schema.Resource{
			{ID: "author", Collection: "authors"},
			{ID: "post", Collection: "posts", Fields: []schema.Field{{
				ID: "author", Type: schema.FieldRelation, Required: true,
				Relation: &schema.Relation{
					Resource: "author", Cardinality: schema.CardinalityOne,
					OnDelete: schema.DeleteRestrict,
				},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("set manifest: %v", err)
	}
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentDelete,
	)
	author, err := service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "author_1", Kind: "author", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Author"},
		Slug:       domaincontent.LocalizedText{"en": "author"},
	})
	if err != nil {
		t.Fatalf("create relation target: %v", err)
	}
	_, err = service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "post_1", Kind: "post", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Post"},
		Slug:       domaincontent.LocalizedText{"en": "post"},
		Metadata: map[string]domaincontent.MetadataValue{
			"author": {Value: string(author.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create valid relation: %v", err)
	}
	_, err = service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "post_2", Kind: "post", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Broken"},
		Slug:       domaincontent.LocalizedText{"en": "broken"},
		Metadata: map[string]domaincontent.MetadataValue{
			"author": {Value: "missing"},
		},
	})
	if err == nil {
		t.Fatalf("missing relation target must be rejected")
	}
	_, err = service.Transition(context.Background(), principal, author.ID, Transition{
		Status: domaincontent.StatusTrashed, ExpectedVersion: author.Version,
	})
	if err == nil {
		t.Fatalf("restrict relation must prevent target deletion")
	}
	err = service.SetManifest(schema.Manifest{
		Name: "relations", Version: "2",
		Resources: []schema.Resource{
			{ID: "author", Collection: "authors"},
			{ID: "post", Collection: "posts", Fields: []schema.Field{{
				ID: "author", Type: schema.FieldRelation,
				Relation: &schema.Relation{
					Resource: "author", Cardinality: schema.CardinalityOne,
					OnDelete: schema.DeleteNullify,
				},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("set nullify manifest: %v", err)
	}
	_, err = service.Transition(context.Background(), principal, author.ID, Transition{
		Status: domaincontent.StatusTrashed, ExpectedVersion: author.Version,
	})
	if err != nil {
		t.Fatalf("nullify relation target deletion: %v", err)
	}
	updatedPost := repository.entries["post_1"]
	if _, exists := updatedPost.Metadata["author"]; exists {
		t.Fatalf("nullify policy did not remove the relation")
	}
	err = service.SetManifest(schema.Manifest{
		Name: "relations", Version: "3",
		Resources: []schema.Resource{
			{ID: "author", Collection: "authors"},
			{ID: "post", Collection: "posts"},
			{ID: "comment", Collection: "comments", Fields: []schema.Field{{
				ID: "post", Type: schema.FieldRelation, Required: true,
				Relation: &schema.Relation{
					Resource: "post", Cardinality: schema.CardinalityOne,
					OnDelete: schema.DeleteCascade,
				},
			}}},
		},
	})
	if err != nil {
		t.Fatalf("set cascade manifest: %v", err)
	}
	cascadeTarget, err := service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "post_3", Kind: "post", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Cascade target"},
		Slug:       domaincontent.LocalizedText{"en": "cascade-target"},
	})
	if err != nil {
		t.Fatalf("create cascade target: %v", err)
	}
	_, err = service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "comment_1", Kind: "comment", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Comment"},
		Slug:       domaincontent.LocalizedText{"en": "comment"},
		Metadata: map[string]domaincontent.MetadataValue{
			"post": {Value: string(cascadeTarget.ID)},
		},
	})
	if err != nil {
		t.Fatalf("create cascade dependent: %v", err)
	}
	_, err = service.Transition(context.Background(), principal, cascadeTarget.ID, Transition{
		Status: domaincontent.StatusTrashed, ExpectedVersion: cascadeTarget.Version,
	})
	if err != nil {
		t.Fatalf("cascade relation target deletion: %v", err)
	}
	if repository.entries["comment_1"].Status != domaincontent.StatusTrashed {
		t.Fatalf("cascade policy did not trash dependent content")
	}
}

func TestManifestValidatesFieldTypesLocalizationAndSensitivity(t *testing.T) {
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	service := newTestService(t, newMemoryRepository(), now)
	manifest := schema.Manifest{
		Name: "typed", Version: "1",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products",
			Fields: []schema.Field{
				{ID: "price", Type: schema.FieldMoney, Required: true},
				{ID: "state", Type: schema.FieldEnum, Enum: []string{"active", "inactive"}},
				{ID: "label", Type: schema.FieldString, Localized: true},
				{ID: "tags", Type: schema.FieldCollection, Items: &schema.Field{ID: "tag", Type: schema.FieldString}},
				{ID: "website", Type: schema.FieldURI},
				{ID: "secret", Type: schema.FieldString, Sensitive: true},
			},
		}},
	}
	if err := service.SetManifest(manifest); err != nil {
		t.Fatalf("set typed manifest: %v", err)
	}
	principal := authz.NewPrincipal("editor", authz.CapabilityContentCreate)
	created, err := service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "product_1", Kind: "product", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Typed"},
		Slug:       domaincontent.LocalizedText{"en": "typed"},
		Metadata: map[string]domaincontent.MetadataValue{
			"price":   {Value: "49.95"},
			"state":   {Value: "active"},
			"label":   {Value: map[string]any{"en": "Dress", "ru": "Платье"}},
			"tags":    {Value: []any{"summer", "sale"}},
			"website": {Value: "https://example.com/products/1"},
			"secret":  {Value: "internal"},
		},
	})
	if err != nil {
		t.Fatalf("create typed content: %v", err)
	}
	if !created.Metadata["secret"].Private {
		t.Fatalf("sensitive field was not marked private")
	}
	_, err = service.Create(context.Background(), principal, domaincontent.Entry{
		ID: "product_2", Kind: "product", Status: domaincontent.StatusDraft,
		Visibility: domaincontent.VisibilityPublic,
		Title:      domaincontent.LocalizedText{"en": "Invalid"},
		Slug:       domaincontent.LocalizedText{"en": "invalid"},
		Metadata: map[string]domaincontent.MetadataValue{
			"price": {Value: true},
			"state": {Value: "unknown"},
		},
	})
	if err == nil {
		t.Fatalf("invalid runtime field types were accepted")
	}
}

func (repository *memoryAudit) NextID(context.Context) (audit.ID, error) {
	return audit.ID(fmt.Sprintf("audit_%d", len(repository.items)+1)), nil
}

func (repository *memoryAudit) Save(_ context.Context, event audit.Event) error {
	repository.items = append(repository.items, event)
	return nil
}

func (repository *memoryAudit) List(_ context.Context, query AuditQuery) ([]audit.Event, Page, error) {
	return append([]audit.Event(nil), repository.items...), Page{
		Number: query.Page, PerPage: query.PerPage, Total: len(repository.items), TotalPages: 1,
	}, nil
}

func (repository *memoryRevisions) NextID(context.Context) (revision.ID, error) {
	return revision.ID(fmt.Sprintf("revision_%d", len(repository.items)+1)), nil
}

func (repository *memoryRevisions) Save(_ context.Context, item revision.Revision) error {
	repository.items = append(repository.items, item)
	return nil
}

func (repository *memoryRevisions) Get(_ context.Context, id revision.ID) (revision.Revision, error) {
	for _, item := range repository.items {
		if item.ID == id {
			return item, nil
		}
	}
	return revision.Revision{}, errors.New("not found")
}

func (repository *memoryRevisions) List(_ context.Context, entryID domaincontent.ID, page, perPage int) ([]revision.Revision, Page, error) {
	var items []revision.Revision
	for _, item := range repository.items {
		if item.EntryID == entryID {
			items = append(items, item)
		}
	}
	return items, Page{Number: page, PerPage: perPage, Total: len(items), TotalPages: 1}, nil
}

func newTestService(t *testing.T, repository *memoryRepository, now time.Time) *Service {
	t.Helper()
	service, err := NewService(
		memoryTransactor{repository: repository, revisions: &memoryRevisions{}},
		nil,
		fixedClock{now: now},
	)
	if err != nil {
		t.Fatalf("create service: %v", err)
	}
	return service
}

func entryAt(now time.Time, status domaincontent.Status) domaincontent.Entry {
	return domaincontent.Entry{
		ID: "post_1", Kind: domaincontent.KindPost, Status: status, Visibility: domaincontent.VisibilityPublic,
		Slug: domaincontent.LocalizedText{"en": "hello"}, Title: domaincontent.LocalizedText{"en": "Hello"},
		Version: 1, CreatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
}

func assertDomainCode(t *testing.T, err error, expected core.ErrorCode) {
	t.Helper()
	var domainError core.DomainError
	if !errors.As(err, &domainError) || domainError.Code != expected {
		t.Fatalf("expected %s, got %v", expected, err)
	}
}
