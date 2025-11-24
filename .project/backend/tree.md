# Backend Directory Structure

```
backend/
├── cmd/
│   └── server/
│       └── main.go                 # Application entry point
├── internal/
│   ├── config/
│   │   ├── config.go               # Environment configuration
│   │   └── config_test.go
│   ├── infrastructure/
│   │   ├── postgres/
│   │   │   ├── client.go           # PostgreSQL connection pool
│   │   │   └── client_test.go
│   │   ├── redis/
│   │   │   ├── client.go           # Redis connection
│   │   │   └── client_test.go
│   │   ├── jwt/
│   │   │   └── manager.go          # JWT validation (no generation)
│   │   ├── session/
│   │   │   └── manager.go          # Session management (Redis)
│   │   ├── buffer/
│   │   │   ├── boltdb.go           # BoltDB buffer implementation
│   │   │   ├── boltdb_test.go
│   │   │   └── types.go            # Buffer data structures
│   │   └── monitor/
│   │       ├── connection.go       # Connection health monitoring
│   │       ├── connection_test.go
│   │       └── status.go           # Connection status types
│   ├── middleware/
│   │   └── auth.go                 # JWT authentication middleware
│   └── router/
│       └── router.go               # HTTP router configuration
│   └── services/
│       └── buffer_processor.go     # Background buffer processing service
├── api/
│   ├── handler/
│   │   ├── profile.go              # Profile HTTP handlers
│   │   ├── profile_test.go
│   │   ├── task.go                 # Task HTTP handlers
│   │   ├── task_test.go
│   │   └── health.go               # Health check handlers
│   └── transport/
│       ├── response.go             # HTTP response structures
│       └── request.go              # HTTP request structures
├── usecase/
│   ├── auth/
│   │   ├── auth.go                 # Authentication business logic
│   │   └── auth_test.go
│   ├── profile/
│   │   ├── profile.go              # Profile business logic
│   │   └── profile_test.go
│   └── task/
│       ├── task.go                 # Task business logic
│       └── task_test.go
├── repository/
│   ├── postgres/
│   │   ├── user_repo.go            # User data access
│   │   ├── user_repo_test.go
│   │   ├── task_repo.go            # Task data access
│   │   └── task_repo_test.go
│   └── redis/
│       └── session_repo.go         # Session data access
├── domain/
│   ├── user.go                     # User domain entity
│   ├── task.go                     # Task domain entity
│   ├── session.go                  # Session domain entity
│   ├── errors.go                   # Domain error definitions
│   └── types.go                    # Common domain types
├── assets/
│   ├── migrations/
│   │   ├── 001_initial.sql         # Database schema
│   │   ├── 002_add_indexes.sql
│   │   └── 003_update_schema.sql
│   └── fixtures/
│       └── test_data.sql           # Test data
├── pkg/
│   └── utils/
│       ├── validation.go           # Input validation utilities
│       ├── logger.go               # Logger utilities
│       └── crypto.go               # Cryptography utilities
├── Dockerfile                      # Multi-stage Docker build
├── docker-compose.yml              # Local development setup
├── docker-compose.test.yml         # Testing environment
├── Makefile                        # Build and development tasks
├── go.mod                          # Go module definition
├── go.sum                          # Go module checksums
├── .env.example                    # Environment variables template
├── .gitignore                      # Git ignore patterns
├── README.md                       # Project documentation
└── .dockerignore                   # Docker ignore patterns
```

## Key Architecture Notes

### Layer Responsibilities

