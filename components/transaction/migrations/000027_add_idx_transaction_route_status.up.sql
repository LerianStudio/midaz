CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_route_status
    ON "transaction" (organization_id, ledger_id, route, status, created_at)
    WHERE deleted_at IS NULL;
