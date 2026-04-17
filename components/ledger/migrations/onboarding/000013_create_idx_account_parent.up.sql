CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_parent
    ON account (organization_id, ledger_id, parent_account_id)
    WHERE deleted_at IS NULL;
