BEGIN;

-- Metadata Outbox table for reliable MongoDB metadata creation
-- Entries are created atomically with PostgreSQL transactions and processed asynchronously
--
-- Status transitions:
--   PENDING -> PROCESSING (worker claims entry)
--   PROCESSING -> PUBLISHED (success)
--   PROCESSING -> FAILED (error, will retry if retry_count < max_retries)
--   FAILED -> PROCESSING (retry attempt)
--   FAILED -> DLQ (max retries exceeded, requires manual intervention)
-- NOTE: No FK constraint on entity_id intentionally - outbox entries may outlive
-- their source entities during async processing. Cleanup job handles orphans.
CREATE TABLE IF NOT EXISTS metadata_outbox (
    id                    UUID PRIMARY KEY NOT NULL DEFAULT gen_random_uuid(),
    entity_id             VARCHAR(255) NOT NULL,            -- ID of the entity (transaction/operation)
    entity_type           TEXT NOT NULL CHECK (entity_type IN ('Transaction', 'Operation')), -- Validated entity types
    metadata              JSONB NOT NULL,                   -- The metadata to create in MongoDB
    status                TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED', 'DLQ')),
    retry_count           INTEGER NOT NULL DEFAULT 0,       -- Number of retry attempts
    max_retries           INTEGER NOT NULL DEFAULT 10,      -- Maximum retry attempts before DLQ
    next_retry_at         TIMESTAMP WITH TIME ZONE,         -- When to retry next (for backoff)
    processing_started_at TIMESTAMP WITH TIME ZONE,         -- When processing began (for stale detection)
    last_error            VARCHAR(512),                     -- Last error message (sanitized, no PII)
    created_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    updated_at            TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    processed_at          TIMESTAMP WITH TIME ZONE          -- When successfully processed
    -- Note: Metadata size validation (64KB limit) enforced in application layer
);

-- Index for polling pending entries efficiently (fixed: removed now() from predicate)
-- Includes PROCESSING entries older than threshold for stale recovery
CREATE INDEX idx_metadata_outbox_pending ON metadata_outbox (status, next_retry_at NULLS FIRST, created_at)
    WHERE status IN ('PENDING', 'FAILED');

-- Separate index for stale PROCESSING detection
CREATE INDEX idx_metadata_outbox_stale_processing ON metadata_outbox (processing_started_at)
    WHERE status = 'PROCESSING';

-- Unique constraint to prevent duplicate pending entries for same entity
-- NOTE: Excludes FAILED status intentionally - if a transaction is replayed from queue while
-- a FAILED entry exists, we allow a new PENDING entry. The new entry takes precedence and
-- the old FAILED entry will be cleaned up after retention period. This is expected behavior.
CREATE UNIQUE INDEX idx_metadata_outbox_entity_pending
    ON metadata_outbox (entity_id, entity_type)
    WHERE status IN ('PENDING', 'PROCESSING');

-- Index for finding entries by entity (for idempotency checks)
CREATE INDEX idx_metadata_outbox_entity ON metadata_outbox (entity_id, entity_type);

-- Index for cleanup of old processed entries
CREATE INDEX idx_metadata_outbox_processed ON metadata_outbox (processed_at)
    WHERE status = 'PUBLISHED';

-- Index for DLQ cleanup
CREATE INDEX idx_metadata_outbox_dlq ON metadata_outbox (updated_at)
    WHERE status = 'DLQ';

-- Access control: restrict to transaction service role only
REVOKE ALL ON metadata_outbox FROM PUBLIC;
-- Note: GRANT should be added in deployment scripts for specific role

COMMENT ON TABLE metadata_outbox IS 'Outbox pattern table for reliable MongoDB metadata creation';
COMMENT ON COLUMN metadata_outbox.status IS 'PENDING=new, PROCESSING=claimed by worker, PUBLISHED=success, FAILED=retriable error, DLQ=permanent failure';
COMMENT ON COLUMN metadata_outbox.processing_started_at IS 'Set when worker claims entry - used to detect stale PROCESSING entries from crashed workers';
COMMENT ON COLUMN metadata_outbox.last_error IS 'Sanitized error message - must NOT contain PII or sensitive data';
COMMENT ON COLUMN metadata_outbox.max_retries IS 'Maximum retry attempts before DLQ. Application MUST call MarkDLQ when retry_count >= max_retries';

COMMIT;
