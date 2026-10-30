# ADR-0004: Storage follows tenant topology

- Status: Accepted

bbolt is suitable for simple local installations and remains the untagged
binary default. Documented production storage is SQLite (`modernc.org/sqlite`,
single-node/single writer, `-tags sqlite`). SQL adapters support externally
managed shared databases. Media remains durable storage that must be backed up
with metadata. Multi-tenant processes give each tenant its own DSN or bbolt
path (ADR-0011).
