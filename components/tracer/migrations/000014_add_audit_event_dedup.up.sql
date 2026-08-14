-- ============================================
-- Migration: 000014_add_audit_event_dedup
-- Description: Add partial unique index for audit event deduplication
-- Date: 2026-03-24
-- ============================================

-- =============================================================================
-- AUDIT EVENTS: Transaction Validation Deduplication
-- =============================================================================
-- This partial unique index prevents duplicate audit events for the same
-- transaction validation. Only the first audit event for a given
-- (resource_id, event_type) combination where resource_type = 'transaction'
-- will be stored.
--
-- Deduplication approach: The repository uses INSERT...SELECT...WHERE NOT EXISTS
-- instead of ON CONFLICT DO NOTHING because audit_events has PostgreSQL RULEs
-- (prevent_audit_event_update, prevent_audit_event_delete) which prevent ON CONFLICT.
-- =============================================================================

CREATE UNIQUE INDEX IF NOT EXISTS idx_audit_events_validation_dedup
    ON audit_events (resource_id, event_type)
    WHERE resource_type = 'transaction';
