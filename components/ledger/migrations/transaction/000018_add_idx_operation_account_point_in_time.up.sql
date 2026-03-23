-- Migration: Add covering index for account-scoped point-in-time balance queries
-- Purpose: Optimize queries that find the latest operation per balance for an account before a timestamp

-- Covering index for account balances point-in-time query (all balances for an account)
-- Query pattern: SELECT DISTINCT ON (balance_id) ... FROM operation WHERE organization_id = ? AND ledger_id = ? AND account_id = ? AND created_at <= ? ORDER BY balance_id, created_at DESC
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_point_in_time
ON operation (organization_id, ledger_id, account_id, balance_id, created_at DESC)
INCLUDE (id, balance_key, available_balance_after, on_hold_balance_after, balance_version_after, asset_code)
WHERE deleted_at IS NULL;
