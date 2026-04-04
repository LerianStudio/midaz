ALTER TABLE operation ADD COLUMN IF NOT EXISTS route_id UUID;
ALTER TABLE operation DROP CONSTRAINT IF EXISTS fk_operation_route_id;
ALTER TABLE operation ADD CONSTRAINT fk_operation_route_id FOREIGN KEY (route_id) REFERENCES operation_route(id) NOT VALID;
