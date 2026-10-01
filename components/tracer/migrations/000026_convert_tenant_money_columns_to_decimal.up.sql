-- ============================================
-- Migration: 000026_convert_tenant_money_columns_to_decimal
-- Description: Convert limits.max_amount, usage_counters.current_usage and
--              transaction_validations.amount from BIGINT to DECIMAL in the
--              schema the migration runs in.
--
--              000005 performed this conversion but probed
--              table_schema = 'public', so a schema-isolated tenant (a schema
--              per tenant, selected through search_path) kept the three
--              columns BIGINT and rejected any fractional amount. Those
--              tenant columns were only ever written by the decimal-aware
--              application, so they hold currency units, not cents: the cast
--              is direct, with NO division by 100.
--
--              BIGINT -> DECIMAL rewrites each table and rebuilds its indexes
--              under ACCESS EXCLUSIVE. Each ALTER waits at most 5s for its
--              lock (lock_timeout, transaction-local) and fails with SQLSTATE
--              55P03 rather than queueing traffic behind it; re-run in a
--              quieter window.
-- Date: 2026-10-01
-- ============================================
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md): each
-- ALTER is guarded on the column still being bigint in current_schema(), so the
-- public schema of a single-tenant database, which 000005 already converted, and
-- any replay are clean no-ops. COMMENT ON COLUMN and SET DEFAULT are naturally
-- idempotent and stay outside the guards.

-- limits.max_amount
DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'limits'
          AND column_name  = 'max_amount') = 'bigint' THEN
        ALTER TABLE limits
            ALTER COLUMN max_amount TYPE DECIMAL USING max_amount::numeric;
    END IF;
END $$;

COMMENT ON COLUMN limits.max_amount IS 'Stored as DECIMAL representing the currency value (e.g., 1000.00 for $1,000)';

-- usage_counters.current_usage
DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'usage_counters'
          AND column_name  = 'current_usage') = 'bigint' THEN
        ALTER TABLE usage_counters
            ALTER COLUMN current_usage TYPE DECIMAL USING current_usage::numeric;
    END IF;
END $$;

ALTER TABLE usage_counters
    ALTER COLUMN current_usage SET DEFAULT 0;

COMMENT ON COLUMN usage_counters.current_usage IS 'Stored as DECIMAL representing the currency value';

-- transaction_validations.amount
DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'transaction_validations'
          AND column_name  = 'amount') = 'bigint' THEN
        ALTER TABLE transaction_validations
            ALTER COLUMN amount TYPE DECIMAL USING amount::numeric;
    END IF;
END $$;

COMMENT ON COLUMN transaction_validations.amount IS 'Stored as DECIMAL representing the currency value';
