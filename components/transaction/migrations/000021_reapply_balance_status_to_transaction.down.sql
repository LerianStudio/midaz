BEGIN;

DROP INDEX IF EXISTS idx_transaction_balance_status_failed;
DROP INDEX IF EXISTS idx_transaction_balance_status_pending;

ALTER TABLE "transaction" DROP CONSTRAINT IF EXISTS transaction_balance_status_check;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_persisted_at;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_status;

COMMIT;
