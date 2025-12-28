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
