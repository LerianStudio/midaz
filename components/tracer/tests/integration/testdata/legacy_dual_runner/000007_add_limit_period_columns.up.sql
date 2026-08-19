-- ============================================
-- Migration: 000007_add_limit_period_columns
-- Description: Add time window and custom period columns to limits table
-- Date: 2026-03-10
-- ============================================

-- Add time window columns for daily time-of-day restrictions
-- Format: "HH:MM" stored as VARCHAR(5) to preserve the exact format
ALTER TABLE limits ADD COLUMN IF NOT EXISTS active_time_start VARCHAR(5);
ALTER TABLE limits ADD COLUMN IF NOT EXISTS active_time_end VARCHAR(5);

-- Add custom period columns for CUSTOM limit type
ALTER TABLE limits ADD COLUMN IF NOT EXISTS custom_start_date TIMESTAMP WITH TIME ZONE;
ALTER TABLE limits ADD COLUMN IF NOT EXISTS custom_end_date TIMESTAMP WITH TIME ZONE;

-- Add constraint: time window must have both or neither
ALTER TABLE limits ADD CONSTRAINT chk_limits_time_window_pair
    CHECK (
        (active_time_start IS NULL AND active_time_end IS NULL) OR
        (active_time_start IS NOT NULL AND active_time_end IS NOT NULL)
    );

-- Add constraint: CUSTOM type requires custom dates
ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_required
    CHECK (
        limit_type != 'CUSTOM' OR 
        (custom_start_date IS NOT NULL AND custom_end_date IS NOT NULL)
    );

-- Add constraint: non-CUSTOM types must not have custom dates
ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_forbidden
    CHECK (
        limit_type = 'CUSTOM' OR 
        (custom_start_date IS NULL AND custom_end_date IS NULL)
    );

-- Add constraint: custom_start_date must be before custom_end_date
ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_order
    CHECK (
        custom_start_date IS NULL OR custom_end_date IS NULL OR
        custom_start_date < custom_end_date
    );

-- Add constraint: time window format validation (HH:MM pattern)
ALTER TABLE limits ADD CONSTRAINT chk_limits_time_format_start
    CHECK (
        active_time_start IS NULL OR
        active_time_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
    );

ALTER TABLE limits ADD CONSTRAINT chk_limits_time_format_end
    CHECK (
        active_time_end IS NULL OR
        active_time_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
    );

-- Index for custom period queries (finding limits active during a date range)
CREATE INDEX IF NOT EXISTS idx_limits_custom_period 
    ON limits(custom_start_date, custom_end_date) 
    WHERE limit_type = 'CUSTOM' AND status = 'ACTIVE';
