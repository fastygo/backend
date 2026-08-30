# Release and pin policy

1. Build the declared Go toolchain and run `make verify`.
2. Run `make live-sql` for every enabled external SQL storage adapter.
3. Verify generated schema/OpenAPI and Level 1 conformance after manifest/API
   changes.
4. For a consumer release, record backend pin, manifest digest, storage
   migration status, and backup artifact age.
5. Promote through a private environment; prove `/readyz`, `/types`, public
   read, authenticated write, and media access.
6. Roll back to the prior compatible binary/manifest only after confirming
   schema compatibility. Never restore a backup merely to roll back code.
