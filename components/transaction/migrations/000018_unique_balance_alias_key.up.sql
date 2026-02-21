-- Remove duplicate balance rows that were created due to the non-unique index.
-- Keep the row with the earliest created_at for each (organization_id, ledger_id, alias, key) group.
DELETE FROM balance
WHERE id IN (
    SELECT id FROM (
        SELECT id, ROW_NUMBER() OVER (
            PARTITION BY organization_id, ledger_id, alias, key
            ORDER BY created_at ASC
        ) AS rn
        FROM balance
        WHERE deleted_at IS NULL
    ) sub
    WHERE rn > 1
);

-- Drop the old non-unique index created by migration 000010.
DROP INDEX IF EXISTS idx_unique_balance_alias_key;

-- Create a proper UNIQUE index to prevent concurrent pods from inserting duplicate balance rows.
CREATE UNIQUE INDEX idx_unique_balance_alias_key ON balance (organization_id, ledger_id, alias, key) WHERE deleted_at IS NULL;
