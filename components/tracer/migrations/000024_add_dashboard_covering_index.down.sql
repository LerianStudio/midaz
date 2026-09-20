-- Reverses 000024_add_dashboard_covering_index.
--
-- CONCURRENTLY for the same reason the build uses it: a plain DROP INDEX takes
-- an ACCESS EXCLUSIVE lock on transaction_validations and would stall the
-- validate hot path. Dropping it costs the dashboard its index-only plans; it
-- costs correctness nothing, because the reads fall back to
-- idx_transaction_validations_created plus a heap fetch.

DROP INDEX CONCURRENTLY IF EXISTS idx_transaction_validations_dashboard;
