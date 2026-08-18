-- ============================================
-- Migration: 000021_add_reservation_outcome_delivery
-- Description: Durable ledger-owned outcome delivery for usage reservations.
-- Date: 2026-08-18
-- ============================================

ALTER TABLE usage_reservations
    ADD COLUMN IF NOT EXISTS delivery_mode VARCHAR(32) NOT NULL DEFAULT 'LEGACY';

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_reservations_delivery_mode_check'
          AND conrelid = 'usage_reservations'::regclass
    ) THEN
        ALTER TABLE usage_reservations
            ADD CONSTRAINT usage_reservations_delivery_mode_check
            CHECK (delivery_mode IN ('LEGACY', 'LEDGER_OUTCOME_V2'));
    END IF;
END $$;

DROP INDEX IF EXISTS idx_usage_reservations_reaper;
CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
    ON usage_reservations(reservation_expires_at)
    WHERE status = 'RESERVED' AND delivery_mode = 'LEGACY';

CREATE TABLE IF NOT EXISTS reservation_outcome_receipts (
    transaction_id UUID PRIMARY KEY,
    outcome_id UUID NOT NULL,
    outcome VARCHAR(16) NOT NULL CHECK (outcome IN ('COMMITTED', 'ABORTED')),
    reservation_count INTEGER NOT NULL DEFAULT 0 CHECK (reservation_count >= 0),
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL
);
