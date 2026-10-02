-- ============================================
-- Migration: 000027_add_limit_reset_time
-- Description: Add the optional reset_time column to limits. A DAILY, WEEKLY
--              or MONTHLY limit with a reset_time starts each period at that
--              UTC time of day instead of midnight UTC. NULL means midnight,
--              so existing rows keep their periods and no backfill runs.
--              Format: "HH:MM" stored as VARCHAR(5), like active_time_start.
-- Date: 2026-10-02
-- ============================================
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md): the
-- column uses ADD COLUMN IF NOT EXISTS and each CHECK is guarded by a
-- pg_constraint lookup. The lookup resolves 'limits' through search_path, so a
-- schema-isolated tenant gets the constraints on its own table.
--
-- Lock wait: the column and both CHECKs are added in one DO block with
-- lock_timeout = 5s, set transaction-locally, so the ALTERs fail fast with
-- SQLSTATE 55P03 instead of queueing traffic behind them. The CHECKs scan
-- limits, where every row holds a NULL reset_time.

DO $$
BEGIN
    PERFORM set_config('lock_timeout', '5s', true);

    ALTER TABLE limits ADD COLUMN IF NOT EXISTS reset_time VARCHAR(5);

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_reset_time_format'
          AND conrelid = 'limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_reset_time_format
            CHECK (
                reset_time IS NULL OR
                reset_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'
            );
    END IF;

    -- Only calendar periods have a boundary to move.
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'chk_limits_reset_time_period_type'
          AND conrelid = 'limits'::regclass
    ) THEN
        ALTER TABLE limits ADD CONSTRAINT chk_limits_reset_time_period_type
            CHECK (
                reset_time IS NULL OR
                limit_type IN ('DAILY', 'WEEKLY', 'MONTHLY')
            );
    END IF;
END $$;
