-- Stage 6: restore the operation→transaction foreign key on PostgreSQL 15+
-- where referencing partitioned tables is widely supported. On older versions
-- this ALTER would fail; we branch at runtime and emit a NOTICE so operators
-- can see why no FK was added.
--
-- The FK is declared NOT VALID so the attach is fast (no full table scan
-- under ACCESS EXCLUSIVE). Operators may run VALIDATE CONSTRAINT separately
-- at a convenient time once they are confident no orphan rows exist.

DO $$
DECLARE
    pg_major INTEGER;
BEGIN
    pg_major := current_setting('server_version_num')::INTEGER / 10000;

    IF pg_major >= 15 THEN
        IF NOT EXISTS (
            SELECT 1
            FROM pg_constraint
            WHERE conname = 'fk_operation_transaction'
        ) THEN
            EXECUTE 'ALTER TABLE operation
                ADD CONSTRAINT fk_operation_transaction
                FOREIGN KEY (transaction_id)
                REFERENCES "transaction" (id)
                NOT VALID';
        END IF;
    ELSE
        RAISE NOTICE 'Skipping fk_operation_transaction: PostgreSQL % does not support referencing partitioned tables from this side. Application-layer invariants remain authoritative.', current_setting('server_version');
    END IF;
END
$$;
