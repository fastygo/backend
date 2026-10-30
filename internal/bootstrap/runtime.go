package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	application "github.com/fastygo/backend/internal/application/content"
	applicationidentity "github.com/fastygo/backend/internal/application/identity"
	applicationmedia "github.com/fastygo/backend/internal/application/media"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/delivery/graphqlapi"
	"github.com/fastygo/backend/internal/delivery/rest"
	"github.com/fastygo/backend/internal/domain/schema"
	"github.com/fastygo/backend/internal/identity"
	"github.com/fastygo/backend/internal/operations/backup"
	"github.com/fastygo/backend/internal/persist"
	"github.com/fastygo/backend/internal/platform"
	"github.com/fastygo/backend/internal/storage/localmedia"
	"github.com/fastygo/backend/internal/tenant"
	"github.com/fastygo/framework/pkg/app"
)

type Storage interface {
	application.Transactor
	applicationtaxonomy.Transactor
	applicationidentity.Transactor
	backup.Transactor
	platform.HealthResource
	platform.CloseResource
}

type Config struct {
	App               app.Config
	Storage           string
	DataSource        string
	BboltPath         string
	Manifest          schema.Manifest
	Principal         rest.PrincipalResolver
	TokenSecret       string
	TokenIssuer       string
	AdminEmail        string
	AdminPassword     string
	AllowInsecureAuth bool
	CookieSecure      bool
	MediaRoot         string
	MediaMaxBytes     int64
	ScheduleInterval  time.Duration
	TenantsPath       string
	Tenants           []tenant.Spec
}

type Runtime struct {
	App          *app.App
	ControlPlane *platform.ControlPlane
	storage      Storage
	sites        []*site
}

type site struct {
	spec     tenant.Spec
	mux      *http.ServeMux
	storage  Storage
	content  *application.Service
	manifest schema.Manifest
}

func LoadConfig() (Config, error) {
	loadDotEnv(".env")
	frameworkConfig, err := app.LoadConfig()
	if err != nil {
		return Config{}, err
	}
	if frameworkConfig.HealthLivePath == "" {
		frameworkConfig.HealthLivePath = "/healthz"
	}
	if frameworkConfig.HealthReadyPath == "" {
		frameworkConfig.HealthReadyPath = "/readyz"
	}
	manifest := DefaultManifest()
	if path := strings.TrimSpace(os.Getenv("HEADLESS_MANIFEST_PATH")); path != "" {
		manifest, err = loadManifest(path)
		if err != nil {
			return Config{}, err
		}
	}
	mediaMaxBytes, err := positiveInt64Env("HEADLESS_MEDIA_MAX_BYTES", 32<<20)
	if err != nil {
		return Config{}, err
	}
	scheduleInterval, err := positiveDurationEnv("HEADLESS_SCHEDULE_INTERVAL", time.Minute)
	if err != nil {
		return Config{}, err
	}
	config := Config{
		App:               frameworkConfig,
		Storage:           strings.ToLower(env("HEADLESS_STORAGE", "bbolt")),
		DataSource:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		BboltPath:         env("HEADLESS_BBOLT_PATH", "var/lib/headless/backend.db"),
		Manifest:          manifest,
		TokenSecret:       strings.TrimSpace(os.Getenv("HEADLESS_TOKEN_SECRET")),
		TokenIssuer:       env("HEADLESS_TOKEN_ISSUER", "headless-backend"),
		AdminEmail:        strings.TrimSpace(os.Getenv("HEADLESS_ADMIN_EMAIL")),
		AdminPassword:     os.Getenv("HEADLESS_ADMIN_PASSWORD"),
		AllowInsecureAuth: strings.EqualFold(env("HEADLESS_ALLOW_INSECURE_AUTH", "false"), "true"),
		CookieSecure:      strings.EqualFold(env("HEADLESS_COOKIE_SECURE", "false"), "true"),
		MediaRoot:         env("HEADLESS_MEDIA_ROOT", "var/lib/headless/media"),
		MediaMaxBytes:     mediaMaxBytes,
		ScheduleInterval:  scheduleInterval,
		TenantsPath:       strings.TrimSpace(os.Getenv("HEADLESS_TENANTS_PATH")),
	}
	if config.TenantsPath != "" {
		specs, err := tenant.LoadFile(config.TenantsPath)
		if err != nil {
			return Config{}, err
		}
		config.Tenants = specs
	}
	return config, nil
}

