CREATE TABLE IF NOT EXISTS segment
(
    id                            UUID PRIMARY KEY NOT NULL,
    name                          TEXT,
    ledger_id                     UUID NOT NULL,
    organization_id               UUID NOT NULL,
    status                        TEXT NOT NULL,
    status_description            TEXT,
    created_at                    TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                    TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (ledger_id)       REFERENCES ledger (id),
    FOREIGN KEY (organization_id) REFERENCES organization (id)
);

CREATE INDEX idx_segment_created_at ON segment (created_at);
REINDEX INDEX idx_segment_created_at;