CREATE TABLE IF NOT EXISTS transaction_revert_claim (
    organization_id UUID NOT NULL,
    ledger_id UUID NOT NULL,
    origin_transaction_id UUID NOT NULL,
    reverse_transaction_id UUID NOT NULL,
    state VARCHAR(32) NOT NULL DEFAULT 'CLAIMED',
    failure_reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, ledger_id, origin_transaction_id),
    UNIQUE (organization_id, ledger_id, reverse_transaction_id),
    CONSTRAINT transaction_revert_claim_state_check CHECK (
        state IN ('CLAIMED', 'MUTATED', 'COMPLETED', 'RECONCILIATION_REQUIRED')
    )
);
