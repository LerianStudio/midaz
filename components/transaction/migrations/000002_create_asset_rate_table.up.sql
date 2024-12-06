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
    created_at                          TIMESTAMP WITH TIME ZONE,
    updated_at                          TIMESTAMP WITH TIME ZONE
)