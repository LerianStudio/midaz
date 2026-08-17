-- Migration: Convert monetary amounts from BIGINT (cents) to DECIMAL (currency value)
--
-- Before: amounts stored as integer cents (e.g., 1000000 = $10,000.00)
-- After:  amounts stored as decimal values (e.g., 10000.00 = $10,000.00)
--
-- The USING clause converts existing data inline during the ALTER (divide by 100).
-- Rule expressions are also updated to reflect decimal thresholds.
--
-- IMPORTANT: Custom rule expressions with cent-based amount comparisons must be
-- reviewed and updated manually after this migration. This migration only handles
-- the known seed data patterns.

-- ============================================================================
-- 1. Convert monetary columns from BIGINT (cents) to DECIMAL (currency value)
-- ============================================================================

-- limits.max_amount
ALTER TABLE limits
    ALTER COLUMN max_amount TYPE DECIMAL USING max_amount / 100.0;

COMMENT ON COLUMN limits.max_amount IS 'Stored as DECIMAL representing the currency value (e.g., 1000.00 for $1,000)';

-- usage_counters.current_usage
ALTER TABLE usage_counters
    ALTER COLUMN current_usage TYPE DECIMAL USING current_usage / 100.0;

ALTER TABLE usage_counters
    ALTER COLUMN current_usage SET DEFAULT 0;

COMMENT ON COLUMN usage_counters.current_usage IS 'Stored as DECIMAL representing the currency value';

-- transaction_validations.amount
ALTER TABLE transaction_validations
    ALTER COLUMN amount TYPE DECIMAL USING amount / 100.0;

COMMENT ON COLUMN transaction_validations.amount IS 'Stored as DECIMAL representing the currency value';

-- ============================================================================
-- 2. Update rule expressions: convert cent-based thresholds to decimal
--    (exact match only — safe for known seed data)
-- ============================================================================

UPDATE rules SET expression = 'amount > 10000.00'
    WHERE expression = 'amount > 1000000';

UPDATE rules SET expression = 'amount < 1000.00'
    WHERE expression = 'amount < 100000';

-- ============================================================================
-- 3. Update rule descriptions to reflect decimal values
-- ============================================================================

UPDATE rules SET description = 'Block transactions above $10,000.00'
    WHERE description = 'Block transactions above $10,000 (1000000 cents)';

UPDATE rules SET description = 'Auto-approve transactions below $1,000.00'
    WHERE description = 'Auto-approve transactions below $1,000 (100000 cents)';

