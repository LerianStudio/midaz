-- ============================================
-- Migration: 000009_add_limit_type_enum_values (DOWN)
-- Description: Note about enum value removal
-- Date: 2026-03-10
-- ============================================
-- Note: PostgreSQL does not support removing enum values directly.
-- Removing WEEKLY and CUSTOM would require:
-- 1. Creating a new enum without these values
-- 2. Updating all tables using the enum
-- 3. Dropping the old enum
-- 4. Renaming the new enum
--
-- This is intentionally left as a no-op because:
-- - Removing enum values is a breaking change
-- - Any existing limits using WEEKLY/CUSTOM would become invalid
-- - The risk of data loss outweighs the benefit of a clean rollback
--
-- If rollback is truly needed, manual intervention is required.
DO $$ BEGIN
  RAISE NOTICE 'Enum values WEEKLY and CUSTOM cannot be automatically removed from limit_type_enum';
END $$;
