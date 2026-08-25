package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/content"
	"github.com/fastygo/backend/internal/domain/revision"
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

func Open(ctx context.Context, driver, dataSource string, dialect Dialect) (*Adapter, error) {
	if !dialect.Valid() {
		return nil, errors.New("SQL dialect is invalid")
	}
	database, err := sql.Open(driver, dataSource)
	if err != nil {
		return nil, fmt.Errorf("open SQL database: %w", err)
	}
	if dialect == DialectSQLite {
		database.SetMaxOpenConns(1)
	}
	adapter := &Adapter{database: database, dialect: dialect}
	if err := adapter.Ping(ctx); err != nil {
		_ = database.Close()
		return nil, fmt.Errorf("ping SQL database: %w", err)
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
		return fmt.Errorf("begin SQL migration: %w", err)
	}
	defer func() { _ = transaction.Rollback() }()
	for _, statement := range migrationStatements(adapter.dialect) {
		if _, err := transaction.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("execute SQL migration: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("commit SQL migration: %w", err)
	}
	return nil
}

func (adapter *Adapter) WithinTransaction(ctx context.Context, operation func(application.Transaction) error) error {
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
	if err := operation(tx{transaction: transaction, dialect: adapter.dialect}); err != nil {
		return err
	}
	return transaction.Commit()
}

func (dialect Dialect) Valid() bool {
	return dialect == DialectSQLite || dialect == DialectMySQL || dialect == DialectPostgreSQL
}

type tx struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (transaction tx) Content() application.Repository {
	return contentRepository{transaction: transaction.transaction, dialect: transaction.dialect}
}

func (transaction tx) Revisions() application.RevisionRepository {
	return revisionRepository{transaction: transaction.transaction, dialect: transaction.dialect}
}

func (transaction tx) Audit() application.AuditRepository {
	return auditRepository{transaction: transaction.transaction, dialect: transaction.dialect}
}

func (transaction tx) Taxonomies() application.TaxonomyRepository {
	return taxonomyRepository{transaction: transaction.transaction, dialect: transaction.dialect}
}

func (transaction tx) Identity() application.IdentityRepository {
	return identityRepository{transaction: transaction.transaction, dialect: transaction.dialect}
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

func (repository contentRepository) List(ctx context.Context, query application.Query) (application.ListResult, error) {
	if query.Page < 1 || query.PerPage < 1 {
		return application.ListResult{}, errors.New("invalid pagination")
	}
	if query.PublicOnly && query.PublicAt.IsZero() {
		query.PublicAt = time.Now().UTC()
	}
	rows, err := repository.transaction.QueryContext(ctx, "SELECT payload FROM content_entries")
	if err != nil {
		return application.ListResult{}, err
	}
	defer rows.Close()
	entries := make([]content.Entry, 0)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return application.ListResult{}, err
		}
		entry, err := decodeEntry(encoded)
		if err != nil {
			return application.ListResult{}, err
		}
		if matches(entry, query) {
			entries = append(entries, entry)
		}
	}
	if err := rows.Err(); err != nil {
		return application.ListResult{}, err
	}
	sortEntries(entries, query.Sort, query.Descending)
	total := len(entries)
	start := min((query.Page-1)*query.PerPage, total)
	end := min(start+query.PerPage, total)
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(query.PerPage)))
	}
	return application.ListResult{
		Entries: append([]content.Entry(nil), entries[start:end]...),
		Page: application.Page{
			Number: query.Page, PerPage: query.PerPage, Total: total, TotalPages: totalPages,
		},
	}, nil
}

func (repository contentRepository) Create(ctx context.Context, entry content.Entry) error {
	encoded, err := json.Marshal(entry)
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
	return repository.replaceSlugs(ctx, entry)
}

func (repository contentRepository) Update(
	ctx context.Context,
	entry content.Entry,
	expectedVersion uint64,
) error {
	encoded, err := json.Marshal(entry)
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
	return repository.replaceSlugs(ctx, entry)
}

func (repository contentRepository) Delete(ctx context.Context, id content.ID, expectedVersion uint64) error {
	if _, err := repository.transaction.ExecContext(
		ctx, bind(repository.dialect, "DELETE FROM content_slugs WHERE entry_id = ?"), id,
	); err != nil {
		return err
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

type revisionRepository struct {
	transaction *sql.Tx
	dialect     Dialect
}

func (repository revisionRepository) NextID(context.Context) (revision.ID, error) {
	return revision.ID("revision_" + uuid.NewString()), nil
}

func (repository revisionRepository) Save(ctx context.Context, item revision.Revision) error {
	encoded, err := json.Marshal(item)
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
	var item revision.Revision
	if err := json.Unmarshal(encoded, &item); err != nil {
		return revision.Revision{}, err
	}
	return item, nil
}

func (repository revisionRepository) List(
	ctx context.Context,
	entryID content.ID,
	page int,
	perPage int,
) ([]revision.Revision, application.Page, error) {
	if page < 1 || perPage < 1 {
		return nil, application.Page{}, errors.New("invalid pagination")
	}
	var total int
	if err := repository.transaction.QueryRowContext(
		ctx, bind(repository.dialect, "SELECT COUNT(*) FROM content_revisions WHERE entry_id = ?"), entryID,
	).Scan(&total); err != nil {
		return nil, application.Page{}, err
	}
	offset := (page - 1) * perPage
	statement := bind(repository.dialect,
		"SELECT payload FROM content_revisions WHERE entry_id = ? ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?",
	)
	rows, err := repository.transaction.QueryContext(ctx, statement, entryID, perPage, offset)
	if err != nil {
		return nil, application.Page{}, err
	}
	defer rows.Close()
	items := make([]revision.Revision, 0, perPage)
	for rows.Next() {
		var encoded []byte
		if err := rows.Scan(&encoded); err != nil {
			return nil, application.Page{}, err
		}
		var item revision.Revision
		if err := json.Unmarshal(encoded, &item); err != nil {
			return nil, application.Page{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, application.Page{}, err
	}
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return items, application.Page{
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
	var entry content.Entry
	if err := json.Unmarshal(encoded, &entry); err != nil {
		return content.Entry{}, err
	}
	return entry, nil
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

func migrationStatements(dialect Dialect) []string {
	payloadType := "TEXT"
	if dialect == DialectMySQL {
		payloadType = "LONGTEXT"
	}
	return []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			applied_at BIGINT NOT NULL
		)`,
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
		`INSERT INTO schema_migrations (version, applied_at)
		 SELECT 1, 0 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 1)`,
		`INSERT INTO schema_migrations (version, applied_at)
		 SELECT 2, 0 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 2)`,
		`INSERT INTO schema_migrations (version, applied_at)
		 SELECT 3, 0 WHERE NOT EXISTS (SELECT 1 FROM schema_migrations WHERE version = 3)`,
	}
}
