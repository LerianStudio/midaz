-- Migration: Unified lean index for all point-in-time balance queries
-- Purpose: Single index serving FindLastOperationBeforeTimestamp, FindLastOperationsForAccountBeforeTimestamp,
--          and ListByAccountIDAtTimestamp with optimal key design
-- Design: balance_version_after promoted to key for native sort (eliminates Incremental Sort);
--         no INCLUDE columns (key redesign reduces scan to ~1 row per balance, making heap fetches negligible)
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_balance_pit
ON operation (organization_id, ledger_id, account_id, balance_id, created_at DESC, balance_version_after DESC)
WHERE deleted_at IS NULL;
