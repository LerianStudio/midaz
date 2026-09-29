-- An admitted operation carries the same TTL as the capacity it holds, so one
-- that holds no reservation (a DENY or REVIEW decision, or an ALLOW that no
-- limit applied to) still expires instead of staying OPEN forever. The
-- admission sets expires_at once, in the transaction that stores the decision;
-- the reaper expires OPEN operations past it that hold no RESERVED row.
DO $migration$
BEGIN
    SET LOCAL lock_timeout = '5s';
    LOCK TABLE reserve_operations IN ACCESS EXCLUSIVE MODE;

    ALTER TABLE reserve_operations ADD COLUMN expires_at TIMESTAMPTZ;

    -- An OPEN operation may gain its expiry once and nothing else; a terminal
    -- transition keeps the expiry it had.
    CREATE OR REPLACE FUNCTION guard_reserve_operation_transition() RETURNS trigger AS $function$
    BEGIN
        IF NEW.integration_id IS DISTINCT FROM OLD.integration_id
            OR NEW.transaction_id IS DISTINCT FROM OLD.transaction_id
            OR OLD.status <> 'OPEN'
            OR (NEW.status = 'OPEN' AND (OLD.expires_at IS NOT NULL OR NEW.expires_at IS NULL))
            OR (NEW.status <> 'OPEN' AND (NEW.status NOT IN ('CONFIRMED', 'RELEASED', 'EXPIRED')
                OR NEW.expires_at IS DISTINCT FROM OLD.expires_at)) THEN
            RAISE EXCEPTION 'reserve operation transition conflicts with recorded outcome'
                USING ERRCODE = '23514', CONSTRAINT = 'reserve_operation_transition';
        END IF;
        RETURN NEW;
    END;
    $function$ LANGUAGE plpgsql;

    -- Stored decisions do not record whether their transaction was long-lived,
    -- so an operation admitted before this migration takes the direct TTL from
    -- its decision time.
    UPDATE reserve_operations AS o
    SET expires_at = d.created_at + interval '5 minutes'
    FROM reserve_decisions AS d
    WHERE o.status = 'OPEN' AND o.expires_at IS NULL
      AND d.integration_id = o.integration_id AND d.transaction_id = o.transaction_id;

    -- lint:ignore CREATE INDEX CONCURRENTLY cannot run inside this transaction
    CREATE INDEX idx_reserve_operations_expiry
        ON reserve_operations (expires_at, integration_id, transaction_id)
        WHERE status = 'OPEN';
END;
$migration$;
