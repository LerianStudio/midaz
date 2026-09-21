-- recorded_at is the server-side instant when the ledger applied or persisted
-- the operation. It never comes from the client. The nullable column
-- column keeps historical rows untouched; PIT reads fall back to created_at.
ALTER TABLE operation ADD COLUMN IF NOT EXISTS recorded_at TIMESTAMP WITH TIME ZONE;
