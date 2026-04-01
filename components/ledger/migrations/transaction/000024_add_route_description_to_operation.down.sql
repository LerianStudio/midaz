-- Drop route_description column from operation table
ALTER TABLE operation DROP COLUMN IF EXISTS route_description;
