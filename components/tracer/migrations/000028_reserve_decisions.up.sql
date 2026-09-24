-- Immutable decisions are separate from capacity reservations: DENY, REVIEW
-- and ALLOW with no applicable limits must all remain replayable. Tenant
-- isolation uses the existing tenant database, never a caller-supplied column.
CREATE TABLE reserve_decisions (
    evaluation_id UUID PRIMARY KEY CHECK (evaluation_id <> '00000000-0000-0000-0000-000000000000'),
    integration_id TEXT NOT NULL CHECK (octet_length(integration_id) BETWEEN 1 AND 256),
    transaction_id UUID NOT NULL CHECK (transaction_id <> '00000000-0000-0000-0000-000000000000'),
    request_id UUID NOT NULL CHECK (request_id <> '00000000-0000-0000-0000-000000000000'),
    context_id TEXT NOT NULL CHECK (octet_length(context_id) BETWEEN 1 AND 256),
    contract_revision TEXT NOT NULL CHECK (contract_revision = 'context-reserve-1'),
    request_fingerprint BYTEA NOT NULL CHECK (octet_length(request_fingerprint) = 32),
    validation_mode TEXT NOT NULL CHECK (validation_mode IN ('limits', 'rules-and-limits')),
    policy_id UUID,
    policy_revision BIGINT,
    policy_snapshot JSONB,
    response JSONB NOT NULL CHECK (jsonb_typeof(response) = 'object'),
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (integration_id, transaction_id),
    UNIQUE (integration_id, request_id),
    FOREIGN KEY (policy_id, policy_revision)
        REFERENCES evaluation_policy_revisions(policy_id, policy_revision) MATCH FULL,
    CHECK ((
        (validation_mode = 'limits' AND policy_id IS NULL AND policy_revision IS NULL AND policy_snapshot IS NULL)
        OR
        (validation_mode = 'rules-and-limits' AND policy_id IS NOT NULL AND policy_revision IS NOT NULL
         AND policy_snapshot IS NOT NULL AND jsonb_typeof(policy_snapshot) = 'object'
         AND policy_snapshot ?& ARRAY['id', 'revision', 'bindingVersion', 'defaultUsed', 'evaluatedRules', 'matchedRules']
         AND policy_snapshot->>'id' = policy_id::text
         AND (policy_snapshot->>'revision')::bigint = policy_revision
         AND (policy_snapshot->>'bindingVersion')::bigint > 0
         AND jsonb_typeof(policy_snapshot->'evaluatedRules') = 'array'
         AND jsonb_typeof(policy_snapshot->'matchedRules') = 'array')
    ) IS TRUE),
    CHECK ((
        response ?& ARRAY['contractRevision', 'transactionId', 'evaluationId', 'decision', 'controls', 'reservationIds', 'reasons']
        AND response->>'contractRevision' = contract_revision
        AND response->>'transactionId' = transaction_id::text
        AND response->>'evaluationId' = evaluation_id::text
        AND response->>'decision' IN ('ALLOW', 'DENY', 'REVIEW')
        AND jsonb_typeof(response->'controls') = 'object'
        AND jsonb_typeof(response->'reservationIds') = 'array'
        AND jsonb_typeof(response->'reasons') = 'array'
        AND (response->>'decision' = 'ALLOW' OR jsonb_array_length(response->'reservationIds') = 0)
    ) IS TRUE)
);

CREATE FUNCTION reject_reserve_decision_mutation() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'reserve decisions are immutable' USING ERRCODE = '23514';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER immutable_reserve_decisions
    BEFORE UPDATE OR DELETE OR TRUNCATE ON reserve_decisions
    FOR EACH STATEMENT EXECUTE FUNCTION reject_reserve_decision_mutation();

-- Existing usage_reservations, uniqueness and counters are intentionally left
-- untouched. Their coordinated migration must accompany the new Reserve writer.
