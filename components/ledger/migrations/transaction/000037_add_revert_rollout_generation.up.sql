ALTER TABLE transaction_revert_claim
    ADD COLUMN IF NOT EXISTS rollout_mode VARCHAR(16),
    ADD COLUMN IF NOT EXISTS rollout_token TEXT,
    ADD COLUMN IF NOT EXISTS redis_generation TEXT;

ALTER TABLE transaction_revert_claim
    DROP CONSTRAINT IF EXISTS transaction_revert_claim_rollout_check;

ALTER TABLE transaction_revert_claim
    ADD CONSTRAINT transaction_revert_claim_rollout_check CHECK (
        (
            (rollout_mode IS NULL AND rollout_token IS NULL)
            OR (
                rollout_mode IN ('legacy', 'bridge')
                AND rollout_token IS NOT NULL
                AND BTRIM(rollout_token) <> ''
            )
        )
        AND (redis_generation IS NULL OR BTRIM(redis_generation) <> '')
        AND (rollout_mode IS NULL OR redis_generation IS NOT NULL)
    );
