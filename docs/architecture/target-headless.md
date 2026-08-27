# Target Headless Architecture

## Decision

GoBackend is a Go 1.25 headless content platform. It implements the observable
GoCMS `go-codex` Level 1 REST contract and the `headless` profile while also
providing a schema-driven data API for product-specific record types.

The backend is not a public-site renderer and does not own a frontend stack.
Svelte, React, Vue, server-rendered Go applications, mobile applications, and
other clients consume the same application services through delivery adapters.

## Platform boundaries

### Framework

`github.com/fastygo/framework` owns:

- `net/http` process composition and feature mounting;
- safe HTTP server defaults and graceful shutdown;
- request IDs, recovery, security headers, rate limiting, metrics, and tracing
  boundaries;
- liveness and readiness endpoints;
- background worker supervision;
- locale negotiation and optional cookie/OIDC primitives.

GoBackend must not maintain parallel implementations of these facilities.
The existing fasthttp router, custom lifecycle manager, HTTP context adapter,
health handler, Zap wrapper, and duplicate configuration surface are removed
as their replacements become wired.

### Panel

`github.com/fastygo/panel` owns UI-neutral control-plane descriptors:

- resources, forms, tables, details, actions, routes, and navigation;
- policy and principal contracts;
- optional data-source interfaces.

Panel does not own content persistence, taxonomy rules, publishing workflows,
or HTTP delivery. GoBackend projects its schemas and capabilities into Panel
descriptors. Any admin frontend may consume that projection.

### GoBackend

GoBackend owns:

- content and generic record domain invariants;
- application services and authorization;
- content types, field schemas, relations, taxonomies, localization, media
  metadata, revisions, lifecycle, settings, menus, users, roles, and audit;
- repository and transaction ports;
- bbolt, SQLite, MySQL/MariaDB, and PostgreSQL adapters;
- REST compatibility and generic data APIs;
- optional GraphQL generated from the same services and schemas;
- migrations, import/export, backup contracts, conformance fixtures, and
  installation artifacts.

## Compatibility surfaces

The required compatibility surface is:

```text
GET /go-json
GET /go-json/go/v2/
GET /go-json/go/v2/posts
GET /go-json/go/v2/posts/{id}
GET /go-json/go/v2/posts/by-slug/{slug}
GET /go-json/go/v2/pages
GET /go-json/go/v2/pages/{id}
GET /go-json/go/v2/pages/by-slug/{slug}
GET /go-json/go/v2/media
GET /go-json/go/v2/media/{id}
GET /go-json/go/v2/taxonomies
GET /go-json/go/v2/taxonomies/{type}
GET /go-json/go/v2/taxonomies/{type}/{id}
GET /go-json/go/v2/menus
GET /go-json/go/v2/menus/{location}
GET /go-json/go/v2/settings
GET /go-json/go/v2/search
```

The product-neutral extension surface is:

```text
/go-json/go/v2/types
/go-json/go/v2/{collection}
/go-json/go/v2/openapi.json
/go-graphql
/go-graphql
```

Both surfaces call the same application services. Delivery adapters may not
implement separate authorization, visibility, lifecycle, or persistence rules.

## Domain model

### Stable CMS resources

`post` and `page` remain reserved compatibility kinds with:

- stable ID and kind;
- localized slug, title, content, excerpt, and SEO fields;
- `draft`, `scheduled`, `published`, `archived`, and `trashed` statuses;
- public/private visibility;
- author, parent, featured media, template, metadata, and taxonomy references;
- created, updated, published, and deleted timestamps;
- immutable revisions and restore.

### Product-defined resources

A versioned manifest defines additional resources such as:

- digital products and course categories;
- apparel products, sizes, colors, collections, and brands;
- corporate pages, teams, offices, and vacancies;
- blog-specific authors, series, and topics.

Schemas support required, nullable, read-only, sensitive, localized, enum,
relation, collection, JSON, date/time, numeric, money, URI, and media fields.
Relations declare cardinality and deletion behavior and are validated by
application services.

