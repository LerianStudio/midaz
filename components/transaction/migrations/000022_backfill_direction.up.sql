UPDATE operation SET direction = CASE
    WHEN UPPER(type) = 'DEBIT' THEN 'debit'
    WHEN UPPER(type) = 'CREDIT' THEN 'credit'
    WHEN UPPER(type) = 'ON_HOLD' THEN 'debit'
    WHEN UPPER(type) = 'RELEASE' THEN 'credit'
    ELSE 'debit'
END WHERE direction IS NULL;

ALTER TABLE operation ALTER COLUMN direction SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_operation_direction'
    ) THEN
        ALTER TABLE operation ADD CONSTRAINT chk_operation_direction CHECK (LOWER(direction) IN ('debit', 'credit'));
    END IF;
END $$;
