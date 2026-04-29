-- Roll back the partial unique index guarding (organization_id, ledger_id,
-- account_id, asset_code, key) for live balance rows.
DROP INDEX CONCURRENTLY IF EXISTS idx_unique_balance_account_key;
