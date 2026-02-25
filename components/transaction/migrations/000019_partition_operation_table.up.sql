CREATE TABLE operation_partitioned (
    id                     UUID NOT NULL,
    transaction_id         UUID NOT NULL,
    description            TEXT NOT NULL,
    type                   TEXT NOT NULL,
    asset_code             TEXT NOT NULL,
    amount                 DECIMAL NOT NULL DEFAULT 0,
    available_balance      DECIMAL NOT NULL DEFAULT 0,
    on_hold_balance        DECIMAL NOT NULL DEFAULT 0,
    available_balance_after DECIMAL NOT NULL DEFAULT 0,
    on_hold_balance_after  DECIMAL NOT NULL DEFAULT 0,
    status                 TEXT NOT NULL,
    status_description     TEXT,
    account_id             UUID NOT NULL,
    account_alias          TEXT NOT NULL,
    balance_id             UUID NOT NULL,
    chart_of_accounts      TEXT NOT NULL,
    organization_id        UUID NOT NULL,
    ledger_id              UUID NOT NULL,
    created_at             TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at             TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at             TIMESTAMP WITH TIME ZONE,
    route                  TEXT,
    balance_affected       BOOLEAN NOT NULL DEFAULT true,
    balance_key            TEXT NOT NULL DEFAULT 'default',
    balance_version_before BIGINT NOT NULL DEFAULT 0,
    balance_version_after  BIGINT NOT NULL DEFAULT 0,
    PRIMARY KEY (id, ledger_id)
) PARTITION BY HASH (ledger_id);

-- NOTE: PostgreSQL < 17 does not support referencing partitioned tables with
-- global foreign keys in this migration shape. Integrity between
-- operation.transaction_id and transaction.id remains enforced by application
-- flow and idempotent write constraints.

CREATE TABLE operation_p00 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 0);
CREATE TABLE operation_p01 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 1);
CREATE TABLE operation_p02 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 2);
CREATE TABLE operation_p03 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 3);
CREATE TABLE operation_p04 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 4);
CREATE TABLE operation_p05 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 5);
CREATE TABLE operation_p06 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 6);
CREATE TABLE operation_p07 PARTITION OF operation_partitioned FOR VALUES WITH (MODULUS 8, REMAINDER 7);

CREATE INDEX IF NOT EXISTS idx_op_part_transaction_id
    ON operation_partitioned (transaction_id);

CREATE INDEX IF NOT EXISTS idx_op_part_org_ledger
    ON operation_partitioned (organization_id, ledger_id);

CREATE INDEX IF NOT EXISTS idx_op_part_created_at
    ON operation_partitioned (created_at);

CREATE INDEX IF NOT EXISTS idx_op_part_account_id
    ON operation_partitioned (organization_id, ledger_id, account_id, id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_op_part_point_in_time
    ON operation_partitioned (organization_id, ledger_id, balance_id, created_at DESC)
    INCLUDE (available_balance_after, on_hold_balance_after, balance_version_after, account_id, asset_code)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_op_part_account_point_in_time
    ON operation_partitioned (organization_id, ledger_id, account_id, balance_id, created_at DESC)
    INCLUDE (available_balance_after, on_hold_balance_after, balance_version_after, asset_code)
    WHERE deleted_at IS NULL;

-- NOTE: Batched pre-copy loop and LOCK TABLE were removed because
-- golang-migrate multi-statement parser runs each statement independently
-- (splits on semicolons, no shared transaction), which breaks both
-- PL/pgSQL blocks and LOCK TABLE semantics.

INSERT INTO operation_partitioned (
    id,
    transaction_id,
    description,
    type,
    asset_code,
    amount,
    available_balance,
    on_hold_balance,
    available_balance_after,
    on_hold_balance_after,
    status,
    status_description,
    account_id,
    account_alias,
    balance_id,
    chart_of_accounts,
    organization_id,
    ledger_id,
    created_at,
    updated_at,
    deleted_at,
    route,
    balance_affected,
    balance_key,
    balance_version_before,
    balance_version_after
)
SELECT
    id,
    transaction_id,
    description,
    type,
    asset_code,
    amount,
    available_balance,
    on_hold_balance,
    available_balance_after,
    on_hold_balance_after,
    status,
    status_description,
    account_id,
    account_alias,
    balance_id,
    chart_of_accounts,
    organization_id,
    ledger_id,
    created_at,
    updated_at,
    deleted_at,
    route,
    balance_affected,
    balance_key,
    balance_version_before,
    balance_version_after
FROM operation
ON CONFLICT DO NOTHING;

ALTER TABLE operation RENAME TO operation_legacy;
ALTER TABLE operation_partitioned RENAME TO operation;
