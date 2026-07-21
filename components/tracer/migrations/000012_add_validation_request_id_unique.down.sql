-- ============================================
-- Migration: 000012_add_validation_request_id_unique (ROLLBACK)
-- Description: Remove UNIQUE constraint on transaction_validations(request_id)
-- Date: 2026-03-18
-- ============================================

-- Remove the UNIQUE index on request_id
DROP INDEX IF EXISTS idx_transaction_validations_request_id_unique;

-- Restore the original non-unique index from migration 000001
CREATE INDEX IF NOT EXISTS idx_transaction_validations_request_id 
    ON transaction_validations(request_id);
