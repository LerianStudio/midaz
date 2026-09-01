-- ============================================
-- Migration: 000019_create_usage_reservations (DOWN)
-- Description: Drop the usage_reservations table and its indexes.
-- Date: 2026-06-05
-- ============================================

-- Indexes are dropped implicitly with the table, but drop them explicitly first
-- to keep the down path symmetric with the up path and tolerant of partial state.
DROP INDEX IF EXISTS idx_usage_reservations_reaper;
DROP INDEX IF EXISTS idx_usage_reservations_request;
DROP TABLE IF EXISTS usage_reservations;
