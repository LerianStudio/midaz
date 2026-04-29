CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_blocked
    ON account (organization_id, ledger_id)
    WHERE blocked = true AND deleted_at IS NULL;
