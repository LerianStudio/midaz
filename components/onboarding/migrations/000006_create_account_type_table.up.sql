CREATE TABLE IF NOT EXISTS account_type (
    id                  UUID PRIMARY KEY NOT NULL,
    organization_id     UUID NOT NULL,
    ledger_id           UUID NOT NULL,
    name                VARCHAR(100) NOT NULL,
    description         TEXT,
    key_value           VARCHAR(50) NOT NULL,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (organization_id) REFERENCES organization (id),
    FOREIGN KEY (ledger_id) REFERENCES ledger (id)
);

CREATE INDEX idx_account_type_organization_id_ledger_id ON account_type (organization_id, ledger_id);

CREATE INDEX idx_account_type_key_value ON account_type (organization_id, ledger_id, key_value) WHERE deleted_at IS NULL;

CREATE INDEX idx_account_type_deleted_at ON account_type (organization_id, ledger_id, deleted_at);

CREATE UNIQUE INDEX idx_account_type_unique_key_value ON account_type (organization_id, ledger_id, key_value) WHERE deleted_at IS NULL;
