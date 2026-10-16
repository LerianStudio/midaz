-- Version the account configuration (blocked). config_version is a per-row counter
-- that rises by exactly one on every UPDATE that really changes blocked, in the
-- order the updates commit. Caches that hold a copy of the flag compare versions of
-- the SAME account to decide which copy is newer; the counter is independent of the
-- balance configuration version and is never compared with it.
--
-- The counter is kept by a trigger rather than by the application so it holds for
-- every writer: the current release, a previous release that does not know the
-- column, and manual SQL.
--
-- Compatibility: the column is NOT NULL with a constant default, so the ALTER is
-- metadata-only (no table rewrite) and every existing row reads 0. The previous
-- release names its columns explicitly in every SELECT and INSERT on account, so it
-- keeps working against this schema.

ALTER TABLE account ADD COLUMN IF NOT EXISTS config_version BIGINT NOT NULL DEFAULT 0;

-- The trigger also lists config_version itself, so a statement that tries to set the
-- counter directly gets it reset to the stored value: only a real change of blocked
-- moves it. A rewrite of the same value does not.
CREATE OR REPLACE FUNCTION account_bump_config_version() RETURNS trigger AS $$
BEGIN
    IF NEW.blocked IS DISTINCT FROM OLD.blocked THEN
        NEW.config_version := OLD.config_version + 1;
    ELSE
        NEW.config_version := OLD.config_version;
    END IF;

    RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- UPDATE OF fires only when one of these columns is a SET target, so an update of
-- the name, status, alias or other fields never runs it.
DROP TRIGGER IF EXISTS account_config_version ON account;

CREATE TRIGGER account_config_version
    BEFORE UPDATE OF blocked, config_version ON account
    FOR EACH ROW EXECUTE FUNCTION account_bump_config_version();
