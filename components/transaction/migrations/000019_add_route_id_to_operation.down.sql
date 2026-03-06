ALTER TABLE operation DROP CONSTRAINT IF EXISTS fk_operation_route_id;
ALTER TABLE operation DROP COLUMN IF EXISTS route_id;
