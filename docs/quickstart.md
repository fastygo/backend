# Quickstart Guide

Welcome to the modular Go backend boilerplate. This guide covers the essentials for setting up the project locally and preparing it for containerized deployments.

## Prerequisites

- Go 1.21+
- Docker & Docker Compose
- Make (optional but recommended)
- PostgreSQL 15+ & Redis 7+ (local or containerized)

## 1. Clone & Bootstrap

```bash
git clone <repo-url> backend
cd backend
go mod tidy
```

## 2. Environment Configuration

1. Copy `.env.example` (will be added later) to `.env`.
2. Update database, Redis, JWT, and buffer settings to match your environment.

## 3. Useful Make Targets

| Command          | Description                               |
| ---------------- | ----------------------------------------- |
| `make build`     | Build the project                         |
| `make run`       | Run the server locally (uses `cmd/server`) |
| `make test`      | Execute unit tests                        |
| `make lint`      | Format and vet Go code                    |
| `make docs`      | Generate API documentation (swag output)  |
| `make docker-build` | Build the Docker image                 |

## 4. Running Locally

```bash
make run
```

The server listens on the port specified by `SERVER_PORT` (default `8080`). Use tools such as `curl` or Postman to hit the `/health` endpoint once handlers are implemented.

## 5. Docker Workflow (Preview)

```bash
docker build -t go-backend:latest .
docker run --env-file .env -p 8080:8080 go-backend:latest
```

For local development, `docker-compose.yml` (added later) will orchestrate PostgreSQL, Redis, and the backend service.

## 6. Adding New Modules

1. Create new domain entities under `domain/`.
2. Define repository interfaces in `repository/`.
3. Implement use cases under `usecase/`.
4. Create HTTP handlers in `api/handler/` and wire them in `internal/router`.
5. Update docs and make sure tests cover the new logic.

## 7. Next Steps

- Fill in configuration loading logic (`internal/config`).
- Implement infrastructure layers (Postgres, Redis, BoltDB).
- Add middleware, handlers, and offline buffer processing.

Refer to `docs/quickstart.md` whenever you onboard new contributors or move between environments. Keep this file updated as the project evolves.

