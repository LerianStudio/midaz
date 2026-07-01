-- Add the per-call holder-skip audit column to the account table.
-- holder_check_skipped records whether an honored per-call control skip
-- (the two-key Overrides opt-in) bypassed the holder existence check for
-- this account, leaving a durable, queryable audit trail.
--
-- This is a metadata-only ALTER on PostgreSQL 11+ (non-volatile constant
-- default). No table rewrite is triggered; pre-existing rows read FALSE
-- without physical update. IF NOT EXISTS keeps the ALTER idempotent.
ALTER TABLE account ADD COLUMN IF NOT EXISTS holder_check_skipped BOOLEAN NOT NULL DEFAULT FALSE;
