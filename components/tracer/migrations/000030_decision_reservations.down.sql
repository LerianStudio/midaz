-- Never erase ownership or merge new-profile identities into the legacy tuple.
-- Reverting a binary also requires the matching legacy writer; suspend traffic.
DO $migration$
BEGIN
    LOCK TABLE usage_reservations, reserve_decisions IN ACCESS EXCLUSIVE MODE NOWAIT;
    IF EXISTS (SELECT 1 FROM usage_reservations WHERE decision_id IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot remove decision-owned reservation history' USING ERRCODE = '23514';
    END IF;
    DROP TRIGGER decision_reservation_has_owner ON usage_reservations;
    DROP FUNCTION check_decision_reservation_owner();
    DROP TRIGGER durable_decision_reservations_truncate ON usage_reservations;
    DROP FUNCTION reject_decision_reservation_truncation();
    DROP TRIGGER durable_decision_reservations ON usage_reservations;
    DROP FUNCTION guard_decision_reservation_mutation();
    DROP INDEX idx_usage_reservations_decision;
    DROP INDEX idx_usage_reservations_request;
    DROP INDEX idx_usage_reservations_reaper;
    ALTER TABLE usage_reservations DROP CONSTRAINT reservation_decision_owner_fk;
    ALTER TABLE usage_reservations DROP CONSTRAINT decision_reservation_no_expiry;
    ALTER TABLE usage_reservations DROP COLUMN decision_id;
    ALTER TABLE reserve_decisions DROP CONSTRAINT reserve_decision_capacity_owner;
    CREATE UNIQUE INDEX idx_usage_reservations_request
        ON usage_reservations(transaction_id, limit_id, scope_key, period_key);
    CREATE INDEX idx_usage_reservations_reaper ON usage_reservations(reservation_expires_at)
        WHERE status = 'RESERVED';
END;
$migration$;
