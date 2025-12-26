CREATE UNIQUE INDEX IF NOT EXISTS idx_account_alias_unique
ON account (organization_id, ledger_id, alias)
WHERE deleted_at IS NULL;
