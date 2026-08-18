DO $$
DECLARE
    armed_phase_installed BOOLEAN := FALSE;
BEGIN
    IF to_regclass('transaction_revert_claim') IS NULL THEN
        RAISE EXCEPTION 'transaction_revert_claim must exist before installing the armed phase';
    END IF;

    LOCK TABLE transaction_revert_claim IN ACCESS EXCLUSIVE MODE;

    SELECT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'transaction_revert_claim'::regclass
          AND conname = 'transaction_revert_claim_state_check'
          AND POSITION('ARMED' IN pg_get_constraintdef(oid)) > 0
    ) INTO armed_phase_installed;

    IF NOT armed_phase_installed THEN
        ALTER TABLE transaction_revert_claim
            DROP CONSTRAINT IF EXISTS transaction_revert_claim_state_check;

        UPDATE transaction_revert_claim
        SET state = 'ARMED',
            failure_reason = COALESCE(failure_reason, 'conservative_upgrade_to_armed'),
            updated_at = NOW()
        WHERE state IN ('CLAIMED', 'RECOVERING');

        ALTER TABLE transaction_revert_claim
            ADD CONSTRAINT transaction_revert_claim_state_check CHECK (
                state IN ('CLAIMED', 'ARMED', 'RECOVERING', 'MUTATED', 'COMPLETED', 'RECONCILIATION_REQUIRED')
            );
    END IF;
END
$$;
