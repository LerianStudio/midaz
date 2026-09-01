-- ============================================
-- Migration: 000011_add_counter_expires_at
-- Description: Add expires_at column to usage_counters for efficient cleanup
-- Date: 2026-03-10
-- ============================================

-- Add expires_at column to store when the counter should be cleaned up
-- This enables efficient batch cleanup of expired counters
ALTER TABLE usage_counters ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP WITH TIME ZONE;

-- Index for cleanup queries (finding expired counters)
-- Partial index only includes rows that have an expiration set
CREATE INDEX IF NOT EXISTS idx_usage_counters_expires_at 
    ON usage_counters(expires_at) 
    WHERE expires_at IS NOT NULL;
