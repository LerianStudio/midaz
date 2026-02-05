-- lint:ignore-file (legacy migration already applied)
ALTER TABLE transaction
    ALTER COLUMN amount TYPE DECIMAL USING (amount / POWER(10, amount_scale::INTEGER))::DECIMAL;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS amount_scale;

ALTER TABLE transaction
    DROP COLUMN IF EXISTS template;

ALTER TABLE transaction
    ADD COLUMN IF NOT EXISTS route TEXT NULL;

ALTER TABLE transaction
    ALTER COLUMN body DROP NOT NULL;

UPDATE transaction
    SET body = NULL;