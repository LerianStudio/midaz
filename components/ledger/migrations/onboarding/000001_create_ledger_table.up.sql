CREATE TABLE IF NOT EXISTS ledger
(
    id                            UUID PRIMARY KEY NOT NULL,
    name                          TEXT NOT NULL,
    organization_id               UUID NOT NULL,
    status                        TEXT NOT NULL,
    status_description            TEXT,
    created_at                    TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                    TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (organization_id) REFERENCES organization (id)
);

CREATE INDEX idx_ledger_created_at ON ledger (created_at);
REINDEX INDEX idx_ledger_created_at;