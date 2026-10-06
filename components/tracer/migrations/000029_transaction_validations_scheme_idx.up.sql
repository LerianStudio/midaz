-- ============================================
-- Migration: 000029_transaction_validations_scheme_idx
-- Description: Index the effective scheme of a validated transaction,
--              transaction_validation_scheme(scheme, transaction_type), so
--              filters on the scheme read one index whether the row was
--              written with the free-form column or with the enum column
--              alone. A filter reaches this index only by calling the same
--              function; a bare COALESCE over the two columns does not.
-- Date: 2026-10-06
-- ============================================
--
-- CONCURRENTLY, like 000024 (idx_transaction_validations_dashboard): a plain
-- CREATE INDEX holds a lock that blocks the validate path for the duration of
-- the build. CONCURRENTLY cannot run inside a transaction block, and the runner
-- sends each file as one simple query that PostgreSQL wraps in an implicit
-- transaction whenever it carries more than one statement, so this file holds
-- exactly one statement. IF NOT EXISTS keeps a replay a clean no-op (Migration
-- Renumbering Invariant, docs/tracer/INVARIANTS.md).
--
-- The index is a B-tree over a short text, the same shape as
-- idx_transaction_validations_transaction_type, which 000032 drops once no
-- read filters on the bare enum column, so a validation insert keeps paying
-- for one scheme index, not two.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_validations_scheme
ON transaction_validations ((transaction_validation_scheme(scheme, transaction_type)));
