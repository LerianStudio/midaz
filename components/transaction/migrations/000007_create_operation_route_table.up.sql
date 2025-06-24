CREATE TABLE IF NOT EXISTS operation_route (
    id UUID PRIMARY KEY NOT NULL,
    title VARCHAR(255) NOT NULL,
    description VARCHAR(250),
    type VARCHAR(20) NOT NULL CHECK (LOWER(type) IN ('debit', 'credit')),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_operation_route_type ON operation_route (type) WHERE deleted_at IS NULL;

CREATE INDEX idx_operation_route_deleted_at ON operation_route (deleted_at);

CREATE UNIQUE INDEX idx_operation_route_title_type_unique ON operation_route (title, type) WHERE deleted_at IS NULL;