-- Backfill direction from type for existing operations
UPDATE operation SET direction = CASE
    WHEN UPPER(type) = 'DEBIT' THEN 'debit'
    WHEN UPPER(type) = 'CREDIT' THEN 'credit'
    WHEN UPPER(type) = 'ON_HOLD' THEN 'debit'
    WHEN UPPER(type) = 'RELEASE' THEN 'credit'
    ELSE 'debit'
END WHERE direction IS NULL;

-- Domain CHECK with NOT VALID: registers the constraint without scanning the table.
-- ACCESS EXCLUSIVE lock for milliseconds only. New INSERTs/UPDATEs are validated
-- against the CHECK from this point on.
ALTER TABLE operation DROP CONSTRAINT IF EXISTS chk_operation_direction;
ALTER TABLE operation ADD CONSTRAINT chk_operation_direction
    CHECK (LOWER(direction) IN ('debit', 'credit')) NOT VALID;

-- VALIDATE performs the full scan to verify existing rows satisfy the CHECK.
-- Uses SHARE UPDATE EXCLUSIVE lock — reads and writes continue normally.
ALTER TABLE operation VALIDATE CONSTRAINT chk_operation_direction;
