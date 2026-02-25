CREATE TABLE balance_partitioned (
    id                UUID NOT NULL,
    organization_id   UUID NOT NULL,
    ledger_id         UUID NOT NULL,
    account_id        UUID NOT NULL,
    alias             TEXT NOT NULL,
    asset_code        TEXT NOT NULL,
    available         DECIMAL NOT NULL DEFAULT 0,
    on_hold           DECIMAL NOT NULL DEFAULT 0,
    version           BIGINT DEFAULT 0,
    account_type      TEXT NOT NULL,
    allow_sending     BOOLEAN NOT NULL,
    allow_receiving   BOOLEAN NOT NULL,
    created_at        TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at        TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMP WITH TIME ZONE,
    key               TEXT NOT NULL DEFAULT 'default',
    PRIMARY KEY (id, ledger_id)
) PARTITION BY HASH (ledger_id);

CREATE TABLE balance_p00 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE balance_p01 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 1);
CREATE TABLE balance_p02 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 2);
CREATE TABLE balance_p03 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 3);
CREATE TABLE balance_p04 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 4);
CREATE TABLE balance_p05 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 5);
CREATE TABLE balance_p06 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 6);
CREATE TABLE balance_p07 PARTITION OF balance_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 7);

CREATE INDEX IF NOT EXISTS idx_balance_part_account_id
    ON balance_partitioned (organization_id, ledger_id, account_id, deleted_at, created_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_balance_part_unique_alias_key
    ON balance_partitioned (organization_id, ledger_id, alias, key)
    WHERE deleted_at IS NULL;

-- NOTE: Batched pre-copy loop and LOCK TABLE were removed because
-- golang-migrate multi-statement parser runs each statement independently
-- (splits on semicolons, no shared transaction), which breaks both
-- PL/pgSQL blocks and LOCK TABLE semantics.

INSERT INTO balance_partitioned (
    id,
    organization_id,
    ledger_id,
    account_id,
    alias,
    asset_code,
    available,
    on_hold,
    version,
    account_type,
    allow_sending,
    allow_receiving,
    created_at,
    updated_at,
    deleted_at,
    key
)
SELECT
    id,
    organization_id,
    ledger_id,
    account_id,
    alias,
    asset_code,
    available,
    on_hold,
    version,
    account_type,
    allow_sending,
    allow_receiving,
    created_at,
    updated_at,
    deleted_at,
    key
FROM balance
ON CONFLICT DO NOTHING;

ALTER TABLE balance RENAME TO balance_legacy;
ALTER TABLE balance_partitioned RENAME TO balance;
