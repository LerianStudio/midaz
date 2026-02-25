ALTER TABLE IF EXISTS operation RENAME TO operation_partitioned;
ALTER TABLE IF EXISTS operation_legacy RENAME TO operation;
DROP TABLE IF EXISTS operation_partitioned CASCADE;
