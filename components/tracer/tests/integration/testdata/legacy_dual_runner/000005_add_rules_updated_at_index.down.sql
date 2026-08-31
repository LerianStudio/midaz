-- Rollback: 000005_add_rules_updated_at_index

DROP INDEX IF EXISTS idx_rules_updated_at;
