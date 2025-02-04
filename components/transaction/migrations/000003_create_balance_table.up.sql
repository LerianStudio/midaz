CREATE TABLE IF NOT EXISTS balance (
  id                                  UUID PRIMARY KEY NOT NULL,
  alias                               TEXT NOT NULL,
  organization_id                     UUID NOT NULL,
  ledger_id                           UUID NOT NULL,
  asset_code                          TEXT NOT NULL,
  available                           NUMERIC NOT NULL DEFAULT 0,
  on_hold                             NUMERIC NOT NULL DEFAULT 0,
  scale                               NUMERIC NOT NULL DEFAULT 0,
  version                             NUMERIC DEFAULT 0,
  accept_negative                     BOOLEAN NOT NULL,
  created_at                          TIMESTAMP WITH TIME ZONE,
  updated_at                          TIMESTAMP WITH TIME ZONE,
  deleted_at                          TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_update_balance ON balance (id, alias, organization_id, ledger_id, version, deleted_at);

REINDEX INDEX idx_update_balance;