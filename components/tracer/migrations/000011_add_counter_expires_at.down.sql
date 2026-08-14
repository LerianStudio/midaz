-- ============================================
-- Migration: 000011_add_counter_expires_at (DOWN)
-- Description: Remove expires_at column from usage_counters
-- Date: 2026-03-10
-- ============================================

-- Drop index first
DROP INDEX IF EXISTS idx_usage_counters_expires_at;

-- Drop column
ALTER TABLE usage_counters DROP COLUMN IF EXISTS expires_at;
