# Quality attributes

| Attribute | Position |
| --- | --- |
| Compatibility | Stable REST/conformance from one application-service layer |
| Tenant isolation | Product supplies a distinct DSN/manifest; Codex never owns product IA |
| Extensibility | Manifest resources and FormSet projection, not product forks |
| Operability | Health/readiness, metrics, backup/token/seed CLIs |
| Security | Capability authorization, sessions, CSRF policy, private deployment |
| Portability | Published module pin; no local module replacements |

Non-goals: storefront SSR, content authoring files on a server, carts,
checkout/payments, product BFF databases, and an automatic second SoT.
