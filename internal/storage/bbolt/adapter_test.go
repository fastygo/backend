package bbolt

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

func TestAdapterPersistsContentAndRevisionsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backend.db")
	now := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	adapter := openTestAdapter(t, path)
	service := newTestService(t, adapter, now)
	principal := authz.NewPrincipal(
		"editor",
		authz.CapabilityContentCreate,
		authz.CapabilityContentEditOwn,
		authz.CapabilityContentPublish,
		authz.CapabilityContentReadPrivate,
	)

	created, err := service.Create(context.Background(), principal, testEntry(now))
	if err != nil {
		t.Fatalf("create content: %v", err)
	}
	created.Title = content.LocalizedText{"en": "Durable title"}
	if _, err := service.Update(context.Background(), principal, created, 1, "durability test"); err != nil {
		t.Fatalf("update content: %v", err)
	}
	if err := adapter.Close(); err != nil {
		t.Fatalf("close adapter: %v", err)
	}

	adapter = openTestAdapter(t, path)
	t.Cleanup(func() { _ = adapter.Close() })
	service = newTestService(t, adapter, now)
	resolved, err := service.Get(context.Background(), principal, "post_1")
	if err != nil {
		t.Fatalf("read reopened content: %v", err)
	}
	if resolved.Version != 2 || resolved.Title["en"] != "Durable title" {
		t.Fatalf("unexpected durable content: %#v", resolved)
	}
	if err := adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		items, page, err := transaction.Revisions().List(context.Background(), resolved.ID, 1, 20)
		if err != nil {
			return err
		}
		if page.Total != 1 || len(items) != 1 || items[0].Version != 1 {
			t.Fatalf("revision was not persisted")
		}
		events, auditPage, err := transaction.Audit().List(context.Background(), application.AuditQuery{
			ResourceID: string(resolved.ID), Page: 1, PerPage: 20,
		})
		if err != nil {
			return err
		}
		if auditPage.Total != 2 || len(events) != 2 || events[0].AfterVersion != 2 {
			t.Fatalf("audit events were not persisted")
		}
		return nil
	}); err != nil {
		t.Fatalf("list revisions: %v", err)
	}
}

func TestAdapterRollsBackFailedTransaction(t *testing.T) {
	adapter := openTestAdapter(t, filepath.Join(t.TempDir(), "rollback.db"))
	t.Cleanup(func() { _ = adapter.Close() })
	sentinel := errors.New("abort")
	err := adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		if err := transaction.Content().Create(context.Background(), testEntry(time.Now().UTC())); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("unexpected rollback error: %v", err)
	}

	err = adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		_, err := transaction.Content().Get(context.Background(), "post_1")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("rolled back content remains visible: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("verify rollback: %v", err)
	}
}

func TestAdapterFiltersPublicContentAndTaxonomy(t *testing.T) {
	adapter := openTestAdapter(t, filepath.Join(t.TempDir(), "filter.db"))
	t.Cleanup(func() { _ = adapter.Close() })
	now := time.Now().UTC()
	err := adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		public := testEntry(now)
		public.Terms = []content.TermRef{{Taxonomy: "topic", TermID: "go"}}
		if err := transaction.Content().Create(context.Background(), public); err != nil {
			return err
		}
		draft := testEntry(now)
		draft.ID = "post_2"
		draft.Slug = content.LocalizedText{"en": "draft"}
		draft.Status = content.StatusDraft
		return transaction.Content().Create(context.Background(), draft)
	})
	if err != nil {
		t.Fatalf("seed content: %v", err)
	}

	err = adapter.WithinTransaction(context.Background(), func(transaction application.Transaction) error {
		result, err := transaction.Content().List(context.Background(), application.Query{
			Page: 1, PerPage: 10, PublicOnly: true, PublicAt: now,
			TaxonomyID: "topic", TermID: "go",
		})
		if err != nil {
			return err
		}
		if result.Page.Total != 1 || len(result.Entries) != 1 || result.Entries[0].ID != "post_1" {
			t.Fatalf("unexpected filtered result: %#v", result)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("filter content: %v", err)
	}
}

type testClock struct {
	value time.Time
}

func (clock testClock) Now() time.Time {
	return clock.value
}

func openTestAdapter(t *testing.T, path string) *Adapter {
	t.Helper()
	adapter, err := Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open adapter: %v", err)
	}
	return adapter
}

func newTestService(t *testing.T, adapter *Adapter, now time.Time) *application.Service {
	t.Helper()
	service, err := application.NewService(adapter, nil, testClock{value: now})
	if err != nil {
		t.Fatalf("create content service: %v", err)
	}
	return service
}

func testEntry(now time.Time) content.Entry {
	return content.Entry{
		ID: "post_1", Kind: content.KindPost, Status: content.StatusPublished, Visibility: content.VisibilityPublic,
		Slug: content.LocalizedText{"en": "hello"}, Title: content.LocalizedText{"en": "Hello"},
		Content: content.LocalizedText{"en": "Body"}, Version: 1,
		CreatedAt: now, UpdatedAt: now, PublishedAt: &now,
	}
}
