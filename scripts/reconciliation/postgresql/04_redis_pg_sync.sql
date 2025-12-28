-- ============================================================================
-- MIDAZ RECONCILIATION SCRIPT: Redis-PostgreSQL Balance Sync Validation
-- ============================================================================
-- Purpose: Detect potential issues with Redis-to-PostgreSQL balance synchronization
--          Identifies balances that may have stale data or sync issues
-- Database: transaction
-- Frequency: Hourly or after BalanceSyncWorker runs
-- Expected: All balance versions should match operation counts
--
-- Note: This script cannot directly query Redis, but can detect symptoms
--       of sync issues by analyzing PostgreSQL data patterns
-- ============================================================================

-- ============================================================================
-- SECTION 1: Balance Version Analysis
-- ============================================================================
SELECT
    '--- BALANCE VERSION ANALYSIS ---' as report_section;

-- Balances where version doesn't match operation count
-- This could indicate Redis updates not yet persisted or sync issues
WITH balance_ops AS (
    SELECT
        b.id as balance_id,
        b.account_id,
        b.alias,
        b.asset_code,
        b.key as balance_key,
        b.version as db_version,
        b.available as db_available,
        b.on_hold as db_on_hold,
        b.updated_at as last_db_update,
        COUNT(DISTINCT o.id) FILTER (WHERE o.balance_affected = true) as affecting_op_count,
        MAX(o.balance_version_after) as max_op_version,
        MAX(o.created_at) as last_operation_time
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id
        AND b.asset_code = o.asset_code
        AND b.key = o.balance_key
        AND o.deleted_at IS NULL
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.account_id, b.alias, b.asset_code, b.key,
             b.version, b.available, b.on_hold, b.updated_at
)
SELECT
    balance_id,
    alias,
    asset_code,
    db_version,
    affecting_op_count,
    max_op_version,
    CASE
        WHEN db_version < COALESCE(max_op_version, 0) THEN 'STALE_VERSION'
        WHEN db_version > COALESCE(max_op_version, 0) + 1 THEN 'VERSION_GAP'
        ELSE 'OK'
    END as version_status,
    last_db_update,
    last_operation_time,
    EXTRACT(EPOCH FROM (NOW() - last_db_update)) as seconds_since_update
FROM balance_ops
WHERE db_version != COALESCE(max_op_version, 0)
  AND affecting_op_count > 0
ORDER BY ABS(db_version - COALESCE(max_op_version, 0)) DESC
LIMIT 50;

-- ============================================================================
-- SECTION 2: Stale Balance Detection
-- ============================================================================
SELECT
    '--- POTENTIALLY STALE BALANCES ---' as report_section;

-- Balances that haven't been updated recently but have recent operations
-- Could indicate Redis sync worker issues
SELECT
    b.id as balance_id,
    b.alias,
    b.asset_code,
    b.version,
    b.available,
    b.updated_at as balance_updated,
    MAX(o.created_at) as last_operation,
    EXTRACT(EPOCH FROM (MAX(o.created_at) - b.updated_at)) as operation_ahead_seconds
FROM balance b
JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND b.key = o.balance_key
    AND o.deleted_at IS NULL
    AND o.balance_affected = true
WHERE b.deleted_at IS NULL
GROUP BY b.id, b.alias, b.asset_code, b.version, b.available, b.updated_at
HAVING MAX(o.created_at) > b.updated_at + INTERVAL '5 minutes'
ORDER BY (MAX(o.created_at) - b.updated_at) DESC
LIMIT 50;

-- ============================================================================
-- SECTION 3: Balance Version Gaps
-- ============================================================================
SELECT
    '--- BALANCE VERSION GAPS ---' as report_section;

-- Find gaps in balance_version_after sequence for each balance
-- Gaps could indicate lost operations or sync issues
WITH version_sequence AS (
    SELECT
        o.account_id,
        o.asset_code,
        o.balance_key,
        o.balance_version_before,
        o.balance_version_after,
        LAG(o.balance_version_after) OVER (
            PARTITION BY o.account_id, o.asset_code, o.balance_key
            ORDER BY o.created_at
        ) as prev_version_after
    FROM operation o
    WHERE o.deleted_at IS NULL
      AND o.balance_affected = true
)
SELECT
    account_id,
    asset_code,
    balance_key,
    prev_version_after as expected_before,
    balance_version_before as actual_before,
    balance_version_after,
    (balance_version_before - prev_version_after) as gap_size
FROM version_sequence
WHERE prev_version_after IS NOT NULL
  AND balance_version_before != prev_version_after
LIMIT 50;

-- ============================================================================
-- SECTION 4: Balance Snapshot Consistency
-- ============================================================================
SELECT
    '--- OPERATION BALANCE SNAPSHOTS ---' as report_section;

