-- Drop action lookup index
DROP INDEX IF EXISTS idx_operation_transaction_route_action;

-- Drop new unique index
DROP INDEX IF EXISTS idx_operation_transaction_route_unique;

-- Drop CHECK constraint
ALTER TABLE operation_transaction_route
    DROP CONSTRAINT IF EXISTS chk_otr_action;

-- Drop action column
ALTER TABLE operation_transaction_route
    DROP COLUMN IF EXISTS action;

-- Recreate original unique index without action
CREATE UNIQUE INDEX idx_operation_transaction_route_unique
    ON operation_transaction_route (operation_route_id, transaction_route_id)
    WHERE deleted_at IS NULL;
