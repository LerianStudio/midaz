-- Runs once on a fresh primary volume. ALTER SYSTEM only writes postgresql.auto.conf:
-- settings apply at the definitive start and pg_basebackup hands them to the standby.
-- Never reload here: synchronous_standby_names would block the init server's commits.

COMMENT ON DATABASE bench_transaction IS 'midaz-bench-engine:disposable';

ALTER SYSTEM SET max_connections = 300;
ALTER SYSTEM SET shared_buffers = '4GB';
ALTER SYSTEM SET max_wal_size = '8GB';
ALTER SYSTEM SET checkpoint_timeout = '15min';
ALTER SYSTEM SET log_checkpoints = on;
ALTER SYSTEM SET track_io_timing = on;
ALTER SYSTEM SET track_wal_io_timing = on;
ALTER SYSTEM SET wal_level = 'replica';
ALTER SYSTEM SET hot_standby = on;
ALTER SYSTEM SET max_wal_senders = 5;
ALTER SYSTEM SET synchronous_commit = on;
ALTER SYSTEM SET synchronous_standby_names = 'FIRST 1 (standby1)';
