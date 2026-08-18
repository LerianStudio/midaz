DO $$
DECLARE
    has_claims BOOLEAN := FALSE;
BEGIN
    IF to_regclass('transaction_revert_claim') IS NOT NULL THEN
        EXECUTE 'SELECT EXISTS (SELECT 1 FROM transaction_revert_claim LIMIT 1)'
            INTO has_claims;

        IF has_claims THEN
            RAISE EXCEPTION
                'cannot remove transaction_revert_claim while reversal claims exist; archive and explicitly clear every claim before rollback'
                USING ERRCODE = '55006';
        END IF;
    END IF;
END
$$;

DROP TABLE IF EXISTS transaction_revert_claim;
