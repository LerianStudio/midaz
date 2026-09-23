-- Persist the normalized intent of a cross-ledger PENDING lifecycle group.
-- Direct and revert groups do not use this table.
CREATE TABLE IF NOT EXISTS transaction_group (
  id              UUID PRIMARY KEY NOT NULL,
  organization_id UUID NOT NULL,
  ledger_id       UUID NOT NULL,
  status          VARCHAR(100) NOT NULL,
  asset_code      VARCHAR(100) NOT NULL,
  intent          JSONB NOT NULL,
  created_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
  updated_at      TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transaction_group_org_ledger_status
  ON transaction_group (organization_id, ledger_id, status);
