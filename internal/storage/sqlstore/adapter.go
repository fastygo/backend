package sqlstore

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	contentapplication "github.com/fastygo/backend/internal/application/content"
	identityapplication "github.com/fastygo/backend/internal/application/identity"
	taxonomyapplication "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
	"github.com/fastygo/backend/internal/operations/backup"
	"github.com/fastygo/backend/internal/persist"
	"github.com/google/uuid"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)

type Dialect string

const (
	DialectSQLite     Dialect = "sqlite"
	DialectMySQL      Dialect = "mysql"
	DialectPostgreSQL Dialect = "postgres"
)

var (
	ErrNotFound = errors.New("record not found")
	ErrConflict = errors.New("record version conflict")
)

type Adapter struct {
	database *sql.DB
	dialect  Dialect
}

var (
	_ contentapplication.Transactor  = (*Adapter)(nil)
	_ taxonomyapplication.Transactor = (*Adapter)(nil)
	_ identityapplication.Transactor = (*Adapter)(nil)
	_ backup.Transactor              = (*Adapter)(nil)
)

func Open(ctx context.Context, driver, dataSource string, dialect Dialect) (*Adapter, error) {
	if !dialect.Valid() {
		return nil, errors.New("SQL dialect is invalid")
	}
	database, err := sql.Open(driver, dataSource)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQL database: %w", err)
	}
	if dialect == DialectSQLite {
		database.SetMaxOpenConns(1)
	}
	adapter := &Adapter{database: database, dialect: dialect}
	if err := adapter.Ping(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("failed to ping SQL database: %w", err)
	}
	if err := adapter.Migrate(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	return adapter, nil
}

func (adapter *Adapter) Close() error {
	if adapter == nil || adapter.database == nil {
		return nil
	}
	return adapter.database.Close()
}

func (adapter *Adapter) Ping(ctx context.Context) error {
	if adapter == nil || adapter.database == nil {
		return errors.New("SQL database is not open")
	}
	return adapter.database.PingContext(ctx)
}

