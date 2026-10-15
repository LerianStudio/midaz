-- ============================================
-- Migration: 000030_transaction_validations_dashboard_scheme_idx (DOWN)
-- Description: Drop the scheme-aware dashboard covering index.
-- Date: 2026-10-06
-- ============================================
--
-- CONCURRENTLY for the same reason the build uses it: a plain DROP INDEX takes
-- an ACCESS EXCLUSIVE lock on transaction_validations and would stall the
-- validate hot path. This file holds exactly one statement so the runner does
-- not wrap it in an implicit transaction. The down of 000031 runs first and
-- restores idx_transaction_validations_dashboard, so metrics stays index-only, and
-- fraud-types falls back to a heap fetch per row in the window.

DROP INDEX CONCURRENTLY IF EXISTS idx_transaction_validations_dashboard_scheme;
