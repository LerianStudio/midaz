CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_balance_pit
ON operation (organization_id, ledger_id, account_id, balance_id, created_at DESC, balance_version_after DESC)
WHERE deleted_at IS NULL;
