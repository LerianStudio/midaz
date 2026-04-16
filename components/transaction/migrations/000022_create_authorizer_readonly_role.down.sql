-- Revoke grants and drop the least-privilege authorizer role. Safe to
-- re-run: REVOKE against a non-existent role is a no-op when wrapped in
-- IF EXISTS semantics. We handle the DROP ROLE last so revoke-then-drop
-- succeeds even if external grants were added out of band.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'midaz_authorizer_ro') THEN
        REVOKE SELECT ON TABLE balance FROM midaz_authorizer_ro;
        REVOKE SELECT ON TABLE operation FROM midaz_authorizer_ro;
        REVOKE USAGE ON SCHEMA public FROM midaz_authorizer_ro;

        EXECUTE format('REVOKE CONNECT ON DATABASE %I FROM midaz_authorizer_ro', current_database());

        DROP ROLE midaz_authorizer_ro;
    END IF;
END $$;
