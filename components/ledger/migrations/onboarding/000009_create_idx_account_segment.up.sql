CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_segment
    ON account (organization_id, ledger_id, segment_id)
    WHERE deleted_at IS NULL;
