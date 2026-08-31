package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/backend/internal/storage/localmedia"
	"github.com/fastygo/backend/internal/storage/sqlstore"
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
}

type Runtime struct {
	App          *app.App
	ControlPlane *platform.ControlPlane
	storage      Storage
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
	return Config{
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
	}, nil
}

func Build(ctx context.Context, config Config) (*Runtime, error) {
	if err := config.Manifest.Validate(); err != nil {
		return nil, err
	}
	if config.Principal == nil && strings.TrimSpace(config.TokenSecret) == "" && !config.AllowInsecureAuth {
		return nil, errors.New("HEADLESS_TOKEN_SECRET is required unless insecure anonymous mode is explicitly enabled")
	}
	storage, err := OpenStorage(ctx, config)
	if err != nil {
		return nil, err
	}
	service, err := application.NewService(storage, nil, nil)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	if err := service.SetManifest(config.Manifest); err != nil {
		_ = storage.Close()
		return nil, err
	}
	principal := config.Principal
	var tokenManager *identity.TokenManager
	if principal == nil && config.TokenSecret != "" {
		tokenManager, err = identity.NewTokenManager(config.TokenSecret, config.TokenIssuer)
		if err != nil {
			_ = storage.Close()
			return nil, err
		}
		principal = tokenManager
	}
	handler, err := rest.NewContentHandler(service, principal, config.Manifest)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	taxonomyService, err := applicationtaxonomy.NewService(storage, nil)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	taxonomyHandler, err := rest.NewTaxonomyHandler(taxonomyService, principal)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	codexHandler, err := rest.NewCodexHandler(service, taxonomyService, principal, config.Manifest)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	codexHandler.SetDefaultLocale(config.App.DefaultLocale)
	locales := config.App.AvailableLocales
	if len(locales) == 0 && config.App.DefaultLocale != "" {
		locales = []string{config.App.DefaultLocale}
	}
	codexHandler.SetAvailableLocales(locales)
	collectionRouter, err := rest.NewCollectionRouter(codexHandler, handler)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	identityService, err := applicationidentity.NewService(storage, tokenManager, nil)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	if err := identityService.Initialize(ctx, config.AdminEmail, config.AdminPassword); err != nil {
		_ = storage.Close()
		return nil, err
	}
	identityHandler, err := rest.NewIdentityHandler(identityService, principal)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	var sessionHandler platform.RouteRegistrar
	if tokenManager != nil {
		sessionHandler, err = rest.NewSessionHandler(identityService, tokenManager, config.CookieSecure)
		if err != nil {
			_ = storage.Close()
			return nil, err
		}
	}
	blobStore, err := localmedia.Open(config.MediaRoot)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	mediaService, err := applicationmedia.NewService(service, blobStore)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	mediaHandler, err := rest.NewMediaHandler(mediaService, principal, config.MediaMaxBytes)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	graphQL, err := graphqlapi.New(service, config.Manifest, principal)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	routes := platform.RouteGroup{handler, codexHandler, collectionRouter, taxonomyHandler, identityHandler, mediaHandler, graphQL}
	if sessionHandler != nil {
		routes = append(routes, sessionHandler)
	}
	feature, err := platform.NewRuntimeFeature("headless-content", routes, storage, storage)
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	controlPlane, err := platform.NewControlPlane("headless", "Content", "/go-admin")
	if err != nil {
		_ = storage.Close()
		return nil, err
	}
	if err := controlPlane.RegisterManifest(config.Manifest); err != nil {
		_ = storage.Close()
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
			if _, err := service.PublishDue(ctx, 100); err != nil {
				slog.Error("scheduled publication failed", "error", err)
			}
		},
	})
	if config.App.MetricsPath != "" {
		builder.WithMetricsEndpoint(config.App.MetricsPath)
	}
	return &Runtime{App: builder.Build(), ControlPlane: controlPlane, storage: storage}, nil
}

func (runtime *Runtime) Close() error {
	if runtime == nil || runtime.storage == nil {
		return nil
	}
	return runtime.storage.Close()
}

func OpenStorage(ctx context.Context, config Config) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(config.Storage)) {
	case "bbolt":
		return bboltstorage.Open(config.BboltPath, 0o600, nil)
	case "sqlite":
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for SQLite")
		}
		if err := os.MkdirAll(filepath.Dir(config.DataSource), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create SQLite directory: %w", err)
		}
		return sqlstore.Open(ctx, "sqlite", config.DataSource, sqlstore.DialectSQLite)
	case "mysql", "mariadb":
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for MySQL or MariaDB")
		}
		return sqlstore.Open(ctx, "mysql", config.DataSource, sqlstore.DialectMySQL)
	case "postgres", "postgresql":
		if config.DataSource == "" {
			return nil, errors.New("DATABASE_URL is required for PostgreSQL")
		}
		return sqlstore.Open(ctx, "pgx", config.DataSource, sqlstore.DialectPostgreSQL)
	default:
		return nil, errors.New("HEADLESS_STORAGE must be bbolt, sqlite, mysql, mariadb, or postgres")
	}
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
