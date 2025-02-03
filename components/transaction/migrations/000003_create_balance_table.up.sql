CREATE TABLE IF NOT EXISTS balance (
  id                                  UUID PRIMARY KEY NOT NULL,
  alias                               TEXT NOT NULL,
  organization_id                     UUID NOT NULL,
  ledger_id                           UUID NOT NULL,
  available                           NUMERIC NOT NULL DEFAULT 0,
  on_hold                             NUMERIC NOT NULL DEFAULT 0,
  scale                               NUMERIC NOT NULL DEFAULT 0,
  version                             NUMERIC DEFAULT 0,
  created_at                          TIMESTAMP WITH TIME ZONE,
  updated_at                          TIMESTAMP WITH TIME ZONE,
  deleted_at                          TIMESTAMP WITH TIME ZONE
)