func Build(ctx context.Context, config Config) (*Runtime, error) {
	if err := config.Manifest.Validate(); err != nil {
		return nil, err
	}
	if config.Principal == nil && strings.TrimSpace(config.TokenSecret) == "" && !config.AllowInsecureAuth {
		return nil, errors.New("HEADLESS_TOKEN_SECRET is required unless insecure anonymous mode is explicitly enabled")
	}
	specs, err := resolvedTenants(config)
	if err != nil {
		return nil, err
	}
	sites := make([]*site, 0, len(specs))
	cleanup := func() {
		for _, item := range sites {
			_ = item.storage.Close()
		}
	}
	for _, spec := range specs {
		item, err := buildSite(ctx, config, spec)
		if err != nil {
			cleanup()
			return nil, err
		}
		sites = append(sites, item)
	}
	byHost := map[string]*site{}
	for _, item := range sites {
		for _, host := range item.spec.Hosts {
			byHost[tenant.NormalizeHost(host)] = item
		}
	}
	storages := make(resourceGroup, 0, len(sites))
	for _, item := range sites {
		storages = append(storages, item.storage)
	}
	router := &hostMux{multi: config.multi(), byHost: byHost, fallback: sites[0]}
	feature, err := platform.NewRuntimeFeature("headless-content", router, storages, storages)
	if err != nil {
		cleanup()
		return nil, err
	}
	controlPlane, err := platform.NewControlPlane("headless", "Content", "/go-admin")
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := controlPlane.RegisterManifest(sites[0].manifest); err != nil {
		cleanup()
		return nil, err
	}
	builder := app.New(config.App).
		DisableStatic().
		WithFeature(feature).
		WithHealthEndpoints(config.App.HealthLivePath, config.App.HealthReadyPath)
	interval := config.ScheduleInterval
	if interval <= 0 {
		interval = time.Minute
	}
	builder.AddBackgroundTask(app.BackgroundTask{
		Name: "scheduled-publication", Interval: interval,
		Run: func(ctx context.Context) {
			for _, item := range sites {
				if _, err := item.content.PublishDue(ctx, 100); err != nil {
					slog.Error("scheduled publication failed", "error", err, "tenant", item.spec.ID)
				}
			}
		},
	})
	if config.App.MetricsPath != "" {
		builder.WithMetricsEndpoint(config.App.MetricsPath)
	}
	return &Runtime{
		App: builder.Build(), ControlPlane: controlPlane,
		storage: sites[0].storage, sites: sites,
	}, nil
}

func (config Config) multi() bool {
	return len(config.Tenants) > 0
}

func resolvedTenants(config Config) ([]tenant.Spec, error) {
	if len(config.Tenants) > 0 {
		if err := tenant.Validate(config.Tenants, true); err != nil {
			return nil, err
		}
		return config.Tenants, nil
	}
	return []tenant.Spec{{
		ID: tenant.DefaultID,
		Storage: tenant.Storage{
			Driver: config.Storage,
			DSN:    config.DataSource,
			Path:   config.BboltPath,
		},
		MediaRoot: config.MediaRoot,
	}}, nil
}

