# Quick start

```bash
cp .env.example .env
export HEADLESS_TOKEN_SECRET="$(openssl rand -base64 48)"
export HEADLESS_ADMIN_EMAIL="admin@example.com"
export HEADLESS_ADMIN_PASSWORD="$(openssl rand -base64 32)"
go test ./...
go run ./cmd/server
```

Check the runtime:

```bash
curl -fsS -H "User-Agent: quickstart/1.0" http://127.0.0.1:8080/readyz
curl -fsS -H "User-Agent: quickstart/1.0" http://127.0.0.1:8080/go-json/go/v2/schema
```

Authenticate the durable bootstrap administrator through
`POST /go-json/go/v2/auth/login`. Remove the two `HEADLESS_ADMIN_*` variables after the first
successful startup. For recovery or service automation, create a standalone administrator token:

```bash
go run ./cmd/headless-token -subject admin -role administrator -ttl 24h
```

Use `HEADLESS_MANIFEST_PATH` to load extra resources from the **product** repo. Without one, the
backend starts with Codex `post`, `page`, `menu`, and `setting` (each with a FormSet form).
`./dev/example.manifest.json` is a generic extra CPT, not a storefront.

## Explicit bootstrap and recovery

Seed only deliberately, after the target storage and product manifest are
configured. It creates missing records and skips existing ones; it is not a
server-start synchronization mechanism.

```bash
go run ./cmd/headless-seed -path /secure/product.seed.json
go run ./cmd/headless-backup -mode export -path /secure/backup.json
```

Recovery requires the paired `backup.json` and `backup.json.media.tar` in an
empty target store:

```bash
go run ./cmd/headless-backup -mode restore -path /secure/backup.json
```

See [operations](operations/) for release, backup, and SLO boundaries.
