-- WARNING: Rolling back will permanently delete balance_status and balance_persisted_at data.
-- Ensure no async transactions are in PENDING state before rollback.
-- Check: SELECT COUNT(*) FROM "transaction" WHERE balance_status = 'PENDING';
BEGIN;

DROP INDEX IF EXISTS idx_transaction_balance_status_failed;
DROP INDEX IF EXISTS idx_transaction_balance_status_pending;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_persisted_at;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_status;

COMMIT;
