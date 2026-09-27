//go:build bbolt || (!sqlite && !mysql && !postgres)

package drivers

import (
	"context"

	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/backend/internal/storage/registry"
)

func init() {
	registry.Register("bbolt", func(_ context.Context, config registry.Config) (any, error) {
		return bboltstorage.Open(config.BboltPath, 0o600, nil)
	})
}
