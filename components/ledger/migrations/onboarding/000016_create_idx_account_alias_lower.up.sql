-- Requires query pattern: lower(alias) LIKE lower('prefix') || '%'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_alias_lower
    ON account (lower(alias) text_pattern_ops)
    WHERE deleted_at IS NULL;
