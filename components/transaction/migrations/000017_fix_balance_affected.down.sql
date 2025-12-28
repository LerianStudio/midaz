-- Down migration: Revert balance_affected fix
--
-- WARNING: This migration cannot accurately restore the original state because
-- we don't track which operations were originally set to false incorrectly.
--
-- This is a DATA REPAIR migration - rolling back would reintroduce the bug.
-- The down migration is intentionally a no-op for safety.

-- No-op: Data repair migrations should not be reversed
SELECT 1;
