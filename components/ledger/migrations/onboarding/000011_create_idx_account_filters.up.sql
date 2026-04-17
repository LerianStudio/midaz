-- Account filter indexes for entity_id, blocked, and parent_account_id
-- These indexes support the new query parameter filters added to the Account listing endpoint

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_entity_id
    ON account (organization_id, ledger_id, entity_id)
    WHERE deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_blocked
    ON account (organization_id, ledger_id)
    WHERE blocked = true AND deleted_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_parent
    ON account (organization_id, ledger_id, parent_account_id)
    WHERE deleted_at IS NULL;
