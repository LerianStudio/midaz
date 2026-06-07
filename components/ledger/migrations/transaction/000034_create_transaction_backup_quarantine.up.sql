-- Create the transaction_backup_quarantine table.
--
-- This table is the durable Postgres landing zone for poison records evicted
-- from the Redis backup queue (backup_queue:{transactions}). A poison record is
-- the ONLY durable copy of an AUTHORIZED financial transaction that the backup
-- consumer cannot replay (unparseable payload, nil Validate, or repeated
-- ledger-settings fetch failure). It MUST be persisted here BEFORE it is ever
-- deleted from Redis, so the raw payload (payload JSONB) is preserved verbatim
-- as the financial copy.
--
-- Columns:
--   * id              — surrogate primary key.
--   * organization_id — owning organization (parsed from the Redis field key).
--   * ledger_id       — owning ledger (parsed from the Redis field key).
--   * transaction_id  — the authorized transaction's ID (parsed from the key).
--   * redis_key       — the originating Redis hash field key; UNIQUE so a record
--                       quarantined across multiple cycles lands exactly once.
--   * payload         — the raw backup record JSON. The financial copy. NOT NULL.
--   * failure_reason  — short classification of why the record was quarantined.
--   * attempts        — number of consumer cycles that failed before quarantine.
--   * first_failed_at — timestamp the record first failed (best-effort).
--   * quarantined_at  — when the row was written here.
--
-- All statements use IF NOT EXISTS for idempotent re-runs.

CREATE TABLE IF NOT EXISTS transaction_backup_quarantine (
  id              UUID PRIMARY KEY NOT NULL,
  organization_id UUID NOT NULL,
  ledger_id       UUID NOT NULL,
  transaction_id  UUID NOT NULL,
  redis_key       TEXT NOT NULL UNIQUE,
  payload         JSONB NOT NULL,
  failure_reason  TEXT,
  attempts        INTEGER NOT NULL DEFAULT 0,
  first_failed_at TIMESTAMP WITH TIME ZONE,
  quarantined_at  TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_transaction_backup_quarantine_org_ledger
  ON transaction_backup_quarantine (organization_id, ledger_id, quarantined_at);
