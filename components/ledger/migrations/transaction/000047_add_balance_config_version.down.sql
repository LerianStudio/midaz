-- Rolling the application back does not need this down migration: the previous
-- release ignores the column and the trigger.
--
-- The down removes the trigger and its function and keeps config_version. Dropping
-- the column would restart every version at 0 when the up runs again, below the
-- versions the configuration copies in the cache still carry, and such a copy would
-- then decide over every later change of its balance. The up adds the column only
-- if it is missing, so re-applying it keeps the stored versions. While the trigger
-- is absent, configuration changes do not raise the version.
DROP TRIGGER IF EXISTS balance_config_version ON balance;

DROP FUNCTION IF EXISTS balance_bump_config_version();
