-- ============================================
-- Rollback: 000021_reservation_amounts_to_decimal
-- Description: Revert the reservation-seam monetary columns DECIMAL -> BIGINT.
--              This direction is LOSSY: it ABORTS (RAISE EXCEPTION) if any
--              persisted reserved_usage / amount carries a fractional part,
--              rather than silently truncating via ROUND. Only when every value
--              is already integral does it cast back to BIGINT. Failing loud is
--              the correct SOX/GLBA posture for money data.
-- ============================================

-- Abort guard: refuse the downgrade if any fractional reservation value exists.
-- Reverting a fractional value to BIGINT would silently lose the fraction, so we
-- fail the migration instead of corrupting the record.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM usage_counters WHERE reserved_usage <> floor(reserved_usage))
       OR EXISTS (SELECT 1 FROM usage_reservations WHERE amount <> floor(amount)) THEN
        RAISE EXCEPTION 'cannot downgrade: fractional reservation values present';
    END IF;
END $$;

-- Conversion: the guard above guarantees every value is integral, so a DIRECT
-- cast back to BIGINT is lossless (no ROUND). Guarded on data_type='numeric' so
-- a replay on an already-reverted database is a clean no-op.
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'usage_counters'
          AND column_name  = 'reserved_usage') = 'numeric' THEN
        -- ACKNOWLEDGE: rollback of the money-path widening; the abort guard above guarantees every value is integral, so this direct DECIMAL->BIGINT cast is lossless; guarded on data_type='numeric' for idempotent replay.
        ALTER TABLE usage_counters ALTER COLUMN reserved_usage TYPE bigint USING reserved_usage::bigint;
        ALTER TABLE usage_counters ALTER COLUMN reserved_usage SET DEFAULT 0;
    END IF;
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'usage_reservations'
          AND column_name  = 'amount') = 'numeric' THEN
        -- ACKNOWLEDGE: rollback of the money-path widening; the abort guard above guarantees every value is integral, so this direct DECIMAL->BIGINT cast is lossless; guarded on data_type='numeric' for idempotent replay.
        ALTER TABLE usage_reservations ALTER COLUMN amount TYPE bigint USING amount::bigint;
    END IF;
END $$;
