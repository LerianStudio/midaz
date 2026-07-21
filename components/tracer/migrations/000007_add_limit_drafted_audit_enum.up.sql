-- Migration: Add LIMIT_DRAFTED to audit event type enum
--
-- Supports the new POST /v1/limits/{id}/draft endpoint (INACTIVE → DRAFT transition).
-- Adds:
--   - 'LIMIT_DRAFTED' to audit_event_type_enum (after LIMIT_DEACTIVATED)
--
-- Note: 'DRAFT' action enum value already exists (added in migration 000003).

-- Add LIMIT_DRAFTED to audit event type enum
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'LIMIT_DRAFTED' AFTER 'LIMIT_DEACTIVATED';

