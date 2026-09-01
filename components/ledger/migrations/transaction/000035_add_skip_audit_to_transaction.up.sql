-- Add per-call skip-audit columns to the transaction table.
-- fees_skipped / tracer_skipped record whether an honored per-call control
-- skip (the two-key Overrides opt-in) bypassed the fee engine or tracer
-- reserve for this transaction, leaving a durable, queryable audit trail.
--
-- This is a metadata-only ALTER on PostgreSQL 11+ (non-volatile constant
-- default). No table rewrite is triggered; pre-existing rows read FALSE
-- without physical update. IF NOT EXISTS keeps the ALTER idempotent.
ALTER TABLE transaction ADD COLUMN IF NOT EXISTS fees_skipped BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE transaction ADD COLUMN IF NOT EXISTS tracer_skipped BOOLEAN NOT NULL DEFAULT FALSE;
