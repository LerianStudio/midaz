-- Rollback: Remove point-in-time balance query indexes

DROP INDEX CONCURRENTLY IF EXISTS idx_operation_point_in_time;
