-- Retain immutable audit history on rollback, as in migrations 000020/000026.
DO $$ BEGIN
  RAISE NOTICE 'Policy binding audit enum value is retained to preserve audit history';
END $$;
