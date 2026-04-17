-- Organization legal_document index for exact match filtering
-- Supports the legal_document query parameter filter on Organization listing endpoint

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_organization_legal_document
    ON organization (legal_document)
    WHERE deleted_at IS NULL;
