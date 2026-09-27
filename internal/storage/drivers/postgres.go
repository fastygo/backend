//go:build postgres

package drivers

import (
	"context"
	"errors"

	"github.com/fastygo/backend/internal/storage/registry"
	"github.com/fastygo/backend/internal/storage/sqlstore"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func init() {
	registry.Register("postgres", func(ctx context.Context, config registry.Config) (any, error) {
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for PostgreSQL")
		}
		return sqlstore.Open(ctx, "pgx", config.DataSource, sqlstore.DialectPostgreSQL)
	})
}
