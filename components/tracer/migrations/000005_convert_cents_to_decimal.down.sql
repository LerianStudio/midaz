-- Rollback: Convert monetary amounts from DECIMAL back to BIGINT (cents)

-- ============================================================================
-- 1. Revert rule descriptions
-- ============================================================================

UPDATE rules SET description = 'Block transactions above $10,000 (1000000 cents)'
    WHERE description = 'Block transactions above $10,000.00';

UPDATE rules SET description = 'Auto-approve transactions below $1,000 (100000 cents)'
    WHERE description = 'Auto-approve transactions below $1,000.00';

-- ============================================================================
-- 2. Revert rule expressions
-- ============================================================================

UPDATE rules SET expression = 'amount > 1000000'
    WHERE expression = 'amount > 10000.00';

UPDATE rules SET expression = 'amount < 100000'
    WHERE expression = 'amount < 1000.00';

-- ============================================================================
-- 3. Convert monetary columns from DECIMAL back to BIGINT (cents)
-- ============================================================================

ALTER TABLE limits
    ALTER COLUMN max_amount TYPE BIGINT USING ROUND(max_amount * 100)::BIGINT;

ALTER TABLE usage_counters
    ALTER COLUMN current_usage TYPE BIGINT USING ROUND(current_usage * 100)::BIGINT;

ALTER TABLE usage_counters
    ALTER COLUMN current_usage SET DEFAULT 0;

ALTER TABLE transaction_validations
    ALTER COLUMN amount TYPE BIGINT USING ROUND(amount * 100)::BIGINT;

