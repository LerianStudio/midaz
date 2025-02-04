CREATE TABLE IF NOT EXISTS account
(
    id                              UUID PRIMARY KEY NOT NULL,
    parent_account_id               UUID,
    organization_id                 UUID NOT NULL,
    ledger_id                       UUID NOT NULL,
    portfolio_id                    UUID,
    segment_id                      UUID,
    entity_id                       TEXT,
    asset_code                      TEXT NOT NULL,
    alias                           TEXT NOT NULL,
    name                            TEXT,
    status                          TEXT NOT NULL,
    status_description              TEXT,
    type                            TEXT NOT NULL,
    created_at                      TIMESTAMP WITH TIME ZONE,
    updated_at                      TIMESTAMP WITH TIME ZONE,
    deleted_at                      TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_account_id) REFERENCES account (id),
    FOREIGN KEY (organization_id)   REFERENCES organization (id),
    FOREIGN KEY (ledger_id)         REFERENCES ledger (id),
    FOREIGN KEY (portfolio_id)      REFERENCES portfolio (id),
    FOREIGN KEY (segment_id)        REFERENCES segment (id)
);