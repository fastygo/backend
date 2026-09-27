package bootstrap

import (
	"context"

	"github.com/fastygo/backend/internal/storage/registry"
)

func OpenStorage(ctx context.Context, config Config) (Storage, error) {
	opened, err := registry.Open(ctx, config.Storage, registry.Config{
		DataSource: config.DataSource,
		BboltPath:  config.BboltPath,
	})
	if err != nil {
		return nil, err
	}
	return opened.(Storage), nil
}
