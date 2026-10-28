# ADR-0010: Canonical Codex HTTP surface is `/codex/v1`

- Status: Accepted

GoBackend exposes a public, admin-neutral Codex HTTP surface at `/codex/v1`.
Every admin (WordPress, Go, PHP, Svelte, local) answers the same paths with
the same bytes. The existing `/go-json/go/v2` envelope stays unchanged.

## Why

`/go-json/go/v2` is a GoCMS compatibility projection: flattened metadata,
`taxonomy_ids` without taxonomy, extra `document` / `requested` / `fallback`
/ `links` fields, and chrome maps. Codex v0.4 (`codex-entry/v1`) is a closed
record: one locale per body, unflattened public metadata, JCS bytes, and
`Codex-Digest`. Mixing those in one route would break both clients.

## Decision

| Prefix | Audience | Body |
| --- | --- | --- |
| `/go-json/go/v2` | GoCMS / FormSet / Panel | Compatibility envelope (maps, extra fields) |
| `/codex/v1` | Any Codex-conform admin | Canonical v0.4 records |

`GET /codex/v1/` returns `{codex_version, profile:"codex-entry/v1",
manifest_digest}` where `codex_version` is the spec constant `0.4.0`
(`CODEX_V04_VERSION` / `rest.CodexV04Version`, not the `fastygo/codex`
module tag) and `manifest_digest` is Codex `Manifest.DigestV2`
(`codex-manifest/v2:sha256:…`) of the served Codex-shaped manifest without
`Canonical()` sorting. The WordPress delivery-map key is `wp`. List
envelopes are exactly `{data, pagination}`. Empty nested values are kept; only top-level empty optionals are
omitted. Every `/codex/v1` body is RFC 8785 JCS with no trailing newline
and no `encoding/json` HTML/`U+2028` escapes. A single entry is the raw JCS of `codex-entry/v1` with
`Codex-Digest` and `ETag`. `GET /codex/v1/settings` lists SettingsV04 records
(own type, not `kind=setting` entries, including optional `contact` /
`hours` / `hours_exceptions` / `map`); `GET /codex/v1/settings/{id}` is raw
JCS with digest headers (`codex-settings/v1`). Terms use `codex-term/v1` and omit `count`. Default
locale is `settings.language`, else `"en"`. Public reads only; everything
else is 404. Errors use the gocms-02 shape with `validation_error` /
`not_found`; `request_id` is optional.

Storage keeps v0.3-shaped chrome maps and FormSet locale documents in private
sidecar metadata so `/go-json/go/v2` can still reconstruct maps. Domain
records follow Codex v0.4 strings plus `locale`.

## Out of scope

A second content REST API. Product storefront routes. Replacing
`/go-json/go/v2`. The official `codex-conform` CLI (not shipped at contract
commit 8299fee); this repo runs `TestCodexV1Conform` until that CLI exists.