The active schema is supplied as a versioned manifest at startup **by the
product composition** (BFF or admin), not as a baked-in storefront profile.
Manifests are validated before storage or HTTP listeners are opened. Runtime
schema-draft editing is intentionally outside the core binary; products deploy
a reviewed manifest and use backup/restore when changing storage layouts.

### Taxonomies

Taxonomies are first-class definitions, not merely generic records. They
support:

- flat and hierarchical modes;
- assignment to declared resource kinds;
- localized terms and slugs;
- parent-cycle prevention;
- stable term IDs;
- assignment validation and taxonomy-aware filtering.

## Storage contract

Application packages own repository interfaces. Drivers implement the same
behavioral contract:

- lookup by ID and localized slug;
- filtered, sorted, paginated listing;
- create and expected-version update;
- trash and restore;
- revision and audit append;
- relation and taxonomy integrity;
- transaction participation;
- migrations and readiness;
- consistent export/import.

The supported adapters are:

| Adapter | Deployment |
| --- | --- |
| bbolt | Single binary plus local data file |
| SQLite | Single binary plus local SQL file |
| MySQL/MariaDB | External SQL service |
| PostgreSQL | External SQL service |

The bbolt adapter uses buckets and transactional documents. SQL adapters use a
portable normalized core with JSON metadata and driver-specific migration
dialects. Stable IDs, localized slugs, lifecycle timestamps, and taxonomy
assignments are indexed; localized search text is materialized outside the JSON
payload. Filtering, deterministic sorting, counting, and pagination run in SQL;
only the selected page payloads are decoded. Versioned migrations backfill
these projections for existing data.

Binary media storage is separate from metadata storage. The core distribution
ships a path-safe local filesystem adapter behind an object-store port; remote
blob adapters can be supplied without changing application services.

## Security

Authorization is enforced in application services before private reads and
mutations. Required capabilities include content, revisions, media,
taxonomies, users, roles, audit, and REST access.

The platform supports persistent users and roles, bcrypt passwords, expiring
Framework-signed bearer tokens, and CLI-issued service tokens. Token revocation,
password reset delivery, and edge login throttling are deployment integrations,
not claims of the core binary. Public clients only receive published public
records. Scheduled, draft, archived, trashed, private metadata, and private
media may not leak through REST, GraphQL, search, or hooks.

## Explicit exclusions

The core binary does not include:

- public SSR themes;
- a templ-based admin UI;
- a mandatory JavaScript toolchain;
- a dynamic in-process plugin loader;
- product-specific task, CRM, marketplace, or storefront business logic;
- mandatory Redis;
- offline buffering that reports writes as successful before durable commit.

Compile-time modules, external workers, webhooks, and frontend applications
extend the platform through documented APIs and events.

## Migration from the current repository

The following current areas are replaced:

- fasthttp router and handlers;
- task/profile/aggregate demo domains and repositories;
- Redis session/JWT mismatch;
- BoltDB offline write buffer;
- custom process lifecycle, health, logging, and HTTP configuration already
  supplied by Framework;
- aspirational examples that are not executable.

The migration preserves the module path, license, contribution metadata, and
useful deployment history. Compatibility tests are ported before old routes are
removed.

## Completion evidence

Completion requires all of the following:

1. Go 1.25 build with pinned Framework and Panel modules.
2. No duplicate platform runtime left in GoBackend.
3. Executable Level 0 and Level 1 headless conformance tests.
4. Identical repository contract tests for bbolt, SQLite, MySQL/MariaDB, and
   PostgreSQL.
5. Public visibility, capability, schema migration, relation, taxonomy,
   revision, and localization negative tests.
6. REST, GraphQL, and OpenAPI generated from the same active schemas and
   application services.
7. Docker deployment verification.
8. Bare-metal build, configuration, migration, service, backup, restore, and
   upgrade verification.
9. Requirement-by-requirement completion audit against go-codex and go-stack.
