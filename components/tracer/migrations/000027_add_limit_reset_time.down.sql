-- ============================================
-- Migration: 000027_add_limit_reset_time (DOWN)
-- Description: Remove the reset_time column and its constraints from limits.
--              Limits that had a reset_time fall back to midnight-UTC periods.
-- Date: 2026-10-02
-- ============================================

ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_reset_time_period_type;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_reset_time_format;

ALTER TABLE limits DROP COLUMN IF EXISTS reset_time;
