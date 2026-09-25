-- A rollback cannot erase decisions that a producer may still replay. An empty
-- installation is reversible; populated installations require forward recovery.
DO $$
BEGIN
    -- Prevent a first decision from committing between the emptiness check and
    -- DROP, even when a migration runner executes this block on its own.
    LOCK TABLE reserve_decisions IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM reserve_decisions) THEN
        RAISE EXCEPTION 'cannot remove stored reserve decisions' USING ERRCODE = '23514';
    END IF;
    DROP TABLE reserve_decisions;
    DROP FUNCTION reject_reserve_decision_mutation();
END;
$$;
