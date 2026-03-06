ALTER TABLE operation ADD COLUMN IF NOT EXISTS route_id UUID;

ALTER TABLE operation ADD CONSTRAINT fk_operation_route_id FOREIGN KEY (route_id) REFERENCES operation_route(id);
