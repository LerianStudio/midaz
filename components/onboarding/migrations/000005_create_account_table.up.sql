CREATE TABLE IF NOT EXISTS account
(
    id                              UUID PRIMARY KEY NOT NULL,
    name                            TEXT,
    parent_account_id               UUID,
    entity_id                       TEXT,
    asset_code                      TEXT NOT NULL,
    organization_id                 UUID NOT NULL,
    ledger_id                       UUID NOT NULL,
    portfolio_id                    UUID,
    segment_id                      UUID,
    status                          TEXT NOT NULL,
    status_description              TEXT,
    alias                           TEXT NOT NULL,
    type                            TEXT NOT NULL,
    created_at                    TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                    TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                      TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_account_id) REFERENCES account (id),
    FOREIGN KEY (organization_id)   REFERENCES organization (id),
    FOREIGN KEY (ledger_id)         REFERENCES ledger (id),
    FOREIGN KEY (portfolio_id)      REFERENCES portfolio (id),
    FOREIGN KEY (segment_id)        REFERENCES segment (id)
);

CREATE INDEX idx_account_created_at ON account (created_at);
REINDEX INDEX idx_account_created_at;