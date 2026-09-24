-- Enum values remain readable together with immutable hash-chained history.
DO $$ BEGIN
    RAISE NOTICE 'Operation completion audit values retained to preserve audit history';
END $$;
