BEGIN;

ALTER TABLE operation
  ALTER COLUMN amount TYPE BIGINT USING amount::BIGINT,
  ALTER COLUMN available_balance TYPE BIGINT USING available_balance::BIGINT,
  ALTER COLUMN on_hold_balance TYPE BIGINT USING on_hold_balance::BIGINT,
  ALTER COLUMN available_balance_after TYPE BIGINT USING available_balance_after::BIGINT,
  ALTER COLUMN on_hold_balance_after TYPE BIGINT USING on_hold_balance_after::BIGINT;

COMMIT;
