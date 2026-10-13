-- A transaction may leave an optional link unused. Existing links stay
-- required, which is how every link was validated before.
ALTER TABLE operation_transaction_route ADD COLUMN IF NOT EXISTS optional BOOLEAN NOT NULL DEFAULT false;
