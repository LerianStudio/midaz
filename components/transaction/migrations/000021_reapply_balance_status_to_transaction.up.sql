BEGIN;

-- Re-apply balance_status + balance_persisted_at for environments where the DB
-- already advanced to migration version 20 (e.g., persistent dev volumes).
-- This migration is intentionally idempotent.

ALTER TABLE "transaction"
    ADD COLUMN IF NOT EXISTS balance_status TEXT;

-- Ensure the check constraint exists deterministically.
ALTER TABLE "transaction"
    DROP CONSTRAINT IF EXISTS transaction_balance_status_check;

ALTER TABLE "transaction"
    ADD CONSTRAINT transaction_balance_status_check
    CHECK (balance_status IS NULL OR balance_status IN ('PENDING', 'CONFIRMED', 'FAILED'));

ALTER TABLE "transaction"
    ADD COLUMN IF NOT EXISTS balance_persisted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_transaction_balance_status_pending
    ON "transaction" (created_at, balance_status)
    WHERE balance_status = 'PENDING';

CREATE INDEX IF NOT EXISTS idx_transaction_balance_status_failed
    ON "transaction" (updated_at, balance_status)
    WHERE balance_status = 'FAILED';

COMMENT ON COLUMN "transaction".balance_status IS 'Tracks async balance update state: PENDING=queued, CONFIRMED=completed, FAILED=DLQ. NULL for sync transactions.';
COMMENT ON COLUMN "transaction".balance_persisted_at IS 'Timestamp set only when balances are durably persisted and used as proof for reconciliation.';

COMMIT;
