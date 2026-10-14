-- Rolling the application back does not need this down migration: the previous
-- release ignores the column and the trigger. Apply it only when no running release
-- reads config_version, otherwise its configuration reads and updates fail.
DROP TRIGGER IF EXISTS balance_config_version ON balance;

DROP FUNCTION IF EXISTS balance_bump_config_version();

ALTER TABLE balance DROP COLUMN IF EXISTS config_version;
