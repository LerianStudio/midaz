CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_name_trgm
    ON account USING gin (name gin_trgm_ops)
    WHERE deleted_at IS NULL;
