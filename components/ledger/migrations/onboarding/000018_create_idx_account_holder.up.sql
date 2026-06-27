CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_holder
    ON account (organization_id, ledger_id, holder_id)
    WHERE deleted_at IS NULL;
