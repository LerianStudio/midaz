-- Bench-local tables of the balance-engine experiment, applied by bootstrap.go after the ledger
-- migrations and the lib-commons outbox schema. Not a product migration.

CREATE TABLE engine_writer_fence (
  tenant_id text PRIMARY KEY,
  owner_id text NOT NULL,
  epoch bigint NOT NULL,
  lease_until timestamptz NOT NULL
);

CREATE TABLE engine_request (
  tenant_id text NOT NULL,
  request_id uuid NOT NULL,
  payload_fingerprint bytea NOT NULL,
  transaction_id uuid,
  outcome text,
  reason text NOT NULL DEFAULT '',
  admission_cutoff timestamptz NOT NULL,
  decided_at timestamptz,
  result jsonb,
  durability_touch bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (tenant_id, request_id)
);

CREATE TABLE account_control (
  organization_id uuid NOT NULL,
  ledger_id uuid NOT NULL,
  account_id uuid NOT NULL,
  blocked boolean NOT NULL DEFAULT false,
  limit_remaining numeric NOT NULL,
  version bigint NOT NULL,
  PRIMARY KEY (organization_id, ledger_id, account_id)
);
