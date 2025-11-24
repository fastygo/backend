package redis

import (
	"context"
	"time"

	goRedis "github.com/redis/go-redis/v9"

	"github.com/e-go/backend/internal/config"
)

// NewClient creates a Redis client and performs a health check.
func NewClient(cfg config.RedisConfig) (*goRedis.Client, error) {
	opts, err := goRedis.ParseURL(cfg.URL)
	if err != nil {
		return nil, err
	}

	if cfg.Password != "" {
		opts.Password = cfg.Password
	}
	if cfg.DB != 0 {
		opts.DB = cfg.DB
	}

	client := goRedis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	return client, nil
}
