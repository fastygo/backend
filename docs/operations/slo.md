# Codex SLOs

These are production targets, not claims that repository CI proves availability.
They require external probes and backup/drill evidence.

| SLI | Objective/window | Owner | Status |
| --- | --- | --- | --- |
| `/readyz` | 99.9% successful / 30 days | BaaS operator | Target |
| `/types` | 99.9% successful / 30 days | BaaS operator | Target |
| Public `by-slug` | p95 ≤ 500ms / 30 days | BaaS operator | Target |
| Authenticated mutation | 99.5% successful / 30 days | BaaS operator | Target |
| Complete backup age | ≤ 24h | BaaS operator | Target |
| Restore drill | ≤ 4h RTO, ≤ 24h RPO / quarter | BaaS operator | Target |
| Compatibility gate | 100% before release | Release operator | Implemented |

The compatibility gate is `make verify`, affected conformance tests, and
`make live-sql` for enabled SQL adapters. It is release evidence, not an uptime
SLO.
