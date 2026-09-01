CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_org_holder
    ON account (organization_id, holder_id)
    WHERE deleted_at IS NULL;
