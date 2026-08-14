-- ============================================
-- Migration: 000018_add_reserved_usage_column
-- Description: Add reserved_usage column to usage_counters for the two-phase
--              reservation seam. reserved_usage holds amounts that are reserved
--              (RESERVED reservations) but not yet committed to current_usage.
--              The atomic reserve CTE guards on
--              current_usage + reserved_usage + amount <= maxAmount.
-- Date: 2026-06-05
-- ============================================

-- reserved_usage is stored in the smallest currency unit (e.g., cents), matching
-- current_usage. NOT NULL DEFAULT 0 so existing rows backfill cleanly; the CHECK
-- keeps the bucket non-negative (a release must never drive it below zero).
ALTER TABLE usage_counters
    ADD COLUMN IF NOT EXISTS reserved_usage BIGINT NOT NULL DEFAULT 0 CHECK (reserved_usage >= 0);
