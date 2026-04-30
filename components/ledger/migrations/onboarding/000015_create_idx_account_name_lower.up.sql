-- Requires query pattern: lower(name) LIKE lower('prefix') || '%'

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_name_lower
    ON account (lower(name) text_pattern_ops)
    WHERE deleted_at IS NULL;
