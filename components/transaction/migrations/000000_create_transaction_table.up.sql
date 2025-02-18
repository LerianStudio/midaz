CREATE TABLE IF NOT EXISTS "transaction" (
    id                                  UUID PRIMARY KEY NOT NULL,
    parent_transaction_id               UUID,
    description                         TEXT NOT NULL,
    template                            TEXT NOT NULL,
    status                              TEXT NOT NULL,
    status_description                  TEXT,
    amount                              BIGINT NOT NULL,
    amount_scale                        BIGINT NOT NULL,
    asset_code                          TEXT NOT NULL,
    chart_of_accounts_group_name        TEXT NOT NULL,
    ledger_id                           UUID NOT NULL,
    organization_id                     UUID NOT NULL,
    body                                JSONB NOT NULL,
    created_at                          TIMESTAMP WITH TIME ZONE,
    updated_at                          TIMESTAMP WITH TIME ZONE,
    deleted_at                          TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_transaction_id) REFERENCES "transaction" (id)
)