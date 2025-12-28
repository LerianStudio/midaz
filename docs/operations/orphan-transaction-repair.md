# Orphan Transaction Repair Strategy

## Overview

This document describes how to identify and handle orphan transactions - transactions that exist in the database without corresponding operations. This situation indicates a data integrity issue that should not occur after the atomicity fix (see `pkg/dbtx`).

## Background

### What Are Orphan Transactions?

An orphan transaction is a record in the `transaction` table that has no corresponding records in the `operation` table. In normal operation:

1. Balance updates, transaction creation, and operation creation should all succeed or all fail together (atomic)
2. Every non-NOTED transaction should have at least 2 operations (debit and credit)

### Root Cause (Before Fix)

Before the atomicity fix, the `CreateBalanceTransactionOperationsAsync` function performed these steps sequentially without a database transaction wrapper:

1. Update balances ✓
2. Create transaction ✓
3. Create operations ✗ (failure here = orphan transaction)

If step 3 failed after step 2 succeeded, an orphan transaction was created.

### Solution Implemented

The atomicity fix wraps steps 1-3 in a single database transaction using `dbtx.RunInTransaction`. Now if any step fails, all changes are rolled back.

## Detection

### Using SQL Queries

Run the queries in `scripts/orphan-detection.sql` to identify orphans:

```bash
# Connect to the transaction database and run:
psql -h $DB_HOST -U $DB_USER -d $DB_NAME -f scripts/orphan-detection.sql
```

### Quick Health Check

```sql
SELECT COUNT(*) AS total_orphan_transactions
FROM transaction t
LEFT JOIN operation o ON t.id = o.transaction_id
WHERE o.id IS NULL
  AND t.deleted_at IS NULL
  AND t.status NOT IN ('NOTED', 'PENDING');
```

**Expected result after fix:** 0

## Repair Strategies

### Strategy 1: Delete Orphan Transactions (Recommended for Test/Dev)

If orphan transactions have no financial impact (e.g., test environments):

```sql
-- First, verify the orphans to be deleted
SELECT t.id, t.organization_id, t.ledger_id, t.status, t.amount
FROM transaction t
LEFT JOIN operation o ON t.id = o.transaction_id
WHERE o.id IS NULL AND t.deleted_at IS NULL AND t.status NOT IN ('NOTED', 'PENDING');

-- Soft delete orphan transactions
UPDATE transaction t
SET deleted_at = NOW()
FROM (
    SELECT t2.id
    FROM transaction t2
    LEFT JOIN operation o ON t2.id = o.transaction_id
    WHERE o.id IS NULL
      AND t2.deleted_at IS NULL
      AND t2.status NOT IN ('NOTED', 'PENDING')
) orphans
WHERE t.id = orphans.id;
```

### Strategy 2: Reconstruct Operations (Production - Manual)

For production environments where financial accuracy is critical:

1. **Identify the orphan transaction**
2. **Check if balance was updated** - Compare expected vs actual balance
3. **Determine original intent** - Review logs, metadata, or source system
4. **Reconstruct operations** - Manually create the missing operations based on the transaction details
5. **Verify balance consistency** - Ensure total debits = total credits

⚠️ **Warning:** This strategy requires careful analysis and should involve finance/accounting review.

### Strategy 3: Reverse Balance Impact (Production - Automated)

If the balance was updated but operations are missing:

1. Identify affected balances from the transaction's asset code and accounts
2. Calculate the reverse adjustment needed
3. Create a correcting transaction with proper operations
4. Mark the orphan as CANCELED with a reference to the correction

### Strategy 4: Mark as FAILED (Simple Cleanup)

If the transaction had no actual financial impact:

```sql
UPDATE transaction
SET status = 'FAILED',
    status_description = 'Marked as failed - orphan transaction detected',
    updated_at = NOW()
WHERE id IN (
    SELECT t.id
    FROM transaction t
    LEFT JOIN operation o ON t.id = o.transaction_id
    WHERE o.id IS NULL
      AND t.deleted_at IS NULL
      AND t.status NOT IN ('NOTED', 'PENDING', 'FAILED')
);
```

## Prevention

The atomicity fix prevents new orphans from being created. To verify:

1. **Code Review:** Ensure all transaction-creating code paths use `dbtx.RunInTransaction`
2. **Integration Tests:** Run atomicity tests that simulate failures at each step
3. **Monitoring:** Set up alerts for the orphan health check query

## Monitoring Recommendation

Add a periodic job that runs the health check query and alerts if orphans are detected:

```sql
-- Alert if any orphans exist
SELECT
    CASE
        WHEN COUNT(*) > 0 THEN 'ALERT: ' || COUNT(*) || ' orphan transactions detected'
        ELSE 'OK: No orphan transactions'
    END AS health_status
FROM transaction t
LEFT JOIN operation o ON t.id = o.transaction_id
WHERE o.id IS NULL
  AND t.deleted_at IS NULL
  AND t.status NOT IN ('NOTED', 'PENDING');
```

## Related Files

- `scripts/orphan-detection.sql` - Detection queries
- `pkg/dbtx/dbtx.go` - Transaction context management
- `components/transaction/internal/services/command/create-balance-transaction-operations-async.go` - Atomic operation wrapper
