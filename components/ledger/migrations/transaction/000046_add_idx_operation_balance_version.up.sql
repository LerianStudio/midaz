-- Serves the top-1-by-version lookup of a balance's operation trail: the trail head read by the
-- balance reconciler and the high-water mark, both ordered by balance_version_after, not time.
-- balance_id is a UUID unique to one balance, so the organization, ledger and account columns
-- stay out of the key; they are filtered on the single row the index returns.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_operation_balance_version
ON operation (balance_id, balance_version_after DESC)
WHERE deleted_at IS NULL AND balance_affected = true;
