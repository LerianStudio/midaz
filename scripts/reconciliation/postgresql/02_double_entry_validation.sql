-- ============================================================================
-- MIDAZ RECONCILIATION SCRIPT: Double-Entry Accounting Validation
-- ============================================================================
-- Purpose: Verify that every transaction follows double-entry accounting rules
--          (Total Credits = Total Debits for each transaction)
-- Database: transaction
-- Frequency: Daily or real-time for critical transactions
-- Expected: All transactions should balance (imbalance = 0)
-- ============================================================================

-- ============================================================================
-- SECTION 1: Summary Statistics
-- ============================================================================
SELECT
    '--- DOUBLE-ENTRY VALIDATION SUMMARY ---' as report_section;

WITH transaction_balance AS (
    SELECT
        t.id as transaction_id,
        t.organization_id,
        t.ledger_id,
        t.status,
        t.asset_code,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
        COUNT(o.id) as operation_count
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
    WHERE t.deleted_at IS NULL
    GROUP BY t.id, t.organization_id, t.ledger_id, t.status, t.asset_code
)
SELECT
    COUNT(*) as total_transactions,
    SUM(CASE WHEN total_credits != total_debits THEN 1 ELSE 0 END) as unbalanced_transactions,
    ROUND(100.0 * SUM(CASE WHEN total_credits != total_debits THEN 1 ELSE 0 END) / NULLIF(COUNT(*), 0), 2) as unbalanced_percentage,
    SUM(CASE WHEN operation_count = 0 THEN 1 ELSE 0 END) as transactions_without_operations,
    SUM(CASE WHEN operation_count = 1 THEN 1 ELSE 0 END) as transactions_with_single_operation
FROM transaction_balance;

-- ============================================================================
-- SECTION 2: Unbalanced Transactions Detail
-- ============================================================================
SELECT
    '--- UNBALANCED TRANSACTIONS ---' as report_section;

WITH transaction_balance AS (
    SELECT
        t.id as transaction_id,
        t.description,
        t.organization_id,
        t.ledger_id,
        t.status,
        t.asset_code,
        t.amount as declared_amount,
        t.created_at,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as total_credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as total_debits,
        COUNT(o.id) as operation_count,
        array_agg(DISTINCT o.account_alias) as involved_accounts
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
    WHERE t.deleted_at IS NULL
    GROUP BY t.id, t.description, t.organization_id, t.ledger_id, t.status,
             t.asset_code, t.amount, t.created_at
)
SELECT
    transaction_id,
    description,
    status,
    asset_code,
    declared_amount,
    total_credits,
    total_debits,
    (total_credits - total_debits) as imbalance,
    operation_count,
    involved_accounts,
    organization_id,
    ledger_id,
    created_at
FROM transaction_balance
WHERE total_credits != total_debits
ORDER BY ABS(total_credits - total_debits) DESC
LIMIT 100;

-- ============================================================================
-- SECTION 3: Transactions Without Operations (Orphans)
-- ============================================================================
SELECT
    '--- TRANSACTIONS WITHOUT OPERATIONS ---' as report_section;

SELECT
    t.id as transaction_id,
    t.description,
    t.status,
    t.amount,
    t.asset_code,
    t.organization_id,
    t.ledger_id,
    t.created_at
FROM transaction t
LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
WHERE t.deleted_at IS NULL
  AND o.id IS NULL
ORDER BY t.created_at DESC
LIMIT 50;

-- ============================================================================
-- SECTION 4: Operation Amount vs Balance Change Consistency
-- ============================================================================
SELECT
    '--- OPERATION BALANCE CHANGE CONSISTENCY ---' as report_section;

