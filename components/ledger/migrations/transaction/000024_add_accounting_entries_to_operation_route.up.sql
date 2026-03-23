-- Add accounting_entries JSONB column to operation_route table
ALTER TABLE operation_route ADD COLUMN IF NOT EXISTS accounting_entries JSONB;
