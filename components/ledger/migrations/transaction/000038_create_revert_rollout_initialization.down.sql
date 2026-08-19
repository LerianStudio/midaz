DO $$
DECLARE
    has_rollout_initialization BOOLEAN := FALSE;
BEGIN
    IF to_regclass('transaction_revert_rollout_initialization') IS NOT NULL THEN
        EXECUTE 'LOCK TABLE transaction_revert_rollout_initialization IN ACCESS EXCLUSIVE MODE';
        EXECUTE 'SELECT EXISTS (SELECT 1 FROM transaction_revert_rollout_initialization LIMIT 1)'
            INTO has_rollout_initialization;

        IF has_rollout_initialization THEN
            RAISE EXCEPTION
                'cannot remove revert rollout initialization after initialization; rollback requires an uninitialized rollout'
                USING ERRCODE = '55006';
        END IF;

        EXECUTE 'DROP TABLE transaction_revert_rollout_initialization';
    END IF;
END
$$;
