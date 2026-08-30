# Deployment

- [Bare-metal and systemd](bare-metal.md)
- [Production checklist](production-checklist.md)
- [Deployment profiles](profiles.md)

The repository also provides a non-root Docker image and Compose profiles for bbolt, PostgreSQL,
MySQL, and MariaDB. SQLite uses the same image with `HEADLESS_STORAGE=sqlite` and a persistent
volume-backed `DATABASE_URL`.

Production topology uses a private Codex bind and a TLS proxy. Admin browser
access is through its isolated same-origin proxy/tunnel; public BFF topology is
defined by the consumer SiteStarter contract.
