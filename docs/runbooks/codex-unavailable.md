# Runbook: Codex unavailable

Check `/healthz` then `/readyz`, process logs, private network/proxy reachability,
storage connectivity, free disk, and manifest readability. Do not direct the
admin to browser CORS or attach the BFF to the tenant database. For recovery
failover, use a compatible process connected to the same DSN/media store.
