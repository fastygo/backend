# Headless Backend

A brand-neutral, manifest-driven content backend for Go 1.25. It uses
[`fastygo/framework`](https://github.com/fastygo/framework) for the process and HTTP lifecycle
and [`fastygo/panel`](https://github.com/fastygo/panel) for UI-neutral control-plane descriptors.

The backend has no SSR, theme, plugin, Redis, or JavaScript runtime dependency. It can serve a
store, marketplace, corporate site, documentation site, or blog by changing the resource manifest.

## Capabilities

- Dynamic resource schemas, fields, relations, localization, and taxonomies
- Draft, scheduled, published, archived, and trashed lifecycle
- Optimistic locking, revisions, revision restore, and scheduled publishing
- Public/private projection and capability-based access control
- REST, executable GraphQL, GraphQL SDL, JSON Schema, and OpenAPI 3.1
- Secure media upload and download with local durable blob storage
- Transactional audit events
- Cross-adapter metadata backup/restore and media archive backup/restore
- bbolt, SQLite, MySQL, MariaDB, and PostgreSQL storage selection
- Framework health, readiness, metrics, graceful shutdown, and background workers
- Docker and bare-metal operation

## Run locally

```bash
cp .env.example .env
# Set HEADLESS_TOKEN_SECRET to at least 32 random bytes.
go run ./cmd/server
```

Local GitCourse development uses SQLite at `./var/lib/headless/backend.sqlite`
and `./dev/gitcourse.manifest.json`. After the first start, seed published
records from the storefront repo:

```bash
go run ./cmd/headless-seed -path ../@GitCourse/cms/gitcourse.data-seed.json
```

The default bbolt deployment stores data under `./var/lib/headless`.

```bash
curl -H "User-Agent: headless-client/1.0" http://127.0.0.1:8080/healthz
curl -H "User-Agent: headless-client/1.0" http://127.0.0.1:8080/readyz
curl -H "User-Agent: headless-client/1.0" http://127.0.0.1:8080/go-json/data/v1/openapi.json
```

## Resource manifest

Set `HEADLESS_MANIFEST_PATH` to a JSON manifest:

```json
{
  "name": "store",
  "version": "1",
  "resources": [
    {
      "id": "product",
      "collection": "products",
      "public": true,
      "rest_visible": true,
      "graphql_visible": true,
      "taxonomies": ["brand", "collection"],
      "fields": [
        {"id": "price", "type": "money", "required": true},
        {"id": "description", "type": "text", "localized": true}
      ]
    }
  ]
}
```

Panel descriptors, REST paths, GraphQL types, JSON Schema, and OpenAPI are generated from this
manifest.

## Authentication

Public reads are anonymous. Mutations and private reads require a Framework-signed bearer token.
On an empty identity store, `HEADLESS_ADMIN_EMAIL` and `HEADLESS_ADMIN_PASSWORD` create the first
administrator. Remove those bootstrap values after startup, then authenticate with:

```http
POST /go-json/data/v1/auth/login
Content-Type: application/json

{"email":"admin@example.com","password":"..."}
```

Users and roles are durable, versioned, capability-protected resources. Passwords use bcrypt and
password hashes are never serialized by the REST API. The token CLI remains available for service
accounts and recovery:

```bash
export HEADLESS_TOKEN_SECRET="$(openssl rand -base64 48)"
go run ./cmd/headless-token -subject admin -role administrator -ttl 24h
```

Use the output as `Authorization: Bearer <token>`.

## API surfaces

- REST resources: `/go-json/data/v1/resources/{resource}`
- go-codex Level 0/1 compatibility: `/go-json`, `/go-json/go/v2/`
- GraphQL: `/go-json/data/v1/graphql`
- GraphQL SDL: `/go-json/data/v1/graphql/schema`
- Schema identity: `/go-json/data/v1/schema`
- Resource JSON Schema: `/go-json/data/v1/schema/{resource}`
- OpenAPI: `/go-json/data/v1/openapi.json`
- Media: `/go-json/data/v1/media`
- Taxonomy definitions and terms: `/go-json/data/v1/taxonomies`
- Login, users, and roles: `/go-json/data/v1/auth/login`, `/go-json/data/v1/users`, `/go-json/data/v1/roles`
- Audit: `/go-json/data/v1/audit`
- Liveness/readiness: `/healthz`, `/readyz`
- Metrics: `/metrics` when enabled

The REST `values` and pagination representation is compatible with `fastygo.data` consumers.
The generated GraphQL resource operations support the SvelteCMS repository contract.
Default go-codex resources cover posts, pages, menus, settings, media, taxonomies, content types,
localized slug lookup, and search.

## Storage

Select the adapter with `HEADLESS_STORAGE`:

- `bbolt`: set `HEADLESS_BBOLT_PATH`
- `sqlite`: set `DATABASE_URL` to the SQLite file
- `mysql` or `mariadb`: set a Go MySQL driver DSN
- `postgres` or `postgresql`: set a PostgreSQL URL

SQL schema migrations run idempotently during startup.

## Backup and restore

Stop writes or place the service in maintenance mode before an operational restore.

```bash
go run ./cmd/headless-backup -mode export -path ./backup.json
go run ./cmd/headless-backup -mode restore -path ./backup.json
```

The command writes `backup.json` and `backup.json.media.tar`. Metadata backup format v3 includes
content, revisions, audit events, taxonomy definitions, and terms. Restore requires empty metadata
and media stores and verifies the manifest digest before importing.

## Docker

```bash
docker compose up --build backend
docker compose --profile sqlite up --build backend-sqlite
docker compose --profile postgres up --build postgres backend-postgres
docker compose --profile mysql up --build mysql backend-mysql
docker compose --profile mariadb up --build mariadb backend-mariadb
```

Docker requires the secrets declared in `.env.example`. The image runs as an unprivileged user.

## Verification

```bash
make verify
```

The production gate runs tests, conformance, vet, Staticcheck, `govulncheck`,
Linux race detection, module verification, and all command builds. On Windows,
the race detector runs in Docker. CI additionally exercises PostgreSQL, MySQL,
and MariaDB through `make live-sql`.

See `docs/deployment/bare-metal.md` for a systemd installation and
`docs/architecture/target-headless.md` for architectural boundaries.
