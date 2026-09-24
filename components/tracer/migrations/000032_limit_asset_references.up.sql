-- Explicit references preserve the original limit/counter/reservation IDs.
-- No asset identity is inferred from a legacy currency code. The migration
-- runner must resolve official account assets and audit each association before
-- enabling the context Reserve path. Unmapped candidates remain visible to it.
DO $migration$
BEGIN
    ALTER TABLE limits ADD CONSTRAINT limits_id_asset_unique UNIQUE (id, asset);
    CREATE TABLE limit_asset_references (
        limit_id UUID PRIMARY KEY CHECK (limit_id <> '00000000-0000-0000-0000-000000000000'),
        asset_namespace TEXT NOT NULL CHECK (length(asset_namespace) > 0 AND btrim(asset_namespace) = asset_namespace),
        asset_id TEXT NOT NULL CHECK (length(asset_id) > 0 AND btrim(asset_id) = asset_id),
        asset_code TEXT NOT NULL CHECK (length(asset_code) > 0 AND btrim(asset_code) = asset_code),
        CONSTRAINT limit_asset_reference_limit_fk FOREIGN KEY (limit_id, asset_code)
            REFERENCES limits (id, asset)
    );

    CREATE FUNCTION reject_limit_asset_reference_mutation() RETURNS trigger AS $function$
    BEGIN
        RAISE EXCEPTION 'limit asset references are immutable' USING ERRCODE = '23514';
    END;
    $function$ LANGUAGE plpgsql;

    CREATE TRIGGER immutable_limit_asset_reference
        BEFORE UPDATE OR DELETE ON limit_asset_references
        FOR EACH ROW EXECUTE FUNCTION reject_limit_asset_reference_mutation();
    CREATE TRIGGER immutable_limit_asset_reference_truncate
        BEFORE TRUNCATE ON limit_asset_references
        FOR EACH STATEMENT EXECUTE FUNCTION reject_limit_asset_reference_mutation();
END;
$migration$;
