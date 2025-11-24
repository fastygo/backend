# Backend Guide: Go Clean Architecture 101

## Prerequisites
- Go 1.21+
- PostgreSQL 15+
- Redis 7+
- Docker & Docker Compose

## Project Setup

### 1. Initialize Go Module
```bash
go mod init github.com/yourorg/backend
```

### 2. Install Dependencies
```bash
go get github.com/valyala/fasthttp@latest
go get github.com/fasthttp/router@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/go-redis/redis/v9@latest
go get github.com/golang-jwt/jwt/v4@latest
go get go.etcd.io/bbolt@latest
go get go.uber.org/zap@latest
go get github.com/robfig/cron/v3@latest
```

### 3. Environment Variables
Create `.env` file:
```env
DB_HOST=localhost
DB_PORT=5432
DB_NAME=backend_db
DB_USER=backend_user
DB_PASSWORD=your_password
REDIS_URL=redis://localhost:6379
JWT_SECRET=your-secret-key
SERVER_PORT=8080

# Offline Resilience
BOLTDB_PATH=./data/buffer.db
BUFFER_MAX_SIZE=1000000
BUFFER_RETENTION_HOURS=24
SYNC_INTERVAL_SECONDS=30
MAX_RETRY_ATTEMPTS=3
```

## Architecture Layers

### Domain Layer (Entities & Business Rules)
```go
// domain/user.go
type User struct {
    ID       string    `json:"id"`
    Email    string    `json:"email"` // PII - handled by IAM only
    Role     string    `json:"role"`
    Status   string    `json:"status"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}
```

### Repository Layer (Data Access)
```go
// repository/postgres/user_repo.go
type UserRepository interface {
    GetByID(ctx context.Context, id string) (*domain.User, error)
    Update(ctx context.Context, user *domain.User) error
}

type postgresUserRepo struct {
    db *pgxpool.Pool
}

func (r *postgresUserRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
    query := `SELECT id, role, status, created_at, updated_at FROM users WHERE id = $1`
    var user domain.User
    err := r.db.QueryRow(ctx, query, id).Scan(
        &user.ID, &user.Role, &user.Status, &user.CreatedAt, &user.UpdatedAt,
    )
    return &user, err
}
```

### Use Case Layer (Business Logic)
```go
// usecase/profile/profile.go
type ProfileUseCase struct {
    userRepo repository.UserRepository
    logger   *zap.Logger
}

func (uc *ProfileUseCase) GetProfile(ctx context.Context, userID string) (*domain.User, error) {
    user, err := uc.userRepo.GetByID(ctx, userID)
    if err != nil {
        uc.logger.Error("failed to get user profile", zap.Error(err))
        return nil, domain.ErrUserNotFound
    }
    return user, nil
}
```

### API Handler Layer (HTTP Interface)
```go
// api/handler/profile.go
func (h *ProfileHandler) GetProfile(ctx *fasthttp.RequestCtx) {
    userID := string(ctx.Request.Header.Peek("X-User-ID"))
    if userID == "" {
        h.respondError(ctx, fasthttp.StatusUnauthorized, "missing user ID")
        return
    }

    user, err := h.profileUseCase.GetProfile(ctx, userID)
    if err != nil {
        h.logger.Error("failed to get profile", zap.Error(err))
        h.respondError(ctx, fasthttp.StatusInternalServerError, "internal error")
        return
    }

    h.respondSuccess(ctx, user)
}
```

## Infrastructure Setup

### Database Connection
```go
// internal/infrastructure/postgres/client.go
func NewPostgresClient(cfg *config.Config) (*pgxpool.Pool, error) {
    connString := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=disable",
        cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName,
    )

    pool, err := pgxpool.New(context.Background(), connString)
    if err != nil {
        return nil, err
    }

    if err := pool.Ping(context.Background()); err != nil {
        return nil, err
    }

    return pool, nil
}
```

### Redis Connection
```go
// internal/infrastructure/redis/client.go
func NewRedisClient(cfg *config.Config) *redis.Client {
    opt, err := redis.ParseURL(cfg.RedisURL)
    if err != nil {
        panic(err)
    }

    client := redis.NewClient(opt)
    return client

}
```

## Offline Resilience & Data Buffering

### BoltDB Buffer Service
```go
// internal/infrastructure/buffer/boltdb.go
import (
    "encoding/json"
    "time"
    bolt "go.etcd.io/bbolt"
)

