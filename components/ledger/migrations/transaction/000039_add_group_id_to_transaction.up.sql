-- group_id links the per-ledger transactions produced by one atomic
-- cross-ledger request. NULL preserves existing rows without a rewrite.
ALTER TABLE transaction
    ADD COLUMN IF NOT EXISTS group_id UUID;

CREATE INDEX IF NOT EXISTS idx_transaction_organization_ledger_group
    ON transaction (organization_id, ledger_id, group_id)
    WHERE deleted_at IS NULL AND group_id IS NOT NULL;
