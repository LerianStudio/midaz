CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_org_ledger_route_code
    ON operation (organization_id, ledger_id, route_code)
    WHERE deleted_at IS NULL AND route_code IS NOT NULL;