-- Check if balance snapshots in operations are consistent
-- available_balance + amount should equal available_balance_after (for credits)
SELECT
    o.id as operation_id,
    o.account_alias,
    o.type,
    o.amount,
    o.available_balance as snapshot_before,
    o.available_balance_after as snapshot_after,
    CASE
        WHEN o.type = 'CREDIT' THEN o.available_balance + o.amount
        WHEN o.type = 'DEBIT' THEN o.available_balance - o.amount
    END as expected_after,
    o.available_balance_after - CASE
        WHEN o.type = 'CREDIT' THEN o.available_balance + o.amount
        WHEN o.type = 'DEBIT' THEN o.available_balance - o.amount
    END as snapshot_discrepancy
FROM operation o
WHERE o.deleted_at IS NULL
  AND o.balance_affected = true
  AND o.available_balance_after != CASE
      WHEN o.type = 'CREDIT' THEN o.available_balance + o.amount
      WHEN o.type = 'DEBIT' THEN o.available_balance - o.amount
  END
ORDER BY o.created_at DESC
LIMIT 50;

-- ============================================================================
-- SECTION 5: High-Frequency Accounts
-- ============================================================================
SELECT
    '--- HIGH-FREQUENCY ACCOUNTS (potential hot spots) ---' as report_section;

-- Identify accounts with high operation frequency
-- These are more susceptible to race conditions and cache inconsistencies
SELECT
    b.account_id,
    b.alias,
    b.asset_code,
    b.version as current_version,
    COUNT(o.id) as total_operations,
    COUNT(o.id) FILTER (WHERE o.created_at > NOW() - INTERVAL '1 hour') as ops_last_hour,
    COUNT(o.id) FILTER (WHERE o.created_at > NOW() - INTERVAL '24 hours') as ops_last_24h,
    MAX(o.created_at) as last_operation
FROM balance b
JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND b.key = o.balance_key
    AND o.deleted_at IS NULL
WHERE b.deleted_at IS NULL
GROUP BY b.account_id, b.alias, b.asset_code, b.version
HAVING COUNT(o.id) FILTER (WHERE o.created_at > NOW() - INTERVAL '1 hour') > 10
ORDER BY ops_last_hour DESC
LIMIT 20;

-- ============================================================================
-- SECTION 6: External Account Analysis
-- ============================================================================
SELECT
    '--- EXTERNAL ACCOUNT ANALYSIS (@external/*) ---' as report_section;

-- External accounts are high-traffic and critical for system balance
SELECT
    b.alias,
    b.asset_code,
    b.available as current_balance,
    b.version,
    COUNT(o.id) as total_operations,
    SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END) as total_credits,
    SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END) as total_debits,
    SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END) -
    SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END) as calculated_balance,
    b.available - (
        SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END) -
        SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END)
    ) as discrepancy
FROM balance b
LEFT JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND b.key = o.balance_key
    AND o.deleted_at IS NULL
    AND o.balance_affected = true
WHERE b.deleted_at IS NULL
  AND b.alias LIKE '@external/%'
GROUP BY b.alias, b.asset_code, b.available, b.version
ORDER BY b.alias;

-- ============================================================================
-- SECTION 7: Telemetry Output (JSON)
-- ============================================================================
SELECT
    '--- TELEMETRY OUTPUT (JSON) ---' as report_section;

WITH stats AS (
    SELECT
        COUNT(*) FILTER (WHERE b.version != COALESCE(max_version, 0) AND op_count > 0) as version_mismatches,
        COUNT(*) FILTER (WHERE b.updated_at < last_op - INTERVAL '5 minutes') as stale_balances,
        COUNT(*) FILTER (WHERE b.alias LIKE '@external/%') as external_accounts
    FROM balance b
    LEFT JOIN (
        SELECT
            account_id,
            asset_code,
            balance_key,
            COUNT(*) as op_count,
            MAX(balance_version_after) as max_version,
            MAX(created_at) as last_op
        FROM operation
        WHERE deleted_at IS NULL AND balance_affected = true
        GROUP BY account_id, asset_code, balance_key
    ) ops ON b.account_id = ops.account_id
        AND b.asset_code = ops.asset_code
        AND b.key = ops.balance_key
    WHERE b.deleted_at IS NULL
)
SELECT json_build_object(
    'check_type', 'redis_pg_sync',
    'timestamp', NOW(),
    'version_mismatches', version_mismatches,
    'stale_balances', stale_balances,
    'external_accounts_monitored', external_accounts,
    'status', CASE
        WHEN version_mismatches = 0 AND stale_balances = 0 THEN 'HEALTHY'
        WHEN version_mismatches > 10 OR stale_balances > 5 THEN 'CRITICAL'
        ELSE 'WARNING'
    END
) as telemetry_data
FROM stats;
