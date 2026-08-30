# Codex SLI catalog

| SLI | Source | Success | Window | Owner | Status/evidence |
| --- | --- | --- | --- | --- | --- |
| Readiness | External `/readyz` probe | 200 and storage available | 30 days | BaaS operator | Target |
| Types discovery | Synthetic `/types` | 200, expected manifest digest/types | 30 days | BaaS operator | Target |
| Public by-slug latency | Edge/synthetic REST probe | 2xx within SLO threshold | 30 days | BaaS operator | Target |
| Authenticated mutation | Controlled CSRF/bearer probe | Expected successful versioned write | 30 days | BaaS operator | Target |
| Backup freshness | Backup inventory | Latest complete metadata + media pair | Daily | BaaS operator | Target |
| Restore | Scheduled drill | Restored tenant passes readiness/schema check | Quarterly | BaaS operator | Target |
| Contract | CI/conformance | All required tests pass | Every release | Release operator | Implemented gate |

Probe URLs need a stable user agent and a non-sensitive controlled record.
Intentional authorization/validation failures do not count as backend errors.
