-- Reverse the atomic swap: restore the legacy balance table under its original
-- name and set phase back to 'dual_write'. CASCADE is never used.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'balance_legacy')
       AND EXISTS (SELECT 1 FROM pg_class WHERE relname = 'balance') THEN
        LOCK TABLE balance IN ACCESS EXCLUSIVE MODE;
        LOCK TABLE balance_legacy IN ACCESS EXCLUSIVE MODE;
    END IF;
END
$$;

ALTER TABLE IF EXISTS balance RENAME TO balance_partitioned;
ALTER TABLE IF EXISTS balance_legacy RENAME TO balance;

UPDATE partition_migration_state
SET phase = 'dual_write', updated_at = now()
WHERE id = 1;
