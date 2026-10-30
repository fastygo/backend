package bootstrap

import (
	"context"
	"fmt"
	"strings"

	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/tenant"
)

// LookupTenant returns the named tenant spec (implicit `default` when
// HEADLESS_TENANTS_PATH is unset).
func LookupTenant(config Config, id string) (tenant.Spec, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return tenant.Spec{}, fmt.Errorf("--tenant is required")
	}
	specs, err := resolvedTenants(config)
	if err != nil {
		return tenant.Spec{}, err
	}
	for _, spec := range specs {
		if spec.ID == id {
			return spec, nil
		}
	}
	return tenant.Spec{}, fmt.Errorf("tenant %q was not found", id)
}

// OpenTenant opens one tenant's storage and manifest without starting HTTP.
func OpenTenant(ctx context.Context, config Config, spec tenant.Spec) (Storage, schema.Manifest, error) {
	manifest, err := tenantManifest(config, spec)
	if err != nil {
		return nil, schema.Manifest{}, err
	}
	storage, err := OpenStorage(ctx, tenantStorageConfig(config, spec))
	if err != nil {
		return nil, schema.Manifest{}, fmt.Errorf("tenant %q storage: %w", spec.ID, err)
	}
	return storage, manifest, nil
}

func tenantStorageConfig(config Config, spec tenant.Spec) Config {
	siteConfig := config
	siteConfig.Storage = spec.Storage.ResolveDriver(config.Storage)
	if dsn := strings.TrimSpace(spec.Storage.DSN); dsn != "" {
		siteConfig.DataSource = dsn
	}
	if path := strings.TrimSpace(spec.Storage.Path); path != "" {
		siteConfig.BboltPath = path
	}
	if root := strings.TrimSpace(spec.MediaRoot); root != "" {
		siteConfig.MediaRoot = root
	}
	return siteConfig
}

func tenantManifest(config Config, spec tenant.Spec) (schema.Manifest, error) {
	if path := strings.TrimSpace(spec.Manifest); path != "" {
		return loadManifest(path)
	}
	return config.Manifest, nil
}
