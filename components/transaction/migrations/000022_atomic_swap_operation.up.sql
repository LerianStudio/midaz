-- Stage 4: atomic RENAME swap of the operation table.
--
-- Preconditions (operator must verify before running):
--   1. Dual-write has been enabled (phase='dual_write') long enough for the
--      backfill tool (cmd/partition-backfill --table=operation) to have run
--      to completion.
--   2. Row counts between `operation` and `operation_partitioned` match.
--      This migration re-asserts the invariant under an ACCESS EXCLUSIVE lock
--      before performing the RENAME and RAISEs EXCEPTION on mismatch so the
--      whole migration transaction rolls back cleanly without data loss.
--
-- golang-migrate runs each file in an implicit transaction for the PostgreSQL
-- driver. The LOCK TABLE + DO block + RENAMEs therefore commit atomically or
-- not at all. This replaces the B2-blocker-era pattern of copying data inline.

DO $$
DECLARE
    legacy_count BIGINT;
    partitioned_count BIGINT;
BEGIN
    LOCK TABLE operation IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE operation_partitioned IN ACCESS EXCLUSIVE MODE;

    SELECT count(*) INTO legacy_count FROM operation;
    SELECT count(*) INTO partitioned_count FROM operation_partitioned;

    IF legacy_count <> partitioned_count THEN
        RAISE EXCEPTION
            'atomic_swap_operation aborted: row count mismatch (legacy=%, partitioned=%). Run cmd/partition-backfill before retrying.',
            legacy_count, partitioned_count;
    END IF;
END
$$;

ALTER TABLE operation RENAME TO operation_legacy;
ALTER TABLE operation_partitioned RENAME TO operation;

UPDATE partition_migration_state
SET phase = 'partitioned', updated_at = now()
WHERE id = 1;
