# Environment matrix

| Input | Owner | Requirement |
| --- | --- | --- |
| `DATABASE_URL` / `HEADLESS_BBOLT_PATH` | Tenant operator | Durable tenant storage |
| `HEADLESS_MEDIA_ROOT` | Tenant operator | Durable storage; backed up with metadata |
| `HEADLESS_MANIFEST_PATH` | Product release | Versioned product schema input |
| `HEADLESS_TOKEN_SECRET` | Operator | Secret, random, rotated by policy |
| `HEADLESS_ADMIN_*` | Bootstrap operator | Remove after verified initial login |
| `APP_BIND`, TLS/proxy | Operator | Private production bind and TLS edge |

`HEADLESS_STORAGE` selects bbolt/sqlite/MySQL/MariaDB/Postgres. A server must
not seed/reconcile product content from an environment variable at startup.
