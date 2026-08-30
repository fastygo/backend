# ADR-0002: Product owns tenant DSN and manifest

- Status: Accepted

Each deployment supplies its `DATABASE_URL` and `HEADLESS_MANIFEST_PATH`.
Codex defaults remain product-neutral. The BFF never opens the Codex tenant
database.
