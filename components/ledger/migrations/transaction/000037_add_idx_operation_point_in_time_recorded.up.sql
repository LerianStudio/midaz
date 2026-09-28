-- Use the server recording axis for PIT reads while preserving legacy rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_account_balance_pit_recorded
ON operation (organization_id, ledger_id, account_id, balance_id, (COALESCE(recorded_at, created_at)) DESC, balance_version_after DESC)
WHERE deleted_at IS NULL;
