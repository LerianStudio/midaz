CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account 
ON operation (organization_id, ledger_id, account_id, deleted_at, created_at);

