CREATE TABLE IF NOT EXISTS operation (
    id                                 UUID PRIMARY KEY NOT NULL,
    transaction_id                     UUID NOT NULL,
    description                        TEXT NOT NULL,
    type                               TEXT NOT NULL,
    asset_code                         TEXT NOT NULL,
    amount                             NUMERIC NOT NULL DEFAULT 0,
    amount_scale                       SMALLINT NOT NULL DEFAULT 0,
    available_balance                  NUMERIC NOT NULL DEFAULT 0,
    on_hold_balance                    NUMERIC NOT NULL DEFAULT 0,
    balance_scale                      SMALLINT NOT NULL DEFAULT 0,
    available_balance_after            NUMERIC NOT NULL DEFAULT 0,
    on_hold_balance_after              NUMERIC NOT NULL DEFAULT 0,
    balance_scale_after                SMALLINT NOT NULL DEFAULT 0,
    status                             TEXT NOT NULL,
    status_description                 TEXT NULL,
    account_id                         UUID NOT NULL,
    account_alias                      TEXT NOT NULL,
    balance_id                         UUID NOT NULL,
    chart_of_accounts                  TEXT NOT NULL,
    organization_id                    UUID NOT NULL,
    ledger_id                          UUID NOT NULL,
    created_at                         TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                         TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                         TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (transaction_id) REFERENCES "transaction" (id)
);

CREATE INDEX idx_operation_organization_transaction_id ON operation (transaction_id);
REINDEX INDEX idx_operation_organization_transaction_id;

CREATE INDEX idx_operation_organization_ledger_id ON operation (organization_id, ledger_id);
REINDEX INDEX idx_operation_organization_ledger_id;

CREATE INDEX idx_operation_created_at ON operation (created_at);
REINDEX INDEX idx_operation_created_at;