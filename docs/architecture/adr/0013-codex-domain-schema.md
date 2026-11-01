# ADR-0013: Domain schema is projected from codex v0.4.2

- Status: Accepted

`github.com/fastygo/codex` (pinned at `v0.4.2`) is the single source of truth for
field, relation, and validation types. `internal/domain/schema` re-exports them as
type aliases and never copies them.

- Field `type` values are the codex field types (`string`, `text`, `number`,
  `boolean`, `date`, `select`, `relation`, `json`, …). Legacy backend types
  (`money`, `integer`, `decimal`, `enum`, `media`) are rejected with a hint.
- Semantics come from codex rules: `fastygo.codex/integer`, `decimal`, `money`
  (safe integer in minor units + ISO 4217 currency), `date`, `uri`, `uuid`,
  `nullable`, `read-only`. Enum = `select` + `options`. Media = `relation` with
  the media UI hint and target `media`.
- `schema.Resource` keeps delivery metadata codex does not model: `collection`,
  `public`, `rest_visible`, `graphql_visible`, `form`, `taxonomies`.
  `Resource.Record()` returns the pure codex `RecordType`.
- Values are validated with codex `CheckMetadataFields` / `CheckMoneyCurrency`.
  The default currency is `HEADLESS_DEFAULT_CURRENCY` (when a record has no
  `currency` field).
- Forms are built through `formset.FromCodex`, panel resources through
  `panel.ResourceFromRecord`.
- The media target `media` is built in; it is not declared in the manifest.
  Codex expects a declared target resource — recorded as a codex gap.

Consequences: breaking manifest format change; JSON Schema, OpenAPI and GraphQL
SDL publish codex rules (money → `Int`).
