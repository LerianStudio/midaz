-- A stored association may already define the meaning of financial history.
-- Never erase it to permit a binary rollback. Empty installations alone can
-- remove this schema; fail promptly if concurrent readers/writers are active.
DO $migration$
BEGIN
    LOCK TABLE limits, limit_asset_references IN ACCESS EXCLUSIVE MODE NOWAIT;
    IF EXISTS (SELECT 1 FROM limit_asset_references) THEN
        RAISE EXCEPTION 'cannot remove stored limit asset references' USING ERRCODE = '23514';
    END IF;
    DROP TABLE limit_asset_references;
    DROP FUNCTION reject_limit_asset_reference_mutation();
    ALTER TABLE limits DROP CONSTRAINT limits_id_asset_unique;
END;
$migration$;
