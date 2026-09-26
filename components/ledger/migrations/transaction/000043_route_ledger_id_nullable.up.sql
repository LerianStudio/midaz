-- Accounting routes belong to the organization. ledger_id stays on routes
-- created under a ledger as provenance and is NULL for routes created at
-- organization level.
ALTER TABLE transaction_route ALTER COLUMN ledger_id DROP NOT NULL;
ALTER TABLE operation_route ALTER COLUMN ledger_id DROP NOT NULL;
