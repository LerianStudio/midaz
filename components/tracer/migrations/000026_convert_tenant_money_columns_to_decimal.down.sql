-- ============================================
-- Migration: 000026_convert_tenant_money_columns_to_decimal (DOWN)
-- Description: Intentionally a no-op.
-- ============================================
-- The up migration converts limits.max_amount, usage_counters.current_usage and
-- transaction_validations.amount to DECIMAL only where they were still BIGINT,
-- and the schema cannot tell afterwards which columns it converted. Turning them
-- back into BIGINT would truncate every fractional amount stored since (1.32
-- becomes 1), silently corrupting limits, counted usage and the validation
-- record, and would also undo the conversion 000005 made on schemas this
-- migration never touched. 000005's own down remains the rollback for the
-- decimal representation.
--
-- If a rollback is truly needed, manual intervention is required.
DO $$ BEGIN
  RAISE NOTICE 'Money columns are left as DECIMAL: reverting them to BIGINT would truncate fractional amounts';
END $$;
