-- Rollback: Remove point-in-time balance query indexes

DROP INDEX IF EXISTS idx_operation_point_in_time;
DROP INDEX IF EXISTS idx_operation_account_point_in_time;
