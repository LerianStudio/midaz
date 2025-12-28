-- WARNING: This will permanently delete all pending outbox entries.
-- Ensure all entries are processed (status = 'PUBLISHED' or 'DLQ') before rollback.
-- Check: SELECT COUNT(*) FROM metadata_outbox WHERE status NOT IN ('PUBLISHED', 'DLQ');
DROP TABLE IF EXISTS metadata_outbox;
