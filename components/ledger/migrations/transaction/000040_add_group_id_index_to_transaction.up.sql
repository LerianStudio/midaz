-- Revert-by-group starts from any member and spans every ledger in the tenant,
-- so it needs a lookup index whose leading column is the group identifier.
--
-- CONCURRENTLY avoids blocking writes on the transaction table while the
-- index builds; it must stay the only statement in this file because it
-- cannot run inside a transaction block.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_group
    ON transaction (group_id)
    WHERE deleted_at IS NULL AND group_id IS NOT NULL;
