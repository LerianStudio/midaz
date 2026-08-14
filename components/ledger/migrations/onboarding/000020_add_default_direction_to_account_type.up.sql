-- Add the default_direction column to the account_type table.
--
-- default_direction records the accounting direction ("credit"/"debit") an
-- account of this type resolves to when the direction is not otherwise
-- determined. Defaults to 'credit' so pre-existing rows and non-external
-- accounts preserve the prior implicit behavior; external accounts never
-- consult the type.
--
-- This is a metadata-only ALTER on PostgreSQL 11+ (non-volatile constant
-- default). No table rewrite is triggered; pre-existing rows read 'credit'
-- without physical update. IF NOT EXISTS keeps the ALTER idempotent.

ALTER TABLE account_type
    ADD COLUMN IF NOT EXISTS default_direction VARCHAR(16) NOT NULL DEFAULT 'credit'
                             CHECK (default_direction IN ('credit', 'debit'));
