ALTER TABLE transaction_validations
    ALTER COLUMN processing_time_ms TYPE DOUBLE PRECISION
    USING processing_time_ms::DOUBLE PRECISION;

ALTER TABLE transaction_validations
    DROP CONSTRAINT IF EXISTS transaction_validations_processing_time_ms_check;

ALTER TABLE transaction_validations
    ADD CONSTRAINT transaction_validations_processing_time_ms_check
    CHECK (processing_time_ms >= 0);
