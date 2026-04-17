CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_entity_id
    ON account (organization_id, ledger_id, entity_id)
    WHERE deleted_at IS NULL;
