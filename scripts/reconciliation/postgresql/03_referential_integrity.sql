-- ============================================================================
-- MIDAZ RECONCILIATION SCRIPT: Cross-Database Referential Integrity
-- ============================================================================
-- Purpose: Verify referential integrity between onboarding and transaction DBs
--          Since they're separate databases, FK constraints don't exist
-- Databases: onboarding + transaction (run against each)
-- Frequency: Weekly or after major data migrations
-- Expected: All foreign key references should resolve
-- ============================================================================

-- ============================================================================
-- SECTION 1: Summary Header
-- ============================================================================
SELECT
    '--- REFERENTIAL INTEGRITY CHECK ---' as report_section,
    'Run these queries against respective databases' as note;

-- ============================================================================
-- PART A: ONBOARDING DATABASE CHECKS
-- Run these against the 'onboarding' database
-- ============================================================================

-- ----------------------------------------------------------------------------
-- A1: Ledgers referencing non-existent organizations
-- ----------------------------------------------------------------------------
SELECT
    '--- A1: ORPHAN LEDGERS (missing organization) ---' as report_section;

SELECT
    l.id as ledger_id,
    l.name as ledger_name,
    l.organization_id as referenced_org_id,
    l.created_at
FROM ledger l
LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL
WHERE l.deleted_at IS NULL
  AND o.id IS NULL;

-- ----------------------------------------------------------------------------
-- A2: Assets referencing non-existent ledgers or organizations
-- ----------------------------------------------------------------------------
SELECT
    '--- A2: ORPHAN ASSETS ---' as report_section;

SELECT
    a.id as asset_id,
    a.code as asset_code,
    a.ledger_id,
    a.organization_id,
    CASE WHEN l.id IS NULL THEN 'MISSING_LEDGER' ELSE 'OK' END as ledger_status,
    CASE WHEN o.id IS NULL THEN 'MISSING_ORG' ELSE 'OK' END as org_status
FROM asset a
LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL
LEFT JOIN organization o ON a.organization_id = o.id AND o.deleted_at IS NULL
WHERE a.deleted_at IS NULL
  AND (l.id IS NULL OR o.id IS NULL);

-- ----------------------------------------------------------------------------
-- A3: Accounts referencing non-existent entities
-- ----------------------------------------------------------------------------
SELECT
    '--- A3: ORPHAN ACCOUNTS ---' as report_section;

SELECT
    acc.id as account_id,
    acc.alias,
    acc.asset_code,
    acc.ledger_id,
    acc.organization_id,
    acc.portfolio_id,
    acc.segment_id,
    acc.parent_account_id,
    CASE WHEN l.id IS NULL THEN 'MISSING' ELSE 'OK' END as ledger_status,
    CASE WHEN o.id IS NULL THEN 'MISSING' ELSE 'OK' END as org_status,
    CASE WHEN acc.portfolio_id IS NOT NULL AND p.id IS NULL THEN 'MISSING' ELSE 'OK' END as portfolio_status,
    CASE WHEN acc.segment_id IS NOT NULL AND s.id IS NULL THEN 'MISSING' ELSE 'OK' END as segment_status,
    CASE WHEN acc.parent_account_id IS NOT NULL AND pa.id IS NULL THEN 'MISSING' ELSE 'OK' END as parent_account_status
FROM account acc
LEFT JOIN ledger l ON acc.ledger_id = l.id AND l.deleted_at IS NULL
LEFT JOIN organization o ON acc.organization_id = o.id AND o.deleted_at IS NULL
LEFT JOIN portfolio p ON acc.portfolio_id = p.id AND p.deleted_at IS NULL
LEFT JOIN segment s ON acc.segment_id = s.id AND s.deleted_at IS NULL
LEFT JOIN account pa ON acc.parent_account_id = pa.id AND pa.deleted_at IS NULL
WHERE acc.deleted_at IS NULL
  AND (l.id IS NULL OR o.id IS NULL
       OR (acc.portfolio_id IS NOT NULL AND p.id IS NULL)
       OR (acc.segment_id IS NOT NULL AND s.id IS NULL)
       OR (acc.parent_account_id IS NOT NULL AND pa.id IS NULL));

