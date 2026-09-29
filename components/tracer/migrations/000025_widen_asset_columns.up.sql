-- ============================================
-- Migration: 000025_widen_asset_columns
-- Description: Widen the money-asset column to VARCHAR(100) on limits
--              (VARCHAR(3)) and transaction_validations (CHAR(3)) so the
--              tracer stores the same asset vocabulary the Midaz ledger
--              accepts: 1 to 100 uppercase letters, no longer ISO-4217 only.
--              VARCHAR(3) -> VARCHAR(100) only raises the length limit and
--              does not rewrite limits. CHAR(3) -> VARCHAR(100) rewrites
--              transaction_validations; existing 3-letter values carry no
--              padding, so they read back unchanged. No index covers either
--              column, so no index is rebuilt.
-- Date: 2026-09-29
-- ============================================
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md): each
-- ALTER is guarded on the column not already being VARCHAR(100), so a replay on
-- an already-widened database is a clean no-op. The probe carries
-- table_schema = current_schema() so it resolves in the tenant's own schema.

-- limits.asset VARCHAR(3) -> VARCHAR(100)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'limits'
                 AND column_name  = 'asset'
                 AND (data_type <> 'character varying'
                      OR character_maximum_length IS DISTINCT FROM 100)) THEN
        ALTER TABLE limits ALTER COLUMN asset TYPE VARCHAR(100);
    END IF;
END $$;

-- transaction_validations.asset CHAR(3) -> VARCHAR(100)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'transaction_validations'
                 AND column_name  = 'asset'
                 AND (data_type <> 'character varying'
                      OR character_maximum_length IS DISTINCT FROM 100)) THEN
        ALTER TABLE transaction_validations ALTER COLUMN asset TYPE VARCHAR(100);
    END IF;
END $$;
