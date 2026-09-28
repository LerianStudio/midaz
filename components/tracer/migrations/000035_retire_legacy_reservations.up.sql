-- Retire the reservations written without a decision. No writer produces
-- them, so every remaining one is closed here and moved out of
-- usage_reservations, leaving a table in which every row belongs to a decision.
--
-- Outstanding RESERVED rows expire the way the reaper expired them: the row
-- becomes EXPIRED and its amount leaves its counter bucket's reserved_usage.
-- current_usage is never touched, so counted spending is unchanged. The amounts
-- are summed per bucket because one UPDATE ... FROM applies a single source row
-- to each target row. The subtraction is floored at zero so a bucket that
-- already drifted below its outstanding holds fails no tenant's migration; the
-- CHECK keeps reserved_usage non-negative either way.
--
-- A row without a decision can never be given one: reservation ownership is
-- immutable and the owner FK names a decision that does not exist. The rows are
-- therefore copied, whatever their status, to retired_legacy_reservations and
-- deleted from usage_reservations. Nothing references a reservation row by
-- foreign key, and audit events carry reservation ids by value, so their
-- history still resolves against the retired copy.
--
-- The CHECK is added NOT VALID: new rows are checked from this commit on, and
-- the next migration validates the existing ones under a lock that does not
-- block writes.
--
-- Operator requirement: the ACCESS EXCLUSIVE lock blocks reserve admission for
-- as long as the legacy rows take to copy and delete, which is proportional to
-- the legacy row count. In multi-tenant deployments it runs once per tenant
-- database.
--
-- Operator requirement: legacy PENDING (long-lived) holds are RESERVED rows
-- like any other and expire here immediately, whatever their remaining TTL.
-- Commit or cancel legacy PENDING transactions before upgrading. A legacy
-- PENDING transaction committed after this migration has no reservation left
-- to confirm, so its spend is not counted for the rest of the limit period.
DO $migration$
BEGIN
    -- Fail rather than queue an exclusive lock behind live reservation traffic.
    SET LOCAL lock_timeout = '5s';
    LOCK TABLE usage_reservations IN ACCESS EXCLUSIVE MODE;

    WITH expired AS (
        UPDATE usage_reservations
        SET status = 'EXPIRED', released_at = NOW()
        WHERE decision_id IS NULL AND status = 'RESERVED'
        RETURNING limit_id, scope_key, period_key, amount
    ), held AS (
        SELECT limit_id, scope_key, period_key, SUM(amount) AS amount
        FROM expired
        GROUP BY limit_id, scope_key, period_key
    )
    UPDATE usage_counters AS c
    SET reserved_usage = GREATEST(c.reserved_usage - held.amount, 0),
        last_updated_at = NOW()
    FROM held
    WHERE c.limit_id = held.limit_id
      AND c.scope_key = held.scope_key
      AND c.period_key = held.period_key;

    CREATE TABLE retired_legacy_reservations (
        id UUID PRIMARY KEY,
        limit_id UUID NOT NULL,
        scope_key VARCHAR(255) NOT NULL,
        period_key VARCHAR(50) NOT NULL,
        amount DECIMAL NOT NULL,
        status VARCHAR(16) NOT NULL,
        transaction_id UUID NOT NULL,
        reservation_expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
        created_at TIMESTAMP WITH TIME ZONE NOT NULL,
        confirmed_at TIMESTAMP WITH TIME ZONE,
        released_at TIMESTAMP WITH TIME ZONE,
        retired_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
    );

    INSERT INTO retired_legacy_reservations (
        id, limit_id, scope_key, period_key, amount, status, transaction_id,
        reservation_expires_at, created_at, confirmed_at, released_at
    )
    SELECT id, limit_id, scope_key, period_key, amount, status, transaction_id,
           reservation_expires_at, created_at, confirmed_at, released_at
    FROM usage_reservations
    WHERE decision_id IS NULL;

    DELETE FROM usage_reservations WHERE decision_id IS NULL;

    ALTER TABLE usage_reservations ADD CONSTRAINT usage_reservations_decision_required
        CHECK (decision_id IS NOT NULL) NOT VALID;

    -- The legacy idempotency index covered only rows without a decision.
    -- lint:ignore DROP INDEX CONCURRENTLY cannot run inside this transaction
    DROP INDEX idx_usage_reservations_request;
END;
$migration$;
