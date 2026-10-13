-- ============================================
-- Migration: 000031_drop_transaction_validations_dashboard_idx (DOWN)
-- Description: Recreate the dashboard covering index exactly as 000024 built
--              it.
-- Date: 2026-10-06
-- ============================================
--
-- It is restored before the down of 000030 drops the scheme-aware index, so
-- the metrics read keeps an index-only plan throughout the rollback.
--
-- CONCURRENTLY: a plain CREATE INDEX holds a lock that blocks the validate path
-- for the duration of the build. This file holds exactly one statement so the
-- runner does not wrap it in an implicit transaction. IF NOT EXISTS keeps a
-- replay a clean no-op.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_transaction_validations_dashboard
ON transaction_validations (created_at)
INCLUDE (decision, transaction_type, asset, amount, processing_time_ms);
