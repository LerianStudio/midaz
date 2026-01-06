BEGIN;

-- Add balance_status column to track async balance update state
-- Values:
--   PENDING   - Balance update queued but not yet confirmed
--   CONFIRMED - Balance update completed successfully
--   FAILED    - Balance update failed after max retries (in DLQ)
-- NULL for sync transactions (balance updated synchronously)
ALTER TABLE "transaction"
    ADD COLUMN balance_status TEXT
    CHECK (balance_status IS NULL OR balance_status IN ('PENDING', 'CONFIRMED', 'FAILED'));

-- Durable proof that balances were persisted successfully.
-- This prevents reconciliation from guessing based on secondary signals.
ALTER TABLE "transaction"
    ADD COLUMN balance_persisted_at TIMESTAMPTZ NULL;

-- Index for efficient status queries.
-- NOTE: Put created_at first to match queries like:
--   WHERE balance_status='PENDING' AND created_at < $1 ORDER BY created_at ASC
CREATE INDEX idx_transaction_balance_status_pending
    ON "transaction" (created_at)
    WHERE balance_status = 'PENDING';

-- Index for failed transactions requiring attention
CREATE INDEX idx_transaction_balance_status_failed
    ON "transaction" (updated_at)
    WHERE balance_status = 'FAILED';

COMMENT ON COLUMN "transaction".balance_status IS 'Tracks async balance update state: PENDING=queued, CONFIRMED=completed, FAILED=DLQ. NULL for sync transactions.';
COMMENT ON COLUMN "transaction".balance_persisted_at IS 'Timestamp set only when balances are durably persisted and used as proof for reconciliation.';

-- IMPORTANT: balance_persisted_at is internal-only and must NOT be exposed via public APIs.

COMMIT;
