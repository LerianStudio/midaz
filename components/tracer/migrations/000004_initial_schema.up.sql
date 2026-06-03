-- ============================================
-- Migration: 000004_initial_schema
-- Description: Create all MVP tables (rules, limits, usage_counters, transaction_validations)
-- Date: 2025-12-28
-- ============================================

-- Enable pgcrypto extension for SHA-256 hashing in audit functions
CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- Enum types
-- Idempotency: wrap each CREATE TYPE in a DO block that swallows duplicate_object.
-- Required by the Migration Renumbering Invariant (docs/PROJECT_RULES.md): when
-- an already-populated database has this file replayed at a new version number
-- (origin/develop → HEAD upgrade path), naive CREATE TYPE would fail.
DO $$ BEGIN
    CREATE TYPE transaction_type_enum AS ENUM ('CARD', 'WIRE', 'PIX', 'CRYPTO');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE decision_enum AS ENUM ('ALLOW', 'DENY', 'REVIEW');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Rule status enum
DO $$ BEGIN
    CREATE TYPE rule_status_enum AS ENUM ('DRAFT', 'ACTIVE', 'INACTIVE', 'DELETED');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Limit enums
DO $$ BEGIN
    CREATE TYPE limit_type_enum AS ENUM ('DAILY', 'MONTHLY', 'PER_TRANSACTION');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE limit_status_enum AS ENUM ('DRAFT', 'ACTIVE', 'INACTIVE', 'DELETED');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Audit event enums
DO $$ BEGIN
    CREATE TYPE audit_event_type_enum AS ENUM (
        'TRANSACTION_VALIDATED',
        'RULE_CREATED', 'RULE_UPDATED', 'RULE_ACTIVATED', 'RULE_DEACTIVATED', 'RULE_DELETED',
        'LIMIT_CREATED', 'LIMIT_UPDATED', 'LIMIT_ACTIVATED', 'LIMIT_DEACTIVATED', 'LIMIT_DELETED'
    );
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE audit_action_enum AS ENUM ('VALIDATE', 'CREATE', 'UPDATE', 'DELETE', 'ACTIVATE', 'DEACTIVATE');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE audit_result_enum AS ENUM ('SUCCESS', 'FAILED', 'ALLOW', 'DENY', 'REVIEW');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    CREATE TYPE resource_type_enum AS ENUM ('transaction', 'rule', 'limit');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

