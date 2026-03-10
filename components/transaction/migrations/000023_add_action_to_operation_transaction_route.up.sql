-- Add action column to operation_transaction_route table
ALTER TABLE operation_transaction_route
    ADD COLUMN action VARCHAR(20) NOT NULL DEFAULT 'direct';

-- Add CHECK constraint for valid action values
ALTER TABLE operation_transaction_route
    ADD CONSTRAINT chk_otr_action CHECK (LOWER(action) IN ('direct', 'hold', 'commit', 'cancel', 'revert'));

-- Drop old unique index
DROP INDEX IF EXISTS idx_operation_transaction_route_unique;

-- Create new unique index including action column
CREATE UNIQUE INDEX idx_operation_transaction_route_unique
    ON operation_transaction_route (operation_route_id, transaction_route_id, action)
    WHERE deleted_at IS NULL;

-- Create action lookup index
CREATE INDEX idx_operation_transaction_route_action
    ON operation_transaction_route (transaction_route_id, action)
    WHERE deleted_at IS NULL;
