-- Rolling the application back does not need this down migration: the previous
-- release ignores the column and the trigger. Apply it only when no running release
-- reads config_version, otherwise its configuration reads and updates fail.
DROP TRIGGER IF EXISTS account_config_version ON account;

DROP FUNCTION IF EXISTS account_bump_config_version();

ALTER TABLE account DROP COLUMN IF EXISTS config_version;
