CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_alias_trgm
    ON account USING gin (alias gin_trgm_ops)
    WHERE deleted_at IS NULL;
