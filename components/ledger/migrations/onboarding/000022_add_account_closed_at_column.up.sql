-- Add the account closing timestamp to the account table.
-- closed_at records the single instant an account was explicitly closed; it is
-- NULL for every open account and is written exclusively by the close command.
--
-- The column is nullable with no default, so this is a metadata-only ALTER: no
-- table rewrite is triggered and every pre-existing row reads NULL without a
-- physical update, which is what keeps historical accounts open. IF NOT EXISTS
-- keeps the ALTER idempotent.
ALTER TABLE account ADD COLUMN IF NOT EXISTS closed_at TIMESTAMPTZ NULL;
