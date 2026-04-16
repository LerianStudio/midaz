-- Reverse the atomic swap: put the legacy operation table back under its
-- original name and restore phase='dual_write' so application-layer dual-write
-- resumes. CASCADE is never used. Data is preserved in the now-renamed
-- operation_partitioned table.

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_class WHERE relname = 'operation_legacy')
       AND EXISTS (SELECT 1 FROM pg_class WHERE relname = 'operation') THEN
        LOCK TABLE operation IN ACCESS EXCLUSIVE MODE;
        LOCK TABLE operation_legacy IN ACCESS EXCLUSIVE MODE;
    END IF;
END
$$;

ALTER TABLE IF EXISTS operation RENAME TO operation_partitioned;
ALTER TABLE IF EXISTS operation_legacy RENAME TO operation;

UPDATE partition_migration_state
SET phase = 'dual_write', updated_at = now()
WHERE id = 1;
