-- This migration is intentionally irreversible. Once the legacy tables are
-- dropped their data cannot be recreated from the partitioned tables alone
-- (schema is identical but rows are the same). RAISE EXCEPTION so that any
-- attempted rollback fails loudly rather than silently succeeding with a
-- schema that no longer matches production reality.

DO $$
BEGIN
    RAISE EXCEPTION 'Migration 000024 is irreversible. Legacy tables were permanently dropped. Restore from backup if recovery is required.';
END
$$;
