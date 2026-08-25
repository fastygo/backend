package sqlstore

import (
	"context"
	"errors"
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
	err = adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
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
	err := adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		if err := transaction.Content().Create(context.Background(), sqlEntry(time.Now().UTC())); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("unexpected rollback result: %v", err)
	}
	err = adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
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

func TestPostgreSQLPlaceholderBinding(t *testing.T) {
	statement := bind(DialectPostgreSQL, "SELECT * FROM entries WHERE id = ? AND version = ?")
	if statement != "SELECT * FROM entries WHERE id = $1 AND version = $2" {
		t.Fatalf("unexpected PostgreSQL binding: %s", statement)
	}
	if bind(DialectMySQL, "SELECT ?") != "SELECT ?" {
		t.Fatalf("MySQL placeholders must remain unchanged")
	}
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
