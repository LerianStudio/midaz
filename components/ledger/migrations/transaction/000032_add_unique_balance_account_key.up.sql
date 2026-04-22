-- Prevent duplicate balances for the same account + key combination.
-- Uses a partial index (WHERE deleted_at IS NULL) so soft-deleted rows
-- do not block new balances with the same key.
--
-- CONCURRENTLY avoids long-lived ACCESS EXCLUSIVE locks on the balance
-- table during deployment. Combined with IF NOT EXISTS, the migration
-- is safe to re-run.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_unique_balance_account_key
    ON balance (organization_id, ledger_id, account_id, asset_code, key)
    WHERE deleted_at IS NULL;
