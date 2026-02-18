CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_id
ON operation (organization_id, ledger_id, account_id, id)
WHERE deleted_at IS NULL;