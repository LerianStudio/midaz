SET LOCAL lock_timeout = '5s';

ALTER TABLE tracer_reservation_obligation
    ADD COLUMN recovery_attempts INTEGER NOT NULL DEFAULT 0 CHECK (recovery_attempts >= 0),
    ADD COLUMN recovery_quarantined BOOLEAN NOT NULL DEFAULT FALSE;

CREATE INDEX tracer_reservation_obligation_priority_idx
    ON tracer_reservation_obligation
    ((state IN ('CONFIRMED','RELEASED')) DESC, next_attempt_at, organization_id, ledger_id, transaction_id)
    WHERE delivered_at IS NULL AND NOT recovery_quarantined;
