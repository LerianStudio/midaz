-- Rollback: Remove unified point-in-time balance query index

DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account_balance_pit;
