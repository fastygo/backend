# ADR-0009: Locale is a dimension, not payload_* columns

- Status: Accepted

A tenant has one DSN. Canonical identity stays on the entry. Translated
FormSet fields live in `entry_locales` (logical): `(entry_id, locale, data)`
with `UNIQUE(entry_id, locale)`. The tenant lists locales and a default.
Reads use `?locale=` and whole-document fallback. Mutations write only the
requested locale.

## Why

`payload_ru` / `payload_en` hardcoded two JSON blobs into FormSet, the
manifest, seeds, and admin. That does not scale to N locales, does not match
wp-json inbound, and duplicates non-localized fields.

## Storage (logical)

```text
entries (id, kind, status, …)
entry_locales (entry_id, locale, data jsonb, status, updated_at)
UNIQUE (entry_id, locale)
```

Adapters (SQLite / Postgres / …) may keep a single documents table plus
locale rows, or JSON metadata during migration, as long as the domain and
REST no longer name `payload_<lang>`.

- Canon: kind, workflow, relations, money, media ids, shared metadata.
- `data`: one FormSet document for that locale (title, body, Extra keys).
- Per-locale `status` so `en` can be published while `de` stays draft.

## API

- `GET …?locale=de` returns one document. If the row is missing or empty,
  serve the tenant fallback chain (`de` → default). Do not mix fields from
  multiple locales in one body.
- Response states `locale` (served), `requested`, and `fallback` when they
  differ.
- `POST`/`PATCH` with `locale` write that row only. No write-through to the
  fallback locale.
- Default list/get without `locale` uses the tenant default.
- Do not return all locales in the default content envelope. A compact
  locale index (slug, status) may exist for hreflang.

## FormSet

Bind one locale document (or an explicit locale map). Drop
`Documents{RU,EN}`, `BindDocuments`, and `PayloadDocuments`. Manifests do
not declare `payload_ru` / `payload_en` storage fields.

## Migration

Existing metadata `payload_ru` / `payload_en` become `entry_locales` rows
(`ru`, `en`). Single-locale tenants keep one row. Seed JSON uses
`locales: { "en": { … } }` (or equivalent), not parallel payload keys.

## Out of scope

BFF-authored translations. WordPress adapters. Field-level maps inside one
JSON object.
