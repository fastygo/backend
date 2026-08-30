# Codex documentation

GoBackend owns the product-neutral BaaS. SiteStarter owns consumer topology;
SvelteCMS owns the browser client.

Read: [glossary](glossary.md) → [contract](contract.md) → [architecture](architecture/target-headless.md)
→ [backend](backend.md) → [security](security.md) → [SLO](operations/slo.md)
→ [ADRs](architecture/adr/) → [operations](operations/) → [deployment](deployment/).

- [Quick start](quickstart.md)
- [Production checklist](deployment/production-checklist.md)
- [Runbooks](runbooks/)

Runtime API contracts are generated from the active manifest:
`/go-json/go/v2/openapi.json`, `/go-json/go/v2/schema`, and optional
`/go-json/go/v2/graphql.sdl`.
