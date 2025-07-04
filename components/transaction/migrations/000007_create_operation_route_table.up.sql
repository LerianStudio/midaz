CREATE TABLE IF NOT EXISTS operation_route (
    id                              UUID PRIMARY KEY NOT NULL,
    organization_id                 UUID NOT NULL,
    ledger_id                       UUID NOT NULL,
    title                           VARCHAR(255) NOT NULL,
    description                     VARCHAR(250),
    type                            VARCHAR(20) NOT NULL CHECK (LOWER(type) IN ('debit', 'credit')),
    account_types                   TEXT,
    account_alias                   TEXT,
    created_at                      TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                      TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_operation_route_organization_id_ledger_id ON operation_route (organization_id, ledger_id);

CREATE INDEX idx_operation_route_type ON operation_route (organization_id, ledger_id, type) WHERE deleted_at IS NULL;

CREATE INDEX idx_operation_route_deleted_at ON operation_route (organization_id, ledger_id, deleted_at);