-- ----------------------------------------------------------------------------
-- A4: Portfolios referencing non-existent entities
-- ----------------------------------------------------------------------------
SELECT
    '--- A4: ORPHAN PORTFOLIOS ---' as report_section;

SELECT
    p.id as portfolio_id,
    p.name,
    p.ledger_id,
    p.organization_id,
    CASE WHEN l.id IS NULL THEN 'MISSING' ELSE 'OK' END as ledger_status,
    CASE WHEN o.id IS NULL THEN 'MISSING' ELSE 'OK' END as org_status
FROM portfolio p
LEFT JOIN ledger l ON p.ledger_id = l.id AND l.deleted_at IS NULL
LEFT JOIN organization o ON p.organization_id = o.id AND o.deleted_at IS NULL
WHERE p.deleted_at IS NULL
  AND (l.id IS NULL OR o.id IS NULL);

-- ============================================================================
-- PART B: TRANSACTION DATABASE CHECKS
-- Run these against the 'transaction' database
-- ============================================================================

-- ----------------------------------------------------------------------------
-- B1: Operations referencing non-existent transactions
-- ----------------------------------------------------------------------------
SELECT
    '--- B1: ORPHAN OPERATIONS (missing transaction) ---' as report_section;

SELECT
    o.id as operation_id,
    o.transaction_id,
    o.account_alias,
    o.amount,
    o.created_at
FROM operation o
LEFT JOIN transaction t ON o.transaction_id = t.id
WHERE o.deleted_at IS NULL
  AND t.id IS NULL
LIMIT 100;

-- ----------------------------------------------------------------------------
-- B2: Operations referencing non-existent balances
-- ----------------------------------------------------------------------------
SELECT
    '--- B2: OPERATIONS WITH MISSING BALANCE ---' as report_section;

SELECT
    o.id as operation_id,
    o.balance_id,
    o.account_alias,
    o.asset_code,
    o.amount,
    o.created_at
FROM operation o
LEFT JOIN balance b ON o.balance_id = b.id
WHERE o.deleted_at IS NULL
  AND o.balance_id IS NOT NULL
  AND b.id IS NULL
LIMIT 100;

-- ----------------------------------------------------------------------------
-- B3: Balances with operations from different accounts
-- ----------------------------------------------------------------------------
SELECT
    '--- B3: BALANCE-OPERATION ACCOUNT MISMATCH ---' as report_section;

SELECT DISTINCT
    b.id as balance_id,
    b.account_id as balance_account,
    b.alias as balance_alias,
    o.account_id as operation_account,
    o.account_alias as operation_alias,
    o.id as operation_id
FROM balance b
JOIN operation o ON b.id = o.balance_id AND o.deleted_at IS NULL
WHERE b.deleted_at IS NULL
  AND b.account_id != o.account_id
LIMIT 100;

-- ----------------------------------------------------------------------------
-- B4: Operation route referencing non-existent transaction routes
-- ----------------------------------------------------------------------------
SELECT
    '--- B4: ORPHAN OPERATION-TRANSACTION ROUTE LINKS ---' as report_section;

SELECT
    otr.id,
    otr.operation_route_id,
    otr.transaction_route_id,
    CASE WHEN or_check.id IS NULL THEN 'MISSING' ELSE 'OK' END as op_route_status,
    CASE WHEN tr_check.id IS NULL THEN 'MISSING' ELSE 'OK' END as tx_route_status
FROM operation_transaction_route otr
LEFT JOIN operation_route or_check ON otr.operation_route_id = or_check.id AND or_check.deleted_at IS NULL
LEFT JOIN transaction_route tr_check ON otr.transaction_route_id = tr_check.id AND tr_check.deleted_at IS NULL
WHERE otr.deleted_at IS NULL
  AND (or_check.id IS NULL OR tr_check.id IS NULL);

