-- Widen transaction_validations.asset to the ledger asset code bound. CHAR(3)
-- to VARCHAR(100) is not binary-coercible, so this rewrites the table and its
-- indexes while holding an ACCESS EXCLUSIVE lock that blocks validation writes
-- for a duration proportional to the table size. It runs apart from the limits
-- migration so reserve admission, which locks limits, is never held behind it.
--
-- Operator requirement: while the table is rewritten, the ACCESS EXCLUSIVE lock
-- blocks /v1/validations writes and dashboard reads for a time proportional to
-- the table size, and the rewrite needs free disk of about twice the table
-- size. Run it in a maintenance window for large audit trails; in multi-tenant
-- deployments it runs once per tenant database.
--
-- The CHECK is added NOT VALID: new and updated rows are checked immediately,
-- and the existing rows are validated by the next migration under a lock that
-- does not block writes. The CHECK restricts the ASCII range to A-Z and never
-- rejects a non-ASCII character: whether a non-ASCII character is an uppercase
-- letter depends on the database locale, so that part of the rule stays with
-- the application.
SET LOCAL lock_timeout = '5s';

-- lint:ignore validation rows are immutable audit records, so a new column could not be backfilled
ALTER TABLE transaction_validations ALTER COLUMN asset TYPE VARCHAR(100);
ALTER TABLE transaction_validations ADD CONSTRAINT transaction_validations_asset_code_format
    CHECK (asset ~ '^[^\x01-\x40\x5B-\x7F]{1,100}$') NOT VALID;
