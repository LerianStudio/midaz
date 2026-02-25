ALTER TABLE IF EXISTS balance RENAME TO balance_partitioned;
ALTER TABLE IF EXISTS balance_legacy RENAME TO balance;
DROP TABLE IF EXISTS balance_partitioned CASCADE;
