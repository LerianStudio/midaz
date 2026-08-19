ALTER TABLE transaction_validations
    ALTER COLUMN processing_time_ms TYPE INTEGER
    USING ROUND(processing_time_ms)::INTEGER;
