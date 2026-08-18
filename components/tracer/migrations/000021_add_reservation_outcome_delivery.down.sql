-- ============================================
-- Migration: 000021_add_reservation_outcome_delivery (DOWN)
-- Description: Refuse rollback while V2 state remains, then restore V1 schema.
-- Date: 2026-08-18
-- ============================================

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM usage_reservations
        WHERE delivery_mode = 'LEDGER_OUTCOME_V2'
    ) THEN
        RAISE EXCEPTION 'cannot roll back reservation outcome delivery while V2 reservations exist';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM reservation_outcome_receipts
    ) THEN
        RAISE EXCEPTION 'cannot roll back reservation outcome delivery while outcome receipts exist';
    END IF;
END $$;

DROP TABLE IF EXISTS reservation_outcome_receipts;

DROP INDEX IF EXISTS idx_usage_reservations_reaper;
ALTER TABLE usage_reservations
    DROP CONSTRAINT IF EXISTS usage_reservations_delivery_mode_check;
ALTER TABLE usage_reservations
    DROP COLUMN IF EXISTS delivery_mode;

CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
    ON usage_reservations(reservation_expires_at)
    WHERE status = 'RESERVED';
