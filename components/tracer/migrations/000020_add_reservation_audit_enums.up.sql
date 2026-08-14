-- ============================================
-- Migration: 000020_add_reservation_audit_enums
-- Description: Extend the audit enums for the two-phase reservation seam.
--              The reservation lifecycle (reserve / confirm / release / expire /
--              skip) writes audit rows whose event_type, action, and resource_type
--              are defined Go-side in pkg/model/audit_event.go, but the PG enums
--              backing audit_events did not yet carry these values. Without them
--              every reservation audit insert — including the reaper's batch-summary
--              EXPIRED row — fails with "invalid input value for enum".
-- Date: 2026-06-05
-- ============================================
-- Note: ALTER TYPE ... ADD VALUE must be the only kind of statement here (no column
-- changes), mirroring 000006/000009. IF NOT EXISTS keeps the migration idempotent;
-- golang-migrate runs each file as its own transaction and PostgreSQL 12+ permits
-- adding (not using) enum values within that transaction.

-- audit_event_type_enum: the five reservation transition event types.
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVATION_RESERVED';
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVATION_CONFIRMED';
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVATION_RELEASED';
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVATION_EXPIRED';
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVATION_SKIPPED';

-- audit_action_enum: the reservation actions (model.AuditAction* constants).
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'RESERVE';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'CONFIRM';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'RELEASE';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'EXPIRE';
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'SKIP';

-- resource_type_enum: reservations are an audited resource type.
ALTER TYPE resource_type_enum ADD VALUE IF NOT EXISTS 'reservation';
