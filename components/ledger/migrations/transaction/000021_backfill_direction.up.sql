-- Backfill direction from type for existing operations
UPDATE operation SET direction = CASE
    WHEN UPPER(type) = 'DEBIT' THEN 'debit'
    WHEN UPPER(type) = 'CREDIT' THEN 'credit'
    WHEN UPPER(type) = 'ON_HOLD' THEN 'debit'
    WHEN UPPER(type) = 'RELEASE' THEN 'credit'
    ELSE 'debit'
END WHERE direction IS NULL;

-- Direction validation is enforced at the application layer (not via CHECK constraint)
-- to avoid blocking INSERTs during rolling updates when v3.5.3 messages arrive
-- without a direction field. See inferDirectionFromType in the Go service layer.
ALTER TABLE operation DROP CONSTRAINT IF EXISTS chk_operation_direction;
