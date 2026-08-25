# Architecture

The implementation follows a dependency-inward Go stack:

- `internal/domain`: protocol- and storage-independent entities and invariants
- `internal/application`: content and media orchestration through owned ports
- `internal/storage`: interchangeable bbolt, SQL, and local media adapters
- `internal/delivery`: REST and GraphQL adapters
- `internal/platform`: Framework lifecycle and Panel descriptor integration
- `internal/bootstrap`: the composition root
- `cmd`: server and operational CLIs

Framework owns HTTP lifecycle, middleware, health, metrics, workers, and graceful shutdown. Panel
owns control-plane descriptors. The backend owns schemas, content, lifecycle, revisions, media,
authorization policy, audit, and persistence adapters.

See [target-headless.md](target-headless.md) for the complete decision record.
