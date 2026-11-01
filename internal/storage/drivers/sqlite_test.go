//go:build sqlite

package drivers

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/fastygo/backend/internal/storage/registry"
)

func TestSQLiteDriverRequiresDSNAndOpens(t *testing.T) {
	if _, err := registry.Open(context.Background(), "sqlite", registry.Config{}); err == nil {
		t.Fatal("empty DATABASE_URL must fail")
	}
	store, err := registry.Open(context.Background(), "sqlite", registry.Config{DataSource: filepath.Join(t.TempDir(), "nested", "data.db")})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if closer, ok := store.(io.Closer); ok {
		_ = closer.Close()
	}
}
