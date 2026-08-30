# ADR-0004: Storage follows tenant topology

- Status: Accepted

bbolt is suitable for simple local installations. SQLite is single-node/single
writer. SQL adapters support externally managed shared databases. Media remains
durable storage that must be backed up with metadata.
