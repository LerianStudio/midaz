-- Rollback: Remove account-scoped point-in-time balance query index

DROP INDEX CONCURRENTLY IF EXISTS idx_operation_account_point_in_time;
