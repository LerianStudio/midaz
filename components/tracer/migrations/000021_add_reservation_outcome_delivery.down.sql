-- ============================================
-- Migration: 000021_add_reservation_outcome_delivery (DOWN)
-- Description: Refuse rollback while V2 state remains, then restore V1 schema.
-- Date: 2026-08-18
-- ============================================

DO $$
DECLARE
    has_v2_reservations BOOLEAN := FALSE;
    has_outcome_receipts BOOLEAN := FALSE;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'usage_reservations'
          AND column_name = 'delivery_mode'
    ) THEN
        EXECUTE 'SELECT EXISTS (
            SELECT 1 FROM usage_reservations
            WHERE delivery_mode = ''LEDGER_OUTCOME_V2''
        )' INTO has_v2_reservations;
    END IF;

    IF has_v2_reservations THEN
        RAISE EXCEPTION 'cannot roll back reservation outcome delivery while V2 reservations exist';
    END IF;

    IF to_regclass('reservation_outcome_receipts') IS NOT NULL THEN
        EXECUTE 'SELECT EXISTS (
            SELECT 1 FROM reservation_outcome_receipts
        )' INTO has_outcome_receipts;
    END IF;

    IF has_outcome_receipts THEN
        RAISE EXCEPTION 'cannot roll back reservation outcome delivery while outcome receipts exist';
    END IF;
END $$;

DROP TABLE IF EXISTS reservation_outcome_receipts;

DROP INDEX IF EXISTS idx_usage_reservations_v2_outstanding;
DROP INDEX IF EXISTS idx_usage_reservations_reserved_counter;
DROP INDEX IF EXISTS idx_usage_reservations_reaper;
ALTER TABLE usage_reservations
    DROP CONSTRAINT IF EXISTS usage_reservations_delivery_mode_check;
ALTER TABLE usage_reservations
    DROP COLUMN IF EXISTS delivery_mode;

CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
    ON usage_reservations(reservation_expires_at)
    WHERE status = 'RESERVED';
