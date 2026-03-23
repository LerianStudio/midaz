-- Add route_code TEXT column to operation table for accounting traceability
ALTER TABLE operation ADD COLUMN IF NOT EXISTS route_code TEXT;
