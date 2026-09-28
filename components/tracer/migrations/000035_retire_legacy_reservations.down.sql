-- Retired legacy reservations cannot be restored: their holds were returned to
-- the counters, and no writer for rows without a decision remains to settle
-- them. Refuse instead of reintroducing rows the service can no longer handle.
DO $migration$
BEGIN
    RAISE EXCEPTION 'retired legacy reservations cannot be restored'
        USING ERRCODE = '0A000';
END;
$migration$;
