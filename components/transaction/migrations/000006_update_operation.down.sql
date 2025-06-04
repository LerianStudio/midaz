BEGIN;

ALTER TABLE operation
  ALTER COLUMN amount TYPE BIGINT USING (amount * POWER(10, amount_scale::INTEGER))::BIGINT,
  ALTER COLUMN available_balance TYPE BIGINT USING (available_balance * POWER(10, balance_scale::INTEGER))::BIGINT,
  ALTER COLUMN on_hold_balance TYPE BIGINT USING on_hold_balance::BIGINT,
  ALTER COLUMN available_balance_after TYPE BIGINT USING (available_balance_after * POWER(10, balance_scale_after::INTEGER))::BIGINT,
  ALTER COLUMN on_hold_balance_after TYPE BIGINT USING on_hold_balance_after::BIGINT;

COMMIT;

ALTER TABLE operation
    ADD COLUMN amount_scale BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN balance_scale BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN balance_scale_after BIGINT NOT NULL DEFAULT 0;
