# Codex security

## Network and identity

Codex is private in production; TLS terminates at a trusted proxy. Cookie
sessions use `/go-json/auth/*`; bearer tokens serve automation. Do not expose
the database, media root, or unsecured Codex bind to the internet.

## CSRF coverage

Cookie-authenticated collection create/update/delete, taxonomy CRUD, identity
CRUD, media upload, transitions, revision restore, GraphQL mutations, and
logout are guarded by CSRF. Bearer requests are exempt. Login endpoints do
not require a pre-issued CSRF token because they establish the session. Do
not claim CSRF on login until a dedicated anti-login-CSRF design exists.

## Secrets and data

Use random `HEADLESS_TOKEN_SECRET`; rotate/remove bootstrap administrator
credentials after initial login. Store secrets outside git. Persist and back up
both database metadata and `HEADLESS_MEDIA_ROOT`.

## CORS

Credentialed CORS is not a product deployment contract. SvelteCMS uses a
same-origin admin proxy or SSH tunnel. A future CORS feature requires an ADR,
origin allowlist, cookie review, and integration tests.
