-- Orphan Transaction Detection Queries
-- These queries identify transactions that exist without corresponding operations,
-- which indicates a data integrity issue that should not occur after the atomicity fix.
--
-- Run these queries against the transaction database to detect orphans.

-- =============================================================================
-- Query 1: Find orphan transactions (transactions with no operations)
-- =============================================================================
-- This is the primary orphan detection query.
-- Expected result after atomicity fix: 0 rows (no orphans)
-- If rows are returned, these transactions need investigation.

SELECT
    t.id AS transaction_id,
    t.organization_id,
    t.ledger_id,
    t.status,
    t.status_description,
    t.amount,
    t.asset_code,
    t.created_at,
    t.updated_at
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id
WHERE
    o.id IS NULL
    AND t.deleted_at IS NULL
    AND t.status != 'NOTED'  -- NOTED transactions may legitimately have no operations
ORDER BY
    t.created_at DESC;


-- =============================================================================
-- Query 2: Orphan count summary by organization and ledger
-- =============================================================================
-- Use this to understand the scope of orphan transactions.

SELECT
    t.organization_id,
    t.ledger_id,
    t.status,
    COUNT(*) AS orphan_count,
    MIN(t.created_at) AS earliest_orphan,
    MAX(t.created_at) AS latest_orphan
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id
WHERE
    o.id IS NULL
    AND t.deleted_at IS NULL
    AND t.status != 'NOTED'
GROUP BY
    t.organization_id, t.ledger_id, t.status
ORDER BY
    orphan_count DESC;


-- =============================================================================
-- Query 3: Orphan transactions created in a specific time window
-- =============================================================================
-- Useful for investigating orphans created during a specific incident.
-- Adjust the date range as needed.

SELECT
    t.id AS transaction_id,
    t.organization_id,
    t.ledger_id,
    t.status,
    t.amount,
    t.asset_code,
    t.created_at
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id
WHERE
    o.id IS NULL
    AND t.deleted_at IS NULL
    AND t.status != 'NOTED'
    AND t.created_at >= NOW() - INTERVAL '24 hours'  -- Adjust as needed
ORDER BY
    t.created_at DESC;


-- =============================================================================
-- Query 4: Transactions with partial operations (should have 2+ but have fewer)
-- =============================================================================
-- Identifies transactions that may have lost some operations.
-- Note: This requires knowledge of expected operation count, typically 2 (debit + credit).

SELECT
    t.id AS transaction_id,
    t.organization_id,
    t.ledger_id,
    t.status,
    t.amount,
    t.asset_code,
    COUNT(o.id) AS operation_count,
    t.created_at
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
WHERE
    t.deleted_at IS NULL
    AND t.status NOT IN ('NOTED', 'PENDING')  -- Completed transactions should have operations
GROUP BY
    t.id, t.organization_id, t.ledger_id, t.status, t.amount, t.asset_code, t.created_at
HAVING
    COUNT(o.id) = 1  -- Only one operation when typically there should be 2
ORDER BY
    t.created_at DESC;


-- =============================================================================
-- Query 5: Health check - total orphan count
-- =============================================================================
-- Quick health check query for monitoring dashboards.

SELECT
    COUNT(*) AS total_orphan_transactions
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id
WHERE
    o.id IS NULL
    AND t.deleted_at IS NULL
    AND t.status NOT IN ('NOTED', 'PENDING');


-- =============================================================================
-- Query 6: Orphans with related balance information
-- =============================================================================
-- Shows orphan transactions along with any balance entries that may have been
-- affected (useful for understanding impact).

SELECT
    t.id AS transaction_id,
    t.organization_id,
    t.ledger_id,
    t.status,
    t.amount,
    t.asset_code,
    t.created_at,
    (
        SELECT COUNT(*)
        FROM balance b
        WHERE b.organization_id = t.organization_id
        AND b.ledger_id = t.ledger_id
        AND b.asset_code = t.asset_code
    ) AS related_balance_count
FROM
    transaction t
LEFT JOIN
    operation o ON t.id = o.transaction_id
WHERE
    o.id IS NULL
    AND t.deleted_at IS NULL
    AND t.status != 'NOTED'
ORDER BY
    t.created_at DESC
LIMIT 100;
