CREATE TABLE IF NOT EXISTS instrument_rate (
    id                                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    base_instrument_id                 UUID NOT NULL,
    counter_instrument_id              UUID NOT NULL,
    amount                             NUMERIC NOT NULL,
    scale                              NUMERIC NOT NULL,
    source                             TEXT NOT NULL,
    status                             TEXT NOT NULL,
    created_at                         TIMESTAMP WITH TIME ZONE,
    updated_at                         TIMESTAMP WITH TIME ZONE,
    deleted_at                         TIMESTAMP WITH TIME ZONE
)