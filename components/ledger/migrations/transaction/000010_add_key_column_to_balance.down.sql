CREATE INDEX idx_balance_alias ON balance (organization_id, ledger_id, alias, deleted_at, created_at);

DROP INDEX IF EXISTS idx_unique_balance_alias_key;

ALTER TABLE balance DROP COLUMN key;