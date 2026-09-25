-- group_id links the per-ledger transactions produced by one atomic
-- cross-ledger request. NULL preserves existing rows without a rewrite.
ALTER TABLE transaction
    ADD COLUMN IF NOT EXISTS group_id UUID;
