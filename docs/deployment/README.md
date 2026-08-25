# Deployment

- [Bare-metal and systemd](bare-metal.md)
- [Production checklist](production-checklist.md)

The repository also provides a non-root Docker image and Compose profiles for bbolt, PostgreSQL,
MySQL, and MariaDB. SQLite uses the same image with `HEADLESS_STORAGE=sqlite` and a persistent
volume-backed `DATABASE_URL`.
