-- Drop accounting_entries column from operation_route table
ALTER TABLE operation_route DROP COLUMN IF EXISTS accounting_entries;
