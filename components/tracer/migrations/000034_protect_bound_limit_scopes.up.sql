SET LOCAL lock_timeout = '5s';

-- The binding attests this exact account set. Updates must not transfer it
-- to different accounts without a new attestation and a new limit identity.
CREATE FUNCTION protect_bound_limit_scopes() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.scopes IS DISTINCT FROM OLD.scopes
       AND EXISTS (SELECT 1 FROM limit_asset_references WHERE limit_id = OLD.id) THEN
        RAISE EXCEPTION 'bound limit account scopes are immutable'
            USING ERRCODE = '23514', CONSTRAINT = 'bound_limit_scopes_immutable';
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER protect_bound_limit_scopes
    BEFORE UPDATE OF scopes ON limits
    FOR EACH ROW EXECUTE FUNCTION protect_bound_limit_scopes();
