# Agent notes

GoBackend is universal headless **Codex/BaaS**: one published module, many
tenant deployments. It owns `/go-json/go/v2`, storage adapters, identity,
media, revisions, and operational CLIs. It does not own product IA, storefront
routes, tenant markdown, seed files, carts, checkout, or a BFF.

Read before non-trivial work:

1. `docs/README.md`
2. `docs/contract.md`, `docs/architecture/target-headless.md`
3. `docs/backend.md`, `docs/security.md`, `docs/operations/`
4. `docs/architecture/adr/`
5. `internal/conformance/`

SiteStarter `.project/` is the cross-plane consumer reference. If a product
needs a missing shared Codex capability, make an ADR and dedicated backend
release; do not add product behavior here.

Hard rules:

- No storefront SSR, product schemas in `DefaultManifest`, cart/checkout, or
  server-side markdown compilation.
- No second content REST API or duplicate auth/visibility/lifecycle rules.
- No committed local `replace` for Framework, Panel, or FormSet.
- `headless-seed` is a one-shot CLI; do not invent server seed-on-start env.
- Do not claim direct credentialed CORS or public `0.0.0.0` bind as production.
- Auth/session/CSRF modifications require REST tests and security-doc updates.

Run `make verify`, `make live-sql`, and
`go test ./internal/conformance/...` when their affected contracts change.
