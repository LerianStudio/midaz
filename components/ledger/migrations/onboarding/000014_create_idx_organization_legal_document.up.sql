CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_organization_legal_document
    ON organization (legal_document)
    WHERE deleted_at IS NULL;
