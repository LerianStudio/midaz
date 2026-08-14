-- ============================================
-- Migration: 000012_add_validation_request_id_unique
-- Description: Add UNIQUE constraint on transaction_validations(request_id)
--              to support idempotency for validation requests
-- Date: 2026-03-18
-- ============================================

-- Remove the non-unique index created in migration 000001
-- A unique index satisfies all queries the non-unique index would, so this is redundant
-- Keeping both would waste disk space and cause write amplification on INSERTs/UPDATEs
DROP INDEX IF EXISTS idx_transaction_validations_request_id;

-- Add UNIQUE constraint on request_id to prevent duplicate validation records
-- This enables idempotent validation requests - if a request_id already exists,
-- we can return the existing validation result instead of creating a duplicate.
CREATE UNIQUE INDEX IF NOT EXISTS idx_transaction_validations_request_id_unique 
    ON transaction_validations(request_id);
