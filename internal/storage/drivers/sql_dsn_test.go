//go:build mysql || postgres

package drivers

import (
	"context"
	"testing"

	"github.com/fastygo/backend/internal/storage/registry"
)

func TestSQLDriverRequiresDSN(t *testing.T) {
	for _, name := range compiledSQLDrivers() {
		if _, err := registry.Open(context.Background(), name, registry.Config{}); err == nil {
			t.Fatalf("%s: empty DATABASE_URL must fail", name)
		}
	}
}
