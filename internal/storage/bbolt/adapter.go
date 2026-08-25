package bbolt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record version conflict")
)

var (
	entriesBucket    = []byte("content_entries")
	revisionsBucket  = []byte("content_revisions")
	auditBucket      = []byte("audit_events")
	taxonomiesBucket = []byte("taxonomy_definitions")
	termsBucket      = []byte("taxonomy_terms")
	usersBucket      = []byte("identity_users")
	rolesBucket      = []byte("identity_roles")
	schemaBucket     = []byte("schema_migrations")
	schemaVersionKey = []byte("current_version")
)

type Adapter struct {
	database *bolt.DB
}

func Open(path string, mode os.FileMode, options *bolt.Options) (*Adapter, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("bbolt path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create bbolt directory: %w", err)
	}
	database, err := bolt.Open(path, mode, options)
	if err != nil {
		return nil, fmt.Errorf("open bbolt: %w", err)
	}
	if err := database.Update(func(transaction *bolt.Tx) error {
		if _, err := transaction.CreateBucketIfNotExists(entriesBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(revisionsBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(auditBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(taxonomiesBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(termsBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(usersBucket); err != nil {
			return err
		}
		if _, err := transaction.CreateBucketIfNotExists(rolesBucket); err != nil {
			return err
		}
		migrations, err := transaction.CreateBucketIfNotExists(schemaBucket)
		if err != nil {
			return err
		}
		return migrations.Put(schemaVersionKey, []byte("3"))
	}); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("initialize bbolt: %w", err)
	}
	return &Adapter{database: database}, nil
}

func (adapter *Adapter) Close() error {
	if adapter == nil || adapter.database == nil {
		return nil
	}
	return adapter.database.Close()
}

func (adapter *Adapter) Ping(ctx context.Context) error {
	if adapter == nil || adapter.database == nil {
		return errors.New("bbolt is not open")
	}
	return adapter.database.View(func(_ *bolt.Tx) error {
		return ctx.Err()
	})
}

func (adapter *Adapter) WithinTransaction(ctx context.Context, operation func(application.Transaction) error) error {
	if adapter == nil || adapter.database == nil {
		return errors.New("bbolt is not open")
	}
	if operation == nil {
		return errors.New("transaction operation is required")
	}
	return adapter.database.Update(func(transaction *bolt.Tx) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return operation(tx{transaction: transaction})
	})
}

type tx struct {
	transaction *bolt.Tx
}

func (transaction tx) Content() application.Repository {
	return contentRepository{transaction: transaction.transaction}
}

func (transaction tx) Revisions() application.RevisionRepository {
	return revisionRepository{transaction: transaction.transaction}
}

func (transaction tx) Audit() application.AuditRepository {
	return auditRepository{transaction: transaction.transaction}
}

func (transaction tx) Taxonomies() application.TaxonomyRepository {
	return taxonomyRepository{transaction: transaction.transaction}
}

func (transaction tx) Identity() application.IdentityRepository {
	return identityRepository{transaction: transaction.transaction}
}

type contentRepository struct {
	transaction *bolt.Tx
}

func (repository contentRepository) Get(_ context.Context, id content.ID) (content.Entry, error) {
	value := repository.transaction.Bucket(entriesBucket).Get([]byte(id))
	if value == nil {
		return content.Entry{}, ErrNotFound
	}
	return decodeEntry(value)
}

func (repository contentRepository) GetBySlug(_ context.Context, kind content.Kind, locale, slug string) (content.Entry, error) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	slug = content.NormalizeSlug(slug)
	var resolved content.Entry
	err := repository.transaction.Bucket(entriesBucket).ForEach(func(_, value []byte) error {
		entry, err := decodeEntry(value)
		if err != nil {
			return err
		}
		if entry.Kind == kind && entry.Slug[locale] == slug {
			resolved = entry
			return errStopIteration
		}
		return nil
	})
	if errors.Is(err, errStopIteration) {
		return resolved, nil
	}
	if err != nil {
		return content.Entry{}, err
	}
	return content.Entry{}, ErrNotFound
}

func (repository contentRepository) List(_ context.Context, query application.Query) (application.ListResult, error) {
	if query.Page < 1 || query.PerPage < 1 {
		return application.ListResult{}, errors.New("invalid pagination")
	}
	if query.PublicOnly && query.PublicAt.IsZero() {
		query.PublicAt = time.Now().UTC()
	}
	entries := make([]content.Entry, 0)
	err := repository.transaction.Bucket(entriesBucket).ForEach(func(_, value []byte) error {
		entry, err := decodeEntry(value)
		if err != nil {
			return err
		}
		if matches(entry, query) {
			entries = append(entries, entry)
		}
		return nil
	})
	if err != nil {
		return application.ListResult{}, err
	}
	sortEntries(entries, query.Sort, query.Descending)
	total := len(entries)
	start := (query.Page - 1) * query.PerPage
	if start > total {
		start = total
	}
	end := min(start+query.PerPage, total)
	pageEntries := append([]content.Entry(nil), entries[start:end]...)
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(query.PerPage)))
	}
	return application.ListResult{
		Entries: pageEntries,
		Page: application.Page{
			Number: query.Page, PerPage: query.PerPage, Total: total, TotalPages: totalPages,
		},
	}, nil
}

