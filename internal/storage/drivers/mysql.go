//go:build mysql

package drivers

import (
	"context"
	"errors"

	"github.com/fastygo/backend/internal/storage/registry"
	"github.com/fastygo/backend/internal/storage/sqlstore"

	_ "github.com/go-sql-driver/mysql"
)

func init() {
	registry.Register("mysql", func(ctx context.Context, config registry.Config) (any, error) {
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for MySQL or MariaDB")
		}
		return sqlstore.Open(ctx, "mysql", config.DataSource, sqlstore.DialectMySQL)
	})
}
