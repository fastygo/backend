# Codex threat model

| Asset | Threat | Baseline control |
| --- | --- | --- |
| Tenant records/revisions | Wrong tenant or unauthorized write | Manifest + DSN verification, capabilities, version checks |
| Sessions/tokens | Theft or cross-origin abuse | Private admin proxy, HttpOnly cookie, CSRF, rotation |
| Database/media | Loss or corruption | Encrypted independent backups and restore drills |
| API contract | Compatibility regression | Conformance/OpenAPI tests and pinned consumer releases |
| Process host | Public exposure | Private bind, TLS edge, unprivileged service account |

Residual risks: CSRF coverage is incomplete for some mutation routes; multi-node
SQLite does not provide write HA; rate limits/TLS/firewall are deployment
controls and must be validated by each operator.