type BufferItem struct {
    ID        string          `json:"id"`
    UserID    string          `json:"user_id"`
    Operation string          `json:"operation"` // "create", "update", "delete"
    Entity    string          `json:"entity"`    // "task", "profile"
    Data      json.RawMessage `json:"data"`
    Timestamp time.Time       `json:"timestamp"`
    Retries   int             `json:"retries"`
    Priority  int             `json:"priority"`  // 1=low, 5=high
}

type BoltDBBuffer struct {
    db *bolt.DB
}

func NewBoltDBBuffer(path string) (*BoltDBBuffer, error) {
    db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
    if err != nil {
        return nil, err
    }

    // Create buckets
    err = db.Update(func(tx *bolt.Tx) error {
        _, err := tx.CreateBucketIfNotExists([]byte("buffer"))
        return err
    })

    return &BoltDBBuffer{db: db}, err
}

func (b *BoltDBBuffer) Add(item BufferItem) error {
    return b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("buffer"))
        key := []byte(fmt.Sprintf("%d_%s", item.Priority, item.ID))

        data, err := json.Marshal(item)
        if err != nil {
            return err
        }

        return bucket.Put(key, data)
    })
}

func (b *BoltDBBuffer) GetBatch(limit int) ([]BufferItem, error) {
    var items []BufferItem

    err := b.db.View(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("buffer"))
        c := bucket.Cursor()

        count := 0
        for k, v := c.First(); k != nil && count < limit; k, v = c.Next() {
            var item BufferItem
            if err := json.Unmarshal(v, &item); err != nil {
                continue // Skip corrupted items
            }
            items = append(items, item)
            count++
        }
        return nil
    })

    return items, err
}

func (b *BoltDBBuffer) Remove(id string) error {
    return b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("buffer"))
        c := bucket.Cursor()

        for k, v := c.First(); k != nil; k, v = c.Next() {
            var item BufferItem
            if err := json.Unmarshal(v, &item); err != nil {
                continue
            }
            if item.ID == id {
                return bucket.Delete(k)
            }
        }
        return nil
    })
}

func (b *BoltDBBuffer) Cleanup(olderThan time.Time) error {
    return b.db.Update(func(tx *bolt.Tx) error {
        bucket := tx.Bucket([]byte("buffer"))
        c := bucket.Cursor()

        for k, v := c.First(); k != nil; k, v = c.Next() {
            var item BufferItem
            if err := json.Unmarshal(v, &item); err != nil {
                continue
            }
            if item.Timestamp.Before(olderThan) {
                bucket.Delete(k)
            }
        }
        return nil
    })
}
```

### Connection Monitor
```go
// internal/infrastructure/monitor/connection.go
type ConnectionStatus struct {
    PostgreSQL bool `json:"postgresql"`
    Redis      bool `json:"redis"`
    LastCheck  time.Time `json:"last_check"`
}

type ConnectionMonitor struct {
    pgPool  *pgxpool.Pool
    redis   *redis.Client
    status  ConnectionStatus
    mu      sync.RWMutex
}

func NewConnectionMonitor(pgPool *pgxpool.Pool, redis *redis.Client) *ConnectionMonitor {
    cm := &ConnectionMonitor{
        pgPool: pgPool,
        redis:  redis,
    }

    // Start monitoring goroutine
    go cm.monitor()

    return cm
}

func (cm *ConnectionMonitor) monitor() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for range ticker.C {
        cm.checkConnections()
    }
}

func (cm *ConnectionMonitor) checkConnections() {
    pgOk := cm.checkPostgreSQL()
    redisOk := cm.checkRedis()

    cm.mu.Lock()
    cm.status = ConnectionStatus{
        PostgreSQL: pgOk,
        Redis:      redisOk,
        LastCheck:  time.Now(),
    }
    cm.mu.Unlock()
}

func (cm *ConnectionMonitor) checkPostgreSQL() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    return cm.pgPool.Ping(ctx) == nil
}

func (cm *ConnectionMonitor) checkRedis() bool {
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    return cm.redis.Ping(ctx).Err() == nil
}

