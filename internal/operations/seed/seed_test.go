package seed

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	application "github.com/fastygo/backend/internal/application/content"
	"github.com/fastygo/backend/internal/domain/authz"
	"github.com/fastygo/backend/internal/domain/schema"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
)

func TestApplyIsIdempotentForManifestRecords(t *testing.T) {
	t.Parallel()
	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "seed.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, nil)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if err := service.SetManifest(schema.Manifest{
		Name: "example", Version: "1",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products", Public: true,
			Fields: []schema.Field{{ID: "sku", Type: schema.FieldString}},
		}},
	}); err != nil {
		t.Fatalf("set manifest: %v", err)
	}
	document := []byte(`{
		"version":"fastygo.data.seed/v1",
		"records":[{
			"resource":"product",
			"idempotency_key":"seed-product-1",
			"values":{
				"title":"Course",
				"slug":"course",
				"status":"published",
				"visibility":"public",
				"locales":{"en":{"id":"gc-001"},"ru":{"id":"gc-001"}}
			}
		}]
	}`)
	principal := authz.AdministratorRole().Principal("seed")
	first, err := Apply(context.Background(), service, principal, bytes.NewReader(document))
	if err != nil || first.Created != 1 {
		t.Fatalf("first seed: %#v %v", first, err)
	}
	second, err := Apply(context.Background(), service, principal, bytes.NewReader(document))
	if err != nil || second.Created != 0 || second.Skipped != 1 {
		t.Fatalf("second seed: %#v %v", second, err)
	}
}
