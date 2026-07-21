-- ============================================
-- Migration: 000013_add_name_unique_constraints
-- Description: Add name uniqueness constraints for rules (context-scoped) and limits (global)
-- Date: 2026-03-23
-- ============================================

-- =============================================================================
-- RULES: Context-Scoped Name Uniqueness
-- =============================================================================
-- Rules should have unique names within a context. The context is derived from
-- the first scope's segmentId. Rules with no scopes (global rules) share a NULL
-- context and must still have unique names among global rules.
--
-- We add a context_id column to store this derived value for efficient indexing.
-- =============================================================================

-- Add context_id column to rules table
-- This stores the first scope's segmentId for uniqueness constraint
ALTER TABLE rules ADD COLUMN IF NOT EXISTS context_id UUID;

-- Populate context_id using the smallest segmentId across scopes (deterministic)
UPDATE rules
SET context_id = (
    SELECT (MIN(elem->>'segmentId'))::uuid
    FROM jsonb_array_elements(scopes) AS elem
    WHERE elem->>'segmentId' IS NOT NULL
      AND elem->>'segmentId' ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
)
WHERE scopes IS NOT NULL
  AND jsonb_array_length(scopes) > 0;

-- Create partial unique index for context-scoped rule name uniqueness
-- Only applies to non-DELETED rules
-- NULLS NOT DISTINCT ensures global rules (NULL context_id) also have unique names
CREATE UNIQUE INDEX IF NOT EXISTS idx_rules_name_per_context_active
    ON rules (context_id, name)
    NULLS NOT DISTINCT
    WHERE status != 'DELETED';

-- =============================================================================
-- LIMITS: Global Name Uniqueness
-- =============================================================================
-- Limits should have globally unique names among non-deleted limits.
-- This is simpler than rules - just a partial unique index on name.
-- =============================================================================

-- Create partial unique index for global limit name uniqueness
-- Only applies to non-DELETED limits
CREATE UNIQUE INDEX IF NOT EXISTS idx_limits_name_active
    ON limits (name)
    WHERE status != 'DELETED';
