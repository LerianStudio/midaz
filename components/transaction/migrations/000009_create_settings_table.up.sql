CREATE TABLE IF NOT EXISTS settings (
    id                              UUID PRIMARY KEY NOT NULL,
    organization_id                 UUID NOT NULL,
    ledger_id                       UUID NOT NULL,
    key                             VARCHAR(255) NOT NULL,
    value                           TEXT,
    description                     VARCHAR(250),
    created_at                      TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                      TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_settings_organization_id_ledger_id ON settings (organization_id, ledger_id);

CREATE INDEX idx_settings_key ON settings (organization_id, ledger_id, key) WHERE deleted_at IS NULL;

CREATE INDEX idx_settings_deleted_at ON settings (organization_id, ledger_id, deleted_at);

CREATE UNIQUE INDEX idx_settings_unique_key ON settings (organization_id, ledger_id, key) WHERE deleted_at IS NULL;
