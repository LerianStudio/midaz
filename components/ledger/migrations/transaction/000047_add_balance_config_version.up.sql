-- Version the balance configuration (settings, allow_sending, allow_receiving).
-- config_version is a per-row counter that rises by exactly one on every UPDATE
-- that really changes one of those columns, in the order the updates commit.
-- Caches that hold a copy of the configuration compare versions of the SAME row to
-- decide which copy is newer; the counter is never compared across rows.
--
-- The counter is kept by a trigger rather than by the application so it holds for
-- every writer: the current release, a previous release that does not know the
-- column, and manual SQL.
--
-- Compatibility: the column is NOT NULL with a constant default, so the ALTER is
-- metadata-only (no table rewrite) and every existing row reads 0. The previous
-- release names its columns explicitly in every SELECT and INSERT on balance, so it
-- keeps working against this schema; its configuration UPDATEs still raise the
-- version, which is what the current release relies on.
--
-- The lock timeout keeps the ALTER from queueing every balance write behind a long
-- reader: it fails instead, and the whole file rolls back as one unit.
SET LOCAL lock_timeout = '5s';

ALTER TABLE balance ADD COLUMN IF NOT EXISTS config_version BIGINT NOT NULL DEFAULT 0;

-- The trigger also lists config_version itself, so a statement that tries to set the
-- counter directly gets it reset to the stored value: only a real configuration
-- change moves it. A rewrite of the same value (IS DISTINCT FROM is false) does not.
-- jsonb equality ignores key order and whitespace, so re-sending the same settings in
-- another layout is not a change.
CREATE OR REPLACE FUNCTION balance_bump_config_version() RETURNS trigger AS $$
BEGIN
    IF NEW.settings IS DISTINCT FROM OLD.settings
       OR NEW.allow_sending IS DISTINCT FROM OLD.allow_sending
       OR NEW.allow_receiving IS DISTINCT FROM OLD.allow_receiving THEN
        NEW.config_version := OLD.config_version + 1;
    ELSE
        NEW.config_version := OLD.config_version;
    END IF;

    RETURN NEW;
END
$$ LANGUAGE plpgsql;

-- UPDATE OF fires only when one of these columns is a SET target, so the balance
-- sync (available, on_hold, version, overdraft_used) never runs it.
DROP TRIGGER IF EXISTS balance_config_version ON balance;

CREATE TRIGGER balance_config_version
    BEFORE UPDATE OF settings, allow_sending, allow_receiving, config_version ON balance
    FOR EACH ROW EXECUTE FUNCTION balance_bump_config_version();
