-- Add snapshot JSONB column to operation table.
-- System-generated per-operation context (e.g., overdraftUsedBefore/After).
--
-- This is a metadata-only ALTER on PostgreSQL 11+ (non-volatile default).
-- No table rewrite is triggered because the default value is a constant.
-- Pre-existing rows receive the default '{}' on read without physical update.
--
-- No GIN index: the column is read-per-row. If future queries need filtering by
-- snapshot keys, add an index in a follow-up migration.
ALTER TABLE operation
    ADD COLUMN snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
