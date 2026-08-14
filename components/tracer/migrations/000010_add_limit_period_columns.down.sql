-- ============================================
-- Migration: 000010_add_limit_period_columns (DOWN)
-- Description: Remove time window and custom period columns from limits table
-- Date: 2026-03-10
-- ============================================

-- Drop index first
DROP INDEX IF EXISTS idx_limits_custom_period;

-- Drop constraints
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_time_format_end;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_time_format_start;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_custom_dates_order;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_custom_dates_forbidden;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_custom_dates_required;
ALTER TABLE limits DROP CONSTRAINT IF EXISTS chk_limits_time_window_pair;

-- Drop columns
ALTER TABLE limits DROP COLUMN IF EXISTS custom_end_date;
ALTER TABLE limits DROP COLUMN IF EXISTS custom_start_date;
ALTER TABLE limits DROP COLUMN IF EXISTS active_time_end;
ALTER TABLE limits DROP COLUMN IF EXISTS active_time_start;
