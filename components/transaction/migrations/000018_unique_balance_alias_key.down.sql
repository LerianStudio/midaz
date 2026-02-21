-- Drop the unique index.
DROP INDEX IF EXISTS idx_unique_balance_alias_key;

-- Recreate the original non-unique index from migration 000010.
CREATE INDEX idx_unique_balance_alias_key ON balance (organization_id, ledger_id, alias, key) WHERE deleted_at IS NULL;
