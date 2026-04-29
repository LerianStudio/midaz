-- Rollback: remove the overdraft fields from the balance table.
--
-- Uses IF EXISTS so the rollback is safe to run against databases that
-- never received the up migration.

ALTER TABLE balance
    DROP COLUMN IF EXISTS direction,
    DROP COLUMN IF EXISTS overdraft_used,
    DROP COLUMN IF EXISTS settings;
