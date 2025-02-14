CREATE TABLE IF NOT EXISTS asset
(
    id                            UUID PRIMARY KEY NOT NULL,
    name                          TEXT,
    type                          TEXT NOT NULL,
    code                          TEXT NOT NULL,
    status                        TEXT NOT NULL,
    status_description            TEXT,
    ledger_id                     UUID NOT NULL,
    organization_id               UUID NOT NULL,
    created_at                    TIMESTAMP WITH TIME ZONE,
    updated_at                    TIMESTAMP WITH TIME ZONE,
    deleted_at                    TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (ledger_id)       REFERENCES ledger (id),
    FOREIGN KEY (organization_id) REFERENCES organization (id)
);