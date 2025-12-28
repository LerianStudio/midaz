BEGIN;

-- Rollback: Convert back to BIGINT (LOSSY operation)
-- WARNING: Rates with more decimal places than scale will lose precision.
-- Example: rate=5.789 with scale=2 becomes 579 (rounds 578.9)

UPDATE asset_rate
SET rate = rate * POWER(10, rate_scale)
WHERE rate_scale > 0;

ALTER TABLE asset_rate
    ALTER COLUMN rate TYPE BIGINT USING ROUND(rate)::BIGINT;

COMMIT;
