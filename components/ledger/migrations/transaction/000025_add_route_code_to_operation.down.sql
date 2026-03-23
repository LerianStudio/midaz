-- Drop route_code column from operation table
ALTER TABLE operation DROP COLUMN IF EXISTS route_code;
