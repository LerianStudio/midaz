SET LOCAL lock_timeout = '5s';
DROP TRIGGER protect_bound_limit_scopes ON limits;
DROP FUNCTION protect_bound_limit_scopes();
