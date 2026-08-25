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
curl -fsS -H "User-Agent: quickstart/1.0" http://127.0.0.1:8080/go-json/data/v1/schema
```

Authenticate the durable bootstrap administrator through
`POST /go-json/data/v1/auth/login`. Remove the two `HEADLESS_ADMIN_*` variables after the first
successful startup. For recovery or service automation, create a standalone administrator token:

```bash
go run ./cmd/headless-token -subject admin -role administrator -ttl 24h
```

Use `HEADLESS_MANIFEST_PATH` to load product-specific resources. Without one, the backend starts
with neutral `post`, `page`, `menu`, and `setting` resources.