func (adapter *Adapter) Migrate(ctx context.Context) error {
	if adapter == nil || adapter.database == nil {
		return errors.New("SQL database is not open")
	}
	transaction, err := adapter.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin SQL migration: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	if _, err := transaction.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at BIGINT NOT NULL
	)`); err != nil {
		return fmt.Errorf("failed to create SQL migration table: %w", err)
	}
	rows, err := transaction.QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return fmt.Errorf("failed to list SQL migrations: %w", err)
	}
	applied := make(map[int]struct{})
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			_ = rows.Close()
			return fmt.Errorf("failed to scan SQL migration: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("failed to close SQL migration rows: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("failed to iterate SQL migrations: %w", err)
	}
	for _, migration := range migrations(adapter.dialect) {
		if _, exists := applied[migration.version]; exists {
			continue
		}
		for _, statement := range migration.statements {
			if _, err := transaction.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("failed to execute SQL migration %d: %w", migration.version, err)
			}
		}
		if migration.version == 4 || migration.version == 5 {
			if err := ensureContentIndexes(ctx, transaction, adapter.dialect); err != nil {
				return fmt.Errorf("failed to create SQL content indexes: %w", err)
			}
			if err := backfillContentProjections(ctx, transaction, adapter.dialect); err != nil {
				return fmt.Errorf("failed to backfill SQL content projections: %w", err)
			}
		}
		if migration.version == 6 {
			if err := backfillContentLocales(ctx, transaction, adapter.dialect); err != nil {
				return fmt.Errorf("failed to backfill SQL content locales: %w", err)
			}
		}
		if _, err := transaction.ExecContext(
			ctx,
			bind(adapter.dialect,
				"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)"),
			migration.version, time.Now().UTC().UnixNano(),
		); err != nil {
			return fmt.Errorf("failed to record SQL migration %d: %w", migration.version, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("failed to commit SQL migration: %w", err)
	}
	return nil
}

func (adapter *Adapter) update(ctx context.Context, operation func(*sql.Tx) error) error {
	if adapter == nil || adapter.database == nil {
		return errors.New("SQL database is not open")
	}
	if operation == nil {
		return errors.New("transaction operation is required")
	}
	transaction, err := adapter.database.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	if err := operation(transaction); err != nil {
		return err
	}
	return transaction.Commit()
}

func (dialect Dialect) Valid() bool {
	return dialect == DialectSQLite || dialect == DialectMySQL || dialect == DialectPostgreSQL
}

func (adapter *Adapter) WithinContentTransaction(
	ctx context.Context,
	operation func(contentapplication.Transaction) error,
) error {
	if operation == nil {
		return errors.New("content transaction operation is required")
	}
	return adapter.update(ctx, func(transaction *sql.Tx) error {
		return operation(contentTx{transaction: transaction, dialect: adapter.dialect})
	})
}

func (adapter *Adapter) WithinTaxonomyTransaction(
	ctx context.Context,
	operation func(taxonomyapplication.Transaction) error,
) error {
	if operation == nil {
		return errors.New("taxonomy transaction operation is required")
	}
	return adapter.update(ctx, func(transaction *sql.Tx) error {
		return operation(taxonomyTx{transaction: transaction, dialect: adapter.dialect})
	})
}

func (adapter *Adapter) WithinIdentityTransaction(
	ctx context.Context,
	operation func(identityapplication.Transaction) error,
) error {
	if operation == nil {
		return errors.New("identity transaction operation is required")
	}
	return adapter.update(ctx, func(transaction *sql.Tx) error {
		return operation(identityTx{transaction: transaction, dialect: adapter.dialect})
	})
}

func (adapter *Adapter) WithinBackupTransaction(
	ctx context.Context,
	operation func(backup.Transaction) error,
) error {
	if operation == nil {
		return errors.New("backup transaction operation is required")
	}
	return adapter.update(ctx, func(transaction *sql.Tx) error {
		return operation(backupTx{transaction: transaction, dialect: adapter.dialect})
	})
}

type contentTx struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (transaction contentTx) Content() contentapplication.Repository {
	return contentRepository(transaction)
}

func (transaction contentTx) Revisions() contentapplication.RevisionRepository {
	return revisionRepository(transaction)
}

func (transaction contentTx) Audit() contentapplication.AuditRepository {
	return auditRepository(transaction)
}

func (transaction contentTx) Taxonomies() contentapplication.TaxonomyReader {
	return taxonomyRepository(transaction)
}

type taxonomyTx struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (transaction taxonomyTx) Taxonomies() taxonomyapplication.Repository {
	return taxonomyRepository(transaction)
}

func (transaction taxonomyTx) Content() taxonomyapplication.ContentRepository {
	return contentRepository(transaction)
}

func (transaction taxonomyTx) Audit() taxonomyapplication.AuditRepository {
	return auditRepository(transaction)
}

type identityTx struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (transaction identityTx) Identity() identityapplication.Repository {
	return identityRepository(transaction)
}

func (transaction identityTx) Audit() identityapplication.AuditRepository {
	return auditRepository(transaction)
}

type backupTx struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (transaction backupTx) Content() backup.ContentRepository {
	return contentRepository(transaction)
}

func (transaction backupTx) Revisions() backup.RevisionRepository {
	return revisionRepository(transaction)
}

func (transaction backupTx) Audit() backup.AuditRepository {
	return auditRepository(transaction)
}

func (transaction backupTx) Taxonomies() backup.TaxonomyRepository {
	return taxonomyRepository(transaction)
}

func (transaction backupTx) Identity() backup.IdentityRepository {
	return identityRepository(transaction)
}

type contentRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository contentRepository) Get(ctx context.Context, id content.ID) (content.Entry, error) {
	return repository.getByQuery(ctx, "SELECT payload FROM content_entries WHERE id = ?", id)
}

func (repository contentRepository) GetBySlug(
	ctx context.Context,
	kind content.Kind,
	locale string,
	slug string,
) (content.Entry, error) {
	return repository.getByQuery(
		ctx,
		`SELECT e.payload FROM content_entries e
		 JOIN content_slugs s ON s.entry_id = e.id
		 WHERE s.kind = ? AND s.locale = ? AND s.slug = ?`,
		kind,
		strings.ToLower(strings.TrimSpace(locale)),
		content.NormalizeSlug(slug),
	)
}

func (repository contentRepository) List(ctx context.Context, query contentapplication.Query) (contentapplication.ListResult, error) {
	if query.Page < 1 || query.PerPage < 1 {
		return contentapplication.ListResult{}, errors.New("invalid pagination")
	}
	if query.PublicOnly && query.PublicAt.IsZero() {
		query.PublicAt = time.Now().UTC()
	}
	where, arguments := contentListWhere(query)
	var total int
	countStatement := bind(repository.dialect,
		"SELECT COUNT(*) FROM content_entries e JOIN content_index i ON i.entry_id = e.id"+where,
	)
	if err := repository.transaction.QueryRowContext(ctx, countStatement, arguments...).Scan(&total); err != nil {
		return contentapplication.ListResult{}, err
	}

	orderColumn := "i.updated_at"
	switch query.Sort {
	case "created_at":
		orderColumn = "i.created_at"
	case "title":
		orderColumn = "i.title_sort"
	}
	direction := "ASC"
	if query.Descending {
		direction = "DESC"
	}
	statement := "SELECT e.payload FROM content_entries e JOIN content_index i ON i.entry_id = e.id" +
		where + " ORDER BY " + orderColumn + " " + direction + ", i.id_sort " + direction +
		" LIMIT ? OFFSET ?"
	pageArguments := append(append([]any(nil), arguments...),
		query.PerPage, (query.Page-1)*query.PerPage,
	)
	rows, err := repository.transaction.QueryContext(ctx, bind(repository.dialect, statement), pageArguments...)
	if err != nil {
		return contentapplication.ListResult{}, err
	}
	defer rows.Close()
	entries := make([]content.Entry, 0, query.PerPage)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return contentapplication.ListResult{}, err
		}
		entry, err := decodeEntry(encoded)
		if err != nil {
			return contentapplication.ListResult{}, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return contentapplication.ListResult{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(query.PerPage)))
	}
	return contentapplication.ListResult{
		Entries: entries,
		Page: contentapplication.Page{
			Number: query.Page, PerPage: query.PerPage, Total: total, TotalPages: totalPages,
		},
	}, nil
}

func (repository contentRepository) Create(ctx context.Context, entry content.Entry) error {
	encoded, err := persist.EncodeEntry(entry)
	if err != nil {
		return err
	}
	statement := bind(repository.dialect,
		"INSERT INTO content_entries (id, kind, status, visibility, author_id, version, updated_at, payload) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
	)
	if _, err := repository.transaction.ExecContext(
		ctx, statement, entry.ID, entry.Kind, entry.Status, entry.Visibility,
		entry.AuthorID, entry.Version, entry.UpdatedAt.UnixNano(), encoded,
	); err != nil {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	if err := repository.replaceProjections(ctx, entry); err != nil {
		return err
	}
	return repository.replaceLocales(ctx, entry)
}

func (repository contentRepository) Update(
	ctx context.Context,
	entry content.Entry,
	expectedVersion uint64,
) error {
	encoded, err := persist.EncodeEntry(entry)
	if err != nil {
		return err
	}
	statement := bind(repository.dialect,
		`UPDATE content_entries
		 SET kind = ?, status = ?, visibility = ?, author_id = ?, version = ?, updated_at = ?, payload = ?
		 WHERE id = ? AND version = ?`,
	)
	result, err := repository.transaction.ExecContext(
		ctx, statement, entry.Kind, entry.Status, entry.Visibility, entry.AuthorID,
		entry.Version, entry.UpdatedAt.UnixNano(), encoded, entry.ID, expectedVersion,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrConflict
	}
	if err := repository.replaceProjections(ctx, entry); err != nil {
		return err
	}
	return repository.replaceLocales(ctx, entry)
}

func (repository contentRepository) Delete(ctx context.Context, id content.ID, expectedVersion uint64) error {
	if _, err := repository.transaction.ExecContext(
		ctx, bind(repository.dialect, "DELETE FROM content_slugs WHERE entry_id = ?"), id,
	); err != nil {
		return err
	}
	for _, table := range []string{"content_search", "content_terms", "content_index", "content_locales"} {
		if _, err := repository.transaction.ExecContext(
			ctx, bind(repository.dialect, "DELETE FROM "+table+" WHERE entry_id = ?"), id,
		); err != nil {
			return err
		}
	}
	result, err := repository.transaction.ExecContext(
		ctx, bind(repository.dialect, "DELETE FROM content_entries WHERE id = ? AND version = ?"),
		id, expectedVersion,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrConflict
	}
	return nil
}

func (repository contentRepository) SlugExists(
	ctx context.Context,
	kind content.Kind,
	locale string,
	slug string,
	exclude content.ID,
) (bool, error) {
	statement := "SELECT COUNT(*) FROM content_slugs WHERE kind = ? AND locale = ? AND slug = ?"
	arguments := []any{kind, strings.ToLower(strings.TrimSpace(locale)), content.NormalizeSlug(slug)}
	if exclude != "" {
		statement += " AND entry_id <> ?"
		arguments = append(arguments, exclude)
	}
	var count int
	if err := repository.transaction.QueryRowContext(ctx, bind(repository.dialect, statement), arguments...).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (repository contentRepository) getByQuery(ctx context.Context, statement string, arguments ...any) (content.Entry, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(ctx, bind(repository.dialect, statement), arguments...).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return content.Entry{}, ErrNotFound
	}
	if err != nil {
		return content.Entry{}, err
	}
	return decodeEntry(encoded)
}

func (repository contentRepository) replaceSlugs(ctx context.Context, entry content.Entry) error {
	if _, err := repository.transaction.ExecContext(
		ctx, bind(repository.dialect, "DELETE FROM content_slugs WHERE entry_id = ?"), entry.ID,
	); err != nil {
		return err
	}
	statement := bind(repository.dialect,
		"INSERT INTO content_slugs (kind, locale, slug, entry_id) VALUES (?, ?, ?, ?)",
	)
	for locale, slug := range entry.Slug {
		if _, err := repository.transaction.ExecContext(
			ctx, statement, entry.Kind, strings.ToLower(strings.TrimSpace(locale)),
			content.NormalizeSlug(slug), entry.ID,
		); err != nil {
			return fmt.Errorf("%w: %v", ErrConflict, err)
		}
	}
	return nil
}

func (repository contentRepository) replaceProjections(ctx context.Context, entry content.Entry) error {
	if err := repository.replaceSlugs(ctx, entry); err != nil {
		return err
	}
	for _, table := range []string{"content_search", "content_terms", "content_index", "content_relations"} {
		if _, err := repository.transaction.ExecContext(
			ctx, bind(repository.dialect, "DELETE FROM "+table+" WHERE entry_id = ?"), entry.ID,
		); err != nil {
			return err
		}
	}
	var publishedAt any
	if entry.PublishedAt != nil {
		publishedAt = entry.PublishedAt.UnixNano()
	}
	var deletedAt any
	if entry.DeletedAt != nil {
		deletedAt = entry.DeletedAt.UnixNano()
	}
	if _, err := repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			`INSERT INTO content_index
			 (entry_id, id_sort, created_at, updated_at, published_at, deleted_at, title_sort)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`),
		entry.ID, hex.EncodeToString([]byte(entry.ID)),
		entry.CreatedAt.UnixNano(), entry.UpdatedAt.UnixNano(),
		publishedAt, deletedAt, hex.EncodeToString([]byte(entry.Title.Value("en", ""))),
	); err != nil {
		return err
	}

	locales := map[string]struct{}{"en": {}}
	for locale := range entry.Title {
		locales[strings.ToLower(strings.TrimSpace(locale))] = struct{}{}
	}
	for locale := range entry.Content {
		locales[strings.ToLower(strings.TrimSpace(locale))] = struct{}{}
	}
	searchStatement := bind(repository.dialect,
		"INSERT INTO content_search (entry_id, locale, search_text) VALUES (?, ?, ?)",
	)
	for locale := range locales {
		if locale == "" {
			continue
		}
		searchText := strings.ToLower(
			entry.Title.Value(locale, "en") + "\n" + entry.Content.Value(locale, "en"),
		)
		if _, err := repository.transaction.ExecContext(
			ctx, searchStatement, entry.ID, locale, searchText,
		); err != nil {
			return err
		}
	}
	termStatement := bind(repository.dialect,
		"INSERT INTO content_terms (taxonomy_id, term_id, entry_id) VALUES (?, ?, ?)",
	)
	for _, term := range entry.Terms {
		if _, err := repository.transaction.ExecContext(
			ctx, termStatement, term.Taxonomy, term.TermID, entry.ID,
		); err != nil {
			return err
		}
	}
	relationStatement := bind(repository.dialect,
		"INSERT INTO content_relations (entry_id, field_id, related_id) VALUES (?, ?, ?)",
	)
	for fieldID, metadata := range entry.Metadata {
		for _, relatedID := range relationTargets(metadata.Value) {
			if _, err := repository.transaction.ExecContext(
				ctx, relationStatement, entry.ID, fieldID, relatedID,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (repository contentRepository) replaceLocales(ctx context.Context, entry content.Entry) error {
	if _, err := repository.transaction.ExecContext(
		ctx, bind(repository.dialect, "DELETE FROM content_locales WHERE entry_id = ?"), entry.ID,
	); err != nil {
		return err
	}
	entry.LiftLocaleMetadata()
	statement := bind(repository.dialect,
		"INSERT INTO content_locales (entry_id, locale, status, updated_at, data) VALUES (?, ?, ?, ?, ?)",
	)
	for locale, document := range entry.Locales {
		encoded, err := json.Marshal(document.Data)
		if err != nil {
			return err
		}
		updatedAt := entry.UpdatedAt.UnixNano()
		if !document.UpdatedAt.IsZero() {
			updatedAt = document.UpdatedAt.UnixNano()
		}
		status := document.Status
		if status == "" {
			status = entry.Status
		}
		if _, err := repository.transaction.ExecContext(
			ctx, statement, entry.ID, content.NormalizeLocale(locale), status, updatedAt, encoded,
		); err != nil {
			return err
		}
	}
	return nil
}

func relationTargets(value any) []string {
	switch typed := value.(type) {
	case string:
		if id := strings.TrimSpace(typed); id != "" {
			return []string{id}
		}
	case []string:
		targets := make([]string, 0, len(typed))
		for _, item := range typed {
			if id := strings.TrimSpace(item); id != "" {
				targets = append(targets, id)
			}
		}
		return targets
	case []any:
		targets := make([]string, 0, len(typed))
		for _, item := range typed {
			if id, ok := item.(string); ok {
				if id = strings.TrimSpace(id); id != "" {
					targets = append(targets, id)
				}
			}
		}
		return targets
	}
	return nil
}

type revisionRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository revisionRepository) NextID(context.Context) (revision.ID, error) {
	return revision.ID("revision_" + uuid.NewString()), nil
}

func (repository revisionRepository) Save(ctx context.Context, item revision.Revision) error {
	encoded, err := persist.EncodeRevision(item)
	if err != nil {
		return err
	}
	_, err = repository.transaction.ExecContext(
		ctx,
		bind(repository.dialect,
			"INSERT INTO content_revisions (id, entry_id, version, created_at, payload) VALUES (?, ?, ?, ?, ?)"),
		item.ID, item.EntryID, item.Version, item.CreatedAt.UnixNano(), encoded,
	)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConflict, err)
	}
	return nil
}

func (repository revisionRepository) Get(ctx context.Context, id revision.ID) (revision.Revision, error) {
	var encoded []byte
	err := repository.transaction.QueryRowContext(
		ctx, bind(repository.dialect, "SELECT payload FROM content_revisions WHERE id = ?"), id,
	).Scan(&encoded)
	if errors.Is(err, sql.ErrNoRows) {
		return revision.Revision{}, ErrNotFound
	}
	if err != nil {
		return revision.Revision{}, err
	}
	item, err := persist.DecodeRevision(encoded)
	if err != nil {
		return revision.Revision{}, err
	}
	return item, nil
}

func (repository revisionRepository) List(
	ctx context.Context,
	entryID content.ID,
	page int,
	perPage int,
) ([]revision.Revision, contentapplication.Page, error) {
	if page < 1 || perPage < 1 {
		return nil, contentapplication.Page{}, errors.New("invalid pagination")
	}
	var total int
	if err := repository.transaction.QueryRowContext(
		ctx, bind(repository.dialect, "SELECT COUNT(*) FROM content_revisions WHERE entry_id = ?"), entryID,
	).Scan(&total); err != nil {
		return nil, contentapplication.Page{}, err
	}
	offset := (page - 1) * perPage
	statement := bind(repository.dialect,
		"SELECT payload FROM content_revisions WHERE entry_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?",
	)
	rows, err := repository.transaction.QueryContext(ctx, statement, entryID, perPage, offset)
	if err != nil {
		return nil, contentapplication.Page{}, err
	}
	defer rows.Close()
	items := make([]revision.Revision, 0, perPage)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, contentapplication.Page{}, err
		}
		item, err := persist.DecodeRevision(encoded)
		if err != nil {
			return nil, contentapplication.Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, contentapplication.Page{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return items, contentapplication.Page{
		Number: page, PerPage: perPage, Total: total, TotalPages: totalPages,
	}, nil
}

func bind(dialect Dialect, statement string) string {
	if dialect != DialectPostgreSQL {
		return statement
	}
	var result strings.Builder
	index := 1
	for _, character := range statement {
		if character == '?' {
			result.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		result.WriteRune(character)
	}
	return result.String()
}

func decodeEntry(encoded []byte) (content.Entry, error) {
	return persist.DecodeEntry(encoded)
}

func contentListWhere(query contentapplication.Query) (string, []any) {
	conditions := make([]string, 0, 10)
	arguments := make([]any, 0, 12)
	if query.PublicOnly {
		conditions = append(conditions,
			"e.status = ?", "e.visibility = ?",
			"i.deleted_at IS NULL", "(i.published_at IS NULL OR i.published_at <= ?)",
		)
		arguments = append(arguments,
			content.StatusPublished, content.VisibilityPublic, query.PublicAt.UnixNano(),
		)
	}
	if len(query.Kinds) > 0 {
		conditions = append(conditions, "e.kind IN ("+placeholders(len(query.Kinds))+")")
		for _, kind := range query.Kinds {
			arguments = append(arguments, kind)
		}
	}
	if len(query.Statuses) > 0 {
		conditions = append(conditions, "e.status IN ("+placeholders(len(query.Statuses))+")")
		for _, status := range query.Statuses {
			arguments = append(arguments, status)
		}
	}
	if query.AuthorID != "" {
		conditions = append(conditions, "e.author_id = ?")
		arguments = append(arguments, query.AuthorID)
	}
	if query.After != nil {
		conditions = append(conditions, "i.updated_at >= ?")
		arguments = append(arguments, query.After.UnixNano())
	}
	if query.Before != nil {
		conditions = append(conditions, "i.updated_at <= ?")
		arguments = append(arguments, query.Before.UnixNano())
	}
	if query.PublishBefore != nil {
		conditions = append(conditions, "i.published_at IS NOT NULL", "i.published_at <= ?")
		arguments = append(arguments, query.PublishBefore.UnixNano())
	}
	if query.TaxonomyID != "" || query.TermID != "" {
		termConditions := []string{"ct.entry_id = e.id"}
		if query.TaxonomyID != "" {
			termConditions = append(termConditions, "ct.taxonomy_id = ?")
			arguments = append(arguments, query.TaxonomyID)
		}
		if query.TermID != "" {
			termConditions = append(termConditions, "ct.term_id = ?")
			arguments = append(arguments, query.TermID)
		}
		conditions = append(conditions,
			"EXISTS (SELECT 1 FROM content_terms ct WHERE "+strings.Join(termConditions, " AND ")+")",
		)
	}
	if query.RelationField != "" && query.RelatedID != "" {
		conditions = append(conditions,
			`EXISTS (
				SELECT 1 FROM content_relations cr
				WHERE cr.entry_id = e.id AND cr.field_id = ? AND cr.related_id = ?
			)`,
		)
		arguments = append(arguments, query.RelationField, query.RelatedID)
	}
	if search := strings.ToLower(strings.TrimSpace(query.Search)); search != "" {
		locale := strings.ToLower(strings.TrimSpace(query.Locale))
		if locale == "" {
			locale = "en"
		}
		pattern := "%" + escapeLike(search) + "%"
		conditions = append(conditions,
			`(EXISTS (SELECT 1 FROM content_search cs
			    WHERE cs.entry_id = e.id AND cs.locale = ? AND cs.search_text LIKE ? ESCAPE '!')
			  OR (NOT EXISTS (SELECT 1 FROM content_search requested
			      WHERE requested.entry_id = e.id AND requested.locale = ?)
			    AND EXISTS (SELECT 1 FROM content_search fallback
			      WHERE fallback.entry_id = e.id AND fallback.locale = 'en'
			        AND fallback.search_text LIKE ? ESCAPE '!')))`,
		)
		arguments = append(arguments, locale, pattern, locale, pattern)
	}
	if len(conditions) == 0 {
		return "", arguments
	}
	return " WHERE " + strings.Join(conditions, " AND "), arguments
}

func placeholders(count int) string {
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	return strings.ReplaceAll(value, "_", "!_")
}

type migration struct {
	version    int
	statements []string
}

func migrations(dialect Dialect) []migration {
	payloadType := "TEXT"
	if dialect == DialectMySQL {
		payloadType = "LONGTEXT"
	}
	return []migration{
		{version: 1, statements: []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS content_entries (
				id VARCHAR(64) PRIMARY KEY,
				kind VARCHAR(63) NOT NULL,
				status VARCHAR(31) NOT NULL,
				visibility VARCHAR(31) NOT NULL,
				author_id VARCHAR(64) NOT NULL,
				version BIGINT NOT NULL,
				updated_at BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
			`CREATE TABLE IF NOT EXISTS content_slugs (
				kind VARCHAR(63) NOT NULL,
				locale VARCHAR(35) NOT NULL,
				slug VARCHAR(191) NOT NULL,
				entry_id VARCHAR(64) NOT NULL,
				PRIMARY KEY (kind, locale, slug)
			)`,
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS content_revisions (
				id VARCHAR(64) PRIMARY KEY,
				entry_id VARCHAR(64) NOT NULL,
				version BIGINT NOT NULL,
				created_at BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS audit_events (
				id VARCHAR(64) PRIMARY KEY,
				occurred_at BIGINT NOT NULL,
				actor_id VARCHAR(64) NOT NULL,
				action VARCHAR(127) NOT NULL,
				resource VARCHAR(63) NOT NULL,
				resource_id VARCHAR(64) NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
		}},
		{version: 2, statements: []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS taxonomy_definitions (
				id VARCHAR(63) PRIMARY KEY,
				version BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS taxonomy_terms (
				id VARCHAR(64) PRIMARY KEY,
				taxonomy_id VARCHAR(63) NOT NULL,
				version BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
		}},
		{version: 3, statements: []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS identity_users (
				id VARCHAR(64) PRIMARY KEY,
				email VARCHAR(254) NOT NULL UNIQUE,
				version BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS identity_roles (
				id VARCHAR(63) PRIMARY KEY,
				version BIGINT NOT NULL,
				payload %s NOT NULL
			)`, payloadType),
		}},
		{version: 4, statements: []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS content_index (
				entry_id VARCHAR(64) PRIMARY KEY,
				id_sort VARCHAR(128) NOT NULL,
				created_at BIGINT NOT NULL,
				updated_at BIGINT NOT NULL,
				published_at BIGINT NULL,
				deleted_at BIGINT NULL,
				title_sort %s NOT NULL
			)`, payloadType),
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS content_search (
				entry_id VARCHAR(64) NOT NULL,
				locale VARCHAR(35) NOT NULL,
				search_text %s NOT NULL,
				PRIMARY KEY (entry_id, locale)
			)`, payloadType),
			`CREATE TABLE IF NOT EXISTS content_terms (
				taxonomy_id VARCHAR(63) NOT NULL,
				term_id VARCHAR(64) NOT NULL,
				entry_id VARCHAR(64) NOT NULL,
				PRIMARY KEY (taxonomy_id, term_id, entry_id)
			)`,
			`CREATE TABLE IF NOT EXISTS content_relations (
				entry_id VARCHAR(64) NOT NULL,
				field_id VARCHAR(63) NOT NULL,
				related_id VARCHAR(64) NOT NULL,
				PRIMARY KEY (entry_id, field_id, related_id)
			)`,
		}},
		{version: 5, statements: []string{
			`CREATE TABLE IF NOT EXISTS content_relations (
				entry_id VARCHAR(64) NOT NULL,
				field_id VARCHAR(63) NOT NULL,
				related_id VARCHAR(64) NOT NULL,
				PRIMARY KEY (entry_id, field_id, related_id)
			)`,
		}},
		{version: 6, statements: []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS content_locales (
				entry_id VARCHAR(64) NOT NULL,
				locale VARCHAR(35) NOT NULL,
				status VARCHAR(31) NOT NULL,
				updated_at BIGINT NOT NULL,
				data %s NOT NULL,
				PRIMARY KEY (entry_id, locale)
			)`, payloadType),
		}},
	}
}

func ensureContentIndexes(ctx context.Context, transaction *sql.Tx, dialect Dialect) error {
	indexes := []struct {
		name    string
		table   string
		columns string
	}{
		{name: "idx_content_entries_kind_status", table: "content_entries", columns: "kind, status"},
		{name: "idx_content_entries_author", table: "content_entries", columns: "author_id"},
		{name: "idx_content_index_updated", table: "content_index", columns: "updated_at, id_sort"},
		{name: "idx_content_index_created", table: "content_index", columns: "created_at, id_sort"},
		{name: "idx_content_index_published", table: "content_index", columns: "published_at, id_sort"},
		{name: "idx_content_relations_lookup", table: "content_relations", columns: "field_id, related_id, entry_id"},
	}
	for _, index := range indexes {
		if dialect == DialectMySQL {
			var count int
			if err := transaction.QueryRowContext(
				ctx,
				`SELECT COUNT(*) FROM information_schema.statistics
				 WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`,
				index.table, index.name,
			).Scan(&count); err != nil {
				return err
			}
			if count > 0 {
				continue
			}
			if _, err := transaction.ExecContext(
				ctx,
				"CREATE INDEX "+index.name+" ON "+index.table+" ("+index.columns+")",
			); err != nil {
				return err
			}
			continue
		}
		if _, err := transaction.ExecContext(
			ctx,
			"CREATE INDEX IF NOT EXISTS "+index.name+" ON "+index.table+" ("+index.columns+")",
		); err != nil {
			return err
		}
	}
	return nil
}

func backfillContentProjections(ctx context.Context, transaction *sql.Tx, dialect Dialect) error {
	repository := contentRepository{transaction: transaction, dialect: dialect}
	lastID := ""
	for {
		rows, err := transaction.QueryContext(
			ctx,
			bind(dialect,
				"SELECT id, payload FROM content_entries WHERE id > ? ORDER BY id LIMIT 100"),
			lastID,
		)
		if err != nil {
			return err
		}
		entries := make([]content.Entry, 0, 100)
		for rows.Next() {
			var id string
			var encoded []byte
			if err := rows.Scan(&id, &encoded); err != nil {
				_ = rows.Close()
				return err
			}
			entry, err := decodeEntry(encoded)
			if err != nil {
				_ = rows.Close()
				return err
			}
			entries = append(entries, entry)
			lastID = id
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, entry := range entries {
			if err := repository.replaceProjections(ctx, entry); err != nil {
				return err
			}
		}
		if len(entries) < 100 {
			return nil
		}
	}
}

func backfillContentLocales(ctx context.Context, transaction *sql.Tx, dialect Dialect) error {
	repository := contentRepository{transaction: transaction, dialect: dialect}
	lastID := ""
	for {
		rows, err := transaction.QueryContext(
			ctx,
			bind(dialect,
				"SELECT id, payload FROM content_entries WHERE id > ? ORDER BY id LIMIT 100"),
			lastID,
		)
		if err != nil {
			return err
		}
		entries := make([]content.Entry, 0, 100)
		for rows.Next() {
			var id string
			var encoded []byte
			if err := rows.Scan(&id, &encoded); err != nil {
				_ = rows.Close()
				return err
			}
			entry, err := decodeEntry(encoded)
			if err != nil {
				_ = rows.Close()
				return err
			}
			entries = append(entries, entry)
			lastID = id
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
		for _, entry := range entries {
			if err := repository.replaceLocales(ctx, entry); err != nil {
				return err
			}
		}
		if len(entries) < 100 {
			return nil
		}
	}
}
