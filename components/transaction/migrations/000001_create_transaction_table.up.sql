CREATE TABLE IF NOT EXISTS transactions (
    id                                  UUID PRIMARY KEY NOT NULL DEFAULT (uuid_generate_v4()),
    parent_transaction_id               UUID,
    description                         TEXT NOT NULL,
    template                            TEXT NOT NULL,
    status                              TEXT NOT NULL,
    amount                              NUMERIC NOT NULL,
    amount_scale                        NUMERIC NOT NULL,
    instrument_code                     TEXT NOT NULL,
    chart_of_accounts_group_name        TEXT NOT NULL,
    ledger_id                           UUID NOT NULL,
    organization_id                     UUID NOT NULL,
    created_at                          TIMESTAMP WITH TIME ZONE,
    updated_at                          TIMESTAMP WITH TIME ZONE,
    deleted_at                          TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_transaction_id) REFERENCES transactions (id)
)