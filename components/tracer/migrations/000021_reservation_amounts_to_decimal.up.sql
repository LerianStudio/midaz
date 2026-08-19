-- ============================================
-- Migration: 000021_reservation_amounts_to_decimal
-- Description: Convert the reservation-seam monetary columns from BIGINT to
--              DECIMAL, matching current_usage / max_amount which became DECIMAL
--              in 000005. reserved_usage (added in 000018) and
--              usage_reservations.amount (added in 000019) were introduced as
--              BIGINT AFTER the cents->decimal conversion, so they hold a whole
--              currency UNIT (the reservation path stores the result of IntPart),
--              NOT cents. The conversion is a DIRECT cast (::decimal) with NO
--              divide-by-100 — a /100 here would corrupt every reserved value.
-- Date: 2026-08-18
-- ============================================
--
-- Idempotency (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md): each
-- ALTER ... TYPE is guarded on the column still being 'bigint', so a replay on an
-- already-converted database is a clean no-op. The CHECK (>= 0) constraints
-- survive ALTER TYPE untouched; reserved_usage's DEFAULT 0 is re-emitted after
-- the type change to keep it byte-visible in this file.

-- usage_counters.reserved_usage
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'usage_counters'
          AND column_name  = 'reserved_usage') = 'bigint' THEN
        -- ACKNOWLEDGE: money-path BIGINT->DECIMAL widening on the reservation seam; direct cast, no cents division; expand-contract is not viable (the app must persist decimals the moment this runs); guarded on data_type='bigint' for idempotent replay.
        ALTER TABLE usage_counters ALTER COLUMN reserved_usage TYPE decimal USING reserved_usage::decimal;
        ALTER TABLE usage_counters ALTER COLUMN reserved_usage SET DEFAULT 0;
    END IF;
END $$;

-- usage_reservations.amount
DO $$
BEGIN
    IF (SELECT data_type FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name   = 'usage_reservations'
          AND column_name  = 'amount') = 'bigint' THEN
        -- ACKNOWLEDGE: money-path BIGINT->DECIMAL widening on the reservation seam; direct cast, no cents division; expand-contract is not viable (the app must persist decimals the moment this runs); guarded on data_type='bigint' for idempotent replay.
        ALTER TABLE usage_reservations ALTER COLUMN amount TYPE decimal USING amount::decimal;
    END IF;
END $$;
