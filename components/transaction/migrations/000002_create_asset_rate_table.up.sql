CREATE TABLE IF NOT EXISTS asset_rate (
    id                                  UUID PRIMARY KEY NOT NULL,
    organization_id                     UUID NOT NULL,
    ledger_id                           UUID NOT NULL,
    external_id                         UUID NOT NULL,
    "from"                              TEXT NOT NULL,
    "to"                                TEXT NOT NULL,
    rate                                BIGINT NOT NULL,
    rate_scale                          NUMERIC NOT NULL,
    source                              TEXT,
    ttl                                 BIGINT NOT NULL,
    created_at                          TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX idx_asset_rate_organization_ledger_id ON asset_rate (organization_id, ledger_id);
REINDEX INDEX idx_asset_rate_organization_ledger_id;

CREATE INDEX idx_asset_rate_created_at ON asset_rate (created_at);
REINDEX INDEX idx_asset_rate_created_at;