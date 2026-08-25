# Contributing

Use the Go toolchain declared in `go.mod`.

Before submitting a change:

```bash
gofmt -w <changed-go-files>
go test ./...
go vet ./...
go mod verify
```

Keep dependencies pointing inward:

- Domain packages must not import delivery, storage, Framework, or Panel.
- Application packages own their repository interfaces.
- Storage and delivery packages implement application ports.
- Framework and Panel integration stays in `internal/platform` and `internal/bootstrap`.
- Product-specific fields and resources belong in manifests, not hard-coded domain branches.

All code comments must be in English. Add tests for authorization, optimistic locking, transaction
rollback, public/private projection, and adapter parity when those contracts are affected.

Do not add SSR themes, frontend build tooling, Redis sessions, custom HTTP lifecycle code, or a
second control-plane abstraction.
