-- Stage 4: atomic RENAME swap of the balance table.
--
-- Preconditions (operator must verify before running):
--   1. Dual-write has been enabled (phase='dual_write') long enough for the
--      backfill tool (cmd/partition-backfill --table=balance) to have run
--      to completion.
--   2. Row counts between `balance` and `balance_partitioned` match.
--      This migration re-asserts the invariant under an ACCESS EXCLUSIVE lock
--      before performing the RENAME and RAISEs EXCEPTION on mismatch so the
--      whole migration transaction rolls back cleanly without data loss.

DO $$
DECLARE
    legacy_count BIGINT;
    partitioned_count BIGINT;
BEGIN
    LOCK TABLE balance IN ACCESS EXCLUSIVE MODE;
    LOCK TABLE balance_partitioned IN ACCESS EXCLUSIVE MODE;

    SELECT count(*) INTO legacy_count FROM balance;
    SELECT count(*) INTO partitioned_count FROM balance_partitioned;

    IF legacy_count <> partitioned_count THEN
        RAISE EXCEPTION
            'atomic_swap_balance aborted: row count mismatch (legacy=%, partitioned=%). Run cmd/partition-backfill before retrying.',
            legacy_count, partitioned_count;
    END IF;
END
$$;

ALTER TABLE balance RENAME TO balance_legacy;
ALTER TABLE balance_partitioned RENAME TO balance;

-- Both tables must reach 'partitioned' phase together; 000022 already set it
-- when the operation swap completed, but issue an idempotent UPDATE so the
-- balance swap is safe to run standalone if operators stage the two swaps.
UPDATE partition_migration_state
SET phase = 'partitioned', updated_at = now()
WHERE id = 1;
