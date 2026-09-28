-- Refuses with "contains null values" while any route has no ledger: there is
-- no ledger to restore for a route created at organization level. Reassign or
-- remove those routes before rolling back.
ALTER TABLE transaction_route ALTER COLUMN ledger_id SET NOT NULL;
ALTER TABLE operation_route ALTER COLUMN ledger_id SET NOT NULL;
