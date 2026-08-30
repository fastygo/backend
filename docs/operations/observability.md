# Observability

Collect structured process logs, `/metrics` when enabled, external liveness and
readiness probes, storage dependency errors, audit events, backup inventory,
and restore-drill results. Alert on readiness failure, error-rate/latency SLO
burn, missing complete backup pair, or failed restore drill.

Do not log bearer tokens, passwords, cookie values, CSRF tokens, full record
payloads, or database DSNs. A consumer correlates Codex request IDs with its
BFF/admin logs at the proxy boundary.