func (repository contentRepository) Create(_ context.Context, entry content.Entry) error {
	bucket := repository.transaction.Bucket(entriesBucket)
	key := []byte(entry.ID)
	if bucket.Get(key) != nil {
		return ErrConflict
	}
	return putJSON(bucket, key, entry)
}

func (repository contentRepository) Update(_ context.Context, entry content.Entry, expectedVersion uint64) error {
	bucket := repository.transaction.Bucket(entriesBucket)
	key := []byte(entry.ID)
	currentValue := bucket.Get(key)
	if currentValue == nil {
		return ErrNotFound
	}
	current, err := decodeEntry(currentValue)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	return putJSON(bucket, key, entry)
}

func (repository contentRepository) Delete(_ context.Context, id content.ID, expectedVersion uint64) error {
	bucket := repository.transaction.Bucket(entriesBucket)
	value := bucket.Get([]byte(id))
	if value == nil {
		return ErrNotFound
	}
	current, err := decodeEntry(value)
	if err != nil {
		return err
	}
	if current.Version != expectedVersion {
		return ErrConflict
	}
	return bucket.Delete([]byte(id))
}

func (repository contentRepository) SlugExists(
	ctx context.Context,
	kind content.Kind,
	locale string,
	slug string,
	exclude content.ID,
) (bool, error) {
	locale = strings.ToLower(strings.TrimSpace(locale))
	slug = content.NormalizeSlug(slug)
	exists := false
	err := repository.transaction.Bucket(entriesBucket).ForEach(func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if content.ID(key) == exclude {
			return nil
		}
		entry, err := decodeEntry(value)
		if err != nil {
			return err
		}
		if entry.Kind == kind && entry.Slug[locale] == slug {
			exists = true
			return errStopIteration
		}
		return nil
	})
	if errors.Is(err, errStopIteration) {
		return exists, nil
	}
	return false, err
}

type revisionRepository struct {
	transaction *bolt.Tx
}

func (repository revisionRepository) NextID(_ context.Context) (revision.ID, error) {
	return revision.ID("revision_" + uuid.NewString()), nil
}

func (repository revisionRepository) Save(_ context.Context, item revision.Revision) error {
	bucket := repository.transaction.Bucket(revisionsBucket)
	key := []byte(item.ID)
	if bucket.Get(key) != nil {
		return ErrConflict
	}
	return putJSON(bucket, key, item)
}

