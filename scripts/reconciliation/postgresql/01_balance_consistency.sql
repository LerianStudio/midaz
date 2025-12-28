-- ============================================================================
-- MIDAZ RECONCILIATION SCRIPT: Balance vs Operations Consistency
-- ============================================================================
-- Purpose: Verify that account balances match the sum of their operations
-- Database: transaction
-- Frequency: Daily or after high-volume transaction periods
-- Expected: All balances should match calculated values (discrepancy = 0)
-- ============================================================================

-- Configuration: Set threshold for what constitutes a significant discrepancy
-- Small rounding errors (< 0.01) may be acceptable due to decimal precision
\set discrepancy_threshold 0.01

-- ============================================================================
-- SECTION 1: Summary Statistics
-- ============================================================================
SELECT
    '--- BALANCE CONSISTENCY SUMMARY ---' as report_section;

WITH balance_calc AS (
    SELECT
        b.id as balance_id,
        b.account_id,
        b.alias,
        b.asset_code,
        b.available as current_balance,
        b.on_hold as current_on_hold,
        b.version,
        b.organization_id,
        b.ledger_id,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as total_credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as total_debits,
        COUNT(o.id) as operation_count
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id
        AND b.asset_code = o.asset_code
        AND b.key = o.balance_key
        AND o.deleted_at IS NULL
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.available, b.on_hold,
             b.version, b.organization_id, b.ledger_id
)
SELECT
    COUNT(*) as total_balances,
    SUM(CASE WHEN ABS(current_balance - (total_credits - total_debits)) > :discrepancy_threshold THEN 1 ELSE 0 END) as balances_with_discrepancy,
    ROUND(100.0 * SUM(CASE WHEN ABS(current_balance - (total_credits - total_debits)) > :discrepancy_threshold THEN 1 ELSE 0 END) / COUNT(*), 2) as discrepancy_percentage,
    SUM(ABS(current_balance - (total_credits - total_debits))) as total_absolute_discrepancy
FROM balance_calc;

-- ============================================================================
-- SECTION 2: Detailed Discrepancy Report
-- ============================================================================
SELECT
    '--- DETAILED DISCREPANCIES ---' as report_section;

WITH balance_calc AS (
    SELECT
        b.id as balance_id,
        b.account_id,
        b.alias,
        b.asset_code,
        b.key as balance_key,
        b.available as current_balance,
        b.on_hold as current_on_hold,
        b.version,
        b.organization_id,
        b.ledger_id,
        b.updated_at as balance_updated_at,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as total_credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as total_debits,
        COUNT(o.id) as operation_count,
        MAX(o.created_at) as last_operation_at
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id
        AND b.asset_code = o.asset_code
        AND b.key = o.balance_key
        AND o.deleted_at IS NULL
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.key, b.available,
             b.on_hold, b.version, b.organization_id, b.ledger_id, b.updated_at
)
SELECT
    balance_id,
    account_id,
    alias,
    asset_code,
    balance_key,
    current_balance,
    total_credits,
    total_debits,
    (total_credits - total_debits) as expected_balance,
    current_balance - (total_credits - total_debits) as discrepancy,
    operation_count,
    version as balance_version,
    balance_updated_at,
    last_operation_at,
    organization_id,
    ledger_id
FROM balance_calc
WHERE ABS(current_balance - (total_credits - total_debits)) > :discrepancy_threshold
ORDER BY ABS(current_balance - (total_credits - total_debits)) DESC
LIMIT 100;

-- ============================================================================
-- SECTION 3: Balances with No Operations (Potential Issues)
-- ============================================================================
SELECT
    '--- BALANCES WITH NO OPERATIONS ---' as report_section;

SELECT
    b.id as balance_id,
    b.account_id,
    b.alias,
    b.asset_code,
    b.available as current_balance,
    b.version,
    b.created_at,
    b.organization_id,
    b.ledger_id
FROM balance b
LEFT JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND o.deleted_at IS NULL
WHERE b.deleted_at IS NULL
  AND b.available != 0
  AND o.id IS NULL
ORDER BY ABS(b.available) DESC
LIMIT 50;

-- ============================================================================
-- SECTION 4: Version Consistency Check
-- ============================================================================
SELECT
    '--- VERSION CONSISTENCY CHECK ---' as report_section;

-- Check if balance version matches the count of operations
WITH version_check AS (
    SELECT
        b.account_id,
        b.alias,
        b.asset_code,
        b.version as balance_version,
        COUNT(DISTINCT o.id) FILTER (WHERE o.balance_affected = true) as affecting_operations
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id
        AND b.asset_code = o.asset_code
        AND b.key = o.balance_key
        AND o.deleted_at IS NULL
    WHERE b.deleted_at IS NULL
    GROUP BY b.account_id, b.alias, b.asset_code, b.version
)
SELECT
    account_id,
    alias,
    asset_code,
    balance_version,
    affecting_operations,
    balance_version - affecting_operations as version_drift
FROM version_check
WHERE balance_version != affecting_operations
LIMIT 50;

-- ============================================================================
-- SECTION 5: Telemetry Output (JSON format for worker consumption)
-- ============================================================================
SELECT
    '--- TELEMETRY OUTPUT (JSON) ---' as report_section;

WITH balance_calc AS (
    SELECT
        b.organization_id,
        b.ledger_id,
        b.available as current_balance,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as expected_balance
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id
        AND b.asset_code = o.asset_code
        AND b.key = o.balance_key
        AND o.deleted_at IS NULL
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.organization_id, b.ledger_id, b.available
)
SELECT json_build_object(
    'check_type', 'balance_consistency',
    'timestamp', NOW(),
    'total_balances', COUNT(*),
    'discrepancies', SUM(CASE WHEN ABS(current_balance - expected_balance) > :discrepancy_threshold THEN 1 ELSE 0 END),
    'total_discrepancy_amount', SUM(ABS(current_balance - expected_balance)),
    'by_organization', (
        SELECT json_agg(json_build_object(
            'org_id', org_id,
            'discrepancy_count', disc_count
        ))
        FROM (
            SELECT organization_id as org_id,
                   SUM(CASE WHEN ABS(current_balance - expected_balance) > :discrepancy_threshold THEN 1 ELSE 0 END) as disc_count
            FROM balance_calc
            GROUP BY organization_id
            HAVING SUM(CASE WHEN ABS(current_balance - expected_balance) > :discrepancy_threshold THEN 1 ELSE 0 END) > 0
        ) org_summary
    )
) as telemetry_data
FROM balance_calc;
