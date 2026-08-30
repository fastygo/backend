# ADR-0003: Seed is a one-shot CLI

- Status: Accepted

`headless-seed` initializes or explicitly imports data. `cmd/server` does not
reconcile seed files at runtime. Content changes are API/admin operations.
