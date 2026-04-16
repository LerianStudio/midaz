-- D7: Least-privilege role for the authorizer balance loader.
--
-- The authorizer only performs SELECT on the `balance` and `operation`
-- tables during cold-start and warm-reload. Granting it full owner
-- privileges (CREATE, DROP, WRITE) violates the principle of least
-- privilege: a compromised authorizer credential could exfiltrate,
-- tamper with, or drop the canonical ledger state.
--
-- This migration creates `midaz_authorizer_ro` with:
--   * CONNECT on the transaction database
--   * USAGE on the public schema
--   * SELECT on the balance and operation tables only
--
-- Writer services (ledger, consumer, transaction) continue to use the
-- default owner role and are unaffected.
--
-- Rollout (env-gated): operators set AUTHORIZER_DB_USE_RO_ROLE=true and
-- AUTHORIZER_DB_USER=midaz_authorizer_ro (with a matching password issued
-- via the secrets manager) to switch the authorizer over. Until that
-- flag is flipped the role exists but is unused — zero impact on
-- existing deployments.
--
-- Password: intentionally NOT set in this migration. Production
-- operators MUST rotate credentials via `ALTER ROLE midaz_authorizer_ro
-- WITH PASSWORD '<from-secrets-manager>';` as part of the rollout runbook.
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'midaz_authorizer_ro') THEN
        CREATE ROLE midaz_authorizer_ro WITH LOGIN NOINHERIT NOCREATEDB NOCREATEROLE NOREPLICATION;
    END IF;
END $$;

-- Allow the role to connect to the current database. The current_database()
-- indirection keeps the migration portable across the onboarding and
-- transaction databases (only `transaction` actually holds balances, but
-- re-running this migration anywhere else is harmless).
DO $$
DECLARE
    db_name text := current_database();
BEGIN
    EXECUTE format('GRANT CONNECT ON DATABASE %I TO midaz_authorizer_ro', db_name);
END $$;

GRANT USAGE ON SCHEMA public TO midaz_authorizer_ro;

-- Grant SELECT only on the two tables the authorizer reads. Operation is
-- included because the partition-state wiring reads it for replay checks.
GRANT SELECT ON TABLE balance TO midaz_authorizer_ro;
GRANT SELECT ON TABLE operation TO midaz_authorizer_ro;

-- Ensure future tables created in `public` do NOT auto-grant to the
-- read-only role — we explicitly opt-in to what the authorizer can see.
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE ALL ON TABLES FROM midaz_authorizer_ro;
