DROP INDEX IF EXISTS idx_operation_transaction_route_deleted_at;
DROP INDEX IF EXISTS idx_operation_transaction_route_transaction_route_id;
DROP INDEX IF EXISTS idx_operation_transaction_route_operation_route_id;
DROP INDEX IF EXISTS idx_operation_transaction_route_unique;

DROP TABLE IF EXISTS operation_transaction_route;

DROP INDEX IF EXISTS idx_transaction_route_organization_id_ledger_id;

DROP TABLE IF EXISTS transaction_route;