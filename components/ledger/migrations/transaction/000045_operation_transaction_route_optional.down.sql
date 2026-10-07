-- Drops every optional mark: after it every link is required again.
ALTER TABLE operation_transaction_route DROP COLUMN IF EXISTS optional;
