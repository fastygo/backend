//go:build sqlite

package drivers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fastygo/backend/internal/storage/registry"
	"github.com/fastygo/backend/internal/storage/sqlstore"

	_ "modernc.org/sqlite"
)

func init() {
	registry.Register("sqlite", func(ctx context.Context, config registry.Config) (any, error) {
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for SQLite")
		}
		if err := os.MkdirAll(filepath.Dir(config.DataSource), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create SQLite directory: %w", err)
		}
		return sqlstore.Open(ctx, "sqlite", config.DataSource, sqlstore.DialectSQLite)
	})
}
