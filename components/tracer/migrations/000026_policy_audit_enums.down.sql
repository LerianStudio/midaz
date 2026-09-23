-- Preserve append-only audit rows: PostgreSQL cannot remove enum values without
-- recreating the type and rewriting its dependent columns. As with migration
-- 000020, rollback intentionally retains the values and all recorded events.
DO $$ BEGIN
  RAISE NOTICE 'Policy audit enum values are retained to preserve audit history';
END $$;
