# ADR-0005: Failover needs the same tenant DSN

- Status: Accepted

A replacement Codex process represents the same tenant only when it uses the
same durable metadata/media stores and a compatible manifest. A fresh local
database is a different world, not failover.
