CREATE TABLE IF NOT EXISTS organization
(
    id                                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_organization_id               UUID,
    legal_name                           TEXT NOT NULL,
    doing_business_as                    TEXT,
    legal_document                       TEXT NOT NULL,
    address                              JSONB NOT NULL,
    status                               TEXT NOT NULL,
    status_description                   TEXT,
    created_at                           TIMESTAMP WITH TIME ZONE,
    updated_at                           TIMESTAMP WITH TIME ZONE,
    deleted_at                           TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_organization_id) REFERENCES organization (id)
);