-- ============================================
-- Rollback: 000025_widen_asset_columns
-- Description: Narrow the money-asset column back to limits.asset VARCHAR(3)
--              and transaction_validations.asset CHAR(3).
--
--              IRREVERSIBLE WITH LONG DATA: once any row stores an asset code
--              longer than 3 characters (BTC still fits, POINTS and USDT
--              do not), narrowing would truncate it. This rollback
--              refuses instead: it raises an explicit error naming the table
--              and leaves the schema untouched. Delete or migrate those rows
--              first if a rollback is truly required. Codes shorter than 3
--              characters survive, but transaction_validations pads them with
--              trailing spaces again under CHAR(3).
-- ============================================
--
-- Idempotency: each ALTER is guarded on the column still being wider than the
-- original type, so a replay on an already-narrowed database is a clean no-op.
-- The probe carries table_schema = current_schema() so it resolves in the
-- tenant's own schema.

-- limits.asset VARCHAR(100) -> VARCHAR(3)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'limits'
                 AND column_name  = 'asset'
                 AND character_maximum_length IS DISTINCT FROM 3) THEN
        IF EXISTS (SELECT 1 FROM limits WHERE char_length(asset) > 3) THEN
            RAISE EXCEPTION 'cannot narrow limits.asset to VARCHAR(3): rows store asset codes longer than 3 characters';
        END IF;

        ALTER TABLE limits ALTER COLUMN asset TYPE VARCHAR(3);
    END IF;
END $$;

-- transaction_validations.asset VARCHAR(100) -> CHAR(3)
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'transaction_validations'
                 AND column_name  = 'asset'
                 AND (data_type <> 'character'
                      OR character_maximum_length IS DISTINCT FROM 3)) THEN
        IF EXISTS (SELECT 1 FROM transaction_validations WHERE char_length(asset) > 3) THEN
            RAISE EXCEPTION 'cannot narrow transaction_validations.asset to CHAR(3): rows store asset codes longer than 3 characters';
        END IF;

        ALTER TABLE transaction_validations ALTER COLUMN asset TYPE CHAR(3);
    END IF;
END $$;
