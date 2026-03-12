ALTER TABLE operation ADD COLUMN IF NOT EXISTS route_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_operation_route_id'
    ) THEN
        ALTER TABLE operation ADD CONSTRAINT fk_operation_route_id FOREIGN KEY (route_id) REFERENCES operation_route(id);
    END IF;
END $$;
