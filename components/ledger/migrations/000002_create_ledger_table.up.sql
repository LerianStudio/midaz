CREATE TABLE IF NOT EXISTS ledger
(
    id                            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name                          TEXT NOT NULL,
    organization_id               UUID NOT NULL,
    status                        TEXT NOT NULL,
    status_description            TEXT,
    created_at                    TIMESTAMP WITH TIME ZONE,
    updated_at                    TIMESTAMP WITH TIME ZONE,
    deleted_at                    TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (organization_id) REFERENCES organization (id)
);