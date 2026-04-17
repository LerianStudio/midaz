-- Note: Dropping pg_trgm will fail if other objects depend on it
-- Only drop if no dependent indexes exist
DROP EXTENSION IF EXISTS pg_trgm;