func (cm *ConnectionMonitor) GetStatus() ConnectionStatus {
    cm.mu.RLock()
    defer cm.mu.RUnlock()
    return cm.status
}

func (cm *ConnectionMonitor) IsOnline() bool {
    status := cm.GetStatus()
    return status.PostgreSQL && status.Redis
}
```

### Buffer Processor Service
```go
// internal/services/buffer_processor.go
import (
    "context"
    "time"
    "github.com/robfig/cron/v3"
)

type BufferProcessor struct {
    buffer    *BoltDBBuffer
    monitor   *ConnectionMonitor
    userRepo  repository.UserRepository
    taskRepo  repository.TaskRepository
    logger    *zap.Logger
    cron      *cron.Cron
}

func NewBufferProcessor(
    buffer *BoltDBBuffer,
    monitor *ConnectionMonitor,
    userRepo repository.UserRepository,
    taskRepo repository.TaskRepository,
    logger *zap.Logger,
) *BufferProcessor {
    bp := &BufferProcessor{
        buffer:   buffer,
        monitor:  monitor,
        userRepo: userRepo,
        taskRepo: taskRepo,
        logger:   logger,
        cron:     cron.New(),
    }

    // Process buffer every 30 seconds if online
    bp.cron.AddFunc("*/30 * * * * *", func() {
        if bp.monitor.IsOnline() {
            bp.processBuffer()
        }
    })

    return bp
}

func (bp *BufferProcessor) Start() {
    bp.cron.Start()
    bp.logger.Info("buffer processor started")
}

func (bp *BufferProcessor) Stop() {
    ctx := bp.cron.Stop()
    <-ctx.Done()
    bp.logger.Info("buffer processor stopped")
}

func (bp *BufferProcessor) processBuffer() {
    items, err := bp.buffer.GetBatch(50) // Process in batches
    if err != nil {
        bp.logger.Error("failed to get buffer items", zap.Error(err))
        return
    }

    for _, item := range items {
        if err := bp.processItem(item); err != nil {
            bp.logger.Error("failed to process buffer item",
                zap.String("id", item.ID),
                zap.Error(err))

            // Increment retry count
            item.Retries++
            if item.Retries < 3 {
                bp.buffer.Add(item) // Re-queue with higher priority
            } else {
                bp.logger.Warn("dropping buffer item after max retries",
                    zap.String("id", item.ID))
            }
        }

        // Remove successfully processed item
        bp.buffer.Remove(item.ID)
    }
}

func (bp *BufferProcessor) processItem(item BufferItem) error {
    ctx := context.Background()

    switch item.Entity {
    case "profile":
        var user domain.User
        if err := json.Unmarshal(item.Data, &user); err != nil {
            return err
        }

        switch item.Operation {
        case "update":
            return bp.userRepo.Update(ctx, &user)
        }

    case "task":
        var task domain.Task
        if err := json.Unmarshal(item.Data, &task); err != nil {
            return err
        }

        switch item.Operation {
        case "create":
            _, err := bp.taskRepo.Create(ctx, &task)
            return err
        case "update":
            return bp.taskRepo.Update(ctx, &task)
        case "delete":
            return bp.taskRepo.Delete(ctx, task.ID)
        }
    }

    return nil
}

func (bp *BufferProcessor) BufferOperation(item BufferItem) error {
    // If online, try to process immediately
    if bp.monitor.IsOnline() {
        if err := bp.processItem(item); err != nil {
            bp.logger.Warn("immediate processing failed, buffering",
                zap.String("id", item.ID), zap.Error(err))
            return bp.buffer.Add(item)
        }
        return nil
    }

    // Offline, buffer the operation
    bp.logger.Info("buffering operation (offline mode)",
        zap.String("entity", item.Entity),
        zap.String("operation", item.Operation))
    return bp.buffer.Add(item)
}
```

### Enhanced Health Checks
```go
// api/handler/health.go
func (h *HealthHandler) Check(ctx *fasthttp.RequestCtx) {
    status := h.monitor.GetStatus()
    bufferSize := h.getBufferSize()

    response := map[string]interface{}{
        "status": func() string {
            if status.PostgreSQL && status.Redis {
                return "healthy"
            }
            return "degraded"
        }(),
        "timestamp": time.Now().Format(time.RFC3339),
        "services": map[string]interface{}{
            "postgresql": map[string]interface{}{
                "status": status.PostgreSQL,
                "last_check": status.LastCheck.Format(time.RFC3339),
            },
            "redis": map[string]interface{}{
                "status": status.Redis,
                "last_check": status.LastCheck.Format(time.RFC3339),
            },
        },
        "buffer": map[string]interface{}{
            "size": bufferSize,
            "status": bufferSize == 0 ? "empty" : "processing",
        },
    }

    if status.PostgreSQL && status.Redis {
        h.respondSuccess(ctx, response)
    } else {
        ctx.SetStatusCode(fasthttp.StatusServiceUnavailable)
        h.respondJSON(ctx, response)
    }
}

