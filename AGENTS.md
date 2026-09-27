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
- No committed local `replace` for Codex, Framework, Panel, or FormSet.
  Pin published tags: Codex v0.3.0, FormSet v0.2.0, Framework v0.4.0, Panel v0.1.0.
- The untagged server binary links bbolt only. SQL drivers use `-tags sqlite`,
  `mysql`, or `postgres`. MariaDB uses the mysql tag.
- `headless-seed` is a one-shot CLI; do not invent server seed-on-start env.
- Do not claim direct credentialed CORS or public `0.0.0.0` bind as production.
- Auth/session/CSRF modifications require REST tests and security-doc updates.
- Cookie mutations require `X-CSRF-Token`. Bearer requests are exempt.

The laptop canary is `go test -count=1 ./...` and `go vet ./...`.
`make verify` (race) and `make live-sql` run on a VPS, not on this machine.
