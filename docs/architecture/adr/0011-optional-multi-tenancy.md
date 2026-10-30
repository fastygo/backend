# ADR-0011: Optional multi-tenancy is one implicit default tenant

- Status: Accepted

A process with no `HEADLESS_TENANTS_PATH` is unchanged: one site from
`DATABASE_URL` / `HEADLESS_BBOLT_PATH`, `HEADLESS_MEDIA_ROOT`, and
`HEADLESS_MANIFEST_PATH`. That site is the same tenant object as a named
entry in `tenants.json`; there is no second request path.

When `HEADLESS_TENANTS_PATH` is set, the file is a JSON array:

```json
[
  {
    "id": "shop",
    "hosts": ["shop.example.com"],
    "storage": { "dsn": "/var/lib/headless/shop.sqlite" },
    "media_root": "/var/lib/headless/shop/media",
    "manifest": "/etc/headless/shop.manifest.json",
    "settings": { "language": "en-US", "title": "Latte Shop" }
  }
]
```

`storage.dsn` is the SQL data source (SQLite file path, Postgres URL, or
MySQL DSN). `storage.path` is the bbolt file. `storage.driver` is optional;
it is inferred from `dsn`/`path` when omitted (`sqlite` for a file DSN,
`bbolt` for `path`). Each tenant has its own storage and media root; data
never mixes.

HTTP selects the tenant by exact `Host` match. The host is lowercased and a
trailing port may be stripped. Unknown hosts in multi mode are `404
not_found` with no default fallback. `X-Forwarded-Host` is ignored; Caddy
(or another trusted proxy) must pass the real `Host` through. Single mode
ignores `Host` and always uses the implicit default tenant.

`/codex/v1`, `/go-json/go/v2`, cookie sessions, CSRF, and GraphQL are scoped
to the matched tenant. Session tokens issued in multi mode carry the tenant
id (`tid`); a token or CSRF value from another tenant is rejected. Process
health (`/healthz`, `/readyz`) stays process-wide.

Documented production storage is SQLite via `modernc.org/sqlite` (`go build
-tags sqlite`). bbolt remains in the untagged binary for local installs and
is not removed.
