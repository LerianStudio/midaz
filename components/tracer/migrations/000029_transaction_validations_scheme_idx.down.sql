-- ============================================
-- Migration: 000029_transaction_validations_scheme_idx (DOWN)
-- Description: Drop the effective-scheme expression index.
-- Date: 2026-10-06
-- ============================================
--
-- CONCURRENTLY for the same reason the build uses it: a plain DROP INDEX takes
-- an ACCESS EXCLUSIVE lock on transaction_validations and would stall the
-- validate hot path. This file holds exactly one statement so the runner does
-- not wrap it in an implicit transaction. Filters on the scheme fall back to
-- idx_transaction_validations_transaction_type plus a heap fetch.

DROP INDEX CONCURRENTLY IF EXISTS idx_transaction_validations_scheme;
