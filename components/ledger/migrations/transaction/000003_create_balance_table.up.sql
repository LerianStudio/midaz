CREATE TABLE IF NOT EXISTS balance (
  id                                  UUID PRIMARY KEY NOT NULL,
  organization_id                     UUID NOT NULL,
  ledger_id                           UUID NOT NULL,
  account_id                          UUID NOT NULL,
  alias                               TEXT NOT NULL,
  asset_code                          TEXT NOT NULL,
  available                           BIGINT NOT NULL DEFAULT 0,
  on_hold                             BIGINT NOT NULL DEFAULT 0,
  scale                               BIGINT NOT NULL DEFAULT 0,
  version                             BIGINT DEFAULT 0,
  account_type                        TEXT NOT NULL,
  allow_sending                       BOOLEAN NOT NULL,
  allow_receiving                     BOOLEAN NOT NULL,
  created_at                          TIMESTAMP WITH TIME ZONE NOT NULL,
  updated_at                          TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
  deleted_at                          TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_balance_account_id ON balance (organization_id, ledger_id, account_id, deleted_at, created_at);
REINDEX INDEX idx_balance_account_id;

CREATE INDEX idx_balance_alias ON balance (organization_id, ledger_id, alias, deleted_at, created_at);
REINDEX INDEX idx_balance_alias;