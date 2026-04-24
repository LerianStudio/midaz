CREATE TABLE IF NOT EXISTS account_registration (
    id                  UUID PRIMARY KEY NOT NULL,
    organization_id     UUID NOT NULL,
    ledger_id           UUID NOT NULL,
    holder_id           UUID NOT NULL,
    idempotency_key     TEXT NOT NULL,
    request_hash        TEXT NOT NULL,
    account_id          UUID,
    crm_alias_id        UUID,
    status              TEXT NOT NULL,
    failure_code        TEXT,
    failure_message     TEXT,
    retry_count         INTEGER NOT NULL DEFAULT 0,
    next_retry_at       TIMESTAMP WITH TIME ZONE,
    claimed_by          TEXT,
    claimed_at          TIMESTAMP WITH TIME ZONE,
    last_recovered_at   TIMESTAMP WITH TIME ZONE,
    created_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at          TIMESTAMP WITH TIME ZONE NOT NULL,
    completed_at        TIMESTAMP WITH TIME ZONE,
    CONSTRAINT account_registration_status_check CHECK (status IN (
        'RECEIVED',
        'HOLDER_VALIDATED',
        'LEDGER_ACCOUNT_CREATED',
        'CRM_ALIAS_CREATED',
        'ACCOUNT_ACTIVATED',
        'COMPLETED',
        'COMPENSATING',
        'COMPENSATED',
        'FAILED_RETRYABLE',
        'FAILED_TERMINAL'
    ))
);

-- Indexes are declared without CONCURRENTLY because this migration runs inside a
-- golang-migrate transaction and CONCURRENTLY is incompatible with transactions.
-- This is safe here because the table is brand new and empty when the indexes are
-- created — no rows to lock. Subsequent indexes added to a populated table must use
-- CONCURRENTLY in a dedicated migration file.
CREATE UNIQUE INDEX idx_account_registration_idempotency
    ON account_registration (organization_id, ledger_id, idempotency_key);

CREATE INDEX idx_account_registration_org_ledger_status
    ON account_registration (organization_id, ledger_id, status);

CREATE INDEX idx_account_registration_status_next_retry
    ON account_registration (status, next_retry_at);

CREATE INDEX idx_account_registration_claimed_at
    ON account_registration (claimed_at);

CREATE INDEX idx_account_registration_account_id
    ON account_registration (account_id);

CREATE INDEX idx_account_registration_holder_id
    ON account_registration (holder_id);
