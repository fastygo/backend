# PostgreSQL 101 Guide for the Backend

## Why PostgreSQL?
- Open-source, battle-tested relational database.
- Supports JSONB, transactions, observability, and robust tooling.
- Native compatibility with Supabase and Go (pgx, pgxpool).

## Setup Basics
1. **Install locally**
   ```bash
   sudo apt install postgresql postgresql-contrib
   ```
2. **Start server & create user**
   ```bash
   sudo -u postgres createuser --interactive
   sudo -u postgres createdb backend_db
   psql -c "ALTER ROLE you SET client_encoding TO 'utf8';"
   ```
3. **Connect with pgAdmin/psql**
   ```bash
   psql postgresql://user:pass@localhost:5432/backend_db
   ````**Use `.pgpass`** to avoid entering password interactively.

## Schema Management
1. **Migration files**
   - Place SQL in `assets/migrations/` using sequential prefixes (`001_users.sql`).
   - Prefer `CREATE TABLE IF NOT EXISTS`, `ALTER TABLE ... IF EXISTS`, `CREATE EXTENSION IF NOT EXISTS`.
2. **Run migrations**
   ```bash
   migrate -path assets/migrations -database "${DATABASE_URL}" up
   ```
3. **Rollback**
   ```bash
   migrate -path assets/migrations -database "${DATABASE_URL}" down
   ```
4. **Fixtures**
   - Save seed data in `assets/fixtures/`.
   - Load via `psql -f assets/fixtures/users.sql`.

## Backup & Restore
1. **Dump schema/data**
   ```bash
   pg_dump --format=custom --file=backups/db.dump "${DATABASE_URL}"
   ```
2. **Restore**
   ```bash
   pg_restore --clean --no-owner --dbname="${DATABASE_URL}" backups/db.dump
   ```
3. **Supabase export**
   Supabase CLI: `supabase db dump --schema public --file=schema.sql`

## Data Access Patterns
1. **Use transactions in Go**
   ```go
   pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
       // operations
   })
   ```
2. **Prepared statements**
   - `tx.Exec(ctx, "INSERT ...", args...)`
3. **JSONB**
   - Store metadata as JSONB, query via `->` operators.

## Monitoring
1. **pg_stat_statements**: track slow queries.
2. **Extensions**: `pgstattuple`, `pg_stat_io`.
3. **Alerting**: set on `pg_stat_activity` long-running queries, connection count.

## Supabase & PostgreSQL
1. Supabase sits on top of PostgreSQL; you can connect directly to the same database from pgAdmin or your Go backend.
2. Export schema/dumps from Supabase (`supabase db dump`) and store under `assets/migrations`.
3. When ready to leave Supabase, run migrations against your new PostgreSQL instance and point the backend to the same schema.

## Tips for Safety
1. Always backup before migrations.
2. Test migrations on staging before production.
3. Use `pg_isready` to check availability during deployment.
4. Keep `DATABASE_URL` in `.env` and never commit secrets.


