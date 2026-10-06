-- ============================================
-- Migration: 000032_drop_transaction_validations_transaction_type_idx (DOWN)
-- Description: Recreate the transaction_type index exactly as 000004 built
--              it.
-- Date: 2026-10-06
-- ============================================
--
-- CONCURRENTLY: a plain CREATE INDEX holds a lock that blocks the validate path
-- for the duration of the build. This file holds exactly one statement so the
-- runner does not wrap it in an implicit transaction. IF NOT EXISTS keeps a
-- replay a clean no-op.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_validations_transaction_type ON transaction_validations(transaction_type);
