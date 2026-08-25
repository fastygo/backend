package sqlstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/content"
)

func TestSQLitePersistsContentRevisionsAndSlugs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.db")
	adapter := openSQLite(t, path)
	now := time.Date(2026, time.August, 25, 13, 0, 0, 0, time.UTC)
	service := newSQLService(t, adapter, now)
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)
	created, err := service.Create(context.Background(), principal, sqlEntry(now))
	if err != nil {
		t.Fatalf("create content: %v", err)
	}
	created.Title = content.LocalizedText{"en": "Updated"}
	if _, err := service.Update(context.Background(), principal, created, 1, "SQL revision"); err != nil {
		t.Fatalf("update content: %v", err)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("close SQLite: %v", err)
	}

	adapter = openSQLite(t, path)
	t.Cleanup(func() { _ = adapter.Close() })
	service = newSQLService(t, adapter, now)
	resolved, err := service.Get(context.Background(), principal, created.ID)
	if err != nil {
		t.Fatalf("read reopened content: %v", err)
	}
	if resolved.Version != 2 || resolved.Title["en"] != "Updated" {
		t.Fatalf("unexpected reopened entry: %#v", resolved)
	}
	err = adapter.WithinContentTransaction(context.Background(), func(transaction application.Transaction) error {
		bySlug, err := transaction.Content().GetBySlug(context.Background(), "product", "en", "product-one")
		if err != nil || bySlug.ID != created.ID {
			t.Fatalf("slug index failed: entry=%#v err=%v", bySlug, err)
		}
		revisions, page, err := transaction.Revisions().List(context.Background(), created.ID, 1, 20)
		if err != nil {
			return err
		}
		if page.Total != 1 || len(revisions) != 1 || revisions[0].Version != 1 {
			t.Fatalf("revision persistence failed")
		}
		events, auditPage, err := transaction.Audit().List(context.Background(), application.AuditQuery{
			ResourceID: string(created.ID), Page: 1, PerPage: 20,
		})
		if err != nil {
			return err
		}
		if auditPage.Total != 2 || len(events) != 2 || events[0].AfterVersion != 2 {
			t.Fatalf("audit persistence failed")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect reopened database: %v", err)
	}
}

func TestSQLiteTransactionRollbackAndVersionConflict(t *testing.T) {
	adapter := openSQLite(t, filepath.Join(t.TempDir(), "rollback.db"))
	t.Cleanup(func() { _ = adapter.Close() })
	sentinel := errors.New("abort")
	err := adapter.WithinContentTransaction(context.Background(), func(transaction application.Transaction) error {
		if err := transaction.Content().Create(context.Background(), sqlEntry(time.Now().UTC())); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("unexpected rollback result: %v", err)
	}
	err = adapter.WithinContentTransaction(context.Background(), func(transaction application.Transaction) error {
		_, err := transaction.Content().Get(context.Background(), "product_1")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("transaction was not rolled back: %v", err)
		}
		entry := sqlEntry(time.Now().UTC())
		if err := transaction.Content().Create(context.Background(), entry); err != nil {
			return err
		}
		entry.Version = 2
		if err := transaction.Content().Update(context.Background(), entry, 99); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale version was accepted: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify SQL contracts: %v", err)
	}
}

func TestSQLiteListFiltersSortsAndPaginatesInSQL(t *testing.T) {
	adapter := openSQLite(t, filepath.Join(t.TempDir(), "list.db"))
	t.Cleanup(func() { _ = adapter.Close() })
	now := time.Date(2026, time.August, 25, 13, 0, 0, 0, time.UTC)
	entries := []content.Entry{
		sqlEntry(now),
		{
			ID: "product_2", Kind: "product", Status: content.StatusPublished,
			Visibility: content.VisibilityPublic, AuthorID: "author-2",
			Title:   content.LocalizedText{"en": "Alpha", "ru": "Альфа"},
			Content: content.LocalizedText{"en": "Needle description", "ru": "Описание"},
			Terms:    []content.TermRef{{Taxonomy: "catalog", TermID: "featured"}},
			Metadata: map[string]content.MetadataValue{"brand": {Value: "brand_1"}},
			Version:  1, CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
			PublishedAt: timePointer(now),
		},
		{
			ID: "product_3", Kind: "product", Status: content.StatusDraft,
			Visibility: content.VisibilityPublic,
			Title:      content.LocalizedText{"en": "Hidden needle"},
			Version:    1, CreatedAt: now.Add(2 * time.Minute), UpdatedAt: now.Add(2 * time.Minute),
		},
		{
			ID: "product_4", Kind: "product", Status: content.StatusPublished,
			Visibility: content.VisibilityPublic, AuthorID: "legacy",
			Title:   content.LocalizedText{"en": "Zebra"},
			Version: 1, CreatedAt: now.Add(3 * time.Minute), UpdatedAt: now.Add(3 * time.Minute),
		},
		{
			ID: "product_5", Kind: "product", Status: content.StatusPublished,
			Visibility: content.VisibilityPublic, AuthorID: "legacy",
			Title:   content.LocalizedText{"en": "Deleted"},
			Version: 1, CreatedAt: now.Add(4 * time.Minute), UpdatedAt: now.Add(4 * time.Minute),
			DeletedAt: timePointer(now),
		},
		{
			ID: "product_6", Kind: "product", Status: content.StatusPublished,
			Visibility: content.VisibilityPublic, AuthorID: "legacy",
			Title:   content.LocalizedText{"en": "apple"},
			Version: 1, CreatedAt: now.Add(5 * time.Minute), UpdatedAt: now.Add(5 * time.Minute),
		},
	}
	err := adapter.WithinContentTransaction(context.Background(), func(transaction application.Transaction) error {
		for _, entry := range entries {
			if err := transaction.Content().Create(context.Background(), entry); err != nil {
				return err
			}
		}
		result, err := transaction.Content().List(context.Background(), application.Query{
			Kinds: []content.Kind{"product"}, Search: "needle", Locale: "en",
			PublicOnly: true, PublicAt: now.Add(time.Hour),
			Page: 1, PerPage: 1, Sort: "title",
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 1 || result.Page.TotalPages != 1 ||
			len(result.Entries) != 1 || result.Entries[0].ID != "product_2" {
			t.Fatalf("unexpected SQL search page: %#v", result)
		}
		result, err = transaction.Content().List(context.Background(), application.Query{
			TaxonomyID: "catalog", TermID: "featured", Page: 1, PerPage: 10,
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 1 || result.Entries[0].ID != "product_2" {
			t.Fatalf("unexpected SQL taxonomy page: %#v", result)
		}
		result, err = transaction.Content().List(context.Background(), application.Query{
			RelationField: "brand", RelatedID: "brand_1", Page: 1, PerPage: 10,
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 1 || result.Entries[0].ID != "product_2" {
			t.Fatalf("unexpected SQL relation page: %#v", result)
		}
		result, err = transaction.Content().List(context.Background(), application.Query{
			AuthorID: "legacy", PublicOnly: true, PublicAt: now.Add(time.Hour),
			Page: 1, PerPage: 10, Sort: "title",
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 2 || len(result.Entries) != 2 ||
			result.Entries[0].ID != "product_4" || result.Entries[1].ID != "product_6" {
			t.Fatalf("SQL public or title-sort contract diverged: %#v", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("list content through SQL projections: %v", err)
	}
}

func TestSQLiteMigrationBackfillsContentProjections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy SQLite database: %v", err)
	}
	if _, err := database.Exec(`CREATE TABLE schema_migrations (
		version BIGINT PRIMARY KEY,
		applied_at BIGINT NOT NULL
	)`); err != nil {
		t.Fatalf("create legacy migration table: %v", err)
	}
	for _, migration := range migrations(DialectSQLite)[:3] {
		for _, statement := range migration.statements {
			if _, err := database.Exec(statement); err != nil {
				t.Fatalf("apply legacy migration %d: %v", migration.version, err)
			}
		}
		if _, err := database.Exec(
			"INSERT INTO schema_migrations (version, applied_at) VALUES (?, 0)",
			migration.version,
		); err != nil {
			t.Fatalf("record legacy migration %d: %v", migration.version, err)
		}
	}
	entry := sqlEntry(time.Now().UTC())
	entry.Terms = []content.TermRef{{Taxonomy: "catalog", TermID: "legacy"}}
	for index := range 105 {
		entry.ID = content.ID(fmt.Sprintf("legacy_%03d", index))
		entry.Slug = content.LocalizedText{"en": string(entry.ID)}
		encoded, err := json.Marshal(entry)
		if err != nil {
			t.Fatalf("encode legacy content %d: %v", index, err)
		}
		if _, err := database.Exec(
			`INSERT INTO content_entries
			 (id, kind, status, visibility, author_id, version, updated_at, payload)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.ID, entry.Kind, entry.Status, entry.Visibility, entry.AuthorID,
			entry.Version, entry.UpdatedAt.UnixNano(), encoded,
		); err != nil {
			t.Fatalf("insert legacy content %d: %v", index, err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close legacy SQLite database: %v", err)
	}

	adapter := openSQLite(t, path)
	t.Cleanup(func() { _ = adapter.Close() })
	err = adapter.WithinContentTransaction(context.Background(), func(transaction application.Transaction) error {
		result, err := transaction.Content().List(context.Background(), application.Query{
			Search: "description", TaxonomyID: "catalog", TermID: "legacy",
			Page: 1, PerPage: 200,
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 105 || len(result.Entries) != 105 {
			t.Fatalf("migration did not backfill content projections: %#v", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify upgraded SQLite database: %v", err)
	}
}

func TestPostgreSQLPlaceholderBinding(t *testing.T) {
	statement := bind(DialectPostgreSQL, "SELECT * FROM entries WHERE id = ? AND version = ?")
	if statement != "SELECT * FROM entries WHERE id = $1 AND version = $2" {
		t.Fatalf("unexpected PostgreSQL binding: %s", statement)
	}
	if bind(DialectMySQL, "SELECT ?") != "SELECT ?" {
		t.Fatalf("MySQL placeholders must remain unchanged")
	}
}

func TestLiveSQLDialects(t *testing.T) {
	tests := map[string]struct {
		environment string
		driver      string
		dialect     Dialect
	}{
		"MariaDB": {
			environment: "TEST_MARIADB_DSN",
			driver:      "mysql",
			dialect:     DialectMySQL,
		},
		"MySQL": {
			environment: "TEST_MYSQL_DSN",
			driver:      "mysql",
			dialect:     DialectMySQL,
		},
		"PostgreSQL": {
			environment: "TEST_POSTGRES_DSN",
			driver:      "pgx",
			dialect:     DialectPostgreSQL,
		},
	}
	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			dataSource := os.Getenv(test.environment)
			if dataSource == "" {
				t.Skip(test.environment + " is not configured")
			}
			ctx := context.Background()
			adapter, err := Open(ctx, test.driver, dataSource, test.dialect)
			if err != nil {
				t.Fatalf("open live %s database: %v", name, err)
			}
			t.Cleanup(func() { _ = adapter.Close() })
			entry := sqlEntry(time.Now().UTC())
			entry.ID = content.ID(fmt.Sprintf("live_%s_%d", test.dialect, time.Now().UnixNano()))
			entry.Slug = content.LocalizedText{"en": string(entry.ID)}
			entry.Title = content.LocalizedText{"en": "Live indexed " + string(entry.ID)}
			entry.Terms = []content.TermRef{{Taxonomy: "live", TermID: "indexed"}}
			upper := entry
			upper.ID += "_upper"
			upper.Slug = content.LocalizedText{"en": string(upper.ID)}
			upper.Title = content.LocalizedText{"en": "Zebra"}
			upper.AuthorID = string(entry.ID)
			upper.PublishedAt = nil
			upper.Terms = nil
			lower := upper
			lower.ID = entry.ID + "_lower"
			lower.Slug = content.LocalizedText{"en": string(lower.ID)}
			lower.Title = content.LocalizedText{"en": "apple"}
			entries := []content.Entry{entry, upper, lower}
			err = adapter.WithinContentTransaction(ctx, func(transaction application.Transaction) error {
				for _, candidate := range entries {
					if err := transaction.Content().Create(ctx, candidate); err != nil {
						return err
					}
				}
				result, err := transaction.Content().List(ctx, application.Query{
					Kinds: []content.Kind{"product"}, Search: string(entry.ID),
					TaxonomyID: "live", TermID: "indexed",
					Page: 1, PerPage: 10,
				})
				if err != nil {
					return err
				}
				found := false
				for _, resolved := range result.Entries {
					found = found || resolved.ID == entry.ID
				}
				if !found {
					t.Fatalf("live SQL query did not return indexed content: %#v", result)
				}
				result, err = transaction.Content().List(ctx, application.Query{
					AuthorID: string(entry.ID), PublicOnly: true, PublicAt: time.Now().Add(time.Hour),
					Page: 1, PerPage: 10, Sort: "title",
				})
				if err != nil {
					return err
				}
				if result.Page.Total != 2 || len(result.Entries) != 2 ||
					result.Entries[0].ID != upper.ID || result.Entries[1].ID != lower.ID {
					t.Fatalf("live SQL public or title-sort contract diverged: %#v", result)
				}
				for _, candidate := range entries {
					if err := transaction.Content().Delete(ctx, candidate.ID, candidate.Version); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("verify live %s database: %v", name, err)
			}
		})
	}
}

func timePointer(value time.Time) *time.Time {
	return &value
}

type sqlClock struct {
	value time.Time
}

func (clock sqlClock) Now() time.Time {
	return clock.value
}

func openSQLite(t *testing.T, path string) *Adapter {
	t.Helper()
	adapter, err := Open(context.Background(), "sqlite", path, DialectSQLite)
	if err != nil {
		t.Fatalf("open SQLite: %v", err)
	}
	return adapter
}

func newSQLService(t *testing.T, adapter *Adapter, now time.Time) *application.Service {
	t.Helper()
	service, err := application.NewService(adapter, nil, sqlClock{value: now})
	if err != nil {
		t.Fatalf("create SQL content service: %v", err)
	}
	return service
}

func sqlEntry(now time.Time) content.Entry {
	return content.Entry{
		ID: "product_1", Kind: "product", Status: content.StatusPublished, Visibility: content.VisibilityPublic,
		Slug: content.LocalizedText{"en": "product-one"}, Title: content.LocalizedText{"en": "Product One"},
		Content: content.LocalizedText{"en": "Description"}, Version: 1,
		CreatedAt: now, UpdatedAt: now, PublishedAt: &now,
	}
}
