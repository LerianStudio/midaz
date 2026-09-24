-- One statement keeps backfill, FK and admission trigger atomic, including for
-- migration runners that execute individual statements outside an explicit tx.
DO $migration$
BEGIN
    LOCK TABLE reserve_decisions IN SHARE ROW EXCLUSIVE MODE;

    CREATE TABLE reserve_operations (
        integration_id TEXT NOT NULL CHECK (octet_length(integration_id) BETWEEN 1 AND 256),
        transaction_id UUID NOT NULL CHECK (transaction_id <> '00000000-0000-0000-0000-000000000000'),
        status TEXT NOT NULL CHECK (status IN ('OPEN', 'CONFIRMED', 'RELEASED')),
        completed_at TIMESTAMPTZ,
        PRIMARY KEY (integration_id, transaction_id),
        CHECK ((status = 'OPEN' AND completed_at IS NULL)
            OR (status IN ('CONFIRMED', 'RELEASED') AND completed_at IS NOT NULL))
    );

    -- Existing decisions provide no proof of accounting completion. Preserve
    -- them as OPEN; elapsed time or the decision itself cannot infer an outcome.
    INSERT INTO reserve_operations (integration_id, transaction_id, status)
        SELECT integration_id, transaction_id, 'OPEN' FROM reserve_decisions;

    ALTER TABLE reserve_decisions ADD CONSTRAINT reserve_decision_operation_fk
        FOREIGN KEY (integration_id, transaction_id)
        REFERENCES reserve_operations(integration_id, transaction_id);

    CREATE FUNCTION guard_reserve_operation_transition() RETURNS trigger AS $function$
    BEGIN
        IF NEW.integration_id IS DISTINCT FROM OLD.integration_id
            OR NEW.transaction_id IS DISTINCT FROM OLD.transaction_id
            OR OLD.status <> 'OPEN' OR NEW.status NOT IN ('CONFIRMED', 'RELEASED') THEN
            RAISE EXCEPTION 'reserve operation transition conflicts with recorded outcome'
                USING ERRCODE = '23514', CONSTRAINT = 'reserve_operation_transition';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;

    CREATE TRIGGER reserve_operation_forward_only
        BEFORE UPDATE ON reserve_operations FOR EACH ROW
        EXECUTE FUNCTION guard_reserve_operation_transition();

    CREATE FUNCTION reject_reserve_operation_removal() RETURNS trigger AS $function$
    BEGIN
        RAISE EXCEPTION 'reserve operation history cannot be removed' USING ERRCODE = '23514';
    END;
    $function$ LANGUAGE plpgsql;

    CREATE TRIGGER durable_reserve_operations
        BEFORE DELETE OR TRUNCATE ON reserve_operations FOR EACH STATEMENT
        EXECUTE FUNCTION reject_reserve_operation_removal();

    CREATE FUNCTION guard_reserve_decision_operation() RETURNS trigger AS $function$
    DECLARE
        operation_status TEXT;
    BEGIN
        INSERT INTO reserve_operations (integration_id, transaction_id, status)
            VALUES (NEW.integration_id, NEW.transaction_id, 'OPEN')
            ON CONFLICT (integration_id, transaction_id) DO NOTHING;

        SELECT status INTO STRICT operation_status FROM reserve_operations
            WHERE integration_id = NEW.integration_id AND transaction_id = NEW.transaction_id
            FOR UPDATE;

        IF operation_status <> 'OPEN' THEN
            RAISE EXCEPTION 'cannot evaluate a completed reserve operation'
                USING ERRCODE = '23514', CONSTRAINT = 'reserve_operation_closed';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;

    -- Defense in depth. The use case must still acquire this operation lock
    -- before account/counter/audit locks, not defer admission until this INSERT.
    CREATE TRIGGER reserve_decision_requires_open_operation
        BEFORE INSERT ON reserve_decisions FOR EACH ROW
        EXECUTE FUNCTION guard_reserve_decision_operation();
END;
$migration$;
