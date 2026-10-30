# Runbook: Codex import over stdio

The server never opens an import port. The desktop app converts markdown
locally and pipes ready `codex-entry/v1` NDJSON into a forced SSH command
that runs `backend codex import --stdio --tenant=<id>` inside the tenant
slot (usually `incus exec`). See ADR-0012.

## Forced command

Install the binary as `backend` (or `headless-backend`). One key per tenant,
no PTY, no forwarding. The key's `command=` runs `backend codex ssh-forced`,
which reads `$SSH_ORIGINAL_COMMAND` (what the client asked for) and applies
a strict allowlist before any storage is opened:

```
[incus exec <slot> --] [backend|headless-backend|/usr/local/bin/backend] codex import --stdio --tenant=<id> [--dry-run]
```

- flags in any order, each at most once; `--stdio` and `--tenant` required;
- `--tenant` must equal the tenant pinned with `ssh-forced --tenant=`;
- the `incus exec <slot> --` prefix (the cms-console default) is accepted
  only when `--slot=<slot>` is pinned and matches;
- only `[A-Za-z0-9 ._=/-]`, at most 256 bytes: quotes, `;`, `&`, `|`, `$`,
  backticks, tabs and newlines are refused; so are extra arguments and every
  other subcommand. A refused command exits non-zero with a one-line reason
  on stderr and writes nothing.

So one key serves both the dry-run and the apply; no second key is needed.

### Backend runs inside the incus slot (sshd on the host)

`$SSH_ORIGINAL_COMMAND` is not inherited by `incus exec`; pass it explicitly.
The value is expanded once inside double quotes (no word splitting, no
evaluation) and is then checked by the allowlist:

```
command="/usr/bin/incus exec shop --env SSH_ORIGINAL_COMMAND=\"$SSH_ORIGINAL_COMMAND\" -- /usr/local/bin/backend codex ssh-forced --tenant=shop --slot=shop",no-agent-forwarding,no-port-forwarding,no-pty,no-X11-forwarding,no-user-rc ssh-ed25519 AAAA... import@desktop
```

### sshd in the same place as the binary

```
command="/usr/local/bin/backend codex ssh-forced --tenant=shop",no-agent-forwarding,no-port-forwarding,no-pty,no-X11-forwarding,no-user-rc ssh-ed25519 AAAA... import@desktop
```

### Client side (cms-console)

The client sends the command it wants, for example
`incus exec shop -- backend codex import --stdio --tenant=shop --dry-run`
(cms-console with `slot` + `tenant`) or `codex import --stdio --tenant=shop`
(`remote_command`). Stdin is the NDJSON batch; stdout is per-record reports
then the tenant `codex-manifest/v2` JCS object. Exit status is non-zero when
any line is `rejected`.

Check a key without importing anything:

```bash
ssh -T -i ~/.ssh/import_shop import@host 'codex import --stdio --tenant=shop --dry-run' < /dev/null
ssh -T -i ~/.ssh/import_shop import@host 'sh'          # refused: only `codex import` is allowed
```

Settings, menus and media are **not** imported here: each site ships them to
its own BFF (cms-console ADR 0011, `cms-site-bff/v1`).

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
