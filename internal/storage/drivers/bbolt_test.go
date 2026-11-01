//go:build bbolt || (!sqlite && !mysql && !postgres)

package drivers

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/fastygo/backend/internal/storage/registry"
)

func TestDefaultBuildRegistersBbolt(t *testing.T) {
	store, err := registry.Open(context.Background(), "bbolt", registry.Config{BboltPath: filepath.Join(t.TempDir(), "data.db")})
	if err != nil {
		t.Fatalf("open bbolt: %v", err)
	}
	closer, ok := store.(io.Closer)
	if !ok {
		t.Fatalf("store %T must be closable", store)
	}
	if err := closer.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Open(context.Background(), "sqlite", registry.Config{}); err == nil {
		t.Fatal("default build must not link sqlite")
	}
}
