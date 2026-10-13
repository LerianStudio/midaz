-- ============================================
-- Migration: 000030_transaction_validations_dashboard_scheme_idx
-- Description: Covering index that keeps the operator dashboard reads
--              index-only now that a validation carries its payment scheme
--              in scheme as well as in transaction_type.
-- Date: 2026-10-06
-- ============================================
--
-- WHY. The fraud-types read groups by transaction_validation_scheme(scheme,
-- transaction_type). idx_transaction_validations_dashboard (000024) carries
-- transaction_type but not scheme, so that read can no longer be answered from
-- the index and pays a heap fetch per row in the window. This index has the
-- same key and the same INCLUDE list plus scheme, so both metrics and
-- fraud-types read every column they touch from the index, and PostgreSQL
-- evaluates the function over the two included columns without visiting the
-- heap. idx_transaction_validations_scheme (000029) cannot do this: it carries
-- only the function result, not created_at, decision or the other payload.
--
-- scheme is INCLUDE payload for the same reason as the 000024 columns: nothing
-- filters or sorts on it here, and VARCHAR(50) bounds its width, so it cannot
-- push a row past the B-tree tuple limit.
--
-- CONCURRENTLY: a plain CREATE INDEX holds a lock that blocks the validate path
-- for the duration of the build. CONCURRENTLY cannot run inside a transaction
-- block, and the runner sends each file as one simple query that PostgreSQL
-- wraps in an implicit transaction whenever it carries more than one statement,
-- so this file holds exactly one statement. IF NOT EXISTS keeps a replay a
-- clean no-op (Migration Renumbering Invariant, docs/tracer/INVARIANTS.md).
--
-- 000031 drops idx_transaction_validations_dashboard once this index exists,
-- so the validate hot path keeps paying for one covering index, not two.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_validations_dashboard_scheme
ON transaction_validations (created_at)
INCLUDE (decision, transaction_type, asset, amount, processing_time_ms, scheme);
