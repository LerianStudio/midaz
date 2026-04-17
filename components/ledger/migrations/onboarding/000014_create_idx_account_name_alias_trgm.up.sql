-- GIN indexes for account name and alias using pg_trgm
-- Supports efficient ILIKE prefix matching for text search filters
-- Requires pg_trgm extension (migration 000013)

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_name_trgm
    ON account USING gin (name gin_trgm_ops)
    WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_alias_trgm
    ON account USING gin (alias gin_trgm_ops)
    WHERE deleted_at IS NULL;
