CREATE TABLE IF NOT EXISTS transaction_revert_claim (
    organization_id UUID NOT NULL,
    ledger_id UUID NOT NULL,
    origin_transaction_id UUID NOT NULL,
    reverse_transaction_id UUID NOT NULL,
    legacy_fence_key TEXT,
    legacy_fence_owner TEXT,
    rollout_mode VARCHAR(16),
    rollout_token TEXT,
    state VARCHAR(32) NOT NULL DEFAULT 'CLAIMED',
    failure_reason VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (organization_id, ledger_id, origin_transaction_id),
    UNIQUE (organization_id, ledger_id, reverse_transaction_id),
    CONSTRAINT transaction_revert_claim_legacy_fence_key_check CHECK (
        legacy_fence_key IS NULL OR BTRIM(legacy_fence_key) <> ''
    ),
    CONSTRAINT transaction_revert_claim_legacy_fence_owner_check CHECK (
        legacy_fence_owner IS NULL OR (
            legacy_fence_key IS NOT NULL
            AND legacy_fence_owner = reverse_transaction_id::TEXT
        )
    ),
    CONSTRAINT transaction_revert_claim_rollout_check CHECK (
        (rollout_mode IS NULL AND rollout_token IS NULL)
        OR (
            rollout_mode IS NOT NULL
            AND rollout_token IS NOT NULL
            AND
            rollout_mode IN ('legacy', 'bridge')
            AND BTRIM(rollout_token) <> ''
        )
    ),
    CONSTRAINT transaction_revert_claim_state_check CHECK (
        state IN ('CLAIMED', 'RECOVERING', 'MUTATED', 'COMPLETED', 'RECONCILIATION_REQUIRED')
    )
);
