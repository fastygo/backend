# Runbook: Codex import over stdio

The server never opens an import port. The desktop app converts markdown
locally and pipes ready `codex-entry/v1` NDJSON into a forced SSH command
that runs `backend codex import --stdio --tenant=<id>` inside the tenant
slot (usually `incus exec`). See ADR-0012.

## Forced command

Install the binary as `backend` (or keep `headless-backend` and point the
command at it). One key per tenant, no PTY, no forwarding:

```
command="/usr/bin/incus exec shop -- /usr/local/bin/backend codex import --stdio --tenant=shop",no-agent-forwarding,no-port-forwarding,no-pty,no-X11-forwarding,no-user-rc ssh-ed25519 AAAA... import@desktop
```

Dry-run uses the same keying only if you add a second command that includes
`--dry-run`. Stdin is the NDJSON batch; stdout is per-record reports then the
tenant `codex-manifest/v2` JCS object. Exit status is non-zero when any line
is `rejected`.

Example upsert and delete:

```json
{"id":"post-about","locale":"en-US","kind":"post","status":"published","visibility":"public","slug":"about","title":"About","content":"# Hello","version":1,"created_at":"2026-10-30T15:00:00Z","updated_at":"2026-10-30T15:00:00Z"}
{"op":"delete","id":"post-retired"}
```

Envelope form with a digest the server verifies:

```json
{"op":"upsert","digest":"codex-entry/v1:sha256:…","entry":{…codex-entry/v1…}}
```

## Caddy per domain

Caddy terminates TLS and must pass `Host` through. The backend matches
`tenants.json` hosts exactly (lowercase, port stripped) and ignores
`X-Forwarded-Host`.

```
shop.example.com {
	reverse_proxy 127.0.0.1:8080
}

press.example.com {
	reverse_proxy 127.0.0.1:8080
}
```

Bind the backend to loopback. One process, two tenants, two `storage.dsn`
SQLite files.

## SQLite consistent backup before restic

Documented production storage is SQLite (`go build -tags sqlite`). Take a
consistent copy with `sqlite3 .backup` before restic, then include media:

```bash
install -d -m 0750 /var/lib/headless/backups
sqlite3 /var/lib/headless/shop.sqlite ".backup '/var/lib/headless/backups/shop.sqlite'"
sqlite3 /var/lib/headless/press.sqlite ".backup '/var/lib/headless/backups/press.sqlite'"
restic backup /var/lib/headless/backups /var/lib/headless/shop/media /var/lib/headless/press/media
```

Run this as a pre-hook so restic never copies a live WAL mid-write. Restore
still needs the matching Codex manifest. bbolt installs keep using
`headless-backup` instead of `.backup`.
