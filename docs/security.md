# Codex security

## Network and identity

Codex is private in production; TLS terminates at a trusted proxy. Cookie
sessions use `/go-json/auth/*`; bearer tokens serve automation. Do not expose
the database, media root, or unsecured Codex bind to the internet.

## CSRF coverage

Cookie-authenticated mutations require `X-CSRF-Token`. That includes collection
writes, status transitions, revision restore, taxonomy changes, media upload,
user and role writes, form bind, logout, and a cookie login that already has a
session cookie. A fresh cookie login has no token yet; browsers that send
`Sec-Fetch-Site: cross-site` are rejected. Bearer requests, including bearer
login, are exempt. GraphQL applies the same cookie check when the parsed
operation is a mutation.

## Secrets and data

Use random `HEADLESS_TOKEN_SECRET`; rotate/remove bootstrap administrator
credentials after initial login. Store secrets outside git. Persist and back up
both database metadata and `HEADLESS_MEDIA_ROOT`.

## CORS

Credentialed CORS is not a product deployment contract. SvelteCMS uses a
same-origin admin proxy or SSH tunnel. A future CORS feature requires an ADR,
origin allowlist, cookie review, and integration tests.
