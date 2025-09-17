ALTER TABLE balance ADD COLUMN key TEXT NOT NULL DEFAULT 'default';

CREATE INDEX idx_unique_balance_alias_key ON balance (organization_id, ledger_id, alias, key) WHERE deleted_at IS NULL;

DROP INDEX IF EXISTS idx_balance_alias;