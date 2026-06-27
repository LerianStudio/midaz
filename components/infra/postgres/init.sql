CREATE USER replicator WITH REPLICATION LOGIN ENCRYPTED PASSWORD 'replicator_password';

SELECT pg_create_physical_replication_slot('replication_slot');
SELECT * FROM pg_create_logical_replication_slot('logical_slot', 'pgoutput');

CREATE DATABASE onboarding;
CREATE DATABASE transaction;

-- tracer (transaction validation / audit-hash-chain service).
-- Created idempotently so a manual re-run against an already-populated cluster
-- does not error. init.sql ONLY executes automatically on a FRESH (empty) data
-- volume. EXISTING-VOLUME RUNBOOK: a deployment whose postgres volume predates
-- this line will NOT get the `tracer` database from init.sql. On such a volume,
-- create it once out-of-band before starting tracer:
--   psql -h midaz-postgres-primary -p 5701 -U midaz -c "CREATE DATABASE tracer;"
-- (tracer's own bootstrap runs migrations against this DB but does NOT create
-- it — connecting to a missing `tracer` DB fails fast and leaves /readyz red.)
SELECT 'CREATE DATABASE tracer'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'tracer')\gexec