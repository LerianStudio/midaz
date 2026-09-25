-- Deploy with the writer that uses the legacy partial-index predicate. Old
-- binaries cannot infer ON CONFLICT from that index: coordinated rollout only.
DO $migration$
BEGIN
    -- Fail rather than queue an exclusive lock behind live application traffic.
    -- The coordinated maintenance window is still mandatory.
    SET LOCAL lock_timeout = '5s';
    LOCK TABLE usage_reservations IN ACCESS EXCLUSIVE MODE;
    ALTER TABLE reserve_decisions ADD CONSTRAINT reserve_decision_capacity_owner
        UNIQUE (evaluation_id, transaction_id);
    ALTER TABLE usage_reservations ADD COLUMN decision_id UUID;
    -- Capacity is provisional until the enclosing transaction stores its final
    -- ALLOW decision. A deferred composite FK permits that order, but never an
    -- orphan or a decision for a different producer transaction at commit.
    ALTER TABLE usage_reservations ADD CONSTRAINT reservation_decision_owner_fk
        FOREIGN KEY (decision_id, transaction_id)
        REFERENCES reserve_decisions(evaluation_id, transaction_id)
        DEFERRABLE INITIALLY DEFERRED;
    ALTER TABLE usage_reservations ADD CONSTRAINT decision_reservation_no_expiry
        CHECK (decision_id IS NULL OR (status <> 'EXPIRED' AND amount > 0));

    DROP INDEX idx_usage_reservations_request;
    CREATE UNIQUE INDEX idx_usage_reservations_request
        ON usage_reservations(transaction_id, limit_id, scope_key, period_key)
        WHERE decision_id IS NULL;
    CREATE UNIQUE INDEX idx_usage_reservations_decision
        ON usage_reservations(decision_id, limit_id, scope_key, period_key)
        WHERE decision_id IS NOT NULL;
    DROP INDEX idx_usage_reservations_reaper;
    CREATE INDEX idx_usage_reservations_reaper ON usage_reservations(reservation_expires_at)
        WHERE status = 'RESERVED' AND decision_id IS NULL;

    CREATE FUNCTION guard_decision_reservation_mutation() RETURNS trigger AS $function$
    BEGIN
        IF TG_OP = 'DELETE' THEN
            IF OLD.decision_id IS NOT NULL THEN
                RAISE EXCEPTION 'decision reservation history cannot be removed' USING ERRCODE = '23514';
            END IF;
            RETURN OLD;
        END IF;
        IF NEW.decision_id IS DISTINCT FROM OLD.decision_id THEN
            RAISE EXCEPTION 'reservation ownership is immutable' USING ERRCODE = '23514';
        END IF;
        IF OLD.decision_id IS NOT NULL AND (
            (NEW.id, NEW.limit_id, NEW.scope_key, NEW.period_key, NEW.amount,
             NEW.transaction_id, NEW.reservation_expires_at, NEW.created_at)
            IS DISTINCT FROM
            (OLD.id, OLD.limit_id, OLD.scope_key, OLD.period_key, OLD.amount,
             OLD.transaction_id, OLD.reservation_expires_at, OLD.created_at)
            OR OLD.status <> 'RESERVED' OR NEW.status NOT IN ('CONFIRMED', 'RELEASED')
        ) THEN
            RAISE EXCEPTION 'decision reservation transition conflicts with stored state' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;
    CREATE TRIGGER durable_decision_reservations BEFORE UPDATE OR DELETE ON usage_reservations
        FOR EACH ROW EXECUTE FUNCTION guard_decision_reservation_mutation();

    CREATE FUNCTION check_decision_reservation_owner() RETURNS trigger AS $function$
    BEGIN
        IF NEW.decision_id IS NOT NULL AND NOT EXISTS (
            SELECT 1 FROM reserve_decisions d
            WHERE d.evaluation_id = NEW.decision_id AND d.transaction_id = NEW.transaction_id
              AND d.response->>'decision' = 'ALLOW'
              AND (d.response->'reservationIds') @> jsonb_build_array(NEW.id::text)
        ) THEN
            RAISE EXCEPTION 'reservation requires an ALLOW decision naming its handle' USING ERRCODE = '23514';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;
    CREATE CONSTRAINT TRIGGER decision_reservation_has_owner AFTER INSERT ON usage_reservations
        DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION check_decision_reservation_owner();

    CREATE FUNCTION reject_decision_reservation_truncation() RETURNS trigger AS $function$
    BEGIN
        IF EXISTS (SELECT 1 FROM usage_reservations WHERE decision_id IS NOT NULL) THEN
            RAISE EXCEPTION 'decision reservation history cannot be truncated' USING ERRCODE = '23514';
        END IF;
        RETURN NULL;
    END;
    $function$ LANGUAGE plpgsql;
    CREATE TRIGGER durable_decision_reservations_truncate BEFORE TRUNCATE ON usage_reservations
        FOR EACH STATEMENT EXECUTE FUNCTION reject_decision_reservation_truncation();
END;
$migration$;
