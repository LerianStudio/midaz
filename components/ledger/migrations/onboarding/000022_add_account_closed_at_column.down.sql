-- ROLLBACK RESTRICTION: closed_at is the source of truth about whether an account
-- is closed, and nothing else records it. Dropping the column on a database that
-- already holds closings destroys that state irrecoverably and reopens those
-- accounts to movement. Apply this rollback only on an environment with no
-- closings and with consumers that do not read the column.
ALTER TABLE account DROP COLUMN IF EXISTS closed_at;
