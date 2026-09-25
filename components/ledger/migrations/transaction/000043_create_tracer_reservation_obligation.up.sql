-- Durable intent precedes Reserve and survives accounting/network uncertainty.
-- No foreign key to transaction: an obligation exists before accounting creates
-- that row and must survive a known pre-accounting rejection.
CREATE TABLE tracer_reservation_obligation (
    organization_id UUID NOT NULL,
    ledger_id UUID NOT NULL,
    transaction_id UUID NOT NULL,
    execution_id UUID NOT NULL,
    tenant_id TEXT NOT NULL CHECK (octet_length(tenant_id) <= 256),
    integration_id TEXT NOT NULL CHECK (octet_length(integration_id) BETWEEN 1 AND 256),
    asset_namespace TEXT NOT NULL CHECK (octet_length(asset_namespace) BETWEEN 1 AND 256),
    contract_revision TEXT NOT NULL CHECK (octet_length(contract_revision) BETWEEN 1 AND 256),
    fingerprint BYTEA NOT NULL CHECK (octet_length(fingerprint) = 32),
    payload BYTEA NOT NULL CHECK (octet_length(payload) > 0),
    created_at TIMESTAMPTZ NOT NULL,
    prepare_deadline TIMESTAMPTZ NOT NULL,
    state TEXT NOT NULL DEFAULT 'PREPARED' CHECK (state IN ('PREPARED','EXECUTING','CONFIRMED','RELEASED')),
    updated_at TIMESTAMPTZ NOT NULL,
    next_attempt_at TIMESTAMPTZ NOT NULL,
    delivered_at TIMESTAMPTZ,
    PRIMARY KEY (organization_id,ledger_id,transaction_id),
    CHECK (prepare_deadline > created_at),
    CHECK (updated_at >= created_at),
    CHECK (delivered_at IS NULL OR state IN ('CONFIRMED','RELEASED')),
    CHECK (organization_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CHECK (ledger_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CHECK (transaction_id <> '00000000-0000-0000-0000-000000000000'::uuid),
    CHECK (execution_id <> '00000000-0000-0000-0000-000000000000'::uuid)
);

CREATE INDEX tracer_reservation_obligation_due_idx
    ON tracer_reservation_obligation (next_attempt_at, organization_id, ledger_id, transaction_id)
    WHERE delivered_at IS NULL;

CREATE FUNCTION protect_tracer_reservation_obligation() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'tracer reservation coordination history cannot be deleted' USING ERRCODE = '23514';
    END IF;
    IF ROW(NEW.organization_id,NEW.ledger_id,NEW.transaction_id,NEW.execution_id,
           NEW.tenant_id,NEW.integration_id,NEW.asset_namespace,NEW.contract_revision,
           NEW.fingerprint,NEW.payload,NEW.created_at,NEW.prepare_deadline)
       IS DISTINCT FROM
       ROW(OLD.organization_id,OLD.ledger_id,OLD.transaction_id,OLD.execution_id,
           OLD.tenant_id,OLD.integration_id,OLD.asset_namespace,OLD.contract_revision,
           OLD.fingerprint,OLD.payload,OLD.created_at,OLD.prepare_deadline) THEN
        RAISE EXCEPTION 'tracer reservation intent is immutable' USING ERRCODE = '23514';
    END IF;
    IF NOT (NEW.state = OLD.state
        OR (OLD.state = 'PREPARED' AND NEW.state IN ('EXECUTING','RELEASED'))
        OR (OLD.state = 'EXECUTING' AND NEW.state IN ('CONFIRMED','RELEASED'))) THEN
        RAISE EXCEPTION 'tracer reservation outcome conflicts' USING ERRCODE = '23514';
    END IF;
    IF OLD.delivered_at IS NOT NULL AND NEW.delivered_at IS DISTINCT FROM OLD.delivered_at THEN
        RAISE EXCEPTION 'tracer reservation delivery acknowledgement is immutable' USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tracer_reservation_obligation_protection
    BEFORE UPDATE OR DELETE ON tracer_reservation_obligation
    FOR EACH ROW EXECUTE FUNCTION protect_tracer_reservation_obligation();
