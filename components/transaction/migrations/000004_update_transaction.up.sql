BEGIN;

ALTER TABLE transaction
    ALTER COLUMN amount TYPE DECIMAL USING (amount / POWER(10, amount_scale::INTEGER))::NUMERIC;

COMMIT;

ALTER TABLE transaction
    DROP COLUMN amount_scale;