-- Rollback: remove the default_direction column from the account_type table.
--
-- Uses IF EXISTS so the rollback is safe to run against databases that
-- never received the up migration.

ALTER TABLE account_type
    DROP COLUMN IF EXISTS default_direction;
