package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config aggregates all runtime settings required by the application.
type Config struct {
	AppName     string
	Environment string
	HTTP        HTTPConfig
	Database    DatabaseConfig
	Redis       RedisConfig
	JWT         JWTConfig
	Buffer      BufferConfig
	Context     ContextConfig
	Logger      LoggerConfig
	Migrations  MigrationsConfig
}

type HTTPConfig struct {
	Host          string
	Port          string
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	IdleTimeout   time.Duration
	MaxConn       int
	EnablePprof   bool
	EnableMetrics bool
}

type DatabaseConfig struct {
	URL             string
	Host            string
	Port            string
	Name            string
	User            string
	Password        string
	MaxOpenConns    int
	MaxIdleConns    int
	MaxConnLifetime time.Duration
	SSLMode         string
}

type RedisConfig struct {
	URL      string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret string
	Issuer string
}

type BufferConfig struct {
	Path            string
	MaxSize         int
	RetentionHours  int
	SyncInterval    time.Duration
	MaxRetry        int
	PriorityBuckets int
}

type ContextConfig struct {
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type LoggerConfig struct {
	Level    string
	Encoding string
}

type MigrationsConfig struct {
	Enabled bool
	Path    string
}

// Load reads configuration from environment variables (optionally .env)
// and applies sane defaults so the service can boot in any environment.
func Load() (*Config, error) {
	_ = godotenv.Load(".env")

	cfg := &Config{
		AppName:     getString("APP_NAME", "go-backend"),
		Environment: getString("APP_ENV", "development"),
		HTTP: HTTPConfig{
			Host:          getString("SERVER_HOST", "0.0.0.0"),
			Port:          getString("SERVER_PORT", "8080"),
			ReadTimeout:   getDuration("SERVER_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:  getDuration("SERVER_WRITE_TIMEOUT", 10*time.Second),
			IdleTimeout:   getDuration("SERVER_IDLE_TIMEOUT", 120*time.Second),
			MaxConn:       getInt("SERVER_MAX_CONN", 0),
			EnablePprof:   getBool("SERVER_ENABLE_PPROF", false),
			EnableMetrics: getBool("SERVER_ENABLE_METRICS", false),
		},
		Database: DatabaseConfig{
			URL:             os.Getenv("DATABASE_URL"),
			Host:            getString("DB_HOST", "localhost"),
			Port:            getString("DB_PORT", "5432"),
			Name:            getString("DB_NAME", "backend_db"),
			User:            getString("DB_USER", "backend_user"),
			Password:        os.Getenv("DB_PASSWORD"),
			MaxOpenConns:    getInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getInt("DB_MAX_IDLE_CONNS", 10),
			MaxConnLifetime: getDuration("DB_CONN_LIFETIME", time.Hour),
			SSLMode:         getString("DB_SSLMODE", "disable"),
		},
		Redis: RedisConfig{
			URL:      getString("REDIS_URL", "redis://localhost:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       getInt("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret: os.Getenv("JWT_SECRET"),
			Issuer: getString("JWT_ISSUER", "go-backend"),
		},
		Buffer: BufferConfig{
			Path:            getString("BOLTDB_PATH", "./data/buffer.db"),
			MaxSize:         getInt("BUFFER_MAX_SIZE", 1_000_000),
			RetentionHours:  getInt("BUFFER_RETENTION_HOURS", 24),
			SyncInterval:    getDuration("SYNC_INTERVAL_SECONDS", 30*time.Second),
			MaxRetry:        getInt("MAX_RETRY_ATTEMPTS", 3),
			PriorityBuckets: getInt("BUFFER_PRIORITY_BUCKETS", 5),
		},
		Context: ContextConfig{
			RequestTimeout:  getDuration("REQUEST_TIMEOUT_SECONDS", 5*time.Second),
			ShutdownTimeout: getDuration("SHUTDOWN_TIMEOUT_SECONDS", 15*time.Second),
		},
		Logger: LoggerConfig{
			Level:    getString("LOG_LEVEL", "info"),
			Encoding: getString("LOG_ENCODING", "json"),
		},
		Migrations: MigrationsConfig{
			Enabled: getBool("RUN_MIGRATIONS", true),
			Path:    getString("MIGRATIONS_PATH", "./assets/migrations"),
		},
	}

	if cfg.Database.URL == "" {
		cfg.Database.URL = buildPostgresURL(cfg)
	}

	return cfg, nil
}

// MustLoad panics if configuration cannot be loaded.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func buildPostgresURL(cfg *Config) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Name,
		cfg.Database.SSLMode,
	)
}

func getString(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func getInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func getBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if parsed, err := strconv.ParseBool(val); err == nil {
			return parsed
		}
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if parsed, err := time.ParseDuration(val); err == nil {
			return parsed
		}
		if seconds, err := strconv.Atoi(val); err == nil {
			return time.Duration(seconds) * time.Second
		}
	}
	return fallback
}

// Address returns the HTTP listen address for the fasthttp server.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%s", c.HTTP.Host, c.HTTP.Port)
}
