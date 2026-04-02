ALTER TABLE operation_route DROP CONSTRAINT IF EXISTS operation_route_operation_type_check;
ALTER TABLE operation_route ADD CONSTRAINT operation_route_operation_type_check CHECK (LOWER(operation_type) IN ('source', 'destination'));
