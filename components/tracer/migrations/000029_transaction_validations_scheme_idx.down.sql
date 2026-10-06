-- ============================================
-- Migration: 000029_transaction_validations_scheme_idx (DOWN)
-- Description: Drop the effective-scheme expression index.
-- Date: 2026-10-06
-- ============================================
--
-- CONCURRENTLY for the same reason the build uses it: a plain DROP INDEX takes
-- an ACCESS EXCLUSIVE lock on transaction_validations and would stall the
-- validate hot path. This file holds exactly one statement so the runner does
-- not wrap it in an implicit transaction. Filters on the scheme call
-- transaction_validation_scheme(scheme, transaction_type), which no other
-- index carries, so without this one they scan the rows their other
-- predicates select.

DROP INDEX CONCURRENTLY IF EXISTS idx_transaction_validations_scheme;
