# Codex security

## Network and identity

Codex is private in production; TLS terminates at a trusted proxy. Cookie
sessions use `/go-json/auth/*`; bearer tokens serve automation. Do not expose
the database, media root, or unsecured Codex bind to the internet.

## CSRF coverage

Cookie-authenticated collection create/update/delete and logout are guarded by
CSRF. Bearer requests are exempt. The following mutation paths require
security review because coverage is not yet proven uniformly by the current
test suite: transitions/revision restore, media upload, taxonomy CRUD, and
identity CRUD. Do not claim “CSRF on every mutation” until tests and handler
policy demonstrate it.

## Secrets and data

Use random `HEADLESS_TOKEN_SECRET`; rotate/remove bootstrap administrator
credentials after initial login. Store secrets outside git. Persist and back up
both database metadata and `HEADLESS_MEDIA_ROOT`.

## CORS

Credentialed CORS is not a product deployment contract. SvelteCMS uses a
same-origin admin proxy or SSH tunnel. A future CORS feature requires an ADR,
origin allowlist, cookie review, and integration tests.
