BEGIN;

ALTER TABLE operation
  ALTER COLUMN amount TYPE DECIMAL USING amount::DECIMAL,
  ALTER COLUMN available_balance TYPE DECIMAL USING available_balance::DECIMAL,
  ALTER COLUMN on_hold_balance TYPE DECIMAL USING on_hold_balance::DECIMAL,
  ALTER COLUMN available_balance_after TYPE DECIMAL USING available_balance_after::DECIMAL,
  ALTER COLUMN on_hold_balance_after TYPE DECIMAL USING on_hold_balance_after::DECIMAL;

COMMIT;
