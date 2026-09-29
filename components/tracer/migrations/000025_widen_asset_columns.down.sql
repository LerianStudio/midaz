-- ============================================
-- Rollback: 000025_widen_asset_columns
-- Description: Narrow the money-asset column back to limits.asset VARCHAR(3)
--              and transaction_validations.asset CHAR(3).
--
--              IRREVERSIBLE WITH LONG DATA: once any row stores an asset code
--              longer than 3 characters (BTC still fits, POINTS and USDT
--              do not), narrowing would truncate it. This rollback checks BOTH
--              tables before altering either: if any row does not fit it
--              raises an explicit error naming the table and leaves the whole
--              schema untouched. Delete or migrate those rows first if a
--              rollback is truly required. Codes shorter than 3 characters
--              survive, but transaction_validations pads them with trailing
--              spaces again under CHAR(3).
--
--              Narrowing either column rewrites its table and rebuilds its
--              indexes under ACCESS EXCLUSIVE; plan a maintenance window as
--              for the up migration. Each ALTER waits at most 5s for its lock
--              (lock_timeout, transaction-local) and fails with SQLSTATE 55P03
--              rather than queueing traffic behind it.
-- ============================================
--
-- Idempotency: each ALTER is guarded on the column still being wider than the
-- original type, so a replay on an already-narrowed database is a clean no-op.
-- The probes carry table_schema = current_schema() so they resolve in the
-- tenant's own schema.

DO $$
DECLARE
    narrow_limits       boolean;
    narrow_validations  boolean;
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    narrow_limits := EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'limits'
          AND column_name  = 'asset'
          AND character_maximum_length IS DISTINCT FROM 3);

    narrow_validations := EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'transaction_validations'
          AND column_name  = 'asset'
          AND (data_type <> 'character'
               OR character_maximum_length IS DISTINCT FROM 3));

    IF narrow_limits AND EXISTS (SELECT 1 FROM limits WHERE char_length(asset) > 3) THEN
        RAISE EXCEPTION 'cannot narrow limits.asset to VARCHAR(3): rows store asset codes longer than 3 characters';
    END IF;

    IF narrow_validations AND EXISTS (SELECT 1 FROM transaction_validations WHERE char_length(asset) > 3) THEN
        RAISE EXCEPTION 'cannot narrow transaction_validations.asset to CHAR(3): rows store asset codes longer than 3 characters';
    END IF;

    IF narrow_limits THEN
        ALTER TABLE limits ALTER COLUMN asset TYPE VARCHAR(3);
    END IF;

    IF narrow_validations THEN
        ALTER TABLE transaction_validations ALTER COLUMN asset TYPE CHAR(3);
    END IF;
END $$;