func (repository revisionRepository) Get(_ context.Context, id revision.ID) (revision.Revision, error) {
	value := repository.transaction.Bucket(revisionsBucket).Get([]byte(id))
	if value == nil {
		return revision.Revision{}, ErrNotFound
	}
	var item revision.Revision
	if err := json.Unmarshal(value, &item); err != nil {
		return revision.Revision{}, fmt.Errorf("decode revision: %w", err)
	}
	return item, nil
}

func (repository revisionRepository) List(
	_ context.Context,
	entryID content.ID,
	page int,
	perPage int,
) ([]revision.Revision, application.Page, error) {
	if page < 1 || perPage < 1 {
		return nil, application.Page{}, errors.New("invalid pagination")
	}
	items := make([]revision.Revision, 0)
	err := repository.transaction.Bucket(revisionsBucket).ForEach(func(_, value []byte) error {
		var item revision.Revision
		if err := json.Unmarshal(value, &item); err != nil {
			return fmt.Errorf("decode revision: %w", err)
		}
		if item.EntryID == entryID {
			items = append(items, item)
		}
		return nil
	})
	if err != nil {
		return nil, application.Page{}, err
	}
	slices.SortFunc(items, func(left, right revision.Revision) int {
		return right.CreatedAt.Compare(left.CreatedAt)
	})
	total := len(items)
	start := (page - 1) * perPage
	if start > total {
		start = total
	}
	end := min(start+perPage, total)
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return append([]revision.Revision(nil), items[start:end]...), application.Page{
		Number: page, PerPage: perPage, Total: total, TotalPages: totalPages,
	}, nil
}

var errStopIteration = errors.New("stop iteration")

func decodeEntry(value []byte) (content.Entry, error) {
	var entry content.Entry
	if err := json.Unmarshal(value, &entry); err != nil {
		return content.Entry{}, fmt.Errorf("decode content entry: %w", err)
	}
	return entry, nil
}

func putJSON(bucket *bolt.Bucket, key []byte, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return bucket.Put(key, encoded)
}

func matches(entry content.Entry, query application.Query) bool {
	if query.PublicOnly && !entry.IsPublicAt(query.PublicAt) {
		return false
	}
	if len(query.Kinds) > 0 && !slices.Contains(query.Kinds, entry.Kind) {
		return false
	}
	if len(query.Statuses) > 0 && !slices.Contains(query.Statuses, entry.Status) {
		return false
	}
	if query.AuthorID != "" && entry.AuthorID != query.AuthorID {
		return false
	}
	if query.After != nil && entry.UpdatedAt.Before(*query.After) {
		return false
	}
	if query.Before != nil && entry.UpdatedAt.After(*query.Before) {
		return false
	}
	if query.PublishBefore != nil && (entry.PublishedAt == nil || entry.PublishedAt.After(*query.PublishBefore)) {
		return false
	}
	if query.TaxonomyID != "" || query.TermID != "" {
		found := false
		for _, term := range entry.Terms {
			if (query.TaxonomyID == "" || term.Taxonomy == query.TaxonomyID) &&
				(query.TermID == "" || term.TermID == query.TermID) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		title := strings.ToLower(entry.Title.Value(query.Locale, "en"))
		body := strings.ToLower(entry.Content.Value(query.Locale, "en"))
		if !strings.Contains(title, search) && !strings.Contains(body, search) {
			return false
		}
	}
	return true
}

func sortEntries(entries []content.Entry, field string, descending bool) {
	slices.SortStableFunc(entries, func(left, right content.Entry) int {
		var comparison int
		switch field {
		case "created_at":
			comparison = left.CreatedAt.Compare(right.CreatedAt)
		case "title":
			comparison = strings.Compare(left.Title.Value("en", ""), right.Title.Value("en", ""))
		default:
			comparison = left.UpdatedAt.Compare(right.UpdatedAt)
		}
		if comparison == 0 {
			comparison = strings.Compare(string(left.ID), string(right.ID))
		}
		if descending {
			return -comparison
		}
		return comparison
	})
}
