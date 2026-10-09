-- The lib-system-facts kit table: the lib-commons outbox schema under its own
-- name, so fact records never share a producer's own outbox_events table.
--
-- Exposed as outbox.SchemaSQL(). The kit never executes it: the producer folds
-- it into one of its own migrations, in its own migration sequence and version
-- table, and applies it in every place the forwarder dispatches from: public
-- for a single-tenant producer, each tenant schema for schema per tenant, each
-- tenant database (on its pool's default search_path) for database per tenant.
--
-- Every statement is idempotent. outbox_event_status is the lib-commons enum,
-- created unqualified under the duplicate_object guard, so this coexists with a
-- producer's own lib-commons outbox in the same schema, whichever runs first.
-- No transaction control: the producer's migration tool owns the transaction.

DO $enum$ BEGIN
    CREATE TYPE outbox_event_status AS ENUM ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED', 'INVALID');
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $enum$;

CREATE TABLE IF NOT EXISTS system_facts_outbox (
    id UUID PRIMARY KEY,
    event_type VARCHAR(255) NOT NULL,
    aggregate_id UUID NOT NULL,
    payload JSONB NOT NULL,
    status outbox_event_status NOT NULL DEFAULT 'PENDING',
    attempts INT NOT NULL DEFAULT 0,
    published_at TIMESTAMPTZ NULL,
    last_error VARCHAR(512),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_system_facts_outbox_status_created_at
    ON system_facts_outbox (status, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_system_facts_outbox_status_updated_at
    ON system_facts_outbox (status, updated_at ASC);

CREATE INDEX IF NOT EXISTS idx_system_facts_outbox_event_type_status_created_at
    ON system_facts_outbox (event_type, status, created_at ASC);
