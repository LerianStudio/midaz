-- Refuses while any active link is optional: dropping the column turns those
-- links required again, and every transaction that leaves them unused would be
-- refused. Make the links required, or remove them, before rolling back. A
-- database without the column, or without optional links, rolls back as usual.
DO $$
DECLARE
    optional_links bigint;
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = current_schema()
          AND table_name = 'operation_transaction_route'
          AND column_name = 'optional'
    ) THEN
        EXECUTE 'SELECT count(*) FROM operation_transaction_route WHERE optional AND deleted_at IS NULL'
            INTO optional_links;

        IF optional_links > 0 THEN
            RAISE EXCEPTION 'operation_transaction_route has % active optional links; make them required or remove them before rolling back', optional_links;
        END IF;
    END IF;
END
$$;

ALTER TABLE operation_transaction_route DROP COLUMN IF EXISTS optional;
