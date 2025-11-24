package postgres

import (
	"database/sql"
	"fmt"
	"path/filepath"

	_ "github.com/lib/pq"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"go.uber.org/zap"

	"github.com/fastygo/backend/internal/config"
)

// RunMigrations executes DB migrations when enabled in configuration.
func RunMigrations(cfg *config.Config, logger *zap.Logger) error {
	if cfg == nil || !cfg.Migrations.Enabled {
		return nil
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	dsn := cfg.Database.URL
	if dsn == "" {
		dsn = fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.Database.User,
			cfg.Database.Password,
			cfg.Database.Host,
			cfg.Database.Port,
			cfg.Database.Name,
			cfg.Database.SSLMode,
		)
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	if err := sqlDB.Ping(); err != nil {
		return err
	}

	driver, err := postgres.WithInstance(sqlDB, &postgres.Config{})
	if err != nil {
		return err
	}

	sourceURL := fmt.Sprintf("file://%s", filepath.ToSlash(cfg.Migrations.Path))
	m, err := migrate.NewWithDatabaseInstance(sourceURL, cfg.Database.Name, driver)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return err
	}

	logger.Info("database migrations applied")
	return nil
}
