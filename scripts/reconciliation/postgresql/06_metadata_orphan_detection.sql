-- ============================================================================
-- MIDAZ RECONCILIATION SCRIPT: Metadata Orphan Detection
-- ============================================================================
-- Purpose: Identify PostgreSQL entities without corresponding MongoDB metadata
--          This can occur when MongoDB metadata creation fails after PostgreSQL commits
-- Database: transaction (run on PostgreSQL, compare with MongoDB)
-- Frequency: Daily or after incidents
-- Expected: 0 orphan entities (all PG entities should have MongoDB metadata)
-- ============================================================================

-- ============================================================================
-- SECTION 1: Transactions Without Metadata
-- ============================================================================
SELECT
    '--- TRANSACTIONS WITHOUT METADATA ---' as report_section;

-- NOTE: This query identifies transactions that may be missing MongoDB metadata.
-- Cross-database validation requires application-level checking via MongoDB queries.
-- This query provides the list of transaction IDs to check against MongoDB.

SELECT
    t.id as transaction_id,
    t.organization_id,
    t.ledger_id,
    t.status,
    t.amount,
    t.asset_code,
    t.created_at,
    t.updated_at,
    '{ "entity_id": "' || t.id || '", "entity_name": "Transaction" }' as mongodb_lookup_query
FROM transaction t
WHERE t.deleted_at IS NULL
  AND t.status NOT IN ('NOTED', 'PENDING')  -- These may legitimately have no metadata
ORDER BY t.created_at DESC
LIMIT 100;

-- ============================================================================
-- SECTION 2: Operations Without Metadata (Likely)
-- ============================================================================
SELECT
    '--- OPERATIONS POTENTIALLY WITHOUT METADATA ---' as report_section;

SELECT
    o.id as operation_id,
    o.transaction_id,
    o.account_alias,
    o.type,
    o.amount,
    o.asset_code,
    o.created_at,
    '{ "entity_id": "' || o.id || '", "entity_name": "Operation" }' as mongodb_lookup_query
FROM operation o
WHERE o.deleted_at IS NULL
  AND o.status != 'PENDING'
ORDER BY o.created_at DESC
LIMIT 100;

-- ============================================================================
-- SECTION 3: Recently Created Entities (High Risk for Metadata Gaps)
-- ============================================================================
SELECT
    '--- RECENTLY CREATED ENTITIES (CHECK METADATA) ---' as report_section;

-- Entities created in the last hour are high-risk for metadata gaps
-- if there were MongoDB failures or network issues

SELECT
    'transaction' as entity_type,
    t.id as entity_id,
    t.created_at,
    EXTRACT(EPOCH FROM (NOW() - t.created_at)) as age_seconds
FROM transaction t
WHERE t.deleted_at IS NULL
  AND t.created_at >= NOW() - INTERVAL '1 hour'

UNION ALL

SELECT
    'operation' as entity_type,
    o.id as entity_id,
    o.created_at,
    EXTRACT(EPOCH FROM (NOW() - o.created_at)) as age_seconds
FROM operation o
WHERE o.deleted_at IS NULL
  AND o.created_at >= NOW() - INTERVAL '1 hour'

ORDER BY age_seconds DESC;

-- ============================================================================
-- SECTION 4: Metadata Consistency Health Score
-- ============================================================================
SELECT
    '--- METADATA CONSISTENCY HEALTH SCORE ---' as report_section;

-- This provides aggregate metrics for monitoring dashboards
WITH entity_counts AS (
    SELECT
        'transaction' as entity_type,
        COUNT(*) as total_entities,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours') as entities_last_24h
    FROM transaction
    WHERE deleted_at IS NULL

    UNION ALL

    SELECT
        'operation' as entity_type,
        COUNT(*) as total_entities,
        COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '24 hours') as entities_last_24h
    FROM operation
    WHERE deleted_at IS NULL
)
SELECT
    entity_type,
    total_entities,
    entities_last_24h,
    CASE
        WHEN entities_last_24h = 0 THEN 'HEALTHY (no recent activity)'
        ELSE 'CHECK MONGODB (metadata verification required)'
    END as metadata_status
FROM entity_counts;

-- ============================================================================
-- SECTION 5: Telemetry Output (JSON)
-- ============================================================================
SELECT
    '--- TELEMETRY OUTPUT (JSON) ---' as report_section;

SELECT json_build_object(
    'check_type', 'metadata_orphan_detection',
    'timestamp', NOW(),
    'transaction_count', (SELECT COUNT(*) FROM transaction WHERE deleted_at IS NULL),
    'operation_count', (SELECT COUNT(*) FROM operation WHERE deleted_at IS NULL),
    'transactions_last_hour', (SELECT COUNT(*) FROM transaction WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL '1 hour'),
    'operations_last_hour', (SELECT COUNT(*) FROM operation WHERE deleted_at IS NULL AND created_at >= NOW() - INTERVAL '1 hour'),
    'mongodb_check_required', true,
    'status', 'REQUIRES_CROSS_DB_VALIDATION'
) as telemetry_data;

-- ============================================================================
-- INSTRUCTIONS FOR CROSS-DATABASE VALIDATION
-- ============================================================================
-- Run this query to get transaction IDs, then verify in MongoDB:
--
-- In MongoDB shell:
-- db.transaction.find({ entity_id: { $in: ["<transaction_id_1>", "<transaction_id_2>"] } })
--
-- If count(MongoDB results) < count(PostgreSQL results), you have orphaned metadata
-- ============================================================================
