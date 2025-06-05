BEGIN;

ALTER TABLE transaction
    ALTER COLUMN amount TYPE DECIMAL USING (amount / POWER(10, amount_scale::INTEGER))::DECIMAL;

COMMIT;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS amount_scale;