func (h *HealthHandler) getBufferSize() int {
    // Implementation to get current buffer size
    return 0 // Placeholder
}
```

## JWT Middleware (Validation Only)
```go
// internal/middleware/auth.go
func JWTAuthMiddleware(jwtSecret string) fasthttp.RequestHandler {
    return func(ctx *fasthttp.RequestCtx) {
        authHeader := ctx.Request.Header.Peek("Authorization")
        if len(authHeader) == 0 {
            ctx.SetStatusCode(fasthttp.StatusUnauthorized)
            return
        }

        tokenString := string(authHeader)
        if strings.HasPrefix(tokenString, "Bearer ") {
            tokenString = tokenString[7:]
        }

        token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
            return []byte(jwtSecret), nil
        })

        if err != nil || !token.Valid {
            ctx.SetStatusCode(fasthttp.StatusUnauthorized)
            return
        }

        // Extract claims and set in context
        if claims, ok := token.Claims.(jwt.MapClaims); ok {
            if userID, ok := claims["user_id"].(string); ok {
                ctx.Request.Header.Set("X-User-ID", userID)
            }
        }
    }
}
```

## Router Setup
```go
// internal/router/router.go
func NewRouter(handlers *api.Handlers, authMiddleware fasthttp.RequestHandler) *router.Router {
    r := router.New()

    // Health check (no auth)
    r.GET("/health", handlers.Health.Check)

    // Protected routes
    r.GET("/api/v1/profile", authMiddleware(handlers.Profile.GetProfile))
    r.PUT("/api/v1/profile", authMiddleware(handlers.Profile.UpdateProfile))

    r.GET("/api/v1/tasks", authMiddleware(handlers.Task.GetTasks))
    r.POST("/api/v1/tasks", authMiddleware(handlers.Task.CreateTask))
    r.PUT("/api/v1/tasks/{id}", authMiddleware(handlers.Task.UpdateTask))
    r.DELETE("/api/v1/tasks/{id}", authMiddleware(handlers.Task.DeleteTask))

    return r
}
```

## Main Application
```go
// cmd/server/main.go
func main() {
    logger, _ := zap.NewProduction()
    defer logger.Sync()

    cfg, err := config.Load()
    if err != nil {
        logger.Fatal("failed to load config", zap.Error(err))
    }

    // Infrastructure
    db, err := postgres.NewClient(cfg)
    if err != nil {
        logger.Fatal("failed to connect to postgres", zap.Error(err))
    }
    defer db.Close()

    redisClient := redis.NewClient(cfg)

    // Repositories
    userRepo := postgres.NewUserRepository(db)
    taskRepo := postgres.NewTaskRepository(db)
    sessionRepo := redis.NewSessionRepository(redisClient)

    // Use Cases
    profileUC := usecase.NewProfileUseCase(userRepo, logger)
    taskUC := usecase.NewTaskUseCase(taskRepo, logger)

    // Handlers
    profileHandler := api.NewProfileHandler(profileUC, logger)
    taskHandler := api.NewTaskHandler(taskUC, logger)
    healthHandler := api.NewHealthHandler(db, redisClient, logger)

    handlers := &api.Handlers{
        Profile: profileHandler,
        Task:    taskHandler,
        Health:  healthHandler,
    }

    // Middleware
    authMiddleware := middleware.JWTAuthMiddleware(cfg.JWTSecret)

    // Router
    r := router.NewRouter(handlers, authMiddleware)

    // Server
    server := &fasthttp.Server{
        Handler: r.Handler,
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 10 * time.Second,
        IdleTimeout:  120 * time.Second,
    }

    logger.Info("starting server", zap.String("port", cfg.ServerPort))
    if err := server.ListenAndServe(":" + cfg.ServerPort); err != nil {
        logger.Fatal("server failed", zap.Error(err))
    }
}
```

## Database Schema
```sql
-- assets/migrations/001_initial.sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    role VARCHAR(50) NOT NULL DEFAULT 'user',
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Indexes
CREATE INDEX idx_tasks_user_id ON tasks(user_id);
CREATE INDEX idx_tasks_status ON tasks(status);
```

## Docker Setup
```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/main .
COPY --from=builder /app/assets ./assets
CMD ["./main"]
```

## Testing
```go
// api/handler/profile_test.go
func TestProfileHandler_GetProfile(t *testing.T) {
    // Mock dependencies
    mockUC := &mocks.ProfileUseCase{}
    logger := zap.NewNop()

    handler := NewProfileHandler(mockUC, logger)

    // Test cases
    t.Run("success", func(t *testing.T) {
        user := &domain.User{ID: "123", Role: "user"}
        mockUC.On("GetProfile", mock.Anything, "123").Return(user, nil)

        req := fasthttp.AcquireRequest()
        resp := fasthttp.AcquireResponse()
        defer fasthttp.ReleaseRequest(req)
        defer fasthttp.ReleaseResponse(resp)

        req.Header.SetMethod("GET")
        req.Header.Set("X-User-ID", "123")

        handler.GetProfile(resp)

        assert.Equal(t, fasthttp.StatusOK, resp.StatusCode())
    })
}
```

## Health Checks
```go
// api/handler/health.go
func (h *HealthHandler) Check(ctx *fasthttp.RequestCtx) {
    // Check database
    if err := h.db.Ping(context.Background()); err != nil {
        h.logger.Error("database health check failed", zap.Error(err))
        h.respondError(ctx, fasthttp.StatusServiceUnavailable, "database unhealthy")
        return
    }

    // Check Redis
    if err := h.redis.Ping(context.Background()).Err(); err != nil {
        h.logger.Error("redis health check failed", zap.Error(err))
        h.respondError(ctx, fasthttp.StatusServiceUnavailable, "redis unhealthy")
        return
    }

    h.respondSuccess(ctx, map[string]string{"status": "healthy"})
}
```

## Schema Automation

1. **Migration Tooling**: Use `golang-migrate` or `rickar/sql-migrate` to version schema changes.
   ```bash
   migrate -path assets/migrations -database "${DATABASE_URL}" up
   ```
2. **CI/CD Integration**: Add a pipeline step that runs migrations before rolling deployments.
3. **Local Development**: Provide `scripts/reset-db.sh` to drop/create schema and load fixtures via `psql`.
4. **Supabase Compatibility**: Keep SQL files idempotent; use `IF NOT EXISTS` and `CREATE EXTENSION IF NOT EXISTS`.
5. **Schema Discovery**: Embed entity definitions in `assets/docs/schema.md` for quick reference when creating new tables/aggregates.

## Monitoring Hooks

1. **Tracing**: Instrument handlers with OpenTelemetry (`go.opentelemetry.io/otel`) and expose a Jaeger endpoint.
   ```go
   tracer := otel.Tracer("backend")
   ctx, span := tracer.Start(ctx, "HandleProfile")
   defer span.End()
   ```
2. **Metrics Exporters**: Use `go.opentelemetry.io/otel/exporters/prometheus`.
   ```go
   http.Handle("/metrics", promhttp.Handler())
   ```
3. **Health & Readiness**: Expose `/health` and `/ready` endpoints that report dependency status (Postgres, Redis, BoltDB buffer).
4. **Alert Hooks**: Configure alerting via Prometheus alerts (high latency, failed migrations, buffer backlogs).
5. **Logging**: Link zap logging with trace IDs via middleware and push to centralized log store (e.g., Loki, Cloud Logging).

## Best Practices
1. **Context Propagation**: Always pass context through all layers
2. **Error Handling**: Use domain-specific errors, log internal errors
3. **Input Validation**: Validate all inputs, use domain types
4. **Logging**: Structured logging with request IDs
5. **Testing**: Unit tests for all layers, integration tests
6. **Security**: No sensitive data, secure headers, input sanitization
7. **Performance**: Connection pooling, prepared statements, caching
