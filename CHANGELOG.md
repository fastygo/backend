# Changelog

## Unreleased

### Breaking
- Domain schema uses `github.com/fastygo/codex` v0.4.2 types via aliases
  (ADR-0013). Manifest fields use codex types and rules; `money`, `integer`,
  `decimal`, `enum`, `media` as field types are rejected.
- Money values are integers in minor units with an ISO 4217 currency.
  String decimals are no longer accepted.

### Added
- `HEADLESS_DEFAULT_CURRENCY` for records without a `currency` field.
- Forms via `github.com/fastygo/formset`, panel resources via
  `panel.ResourceFromRecord` (lead columns `title`, `status`; trail `updated_at`).
- CI: `go test -race`, gofmt, `go mod tidy -diff`, golangci-lint v2.11.4,
  vet/test of storage drivers per build tag (sqlite, mysql, postgres).
- Tests: storage registry, drivers, contentcompat, end-to-end REST routes.

### Decisions
- govet `shadow` disabled in golangci config (74 stylistic hits, no bugs).
- Canonical JCS codex resources come from `Resource.Record()` so legacy
  types cannot leak into hashed documents.
