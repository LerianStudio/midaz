CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_portfolio ON account (organization_id, ledger_id, portfolio_id)
    WHERE deleted_at IS NULL;
