# Codex contract

Codex is a published product-neutral process. A product installs
`github.com/fastygo/backend/cmd/server@<pin>` and supplies its tenant
`DATABASE_URL`, manifest, and secrets.

| Contract | Consumer expectation |
| --- | --- |
| Content API | `/go-json/go/v2` REST envelope and manifest discovery |
| Schema | Reserved core resources plus product manifest resources |
| Admin auth | Cookie session `/go-json/auth/*` and CSRF policy |
| Server identity | Bearer capabilities for services and automation |
| Storage | Codex alone opens the tenant database and media root |
| Lifecycle | Version preconditions, revisions, visibility, scheduling |
| Operations | `/healthz`, `/readyz`, optional `/metrics`, backup CLI |

GraphQL is an optional Codex delivery adapter, not the portable product
content contract. Product BFF/admin behavior remains outside this module.
