DO $migration$
BEGIN
    SET LOCAL lock_timeout = '5s';
    LOCK TABLE reserve_operations IN ACCESS EXCLUSIVE MODE;

    DROP INDEX idx_reserve_operations_expiry;

    CREATE OR REPLACE FUNCTION guard_reserve_operation_transition() RETURNS trigger AS $function$
    BEGIN
        IF NEW.integration_id IS DISTINCT FROM OLD.integration_id
            OR NEW.transaction_id IS DISTINCT FROM OLD.transaction_id
            OR OLD.status <> 'OPEN' OR NEW.status NOT IN ('CONFIRMED', 'RELEASED', 'EXPIRED') THEN
            RAISE EXCEPTION 'reserve operation transition conflicts with recorded outcome'
                USING ERRCODE = '23514', CONSTRAINT = 'reserve_operation_transition';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;

    ALTER TABLE reserve_operations DROP COLUMN expires_at;
END;
$migration$;
