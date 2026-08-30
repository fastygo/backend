# Codex backend model

## Ownership

Codex owns generic records, manifests, FormSet projection, authorization,
revisions, lifecycle, users/roles, audit, storage adapters, and media
metadata. Product sites own user experience, routes, content, and tenant
deployment choice.

## Runtime

```text
BFF published reads / Admin authenticated writes
    -> Codex REST delivery
    -> application services + authorization
    -> tenant storage + media root
```

`HEADLESS_MANIFEST_PATH` adds product resources to reserved Codex kinds. The
manifest is input to the process, not compiled product logic.

## Data operations

- `headless-seed` is a deliberate one-shot bootstrap/import CLI; it is not an
  environment-driven reconciliation feature of `cmd/server`.
- `headless-backup` exports/restores metadata plus a media archive. Recovery
  needs both artifacts and a manifest digest match.
- SQL migrations run at startup for supported SQL adapters. Products must
  perform their own release compatibility and backup gate.
- Multiple BaaS processes show one tenant only when they use the same DSN and
  compatible manifest. SQLite is a single-writer topology, not write scaling.

## Health

`/healthz` is liveness. `/readyz` includes Codex storage readiness. Product
BFF readiness is separate and cannot be inferred from a healthy Codex process.
