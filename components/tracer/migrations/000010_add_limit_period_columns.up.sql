-- ============================================
-- Migration: 000010_add_limit_period_columns
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

-- Add CHECK constraints with idempotency guards.
--
-- PostgreSQL has no ADD CONSTRAINT IF NOT EXISTS, so each ADD CONSTRAINT is
-- gated by a pg_constraint lookup. This is required by the Migration Renumbering
-- Invariant (docs/tracer/INVARIANTS.md) so the file can replay safely on a database
-- where the constraints already exist (origin/develop → HEAD upgrade path).
--
-- Note: we do NOT re-introduce NOT VALID on chk_limits_custom_dates_required.
-- NOT VALID is unnecessary here because the CUSTOM enum value does not exist in
-- pre-renumber clusters and no CUSTOM rows can be present; the guard below is a
-- different mechanism (skip re-add when already present), not a validation-skip.

-- Add constraint: time window must have both or neither
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_time_window_pair'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_time_window_pair
            CHECK (
                (active_time_start IS NULL AND active_time_end IS NULL) OR
                (active_time_start IS NOT NULL AND active_time_end IS NOT NULL)
            );
    END IF;
END $$;

-- Add constraint: CUSTOM type requires custom dates
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_custom_dates_required'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_required
            CHECK (
                limit_type != 'CUSTOM' OR
                (custom_start_date IS NOT NULL AND custom_end_date IS NOT NULL)
            );
    END IF;
END $$;

-- Add constraint: non-CUSTOM types must not have custom dates
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_custom_dates_forbidden'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_forbidden
            CHECK (
                limit_type = 'CUSTOM' OR
                (custom_start_date IS NULL AND custom_end_date IS NULL)
            );
    END IF;
END $$;

-- Add constraint: custom_start_date must be before custom_end_date
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_custom_dates_order'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_custom_dates_order
            CHECK (
                custom_start_date IS NULL OR custom_end_date IS NULL OR
                custom_start_date < custom_end_date
            );
    END IF;
END $$;

-- Add constraint: time window format validation (HH:MM pattern)
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_time_format_start'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_time_format_start
            CHECK (
                active_time_start IS NULL OR
                active_time_start ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_time_format_end'
          AND conrelid = 'public.limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_time_format_end
            CHECK (
                active_time_end IS NULL OR
                active_time_end ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            );
    END IF;
END $$;

-- Index for custom period queries (finding limits active during a date range)
CREATE INDEX IF NOT EXISTS idx_limits_custom_period 
    ON limits(custom_start_date, custom_end_date) 
    WHERE limit_type = 'CUSTOM' AND status = 'ACTIVE';
