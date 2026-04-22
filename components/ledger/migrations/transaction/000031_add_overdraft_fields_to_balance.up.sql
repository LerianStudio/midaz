-- Add overdraft fields to the balance table.
--
-- Adds three columns required by the overdraft feature:
--   * direction       — accounting direction of the balance ("credit"/"debit").
--                        Defaults to 'credit' so legacy rows match the prior
--                        implicit behavior.
--   * overdraft_used  — amount of overdraft currently consumed. Non-negative.
--   * settings        — optional per-balance configuration stored as JSONB
--                        (allowOverdraft, overdraftLimitEnabled, overdraftLimit,
--                        balanceScope).
--
-- All statements use IF NOT EXISTS for idempotent re-runs.

ALTER TABLE balance
    ADD COLUMN IF NOT EXISTS direction       VARCHAR(16)   NOT NULL DEFAULT 'credit'
                                              CHECK (direction IN ('credit', 'debit')),
    ADD COLUMN IF NOT EXISTS overdraft_used  DECIMAL        NOT NULL DEFAULT 0
                                              CHECK (overdraft_used >= 0),
    ADD COLUMN IF NOT EXISTS settings        JSONB;
