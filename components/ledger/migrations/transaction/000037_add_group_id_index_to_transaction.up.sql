-- Revert-by-group starts from any member and spans every ledger in the tenant,
-- so it needs a lookup index whose leading column is the group identifier.
CREATE INDEX IF NOT EXISTS idx_transaction_group
    ON transaction (group_id)
    WHERE deleted_at IS NULL AND group_id IS NOT NULL;