1. **cmd/server/** - Application bootstrap, dependency injection
2. **internal/** - Private application code, cannot be imported by other modules
   - **config/** - Configuration loading from environment
   - **infrastructure/** - External service connections (DB, Redis, etc.)
   - **middleware/** - HTTP middleware (auth, logging, etc.)
   - **router/** - Route definitions and HTTP server setup
3. **api/** - HTTP interface layer
   - **handler/** - HTTP request handlers (controllers)
   - **transport/** - Request/response DTOs and serialization
4. **usecase/** - Business logic layer (application services)
5. **repository/** - Data access layer (repository pattern)
6. **domain/** - Domain entities, business rules, errors
7. **assets/** - Static assets like database migrations
8. **pkg/** - Reusable packages that can be imported by other modules

### File Naming Conventions

- **Entities**: `user.go`, `task.go`
- **Use Cases**: `{usecase}.go` (e.g., `profile.go`, `auth.go`)
- **Repositories**: `{entity}_repo.go` (e.g., `user_repo.go`)
- **Handlers**: `{resource}.go` (e.g., `profile.go`, `task.go`)
- **Tests**: `{filename}_test.go`
- **Interfaces**: Define in same file as implementation or separate `interfaces.go`

### Dependency Direction

```
main.go → router → handlers → usecases → repositories → domain
                          ↓
                   middleware → infrastructure
```

### Test Coverage

- **Unit Tests**: Each layer tested in isolation with mocks
- **Integration Tests**: Full request/response cycles
- **E2E Tests**: Via separate test environment

### Environment Structure

```
├── development/                    # Local development
├── testing/                        # Automated testing
└── production/                     # Production deployment
```

## Docker Multi-Stage Build

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Runtime stage
FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/
COPY --from=builder /app/main .
COPY --from=builder /app/assets ./assets
EXPOSE 8080
CMD ["./main"]
```

## Development Setup

```yaml
# docker-compose.yml
version: '3.8'
services:
  postgres:
    image: postgres:15-alpine
    environment:
      POSTGRES_DB: backend_db
      POSTGRES_USER: backend_user
      POSTGRES_PASSWORD: password
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"

  backend:
    build: .
    ports:
      - "8080:8080"
    environment:
      DB_HOST: postgres
      REDIS_URL: redis://redis:6379
    depends_on:
      - postgres
      - redis

├── data/
│   └── buffer.db                 # BoltDB buffer database (auto-created)
├── Dockerfile                      # Multi-stage Docker build
├── docker-compose.yml              # Local development setup
├── docker-compose.test.yml         # Testing environment
├── Makefile                        # Build and development tasks
├── go.mod                          # Go module definition
├── go.sum                          # Go module checksums
├── .env.example                    # Environment variables template
├── .gitignore                      # Git ignore patterns
├── README.md                       # Project documentation
└── .dockerignore                   # Docker ignore patterns
```

## Offline Resilience Architecture

### Data Flow During Connection Loss
```
Client Request → API Handler → Use Case → Repository
       ↓                ↓           ↓          ↓
   Immediate OK     Buffer Check  Offline?    Buffer Item
   Response         (if offline)   → Yes      Created
```

### Buffer Processing Flow
```
Cron Job (30s) → Connection Check → Online? → Process Buffer Items
       ↓                ↓              ↓             ↓
   Wait 30s        PostgreSQL + Redis  No     Read Batch (50 items)
   Next Cycle         Status           Wait    Process Each Item
                                          Next Cycle
```

### Key Components
1. **BoltDB Buffer**: Embedded key-value store for offline data
2. **Connection Monitor**: Real-time health checking of external services
3. **Buffer Processor**: Background service for syncing buffered data
4. **Enhanced Health Checks**: Connection status and buffer metrics

### Buffer Data Structure
```go
type BufferItem struct {
    ID        string          // Unique operation ID
    UserID    string          // User context
    Operation string          // "create", "update", "delete"
    Entity    string          // "task", "profile"
    Data      json.RawMessage // Operation payload
    Timestamp time.Time       // When buffered
    Retries   int             // Retry attempts
    Priority  int             // Processing priority (1-5)
}
```

### Configuration Options
- **BOLTDB_PATH**: Database file location (default: ./data/buffer.db)
- **BUFFER_MAX_SIZE**: Maximum buffer entries (default: 1M)
- **BUFFER_RETENTION_HOURS**: Auto-cleanup age (default: 24h)
- **SYNC_INTERVAL_SECONDS**: Background sync frequency (default: 30s)
- **MAX_RETRY_ATTEMPTS**: Max retry attempts per item (default: 3)

volumes:
  postgres_data:
```