func buildSite(ctx context.Context, config Config, spec tenant.Spec) (*site, error) {
	manifest := config.Manifest
	if path := strings.TrimSpace(spec.Manifest); path != "" {
		loaded, err := loadManifest(path)
		if err != nil {
			return nil, err
		}
		manifest = loaded
	}
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
	storage, err := OpenStorage(ctx, siteConfig)
	if err != nil {
		return nil, fmt.Errorf("tenant %q storage: %w", spec.ID, err)
	}
	fail := func(err error) (*site, error) {
		_ = storage.Close()
		return nil, err
	}
	service, err := application.NewService(storage, nil, nil)
	if err != nil {
		return fail(err)
	}
	if err := service.SetManifest(manifest); err != nil {
		return fail(err)
	}
	principal := config.Principal
	var tokenManager *identity.TokenManager
	if principal == nil && config.TokenSecret != "" {
		tokenManager, err = identity.NewTokenManager(config.TokenSecret, config.TokenIssuer)
		if err != nil {
			return fail(err)
		}
		if config.multi() {
			tokenManager.BindTenant(spec.ID)
		}
		principal = tokenManager
	}
	handler, err := rest.NewContentHandler(service, principal, manifest)
	if err != nil {
		return fail(err)
	}
	taxonomyService, err := applicationtaxonomy.NewService(storage, nil)
	if err != nil {
		return fail(err)
	}
	taxonomyHandler, err := rest.NewTaxonomyHandler(taxonomyService, principal)
	if err != nil {
		return fail(err)
	}
	codexHandler, err := rest.NewCodexHandler(service, taxonomyService, principal, manifest)
	if err != nil {
		return fail(err)
	}
	defaultLocale := config.App.DefaultLocale
	if language := spec.Language(); language != "" {
		defaultLocale = language
	}
	codexHandler.SetDefaultLocale(defaultLocale)
	locales := config.App.AvailableLocales
	if len(locales) == 0 && defaultLocale != "" {
		locales = []string{defaultLocale}
	}
	codexHandler.SetAvailableLocales(locales)
	collectionRouter, err := rest.NewCollectionRouter(codexHandler, handler)
	if err != nil {
		return fail(err)
	}
	identityService, err := applicationidentity.NewService(storage, tokenManager, nil)
	if err != nil {
		return fail(err)
	}
	if err := identityService.Initialize(ctx, config.AdminEmail, config.AdminPassword); err != nil {
		return fail(err)
	}
	identityHandler, err := rest.NewIdentityHandler(identityService, principal)
	if err != nil {
		return fail(err)
	}
	var sessionHandler platform.RouteRegistrar
	if tokenManager != nil {
		sessionHandler, err = rest.NewSessionHandler(identityService, tokenManager, config.CookieSecure)
		if err != nil {
			return fail(err)
		}
	}
	blobStore, err := localmedia.Open(siteConfig.MediaRoot)
	if err != nil {
		return fail(err)
	}
	mediaService, err := applicationmedia.NewService(service, blobStore)
	if err != nil {
		return fail(err)
	}
	mediaHandler, err := rest.NewMediaHandler(mediaService, principal, config.MediaMaxBytes)
	if err != nil {
		return fail(err)
	}
	graphQL, err := graphqlapi.New(service, manifest, principal)
	if err != nil {
		return fail(err)
	}
	routes := platform.RouteGroup{handler, codexHandler, collectionRouter, taxonomyHandler, identityHandler, mediaHandler, graphQL}
	if sessionHandler != nil {
		routes = append(routes, sessionHandler)
	}
	mux := http.NewServeMux()
	routes.Routes(mux)
	return &site{spec: spec, mux: mux, storage: storage, content: service, manifest: manifest}, nil
}

func (runtime *Runtime) Close() error {
	if runtime == nil {
		return nil
	}
	if len(runtime.sites) > 0 {
		var first error
		for _, item := range runtime.sites {
			if err := item.storage.Close(); err != nil && first == nil {
				first = err
			}
		}
		return first
	}
	if runtime.storage == nil {
		return nil
	}
	return runtime.storage.Close()
}

func DefaultManifest() schema.Manifest {
	return schema.WithCoreResources(schema.Manifest{Name: "headless", Version: "1"})
}

func loadDotEnv(path string) {
	encoded, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(encoded), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		}
		_ = os.Setenv(key, value)
	}
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loadManifest(path string) (schema.Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return schema.Manifest{}, fmt.Errorf("failed to open headless manifest: %w", err)
	}
	defer file.Close()
	var document persist.Manifest
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return schema.Manifest{}, fmt.Errorf("failed to decode headless manifest: %w", err)
	}
	manifest := schema.WithCoreResources(document.Domain())
	if err := manifest.Validate(); err != nil {
		return schema.Manifest{}, fmt.Errorf("failed to validate headless manifest: %w", err)
	}
	return manifest, nil
}

func positiveInt64Env(key string, fallback int64) (int64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	resolved, err := strconv.ParseInt(value, 10, 64)
	if err != nil || resolved <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return resolved, nil
}

func positiveDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	resolved, err := time.ParseDuration(value)
	if err != nil || resolved <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return resolved, nil
}
