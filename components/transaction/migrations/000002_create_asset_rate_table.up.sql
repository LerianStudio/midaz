CREATE TABLE IF NOT EXISTS asset_rate (
    id                                 UUID PRIMARY KEY NOT NULL,
    base_asset_code                    TEXT NOT NULL,
    counter_asset_code                 TEXT NOT NULL,
    amount                             NUMERIC NOT NULL,
    scale                              NUMERIC NOT NULL,
    source                             TEXT NOT NULL,
    organization_id                    UUID NOT NULL,
    ledger_id                          UUID NOT NULL,
    created_at                         TIMESTAMP WITH TIME ZONE
)