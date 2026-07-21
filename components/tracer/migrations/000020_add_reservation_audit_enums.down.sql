-- ============================================
-- Migration: 000020_add_reservation_audit_enums (DOWN)
-- Description: Note about enum value removal.
-- Date: 2026-06-05
-- ============================================
-- Note: PostgreSQL does not support removing enum values directly. Removing the
-- reservation values would require recreating each enum without them, rewriting
-- every table column that uses the type, dropping the old enum, and renaming the
-- new one.
--
-- This is intentionally a no-op, mirroring 000009, because:
-- - Removing enum values is a breaking change.
-- - Any existing audit_events row carrying a reservation event_type / action /
--   resource_type would become invalid and the rewrite would risk data loss.
--
-- If rollback is truly needed, manual intervention is required.
DO $$ BEGIN
  RAISE NOTICE 'Reservation enum values cannot be automatically removed from audit_event_type_enum / audit_action_enum / resource_type_enum';
END $$;
