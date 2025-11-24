package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/fastygo/backend/internal/config"
)

// NewPool creates and validates a pgx connection pool.
func NewPool(ctx context.Context, cfg config.DatabaseConfig, logger *zap.Logger) (*pgxpool.Pool, error) {
	if logger == nil {
		logger = zap.NewNop()
	}

	connString := cfg.URL
	if connString == "" {
		connString = fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=%s",
			cfg.User,
			cfg.Password,
			cfg.Host,
			cfg.Port,
			cfg.Name,
			cfg.SSLMode,
		)
	}

	pgxCfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	if cfg.MaxOpenConns > 0 {
		pgxCfg.MaxConns = int32(cfg.MaxOpenConns)
	}
	if cfg.MaxIdleConns > 0 {
		pgxCfg.MinConns = int32(cfg.MaxIdleConns)
	}
	if cfg.MaxConnLifetime > 0 {
		pgxCfg.MaxConnLifetime = cfg.MaxConnLifetime
	}

	pool, err := pgxpool.NewWithConfig(ctx, pgxCfg)
	if err != nil {
		return nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}

	logger.Info("connected to postgres", zap.String("host", cfg.Host), zap.String("db", cfg.Name))
	return pool, nil
}

// Close releases the pool and logs the result.
func Close(pool *pgxpool.Pool, logger *zap.Logger) {
	if pool == nil {
		return
	}
	pool.Close()
	if logger != nil {
		logger.Info("postgres pool closed")
	}
}
