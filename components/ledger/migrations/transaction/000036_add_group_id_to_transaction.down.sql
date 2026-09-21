DROP INDEX IF EXISTS idx_transaction_organization_ledger_group;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS group_id;
