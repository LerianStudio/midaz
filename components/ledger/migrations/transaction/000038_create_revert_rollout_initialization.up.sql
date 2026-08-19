CREATE TABLE IF NOT EXISTS transaction_revert_rollout_initialization (
    singleton BOOLEAN NOT NULL DEFAULT TRUE,
    redis_generation UUID NOT NULL,
    initialization_request_id UUID NOT NULL,
    state VARCHAR(16) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE transaction_revert_rollout_initialization
    ADD COLUMN IF NOT EXISTS singleton BOOLEAN DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS redis_generation UUID,
    ADD COLUMN IF NOT EXISTS initialization_request_id UUID,
    ADD COLUMN IF NOT EXISTS state VARCHAR(16),
    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ DEFAULT NOW(),
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ DEFAULT NOW();

ALTER TABLE transaction_revert_rollout_initialization
    ALTER COLUMN singleton SET NOT NULL,
    ALTER COLUMN redis_generation SET NOT NULL,
    ALTER COLUMN initialization_request_id SET NOT NULL,
    ALTER COLUMN state SET NOT NULL,
    ALTER COLUMN created_at SET NOT NULL,
    ALTER COLUMN updated_at SET NOT NULL;

ALTER TABLE transaction_revert_rollout_initialization
    DROP CONSTRAINT IF EXISTS transaction_revert_rollout_initialization_pkey,
    DROP CONSTRAINT IF EXISTS transaction_revert_rollout_initialization_singleton_check,
    DROP CONSTRAINT IF EXISTS transaction_revert_rollout_initialization_state_check;

ALTER TABLE transaction_revert_rollout_initialization
    ADD CONSTRAINT transaction_revert_rollout_initialization_pkey PRIMARY KEY (singleton),
    ADD CONSTRAINT transaction_revert_rollout_initialization_singleton_check CHECK (singleton),
    ADD CONSTRAINT transaction_revert_rollout_initialization_state_check
        CHECK (state IN ('PREPARING', 'PREPARED'));
