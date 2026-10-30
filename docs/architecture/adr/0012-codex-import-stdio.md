# ADR-0012: Codex import is stdio NDJSON, not a network listener

- Status: Accepted

Long-read Codex records enter a tenant through `backend codex import --stdio
--tenant=<id>`. The process reads NDJSON on stdin and writes an NDJSON report
on stdout. There is no import HTTP port. Authorization is the SSH
`authorized_keys` forced command (and `incus exec` on the host). The desktop
app converts markdown locally; this server stores the ready `codex-entry/v1`
bytes and does not compile markdown.

Each line is either a bare `codex-entry/v1` object (upsert) or an envelope:

```json
{"op":"upsert","digest":"codex-entry/v1:sha256:…","entry":{…}}
{"op":"delete","id":"post-1"}
{"op":"delete","kind":"post","slug":"about","locale":"en-US"}
```

The importer validates the closed entry profile and verifies the digest with
`github.com/fastygo/codex/canonical` v0.4.2. Allowed kinds are tenant-manifest
long reads: `post`, `page`, and any other REST resource except `menu`,
`setting`, and `media`. Other types are `rejected`.

Upsert matches an existing row by id, else by kind+slug+locale. An unchanged
digest is a no-op (`unchanged`). An overwrite calls the content update path,
which stores a revision of the previous version. Delete trashes the matched
row (revision stored). `--dry-run` prints the same statuses and writes
nothing.

The batch is planned first. If any line would be rejected, no writes happen
and the process exits non-zero. After a clean plan, applies run in order,
each in its own storage transaction. A crash during apply can leave a prefix
of the batch written; dry-run never writes. The last stdout line is the
tenant's `codex-manifest/v2` JCS object.
