-- ============================================
-- Migration: 000022_add_reservation_outcome_delivery (DOWN)
-- Description: Refuse rollback while V2 state remains, then restore V1 schema.
-- Date: 2026-08-18
-- ============================================

DO $$
DECLARE
    has_v2_reservations BOOLEAN := FALSE;
    has_outcome_receipts BOOLEAN := FALSE;
BEGIN
    -- Serialize the guard and destructive DDL with every reservation/outcome
    -- writer. Without these locks a V2 row could commit after the checks and be
    -- silently erased by the rollback in the same migration.
    IF to_regclass('reservation_outcome_receipts') IS NOT NULL THEN
        LOCK TABLE reservation_outcome_receipts IN ACCESS EXCLUSIVE MODE;
    END IF;

    IF to_regclass('usage_reservations') IS NOT NULL THEN
        LOCK TABLE usage_reservations IN ACCESS EXCLUSIVE MODE;
    END IF;

    IF to_regclass('usage_counters') IS NOT NULL THEN
        LOCK TABLE usage_counters IN ACCESS EXCLUSIVE MODE;
    END IF;

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

    DROP TRIGGER IF EXISTS trg_protect_live_v2_reservation_counter ON usage_counters;
    DROP TRIGGER IF EXISTS trg_protect_live_v2_reservation_transition ON usage_reservations;
    DROP FUNCTION IF EXISTS protect_live_v2_reservation_counter();
    DROP FUNCTION IF EXISTS protect_live_v2_reservation_transition();

    DROP TABLE IF EXISTS reservation_outcome_receipts;

    DROP INDEX IF EXISTS idx_usage_reservations_v2_outstanding;
    DROP INDEX IF EXISTS idx_usage_reservations_reserved_counter;
    DROP INDEX IF EXISTS idx_usage_reservations_reaper;

    IF to_regclass('usage_reservations') IS NOT NULL THEN
        ALTER TABLE usage_reservations
            DROP CONSTRAINT IF EXISTS usage_reservations_delivery_expiry_check;
        ALTER TABLE usage_reservations
            DROP CONSTRAINT IF EXISTS usage_reservations_delivery_mode_check;
        ALTER TABLE usage_reservations
            ALTER COLUMN reservation_expires_at SET NOT NULL;
        ALTER TABLE usage_reservations
            DROP COLUMN IF EXISTS delivery_mode;

        CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
            ON usage_reservations(reservation_expires_at)
            WHERE status = 'RESERVED';
    END IF;
END $$;
