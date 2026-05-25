-- The DROP CONSTRAINT below is intentionally retained. It is a no-op on databases
-- that never had `chk_operation_direction` (the constraint was never shipped to
-- production), but we keep the statement so migration 021 still has some DDL and
-- so any environment that did experiment with the constraint converges with prod.
-- IDEMPOTENT: IF EXISTS means re-running is safe.
--
-- Direction validation is enforced at the application layer (see
-- validateOperationDirection in services/command/create_balance_transaction_operations_async.go).
ALTER TABLE operation DROP CONSTRAINT IF EXISTS chk_operation_direction;
