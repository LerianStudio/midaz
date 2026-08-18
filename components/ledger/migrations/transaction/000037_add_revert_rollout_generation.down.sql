DO $$
DECLARE
    has_claims BOOLEAN := FALSE;
BEGIN
    IF to_regclass('transaction_revert_claim') IS NOT NULL THEN
        EXECUTE 'LOCK TABLE transaction_revert_claim IN ACCESS EXCLUSIVE MODE';
        EXECUTE 'SELECT EXISTS (SELECT 1 FROM transaction_revert_claim LIMIT 1)'
            INTO has_claims;

        IF has_claims THEN
            RAISE EXCEPTION
                'cannot remove revert rollout generation while reversal claims exist; rollback requires an empty claim table'
                USING ERRCODE = '55006';
        END IF;

        EXECUTE 'ALTER TABLE transaction_revert_claim DROP CONSTRAINT IF EXISTS transaction_revert_claim_rollout_check';
        EXECUTE 'ALTER TABLE transaction_revert_claim DROP COLUMN IF EXISTS redis_generation';
        EXECUTE 'ALTER TABLE transaction_revert_claim DROP COLUMN IF EXISTS rollout_token';
        EXECUTE 'ALTER TABLE transaction_revert_claim DROP COLUMN IF EXISTS rollout_mode';
    END IF;
END
$$;
