-- ============================================
-- Migration: 000021_add_reservation_outcome_delivery
-- Description: Durable ledger-owned outcome delivery for usage reservations.
-- Date: 2026-08-18
-- ============================================

ALTER TABLE usage_reservations
    ADD COLUMN IF NOT EXISTS delivery_mode VARCHAR(32) NOT NULL DEFAULT 'LEGACY';

-- V2 has no autonomous expiry. NULL is intentionally chosen over a far-future
-- timestamp because old binaries use a fixed `reservation_expires_at < now`
-- predicate: NULL makes V2 rows structurally unreachable to that reaper.
ALTER TABLE usage_reservations
    ALTER COLUMN reservation_expires_at DROP NOT NULL;

UPDATE usage_reservations
SET reservation_expires_at = NULL
WHERE delivery_mode = 'LEDGER_OUTCOME_V2'
  AND reservation_expires_at IS NOT NULL;

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

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'usage_reservations_delivery_expiry_check'
          AND conrelid = 'usage_reservations'::regclass
    ) THEN
        ALTER TABLE usage_reservations
            ADD CONSTRAINT usage_reservations_delivery_expiry_check
            CHECK (
                (delivery_mode = 'LEGACY' AND reservation_expires_at IS NOT NULL)
                OR
                (delivery_mode = 'LEDGER_OUTCOME_V2' AND reservation_expires_at IS NULL)
            );
    END IF;
END $$;

DROP INDEX IF EXISTS idx_usage_reservations_reaper;
CREATE INDEX IF NOT EXISTS idx_usage_reservations_reaper
    ON usage_reservations(reservation_expires_at)
    WHERE status = 'RESERVED' AND delivery_mode = 'LEGACY';

CREATE INDEX IF NOT EXISTS idx_usage_reservations_reserved_counter
    ON usage_reservations(limit_id, scope_key, period_key)
    WHERE status = 'RESERVED';

CREATE INDEX IF NOT EXISTS idx_usage_reservations_v2_outstanding
    ON usage_reservations(created_at)
    WHERE status = 'RESERVED' AND delivery_mode = 'LEDGER_OUTCOME_V2';

CREATE TABLE IF NOT EXISTS reservation_outcome_receipts (
    transaction_id UUID PRIMARY KEY,
    outcome_id UUID NOT NULL,
    outcome VARCHAR(16) NOT NULL CHECK (outcome IN ('COMMITTED', 'ABORTED')),
    reservation_count INTEGER NOT NULL DEFAULT 0 CHECK (reservation_count >= 0),
    applied_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- An old reaper updates the counter first and the reservation second inside one
-- transaction. Rejecting the second statement rolls the counter move back. The
-- only authorized V2 transition is one backed by the receipt inserted by the
-- new ApplyOutcome transaction.
CREATE OR REPLACE FUNCTION protect_live_v2_reservation_transition()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    accepted_outcome VARCHAR(16);
BEGIN
    IF OLD.delivery_mode = 'LEDGER_OUTCOME_V2'
       AND OLD.status = 'RESERVED'
       AND TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'live V2 reservation cannot be deleted before its durable outcome'
            USING ERRCODE = 'check_violation';
    END IF;

    IF OLD.delivery_mode = 'LEDGER_OUTCOME_V2'
       AND TG_OP = 'UPDATE'
       AND (
           NEW.id IS DISTINCT FROM OLD.id
           OR NEW.limit_id IS DISTINCT FROM OLD.limit_id
           OR NEW.scope_key IS DISTINCT FROM OLD.scope_key
           OR NEW.period_key IS DISTINCT FROM OLD.period_key
           OR NEW.amount IS DISTINCT FROM OLD.amount
           OR NEW.transaction_id IS DISTINCT FROM OLD.transaction_id
           OR NEW.reservation_expires_at IS DISTINCT FROM OLD.reservation_expires_at
           OR NEW.created_at IS DISTINCT FROM OLD.created_at
           OR NEW.delivery_mode IS DISTINCT FROM OLD.delivery_mode
       ) THEN
        RAISE EXCEPTION 'V2 reservation identity and protocol are immutable'
            USING ERRCODE = 'check_violation';
    END IF;

    IF OLD.delivery_mode = 'LEDGER_OUTCOME_V2'
       AND OLD.status = 'RESERVED'
       AND TG_OP = 'UPDATE'
       AND NEW.status <> OLD.status THEN
        SELECT outcome
        INTO accepted_outcome
        FROM reservation_outcome_receipts
        WHERE transaction_id = OLD.transaction_id;

        IF accepted_outcome IS NULL
           OR (accepted_outcome = 'COMMITTED' AND NEW.status <> 'CONFIRMED')
           OR (accepted_outcome = 'ABORTED' AND NEW.status <> 'RELEASED') THEN
            RAISE EXCEPTION 'live V2 reservation requires a matching durable outcome receipt'
                USING ERRCODE = 'check_violation';
        END IF;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;

    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS trg_protect_live_v2_reservation_transition ON usage_reservations;
CREATE TRIGGER trg_protect_live_v2_reservation_transition
BEFORE UPDATE OR DELETE ON usage_reservations
FOR EACH ROW
EXECUTE FUNCTION protect_live_v2_reservation_transition();

-- Pre-V2 cleanup binaries delete counters solely by expires_at and ignore live
-- reserved_usage. Silently skip only counters that still back a live V2 hold;
-- unrelated cleanup remains available during a rolling deployment.
CREATE OR REPLACE FUNCTION protect_live_v2_reservation_counter()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM usage_reservations
        WHERE limit_id = OLD.limit_id
          AND scope_key = OLD.scope_key
          AND period_key = OLD.period_key
          AND status = 'RESERVED'
          AND delivery_mode = 'LEDGER_OUTCOME_V2'
    ) THEN
        RETURN NULL;
    END IF;

    RETURN OLD;
END;
$$;

DROP TRIGGER IF EXISTS trg_protect_live_v2_reservation_counter ON usage_counters;
CREATE TRIGGER trg_protect_live_v2_reservation_counter
BEFORE DELETE ON usage_counters
FOR EACH ROW
EXECUTE FUNCTION protect_live_v2_reservation_counter();
