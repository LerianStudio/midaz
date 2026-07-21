-- ============================================
-- Migration: 000013_add_name_unique_constraints (DOWN)
-- Description: Remove name uniqueness constraints and context_id column
-- Date: 2026-03-23
-- ============================================

-- Remove unique indexes
DROP INDEX IF EXISTS idx_rules_name_per_context_active;
DROP INDEX IF EXISTS idx_limits_name_active;

-- Remove context_id column from rules
ALTER TABLE rules DROP COLUMN IF EXISTS context_id;
