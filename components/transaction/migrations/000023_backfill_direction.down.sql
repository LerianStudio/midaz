ALTER TABLE operation DROP CONSTRAINT IF EXISTS chk_operation_direction;
ALTER TABLE operation ALTER COLUMN direction DROP NOT NULL;
