CREATE TABLE IF NOT EXISTS transaction_route (
    id                              UUID PRIMARY KEY NOT NULL,
    organization_id                 UUID NOT NULL,
    ledger_id                       UUID NOT NULL,
    title                           VARCHAR(255) NOT NULL,
    description                     VARCHAR(250),
    created_at                      TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                      TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_transaction_route_organization_id_ledger_id ON transaction_route (organization_id, ledger_id);

CREATE TABLE IF NOT EXISTS operation_transaction_route (
    id UUID PRIMARY KEY NOT NULL,
    operation_route_id UUID NOT NULL REFERENCES operation_route(id),
    transaction_route_id UUID NOT NULL REFERENCES transaction_route(id),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE UNIQUE INDEX idx_operation_transaction_route_unique 
ON operation_transaction_route (operation_route_id, transaction_route_id) 
WHERE deleted_at IS NULL;

CREATE INDEX idx_operation_transaction_route_operation_route_id
ON operation_transaction_route (operation_route_id)
WHERE deleted_at IS NULL;

CREATE INDEX idx_operation_transaction_route_transaction_route_id
ON operation_transaction_route (transaction_route_id)
WHERE deleted_at IS NULL;

CREATE INDEX idx_operation_transaction_route_deleted_at
ON operation_transaction_route (deleted_at);