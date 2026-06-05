-- ============================================
-- Migration: 000018_add_reserved_usage_column (DOWN)
-- Description: Remove reserved_usage column from usage_counters.
-- Date: 2026-06-05
-- ============================================

ALTER TABLE usage_counters DROP COLUMN IF EXISTS reserved_usage;
