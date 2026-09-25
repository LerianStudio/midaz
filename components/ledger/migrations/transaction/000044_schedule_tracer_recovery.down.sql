DO $$ BEGIN
    RAISE EXCEPTION 'tracer recovery scheduling requires coordinated drainage before downgrade';
END $$;