-- Verify that available_balance_after - available_balance = amount (for credits)
-- and available_balance - available_balance_after = amount (for debits)
SELECT
    o.id as operation_id,
    o.transaction_id,
    o.account_alias,
    o.type,
    o.amount,
    o.available_balance as balance_before,
    o.available_balance_after as balance_after,
    CASE
        WHEN o.type = 'CREDIT' THEN o.available_balance_after - o.available_balance
        WHEN o.type = 'DEBIT' THEN o.available_balance - o.available_balance_after
        ELSE 0
    END as calculated_change,
    o.amount - CASE
        WHEN o.type = 'CREDIT' THEN o.available_balance_after - o.available_balance
        WHEN o.type = 'DEBIT' THEN o.available_balance - o.available_balance_after
        ELSE 0
    END as amount_vs_change_diff
FROM operation o
WHERE o.deleted_at IS NULL
  AND o.balance_affected = true
  AND o.amount != CASE
      WHEN o.type = 'CREDIT' THEN o.available_balance_after - o.available_balance
      WHEN o.type = 'DEBIT' THEN o.available_balance - o.available_balance_after
      ELSE 0
  END
ORDER BY o.created_at DESC
LIMIT 50;

-- ============================================================================
-- SECTION 5: Single-Sided Transactions (Missing Counterpart)
-- ============================================================================
SELECT
    '--- SINGLE-SIDED TRANSACTIONS ---' as report_section;

WITH transaction_sides AS (
    SELECT
        t.id as transaction_id,
        t.description,
        t.status,
        COUNT(DISTINCT CASE WHEN o.type = 'CREDIT' THEN o.id END) as credit_count,
        COUNT(DISTINCT CASE WHEN o.type = 'DEBIT' THEN o.id END) as debit_count
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
    WHERE t.deleted_at IS NULL
    GROUP BY t.id, t.description, t.status
)
SELECT *
FROM transaction_sides
WHERE (credit_count = 0 AND debit_count > 0)
   OR (debit_count = 0 AND credit_count > 0)
LIMIT 50;

-- ============================================================================
-- SECTION 6: Transaction Amount vs Sum of Operations
-- ============================================================================
SELECT
    '--- TRANSACTION DECLARED AMOUNT VS OPERATIONS ---' as report_section;

SELECT
    t.id as transaction_id,
    t.description,
    t.amount as declared_amount,
    SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END) as sum_credits,
    t.amount - SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END) as amount_difference
FROM transaction t
JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
WHERE t.deleted_at IS NULL
  AND t.amount IS NOT NULL
GROUP BY t.id, t.description, t.amount
HAVING t.amount != SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END)
LIMIT 50;

-- ============================================================================
-- SECTION 7: Telemetry Output (JSON format for worker consumption)
-- ============================================================================
SELECT
    '--- TELEMETRY OUTPUT (JSON) ---' as report_section;

WITH transaction_balance AS (
    SELECT
        t.id,
        t.organization_id,
        t.ledger_id,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) as credits,
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) as debits,
        COUNT(o.id) as ops
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id AND o.deleted_at IS NULL
    WHERE t.deleted_at IS NULL
    GROUP BY t.id, t.organization_id, t.ledger_id
)
SELECT json_build_object(
    'check_type', 'double_entry_validation',
    'timestamp', NOW(),
    'total_transactions', COUNT(*),
    'unbalanced_count', SUM(CASE WHEN credits != debits THEN 1 ELSE 0 END),
    'orphan_transactions', SUM(CASE WHEN ops = 0 THEN 1 ELSE 0 END),
    'total_imbalance_amount', SUM(ABS(credits - debits)),
    'by_ledger', (
        SELECT json_agg(json_build_object(
            'org_id', organization_id,
            'ledger_id', ledger_id,
            'unbalanced_count', unbal_count
        ))
        FROM (
            SELECT organization_id, ledger_id,
                   SUM(CASE WHEN credits != debits THEN 1 ELSE 0 END) as unbal_count
            FROM transaction_balance
            GROUP BY organization_id, ledger_id
            HAVING SUM(CASE WHEN credits != debits THEN 1 ELSE 0 END) > 0
        ) ledger_summary
    )
) as telemetry_data
FROM transaction_balance;
