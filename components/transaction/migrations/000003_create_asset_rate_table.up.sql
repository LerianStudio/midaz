CREATE TABLE IF NOT EXISTS asset_rate (
    id                                 UUID PRIMARY KEY NOT NULL DEFAULT (uuid_generate_v4()),
    base_asset_id                      UUID NOT NULL,
    counter_asset_id                   UUID NOT NULL,
    amount                             NUMERIC NOT NULL,
    scale                              NUMERIC NOT NULL,
    source                             TEXT NOT NULL,
    status                             TEXT NOT NULL,
    status_description                 TEXT,
    organization_id                    UUID NOT NULL,
    ledger_id                          UUID NOT NULL,
    created_at                         TIMESTAMP WITH TIME ZONE,
    updated_at                         TIMESTAMP WITH TIME ZONE,
    deleted_at                         TIMESTAMP WITH TIME ZONE
)