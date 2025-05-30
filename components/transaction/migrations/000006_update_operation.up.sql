BEGIN;

ALTER TABLE operation
  ALTER COLUMN amount TYPE DECIMAL USING (amount / POWER(10, amount_scale::INTEGER))::NUMERIC,
  ALTER COLUMN available_balance TYPE DECIMAL USING (available_balance / POWER(10, balance_scale::INTEGER))::NUMERIC,
  ALTER COLUMN on_hold_balance TYPE DECIMAL USING on_hold_balance::DECIMAL,
  ALTER COLUMN available_balance_after TYPE DECIMAL USING (available_balance_after / POWER(10, balance_scale_after::INTEGER))::NUMERIC,
  ALTER COLUMN on_hold_balance_after TYPE DECIMAL USING on_hold_balance_after::DECIMAL;

COMMIT;
