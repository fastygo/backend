# Go Backend Implementation Plan

## Phase 1: Foundation & Core Infrastructure

1. **Project Initialization**

- Initialize `go.mod`
- Create directory structure according to `backend/tree.md`
- Setup `Makefile` for build, test, and lint commands

2. **Configuration & Logging**

- Implement `internal/config` to load environment variables
- Implement `pkg/utils/logger` (Zap)
- Define structured error types in `internal/domain/errors.go`
- **Improvement**: Create a standardized JSON error response helper

3. **Database Connections**

- PostgreSQL setup with `pgx/v5` pool (`internal/infrastructure/postgres`)
- Redis setup (`internal/infrastructure/redis`)
- BoltDB setup for offline buffer (`internal/infrastructure/buffer`)
- **Improvement**: Implement `migrations` execution on startup using `golang-migrate`

4. **Context Adaptation & Graceful Shutdown**

- **Improvement**: Create a `ContextAdapter` middleware/utility to bridge `fasthttp.RequestCtx` with standard `context.Context` (handling timeouts and cancellation)
- **Improvement**: Implement a `LifecycleManager` in `cmd/server` to handle SIGTERM/SIGINT for graceful shutdown of DBs and HTTP server

## Phase 2: Domain & Data Access Layer

5. **Domain Entities**

- Define `User`, `Task`, `Session` structs in `internal/domain`

6. **Repository Implementation**

- Implement `UserRepository` (Postgres)
- Implement `TaskRepository` (Postgres)
- Implement `SessionRepository` (Redis)
- Ensure all repositories accept `context.Context`

## Phase 3: Offline Resilience & Business Logic

7. **Buffer Mechanism**

- Implement `BoltDBBuffer` methods (Add, GetBatch, Remove)
- Implement `BufferProcessor` service (Cron job)
- Implement logic to queue writes when primary DB is down

8. **Use Cases**

- `AuthUseCase` (Login, Refresh)
- `ProfileUseCase` (Get, Update)
- `TaskUseCase` (CRUD)
- Integrate Buffer logic into Use Cases (write to buffer if Repos fail)

## Phase 4: HTTP API & Documentation

9. **Middleware**

- JWT Authentication middleware
- RequestID & Logging middleware
- CORS middleware

10. **Handlers**

 - Implement `AuthHandler`, `ProfileHandler`, `TaskHandler`
 - Use `ContextAdapter` to pass strict contexts to Use Cases
 - **Improvement**: Add `swaggo/swag` annotations to handlers for API documentation generation

11. **Router & Server**

 - Setup `fasthttp/router`
 - Wire dependency injection in `main.go`
 - Configure `fasthttp.Server` parameters

## Phase 5: Testing & Deployment

12. **Dockerization**

 - Create multi-stage `Dockerfile`
 - Update `docker-compose.yml` with health checks

13. **Testing**

 - Create mocks for UseCases and Repositories
 - Implement integration tests for critical paths