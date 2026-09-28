-- Refuse rollback while any stored validation code does not fit the previous
-- three-character column. Never truncate or pad: validation rows are immutable
-- audit records. Restoring CHAR(3) rewrites the table and blocks validation
-- writes for a duration proportional to its size.
SET LOCAL lock_timeout = '5s';

DO $migration$
BEGIN
    LOCK TABLE transaction_validations IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM transaction_validations WHERE length(asset) <> 3) THEN
        RAISE EXCEPTION 'cannot restore the three-character transaction_validations.asset column while longer or shorter asset codes are stored'
            USING ERRCODE = '23514';
    END IF;

    ALTER TABLE transaction_validations DROP CONSTRAINT transaction_validations_asset_code_format;
    ALTER TABLE transaction_validations ALTER COLUMN asset TYPE CHAR(3);
END;
$migration$;
