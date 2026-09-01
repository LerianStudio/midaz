-- ============================================
-- Rollback: 000022_rename_currency_to_asset
-- Description: Revert the money-asset column asset -> currency IN PLACE on
--              limits and transaction_validations. Symmetric with the up
--              migration and lossless: RENAME COLUMN preserves type/length, so
--              limits.currency returns as VARCHAR(3) and
--              transaction_validations.currency as CHAR(3).
-- ============================================
--
-- Idempotency: each rename is guarded on the NEW column (asset) still being
-- present, so a replay on an already-reverted database is a clean no-op. The
-- probe carries table_schema = current_schema() so it resolves in the tenant's
-- own schema.

-- limits.asset -> currency
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'limits'
                 AND column_name  = 'asset') THEN
        ALTER TABLE limits RENAME COLUMN asset TO currency;
    END IF;
END $$;

-- transaction_validations.asset -> currency
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = current_schema()
                 AND table_name   = 'transaction_validations'
                 AND column_name  = 'asset') THEN
        ALTER TABLE transaction_validations RENAME COLUMN asset TO currency;
    END IF;
END $$;
