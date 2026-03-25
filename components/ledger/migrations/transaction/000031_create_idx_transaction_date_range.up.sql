CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_date_range
ON "transaction" (organization_id, ledger_id, created_at)
WHERE deleted_at IS NULL;
