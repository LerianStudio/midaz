-- Validate the existing rows against the asset code CHECK added NOT VALID by
-- the previous migration. VALIDATE CONSTRAINT holds SHARE UPDATE EXCLUSIVE, so
-- validation writes continue while the table is scanned.
SET LOCAL lock_timeout = '5s';

ALTER TABLE transaction_validations VALIDATE CONSTRAINT transaction_validations_asset_code_format;
