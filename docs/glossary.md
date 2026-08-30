# Glossary

- **Codex / BaaS:** This product-neutral GoBackend process.
- **Tenant:** One product's manifest, metadata DSN, media root, and secrets.
- **BFF:** Product public application; it consumes Codex HTTP and never opens
  the Codex tenant database.
- **Admin:** SvelteCMS browser application at an isolated origin in production.
- **Manifest:** Versioned product definition of extra resource kinds/fields.
- **Seed:** Explicit bootstrap/import data, not runtime content reconciliation.
- **Same world:** A replacement process using the same tenant DSN/media store
  and compatible manifest.
- **Pin:** Exact released GoBackend version selected by a consumer.
