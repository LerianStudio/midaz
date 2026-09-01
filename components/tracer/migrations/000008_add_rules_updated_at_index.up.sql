-- Migration: 000008_add_rules_updated_at_index
-- Purpose: Add B-tree index on rules.updated_at for efficient polling queries.
-- Used by GetRulesUpdatedSince to find rules changed since last sync.

CREATE INDEX IF NOT EXISTS idx_rules_updated_at ON rules (updated_at);
