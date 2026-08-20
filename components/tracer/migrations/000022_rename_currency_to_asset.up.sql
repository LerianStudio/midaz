-- ============================================
-- Migration: 000022_rename_currency_to_asset
-- Description: Rename the money-asset column currency -> asset IN PLACE on
--              limits (VARCHAR(3)) and transaction_validations (CHAR(3)).
--              RENAME COLUMN is metadata-only in PostgreSQL: it preserves the
--              column type and length, acquires no table rewrite, and is NOT a
--              widen. No index covers either column, so no ALTER INDEX is
--              needed. ISO-4217 semantics are unchanged; only the name moves.
-- Date: 2026-08-19
-- ============================================
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md): each
-- rename is guarded on the OLD column still being present, so a replay on an
-- already-renamed database is a clean no-op. The probe carries
-- table_schema = current_schema() so it resolves in the tenant's own schema.

-- limits.currency -> asset
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'limits'
                 AND column_name  = 'currency') THEN
        ALTER TABLE limits RENAME COLUMN currency TO asset;
    END IF;
END $$;

-- transaction_validations.currency -> asset
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'transaction_validations'
                 AND column_name  = 'currency') THEN
        ALTER TABLE transaction_validations RENAME COLUMN currency TO asset;
    END IF;
END $$;
