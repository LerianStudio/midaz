-- Widen descriptive codes in place. Asset identity still comes from the
-- authenticated namespace/reference; never recreate limits or usage rows.
DO $migration$
BEGIN
    ALTER TABLE limits ALTER COLUMN asset TYPE TEXT;
    ALTER TABLE limits ADD CONSTRAINT limits_asset_code_bytes
        CHECK (octet_length(asset) BETWEEN 1 AND 256);
END;
$migration$;