-- ============================================================================
-- PART C: CROSS-DATABASE CHECKS (Export/Import Pattern)
-- These generate data to be compared across databases
-- ============================================================================

-- ----------------------------------------------------------------------------
-- C1: Export account IDs from onboarding (run on onboarding DB)
-- ----------------------------------------------------------------------------
SELECT
    '--- C1: EXPORT ACCOUNT IDS (run on onboarding) ---' as report_section;

-- Create temp table or export to file
-- SELECT id FROM account WHERE deleted_at IS NULL;

-- ----------------------------------------------------------------------------
-- C2: Check balances reference valid accounts (run on transaction DB)
-- After importing account IDs from onboarding
-- ----------------------------------------------------------------------------
SELECT
    '--- C2: BALANCES WITH INVALID ACCOUNT REFERENCE ---' as report_section;

-- This query assumes you've imported valid account IDs into a temp table
-- CREATE TEMP TABLE valid_accounts (id UUID);
-- COPY valid_accounts FROM '/tmp/account_ids.csv';

-- Then run:
-- SELECT b.id, b.account_id, b.alias
-- FROM balance b
-- LEFT JOIN valid_accounts va ON b.account_id = va.id
-- WHERE b.deleted_at IS NULL AND va.id IS NULL;

-- ============================================================================
-- SECTION 2: Summary Statistics for Telemetry
-- ============================================================================
SELECT
    '--- INTEGRITY SUMMARY (ONBOARDING) ---' as report_section;

SELECT json_build_object(
    'check_type', 'referential_integrity_onboarding',
    'timestamp', NOW(),
    'orphan_ledgers', (SELECT COUNT(*) FROM ledger l LEFT JOIN organization o ON l.organization_id = o.id AND o.deleted_at IS NULL WHERE l.deleted_at IS NULL AND o.id IS NULL),
    'orphan_assets', (SELECT COUNT(*) FROM asset a LEFT JOIN ledger l ON a.ledger_id = l.id AND l.deleted_at IS NULL LEFT JOIN organization o ON a.organization_id = o.id AND o.deleted_at IS NULL WHERE a.deleted_at IS NULL AND (l.id IS NULL OR o.id IS NULL)),
    'orphan_accounts', (SELECT COUNT(*) FROM account acc LEFT JOIN ledger l ON acc.ledger_id = l.id AND l.deleted_at IS NULL LEFT JOIN organization o ON acc.organization_id = o.id AND o.deleted_at IS NULL WHERE acc.deleted_at IS NULL AND (l.id IS NULL OR o.id IS NULL)),
    'orphan_portfolios', (SELECT COUNT(*) FROM portfolio p LEFT JOIN ledger l ON p.ledger_id = l.id AND l.deleted_at IS NULL LEFT JOIN organization o ON p.organization_id = o.id AND o.deleted_at IS NULL WHERE p.deleted_at IS NULL AND (l.id IS NULL OR o.id IS NULL))
) as telemetry_data;

-- Run this on transaction database:
-- SELECT json_build_object(
--     'check_type', 'referential_integrity_transaction',
--     'timestamp', NOW(),
--     'orphan_operations', (SELECT COUNT(*) FROM operation o LEFT JOIN transaction t ON o.transaction_id = t.id WHERE o.deleted_at IS NULL AND t.id IS NULL),
--     'operations_missing_balance', (SELECT COUNT(*) FROM operation o LEFT JOIN balance b ON o.balance_id = b.id WHERE o.deleted_at IS NULL AND o.balance_id IS NOT NULL AND b.id IS NULL),
--     'balance_account_mismatches', (SELECT COUNT(DISTINCT b.id) FROM balance b JOIN operation o ON b.id = o.balance_id AND o.deleted_at IS NULL WHERE b.deleted_at IS NULL AND b.account_id != o.account_id)
-- ) as telemetry_data;
