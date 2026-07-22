-- Postgres bootstrap for the Midaz infra stack.
--
-- This script is applied in TWO situations and must stay idempotent for both:
--   1. Auto-run by postgres on a FRESH data volume (mounted at
--      /docker-entrypoint-initdb.d/init.sql).
--   2. Re-run on EVERY `make up` by the midaz-postgres-init sidecar against the
--      already-healthy primary.
-- postgres only auto-runs docker-entrypoint-initdb.d on a fresh volume, so an
-- EXISTING volume that predates a newly-added database (e.g. tracer) would never
-- get it and that service crash-loops on `database "..." does not exist`
-- (SQLSTATE 3D000). The sidecar closes that gap; every statement below is
-- guarded so re-applying against a populated cluster provisions only what is
-- missing and never errors on what already exists.

-- Replication role for the physical/logical replica. The password literal here
-- must match REPLICATION_PASSWORD in .env (the replica authenticates with that
-- env value) — keep the two in sync; the guard only checks role existence, so
-- changing one without the other silently breaks replica auth.
SELECT 'CREATE USER replicator WITH REPLICATION LOGIN ENCRYPTED PASSWORD ''replicator_password'''
  WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'replicator')\gexec

-- Replication slots consumed by the replica and by logical decoding.
SELECT pg_create_physical_replication_slot('replication_slot')
  WHERE NOT EXISTS (SELECT FROM pg_replication_slots WHERE slot_name = 'replication_slot');
SELECT pg_create_logical_replication_slot('logical_slot', 'pgoutput')
  WHERE NOT EXISTS (SELECT FROM pg_replication_slots WHERE slot_name = 'logical_slot');

-- Application databases.
SELECT 'CREATE DATABASE onboarding'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'onboarding')\gexec
SELECT 'CREATE DATABASE transaction'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'transaction')\gexec
-- tracer: transaction validation / audit-hash-chain service. Its own bootstrap
-- runs migrations against this DB but does NOT create it.
SELECT 'CREATE DATABASE tracer'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'tracer')\gexec
