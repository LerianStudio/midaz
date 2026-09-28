-- Listing a ledger's transactions filtered by group stays inside one
-- organization and ledger, so this lookup leads with the scope columns.
--
-- CONCURRENTLY avoids blocking writes on the transaction table while the
-- index builds; it must stay the only statement in this file because it
-- cannot run inside a transaction block.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_organization_ledger_group
    ON transaction (organization_id, ledger_id, group_id)
    WHERE deleted_at IS NULL AND group_id IS NOT NULL;
