# Backup and restore

`headless-backup` produces metadata export plus `.media.tar`. Treat the pair
and the release manifest digest as one recoverable unit.

1. Verify backup age, checksum, encryption, and independent storage daily.
2. Before restore, stop writes and select an empty metadata/media target.
3. Run `headless-backup -mode restore`, supplying the paired artifacts.
4. Start Codex with the compatible manifest; verify `/readyz`, `/schema`,
   expected `/types`, a controlled read, and media access.
5. Record measured RTO/RPO in the restore drill.

Do not call a fresh SQLite/bbolt file a restore or a failover.
