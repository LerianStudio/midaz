BEGIN;

ALTER TABLE transaction
    ALTER COLUMN amount TYPE DECIMAL USING (amount / POWER(10, amount_scale::INTEGER))::DECIMAL;

COMMIT;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS amount_scale;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS template;

ALTER TABLE transaction
    ADD COLUMN IF NOT EXISTS route TEXT NULL;

ALTER TABLE transaction 
    ALTER COLUMN IF EXISTS body DROP NOT NULL;

UPDATE transaction
    SET body = NULL;

VACUUM FULL transaction;