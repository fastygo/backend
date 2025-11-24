# Supabase to Go Backend Migration Guide

## Overview
This guide captures what to export from Supabase (schema, auth, storage, policies) so the Go backend can bootstrap locally and deploy cleanly with PostgreSQL + Redis. Treat Supabase as the prototyping data plane: once structure is validated, copy schema/data and switch to the Go backend without losing readiness.

## Step 1 – Export Core Schema
1. Run `supabase db dump --schema public > schema.sql` (or use the SQL editor to download each table definition).
2. Save the schema under `backend/assets/migrations/` with human-friendly names (`001_users.sql`, etc.).
3. Keep Supabase-specific extensions (e.g., `uuid-ossp`) but wrap them with `CREATE EXTENSION IF NOT EXISTS`.
4. Store indexes, constraints, triggers, and policies in the same SQL files.

## Step 2 – Export Reference Data
1. Use `supabase db dump --data-only --tables=profiles,tasks` to capture seed data.
2. Convert timestamps to UTC and replace Supabase service role secrets with env vars.
3. Add this data to `backend/assets/fixtures/` with instructions for `psql -f`.

## Step 3 – Capture Auth & IAM Details
1. Export Supabase Auth tables (`auth.users`, `auth.refresh_tokens`) to understand identity mapping.
2. Document JWT claims used (e.g., `user_id`, `app_metadata.role`) so the Go middleware can replicate them.
3. If using Supabase Policies, capture the logic so equivalent SQL views or row-level security rules exist in Go-managed Postgres.

## Step 4 – Identify Storage Assets
1. If you rely on Supabase Storage, note bucket names and file metadata.
2. Plan equivalent storage (MinIO/S3) and expose signed URLs through the Go backend.

## Step 5 – Monitoring & Metrics
1. Supabase exposes query stats/metrics. Record thresholds (slow query > 1s) and add them to Go monitoring config (`assets/docs/monitoring.md`).
2. Mirror Supabase health expectations via `/health` and `/ready` endpoints.

## Step 6 – Migration Checklist
1. Run migrations with `scripts/migrate.sh` (or `golang-migrate`).
2. Load fixtures via `psql -f assets/fixtures/*.sql`.
3. Validate JWT claims and session storage (Redis) match Supabase behavior.
4. Use Postman collection to hit endpoints; compare results against Supabase responses.
5. Once green, switch frontend/API gateway to point to the Go backend.

## Notes
- Treat Supabase as a rapid prototyping environment. Once schema/data is finalized, remain in Go backend for full control (Bolt buffering, Redis sessions, custom middleware).
- Document any Supabase-specific logic in `backend/assets/docs/supabase-fallback.md` so future teams know what was migrated.


