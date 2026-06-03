-- Migration 000014 rollback: Remove audit event deduplication index

DROP INDEX IF EXISTS idx_audit_events_validation_dedup;
