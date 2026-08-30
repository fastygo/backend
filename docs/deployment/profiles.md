# Codex deployment profiles

| Profile | Storage/bind | Intended use |
| --- | --- | --- |
| Local development | bbolt or SQLite, loopback | Developer validation only |
| Local-colocate | SQLite, two local processes | Bootstrap/demo; not HA or public topology |
| Single-node tenant | Durable bbolt/SQLite, private bind | Small controlled deployment |
| Shared production | Postgres/MySQL/MariaDB plus durable media, private bind | Production target |

The public site BFF, isolated admin, and Codex are separate planes in
production. `APP_BIND=0.0.0.0` in `.env.example` is a convenience default, not
a production recommendation; set a private bind behind TLS instead.
