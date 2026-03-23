CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_route_id ON "transaction" (route_id) WHERE route_id IS NOT NULL;
