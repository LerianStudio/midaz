-- Quiescent, empty installations alone can drop the completion fence. Fail
-- promptly if a writer is active; never erase a pending or terminal operation.
DO $migration$
BEGIN
    LOCK TABLE reserve_operations, reserve_decisions IN ACCESS EXCLUSIVE MODE NOWAIT;
    IF EXISTS (SELECT 1 FROM reserve_operations) THEN
        RAISE EXCEPTION 'cannot remove stored reserve operations' USING ERRCODE = '23514';
    END IF;

    DROP TRIGGER reserve_decision_requires_open_operation ON reserve_decisions;
    ALTER TABLE reserve_decisions DROP CONSTRAINT reserve_decision_operation_fk;
    DROP FUNCTION guard_reserve_decision_operation();
    DROP TABLE reserve_operations;
    DROP FUNCTION guard_reserve_operation_transition();
    DROP FUNCTION reject_reserve_operation_removal();
END;
$migration$;
