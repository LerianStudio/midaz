DO $$
DECLARE
    has_claims BOOLEAN := FALSE;
BEGIN
    IF to_regclass('transaction_revert_claim') IS NOT NULL THEN
        LOCK TABLE transaction_revert_claim IN ACCESS EXCLUSIVE MODE;
        SELECT EXISTS (SELECT 1 FROM transaction_revert_claim LIMIT 1)
            INTO has_claims;

        IF has_claims THEN
            RAISE EXCEPTION
                'cannot remove the revert armed phase while reversal claims exist; rollback requires an empty claim table'
                USING ERRCODE = '55006';
        END IF;

        ALTER TABLE transaction_revert_claim
            DROP CONSTRAINT IF EXISTS transaction_revert_claim_state_check;

        ALTER TABLE transaction_revert_claim
            ADD CONSTRAINT transaction_revert_claim_state_check CHECK (
                state IN ('CLAIMED', 'RECOVERING', 'MUTATED', 'COMPLETED', 'RECONCILIATION_REQUIRED')
            );
    END IF;
END
$$;
