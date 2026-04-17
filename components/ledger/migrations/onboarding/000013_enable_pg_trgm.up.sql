-- Enable pg_trgm extension for trigram-based text search
-- Required for efficient ILIKE prefix matching on name and alias columns

CREATE EXTENSION IF NOT EXISTS pg_trgm;
