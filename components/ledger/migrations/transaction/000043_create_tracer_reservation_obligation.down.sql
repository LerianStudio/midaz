-- A binary rollback cannot erase the only evidence needed to complete holds.
DO $$
BEGIN
    LOCK TABLE tracer_reservation_obligation IN ACCESS EXCLUSIVE MODE NOWAIT;
    IF EXISTS (SELECT 1 FROM tracer_reservation_obligation) THEN
        RAISE EXCEPTION 'tracer reservation coordination history prevents rollback' USING ERRCODE = '23514';
    END IF;
    DROP TABLE tracer_reservation_obligation;
END;
$$;
DROP FUNCTION protect_tracer_reservation_obligation();
