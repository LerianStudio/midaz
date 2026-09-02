CREATE TABLE IF NOT EXISTS account_exception (
    id                      UUID PRIMARY KEY NOT NULL,
    organization_id         UUID NOT NULL,
    ledger_id               UUID NOT NULL,
    account_id              UUID NOT NULL,
    operational_type_codes  JSONB NOT NULL,
    balance_key             VARCHAR(100),
    context                 VARCHAR(256) NOT NULL,
    effective_at            TIMESTAMP WITH TIME ZONE,
    expires_at              TIMESTAMP WITH TIME ZONE,
    created_at              TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at              TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at              TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (organization_id) REFERENCES organization (id),
    FOREIGN KEY (ledger_id) REFERENCES ledger (id),
    FOREIGN KEY (account_id) REFERENCES account (id)
);

CREATE INDEX IF NOT EXISTS idx_account_exception_org_ledger_account ON account_exception (organization_id, ledger_id, account_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_account_exception_deleted_at ON account_exception (organization_id, ledger_id, deleted_at);
