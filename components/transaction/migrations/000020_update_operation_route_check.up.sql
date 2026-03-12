ALTER TABLE operation_route DROP CONSTRAINT IF EXISTS operation_route_operation_type_check;
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'operation_route_operation_type_check'
    ) THEN
        ALTER TABLE operation_route ADD CONSTRAINT operation_route_operation_type_check CHECK (LOWER(operation_type) IN ('source', 'destination', 'bidirectional'));
    END IF;
END $$;
