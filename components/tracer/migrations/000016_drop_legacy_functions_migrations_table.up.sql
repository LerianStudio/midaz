-- Removes the legacy tracking table created by the custom function migrator.
-- Idempotent via IF EXISTS — no-op in environments that never had the legacy runner.
DROP TABLE IF EXISTS schema_migrations_functions;