-- Rules table
-- Note: priority field removed from MVP (TRD v1.2.4) - all rules evaluated, DENY takes precedence
CREATE TABLE IF NOT EXISTS rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    expression TEXT NOT NULL,
    action decision_enum NOT NULL,
    scopes JSONB NOT NULL DEFAULT '[]',
    status rule_status_enum NOT NULL DEFAULT 'DRAFT',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    activated_at TIMESTAMP WITH TIME ZONE,
    deactivated_at TIMESTAMP WITH TIME ZONE,
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_rules_status ON rules(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_rules_scopes ON rules USING GIN(scopes) WHERE status = 'ACTIVE';

-- Limits table
-- max_amount is stored in the smallest currency unit (e.g., cents)
CREATE TABLE IF NOT EXISTS limits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    limit_type limit_type_enum NOT NULL,
    max_amount BIGINT NOT NULL CHECK (max_amount > 0),
    currency VARCHAR(3) NOT NULL,
    scopes JSONB NOT NULL DEFAULT '[]',
    status limit_status_enum NOT NULL DEFAULT 'DRAFT',
    -- Next reset timestamp (null for PER_TRANSACTION)
    -- DAILY: next midnight UTC, MONTHLY: next 1st of month at midnight UTC
    reset_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_limits_status ON limits(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_limits_scopes ON limits USING GIN(scopes) WHERE status = 'ACTIVE';

-- Usage counters table (depends on limits)
-- Tracks usage per limit/scope/period combination
-- current_usage is stored in the smallest currency unit (e.g., cents)
-- scope_key: serialized scope values (e.g., "acct:abc-123", "segment:gold")
-- period_key: date key (e.g., "2025-12-28" for DAILY, "2025-12" for MONTHLY)
CREATE TABLE IF NOT EXISTS usage_counters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    limit_id UUID NOT NULL REFERENCES limits(id) ON DELETE CASCADE,
    scope_key VARCHAR(255) NOT NULL,
    period_key VARCHAR(50) NOT NULL,
    current_usage BIGINT NOT NULL DEFAULT 0 CHECK (current_usage >= 0),
    last_updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    CONSTRAINT usage_counters_composite_unique UNIQUE (limit_id, scope_key, period_key)
);

-- Note: UNIQUE constraint already creates implicit index on (limit_id, scope_key, period_key)
-- Period key for cleanup (daily batch job)
CREATE INDEX IF NOT EXISTS idx_usage_counters_period ON usage_counters(period_key);
-- Limit ID for cascade queries
-- Note: Technically redundant (composite unique index covers limit_id queries), but kept
-- intentionally to make cascade operation intent explicit and aid query plan readability
CREATE INDEX IF NOT EXISTS idx_usage_counters_limit ON usage_counters(limit_id);

-- Transaction validations table
-- Immutable record for SOX/GLBA compliance (7-year retention)
-- Stores transaction data and context objects for querying and compliance
CREATE TABLE IF NOT EXISTS transaction_validations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Request identification
    request_id UUID NOT NULL,
    
    -- Transaction data
    transaction_type transaction_type_enum NOT NULL,
    sub_type VARCHAR(50),
    amount BIGINT NOT NULL CHECK (amount > 0),
    currency CHAR(3) NOT NULL,
    transaction_timestamp TIMESTAMP WITH TIME ZONE NOT NULL,
    
    -- Context objects (JSONB to preserve all data)
    account JSONB NOT NULL,           -- {accountId, type, status, metadata}
    segment JSONB,                    -- {segmentId, name, metadata} (nullable)
    portfolio JSONB,                  -- {portfolioId, name, metadata} (nullable)
    merchant JSONB,                   -- {merchantId, name, category, country, metadata} (nullable)
    
    -- Additional data
    metadata JSONB DEFAULT '{}',
    
    -- Response/Decision data
    decision decision_enum NOT NULL,
    reason TEXT,
    matched_rule_ids UUID[] NOT NULL DEFAULT '{}',
    evaluated_rule_ids UUID[] NOT NULL DEFAULT '{}',
    limit_usage_details JSONB NOT NULL DEFAULT '[]',
    processing_time_ms INTEGER NOT NULL CHECK (processing_time_ms >= 0),
    
    -- Audit timestamp (immutable)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Indexes for filtering (aligned with GET /v1/validations query params)
CREATE INDEX IF NOT EXISTS idx_transaction_validations_created ON transaction_validations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transaction_validations_decision ON transaction_validations(decision);
CREATE INDEX IF NOT EXISTS idx_transaction_validations_transaction_type ON transaction_validations(transaction_type);
CREATE INDEX IF NOT EXISTS idx_transaction_validations_request_id ON transaction_validations(request_id);
CREATE INDEX IF NOT EXISTS idx_transaction_validations_account_id ON transaction_validations(((account->>'accountId')::uuid));
CREATE INDEX IF NOT EXISTS idx_transaction_validations_segment_id ON transaction_validations(((segment->>'segmentId')::uuid)) WHERE segment IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_transaction_validations_portfolio_id ON transaction_validations(((portfolio->>'portfolioId')::uuid)) WHERE portfolio IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_transaction_validations_matched_rules ON transaction_validations USING GIN(matched_rule_ids);

-- Immutability rules: prevent UPDATE and DELETE on transaction_validations
-- Required for SOX/GLBA compliance - audit records must never be modified
-- Prevent UPDATE operations - rule replaces the UPDATE with nothing
CREATE OR REPLACE RULE prevent_transaction_validation_update AS
    ON UPDATE TO transaction_validations
    DO INSTEAD NOTHING;

-- Prevent DELETE operations - rule replaces the DELETE with nothing
CREATE OR REPLACE RULE prevent_transaction_validation_delete AS
    ON DELETE TO transaction_validations
    DO INSTEAD NOTHING;

-- TRUNCATE protection trigger (requires prevent_truncate function)
-- Note: Requires prevent_truncate() function from migration 000003_prevent_truncate.up.sql
-- Uses CREATE OR REPLACE TRIGGER (PG 14+; prod targets PG 16) for idempotent replay.
CREATE OR REPLACE TRIGGER prevent_transaction_validation_truncate_trigger
    BEFORE TRUNCATE ON transaction_validations
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_truncate();

-- ============================================================================
-- TRUNCATE Protection for SOX/GLBA Compliance
-- ============================================================================
-- TRUNCATE bypasses RULEs, so additional protection is needed.
--
-- The prevent_truncate() function is created by migration 000003_prevent_truncate.up.sql,
-- and this file installs the TRUNCATE-protection triggers on the immutable tables below.
--
-- FOR PRODUCTION:
--   Create a separate application role without TRUNCATE privileges:
--
--   -- Create application role (run as superuser/admin)
--   CREATE ROLE tracer_app WITH LOGIN PASSWORD 'your_secure_password';
--   
--   -- Grant necessary privileges
--   GRANT CONNECT ON DATABASE tracer TO tracer_app;
--   GRANT USAGE ON SCHEMA public TO tracer_app;
--   
--   -- For transaction_validations: only SELECT and INSERT (immutable audit table)
--   GRANT SELECT, INSERT ON transaction_validations TO tracer_app;
--   -- Note: UPDATE and DELETE are blocked by RULEs, but not granting them adds defense-in-depth
--   
--   -- For other tables: grant full CRUD as needed
--   GRANT SELECT, INSERT, UPDATE, DELETE ON rules TO tracer_app;
--   GRANT SELECT, INSERT, UPDATE, DELETE ON limits TO tracer_app;
--   GRANT SELECT, INSERT, UPDATE, DELETE ON usage_counters TO tracer_app;
--   
--   -- Grant sequence usage for auto-generated IDs
--   GRANT USAGE ON ALL SEQUENCES IN SCHEMA public TO tracer_app;
--
-- The application should connect using tracer_app role, not the owner role.
-- ============================================================================

-- ============================================
-- Audit Events table
-- Centralized audit trail for all Tracer operations
-- Implements hash chain for SOX compliance and data integrity
-- ============================================

-- Audit events table (append-only, immutable)
-- Stores all audit events: validations, rule changes, limit changes
-- Note: Validation-specific fields (accountId, segmentId, etc.) are stored in context JSONB

DO $$ BEGIN
    CREATE TYPE actor_type_enum AS ENUM ('user', 'system');
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS audit_events (
    -- Internal fields (system-managed, not exposed in API)
    id                  BIGSERIAL PRIMARY KEY,
    created_at          TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    hash                VARCHAR(64)     NOT NULL,
    previous_hash       VARCHAR(64),

    -- Core indexed fields
    event_id            UUID            NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    event_type          audit_event_type_enum NOT NULL,
    action              audit_action_enum     NOT NULL,

    -- Result field (unified)
    -- For validations: ALLOW, DENY, REVIEW
    -- For CRUD operations: SUCCESS, FAILED
    result              audit_result_enum     NOT NULL,

    -- Resource fields (indexed)
    resource_id         VARCHAR(255)    NOT NULL,
    resource_type       resource_type_enum    NOT NULL,

    -- Actor fields (uses actor_type_enum for type safety and validation)
    actor_type          actor_type_enum NOT NULL,
    actor_id            VARCHAR(255)    NOT NULL,
    actor_name          VARCHAR(255)    NOT NULL,
    actor_role          VARCHAR(100),
    actor_ip_address    VARCHAR(45)     NOT NULL,  -- IPv4 (15) or IPv6 (45) address as string

    -- Context field (JSONB)
    -- For validations: { request: {...}, response: {reason, processingTimeMs, matchedRuleIds, evaluatedRuleIds, limitUsageDetails} }
    -- For CRUD: { before: {...}, after: {...}, reason: "..." }
    -- Note: accountId, segmentId, portfolioId, transactionType are in context.request (indexed via JSONB)
    context             JSONB           NOT NULL DEFAULT '{}',

    -- Metadata field (JSONB) - additional info like ticketId, correlationId
    metadata            JSONB           NOT NULL DEFAULT '{}'
);

-- Primary query indexes
CREATE INDEX IF NOT EXISTS idx_audit_events_event_type ON audit_events(event_type);
CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(action);
CREATE INDEX IF NOT EXISTS idx_audit_events_result ON audit_events(result);

-- Resource lookup indexes
CREATE INDEX IF NOT EXISTS idx_audit_events_resource ON audit_events(resource_type, resource_id);

-- Actor lookup indexes
CREATE INDEX IF NOT EXISTS idx_audit_events_actor ON audit_events(actor_type, actor_id);

-- JSONB indexes for validation event filtering
-- All validation-specific fields are in context, indexed via JSONB expressions

-- Index for filtering by accountId (in context.request.account.id)
CREATE INDEX IF NOT EXISTS idx_audit_events_account ON audit_events
    USING btree ((context->'request'->'account'->>'id'))
    WHERE event_type = 'TRANSACTION_VALIDATED';

-- Index for filtering by segmentId (in context.request.account.segmentId)
CREATE INDEX IF NOT EXISTS idx_audit_events_segment ON audit_events
    USING btree ((context->'request'->'account'->>'segmentId'))
    WHERE event_type = 'TRANSACTION_VALIDATED';

-- Index for filtering by portfolioId (in context.request.account.portfolioId)
CREATE INDEX IF NOT EXISTS idx_audit_events_portfolio ON audit_events
    USING btree ((context->'request'->'account'->>'portfolioId'))
    WHERE event_type = 'TRANSACTION_VALIDATED';

-- Index for filtering by transactionType (in context.request.transactionType)
CREATE INDEX IF NOT EXISTS idx_audit_events_transaction_type ON audit_events
    USING btree ((context->'request'->>'transactionType'))
    WHERE event_type = 'TRANSACTION_VALIDATED';

-- GIN index for filtering by matchedRuleIds (in context.response.matchedRuleIds)
CREATE INDEX IF NOT EXISTS idx_audit_events_matched_rules ON audit_events
    USING GIN ((context->'response'->'matchedRuleIds'))
    WHERE event_type = 'TRANSACTION_VALIDATED';

-- Hash chain verification index
CREATE INDEX IF NOT EXISTS idx_audit_events_hash ON audit_events(hash);

-- Apply hash chain trigger before insert
-- Note: Requires calculate_audit_event_hash() function (created in an earlier migration).
-- Uses CREATE OR REPLACE TRIGGER (PG 14+; prod targets PG 16) for idempotent replay.
CREATE OR REPLACE TRIGGER audit_events_hash_chain
    BEFORE INSERT ON audit_events
    FOR EACH ROW
    EXECUTE FUNCTION calculate_audit_event_hash();

-- Immutability: prevent UPDATE and DELETE (SOX compliance)
CREATE OR REPLACE RULE prevent_audit_event_update AS
    ON UPDATE TO audit_events
    DO INSTEAD NOTHING;

CREATE OR REPLACE RULE prevent_audit_event_delete AS
    ON DELETE TO audit_events
    DO INSTEAD NOTHING;

-- TRUNCATE protection trigger (requires prevent_truncate function)
-- Note: Requires prevent_truncate() function (created in an earlier migration).
-- Uses CREATE OR REPLACE TRIGGER (PG 14+; prod targets PG 16) for idempotent replay.
CREATE OR REPLACE TRIGGER prevent_audit_event_truncate_trigger
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_truncate();

