BEGIN;

-- Migration: Change asset_rate.rate from BIGINT to NUMERIC for decimal precision
-- CRITICAL: This migration includes mandatory data conversion

-- Step 1: Handle NULL scale values (defensive)
UPDATE asset_rate
SET rate_scale = 0
WHERE rate_scale IS NULL;

-- Step 2: Convert scaled integers to direct decimal values BEFORE type change
UPDATE asset_rate
SET rate = rate / POWER(10, rate_scale)
WHERE rate_scale > 0;

-- Step 3: Alter column type
ALTER TABLE asset_rate
    ALTER COLUMN rate TYPE NUMERIC(38, 18);

-- Step 4: Document the change
COMMENT ON COLUMN asset_rate.rate IS 'Direct decimal rate value (e.g., 5.25). Migrated from scaled integer.';

COMMIT;
