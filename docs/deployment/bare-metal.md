# Bare-metal installation

## Build

Build on the target host or cross-compile a static Linux binary:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o headless-backend ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o headless-backup ./cmd/headless-backup
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -trimpath -ldflags="-s -w" -o headless-token ./cmd/headless-token
```

## Install

```bash
sudo useradd --system --home /var/lib/headless-backend --shell /usr/sbin/nologin headless
sudo install -o root -g root -m 0755 headless-backend /usr/local/bin/headless-backend
sudo install -o root -g root -m 0755 headless-backup /usr/local/bin/headless-backup
sudo install -o root -g root -m 0755 headless-token /usr/local/bin/headless-token
sudo install -d -o headless -g headless -m 0750 /var/lib/headless-backend/media
sudo install -d -o root -g headless -m 0750 /etc/headless-backend
sudo install -o root -g root -m 0644 deploy/systemd/headless-backend.service \
  /etc/systemd/system/headless-backend.service
```

Create `/etc/headless-backend/backend.env`:

```dotenv
APP_BIND=127.0.0.1:8080
APP_AVAILABLE_LOCALES=en,ru
HEALTH_LIVE_PATH=/healthz
HEALTH_READY_PATH=/readyz
METRICS_PATH=/metrics
LOG_FORMAT=json

HEADLESS_STORAGE=bbolt
HEADLESS_BBOLT_PATH=/var/lib/headless-backend/backend.db
HEADLESS_MEDIA_ROOT=/var/lib/headless-backend/media
HEADLESS_MEDIA_MAX_BYTES=33554432
HEADLESS_SCHEDULE_INTERVAL=1m
HEADLESS_TOKEN_ISSUER=headless-backend
HEADLESS_TOKEN_SECRET=replace-with-at-least-32-random-bytes
HEADLESS_ADMIN_EMAIL=admin@example.com
HEADLESS_ADMIN_PASSWORD=replace-with-a-long-random-bootstrap-password
```

Protect the environment file and start the service:

```bash
sudo chown root:headless /etc/headless-backend/backend.env
sudo chmod 0640 /etc/headless-backend/backend.env
sudo systemctl daemon-reload
sudo systemctl enable --now headless-backend
curl -fsS http://127.0.0.1:8080/readyz
```

After the first administrator can log in, remove `HEADLESS_ADMIN_EMAIL` and
`HEADLESS_ADMIN_PASSWORD` from the environment file and restart. Existing durable users are not
changed during subsequent starts.

Put TLS and request-size limits at a reverse proxy. Keep the backend bound to loopback unless it
is behind a trusted private network.

## SQLite or external SQL

For SQLite:

```dotenv
HEADLESS_STORAGE=sqlite
DATABASE_URL=/var/lib/headless-backend/backend.sqlite
```

For PostgreSQL:

```dotenv
HEADLESS_STORAGE=postgres
DATABASE_URL=postgres://headless:password@db.example.com:5432/headless?sslmode=verify-full
```

For MySQL or MariaDB:

```dotenv
HEADLESS_STORAGE=mysql
DATABASE_URL=headless:password@tcp(db.example.com:3306)/headless?parseTime=true&tls=true&charset=utf8mb4
```

The database account needs permission to create and alter the backend-owned tables during startup.

## Backup

Run backups as the service user so bbolt and media permissions remain consistent:

```bash
sudo -u headless /usr/local/bin/headless-backup \
  -mode export -path /var/lib/headless-backend/backups/backend.json
```

This creates the metadata backup and a sibling `.media.tar` archive. Copy both files to independent
storage. Test restore regularly on a separate empty installation.

## Upgrade

1. Export metadata and media backups.
2. Stop the service.
3. Replace the three binaries.
4. Start the service; idempotent migrations run before readiness succeeds.
5. Check `/readyz`, `/metrics`, and recent logs.

Rollback requires restoring both the previous binaries and a backup compatible with their manifest.
