-- Asset codes follow the ledger rule: 1-100 uppercase letters. Widen limits in
-- place; limit and counter rows keep their identities. VARCHAR(3) to
-- VARCHAR(100) changes only the type modifier, so the table is not rewritten.
--
-- The CHECK restricts the ASCII range to A-Z and never rejects a non-ASCII
-- character: whether a non-ASCII character is an uppercase letter depends on
-- the database locale, so that part of the rule stays with the application.
-- Limits is sized by administrators, so validating the CHECK inline is cheap.
SET LOCAL lock_timeout = '5s';

-- lint:ignore metadata-only widening of a VARCHAR type modifier
ALTER TABLE limits ALTER COLUMN asset TYPE VARCHAR(100);
ALTER TABLE limits ADD CONSTRAINT limits_asset_code_format
    CHECK (asset ~ '^[^\x01-\x40\x5B-\x7F]{1,100}$');
