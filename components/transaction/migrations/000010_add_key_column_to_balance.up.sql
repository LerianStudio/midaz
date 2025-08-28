ALTER TABLE balance ADD COLUMN key TEXT NOT NULL DEFAULT 'default';

CREATE INDEX idx_balance_alias_key ON balance (organization_id, ledger_id, alias, key, deleted_at, created_at);