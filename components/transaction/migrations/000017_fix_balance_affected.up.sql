-- Migration: Fix balance_affected field for operations from non-annotation transactions
--
-- Root Cause: create-operation.go didn't set BalanceAffected, defaulting to false in Go
--
-- This migration repairs existing data by:
-- 1. Finding operations where balance_affected = false (incorrectly set)
-- 2. Joining with transactions to identify non-annotation transactions (status != 'NOTED')
-- 3. Setting balance_affected = true for those operations
--
-- Operations from annotation transactions (status = 'NOTED') correctly have
-- balance_affected = false and will NOT be modified.

-- First, let's see what we're about to fix (for audit purposes in logs)
DO $$
DECLARE
    affected_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO affected_count
    FROM operation o
    INNER JOIN transaction t ON o.transaction_id = t.id
    WHERE o.balance_affected = false
      AND o.deleted_at IS NULL
      AND t.deleted_at IS NULL
      AND t.status != 'NOTED';

    RAISE NOTICE 'Operations to be repaired: %', affected_count;
END $$;

-- Repair the data
UPDATE operation o
SET
    balance_affected = true,
    updated_at = NOW()
FROM transaction t
WHERE o.transaction_id = t.id
  AND o.balance_affected = false
  AND o.deleted_at IS NULL
  AND t.deleted_at IS NULL
  AND t.status != 'NOTED';

-- Verify the fix (log count of remaining issues)
DO $$
DECLARE
    remaining_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO remaining_count
    FROM operation o
    INNER JOIN transaction t ON o.transaction_id = t.id
    WHERE o.balance_affected = false
      AND o.deleted_at IS NULL
      AND t.deleted_at IS NULL
      AND t.status != 'NOTED';

    IF remaining_count > 0 THEN
        RAISE WARNING 'Migration incomplete: % operations still have balance_affected = false for non-NOTED transactions', remaining_count;
    ELSE
        RAISE NOTICE 'Migration complete: All non-annotation operations now have balance_affected = true';
    END IF;
END $$;
