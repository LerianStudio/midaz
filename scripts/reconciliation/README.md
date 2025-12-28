# Midaz Database Reconciliation Scripts

This directory contains reconciliation scripts for validating data consistency across Midaz's multi-database architecture (PostgreSQL + MongoDB + Redis).

## Overview

Midaz uses a distributed database architecture:
- **PostgreSQL (onboarding)**: Organizations, ledgers, assets, accounts, portfolios, segments
- **PostgreSQL (transaction)**: Transactions, operations, balances, asset rates, routes
- **MongoDB**: Flexible metadata for all entities
- **Redis/Valkey**: Balance caching with TTL-based persistence

These scripts detect consistency issues that may arise from:
- Network partitions between services
- Failed message queue processing
- Redis cache expiration before sync
- Race conditions in high-concurrency scenarios
- Application bugs or data corruption

## Quick Start

```bash
# Run all reconciliation checks
./run_reconciliation.sh

# Run with JSON output only (for worker consumption)
./run_reconciliation.sh --json-only

# Specify output directory
./run_reconciliation.sh --output-dir /var/log/midaz/reconciliation
```

## Scripts

### PostgreSQL Scripts

| Script | Purpose | Frequency |
|--------|---------|-----------|
| `01_balance_consistency.sql` | Verify balances match operation sums | Daily |
| `02_double_entry_validation.sql` | Ensure transactions balance (debits = credits) | Daily |
| `03_referential_integrity.sql` | Check cross-table and cross-database FKs | Weekly |
| `04_redis_pg_sync.sql` | Detect Redis-PostgreSQL sync issues | Hourly |

### MongoDB Scripts

| Script | Purpose | Frequency |
|--------|---------|-----------|
| `01_metadata_sync.js` | Validate metadata document integrity | Daily |
| `02_cross_db_validation.js` | Compare PG entities with Mongo metadata | Daily |

## Reconciliation Checks

### 1. Balance Consistency (`01_balance_consistency.sql`)

**What it checks:**
- `balance.available` = SUM(CREDIT operations) - SUM(DEBIT operations)
- Balance version matches operation count
- Balances with non-zero values but no operations

**Expected results:** 0 discrepancies
**Acceptable variance:** < 0.01 (decimal precision issues)

```sql
-- Run on transaction database
docker exec midaz-postgres-primary psql -U midaz -d transaction -f /scripts/01_balance_consistency.sql
```

### 2. Double-Entry Validation (`02_double_entry_validation.sql`)

**What it checks:**
- Every transaction has credits = debits
- No orphan transactions (transactions without operations)
- Operation amount matches balance change snapshots

**Expected results:** 0 unbalanced transactions
**Critical:** Any imbalance indicates a serious bug

### 3. Referential Integrity (`03_referential_integrity.sql`)

**What it checks:**
- Ledgers reference valid organizations
- Accounts reference valid ledgers, portfolios, segments
- Operations reference valid transactions and balances
- No orphan records in either database

**Expected results:** 0 orphan records

### 4. Redis-PostgreSQL Sync (`04_redis_pg_sync.sql`)

**What it checks:**
- Balance versions are current with operations
- No stale balances (operations newer than balance update)
- Version sequence gaps (potential lost updates)
- High-frequency account monitoring

**Expected results:** No stale balances > 5 minutes

### 5. Metadata Sync (`01_metadata_sync.js`)

**What it checks:**
- All PostgreSQL entities have MongoDB metadata documents
- No duplicate entity_ids in MongoDB
- No null/missing required fields
- Soft delete consistency

### 6. Cross-DB Validation (`02_cross_db_validation.js`)

**What it checks:**
- Entity counts match between PostgreSQL and MongoDB
- Entity IDs exist in both databases
- No orphan metadata documents

## Telemetry Output

All scripts output JSON telemetry suitable for worker consumption:

```json
{
  "check_type": "balance_consistency",
  "timestamp": "2025-12-28T12:00:00Z",
  "total_balances": 2734,
  "discrepancies": 0,
  "total_discrepancy_amount": 0,
  "status": "HEALTHY"
}
```

## Integration with Go Workers

These scripts are designed to be called by Go reconciliation workers:

```go
// Example worker integration
func (w *ReconciliationWorker) RunBalanceCheck(ctx context.Context) (*Report, error) {
    // Execute SQL script
    result, err := w.pgPool.Query(ctx, balanceConsistencySQL)
    if err != nil {
        return nil, err
    }

    // Parse telemetry JSON
    var telemetry TelemetryData
    if err := json.Unmarshal(result, &telemetry); err != nil {
        return nil, err
    }

    // Send to telemetry service
    if telemetry.Discrepancies > 0 {
        w.alertService.Send(ctx, Alert{
            Severity: "WARNING",
            Message:  fmt.Sprintf("Found %d balance discrepancies", telemetry.Discrepancies),
        })
    }

    return &Report{
        CheckType: telemetry.CheckType,
        Status:    telemetry.Status,
        Details:   telemetry,
    }, nil
}
```

## Database Connection

### Docker (default)

```bash
# PostgreSQL
docker exec midaz-postgres-primary psql -U midaz -d onboarding
docker exec midaz-postgres-primary psql -U midaz -d transaction

# MongoDB
docker exec midaz-mongodb mongosh --port 5703 onboarding
docker exec midaz-mongodb mongosh --port 5703 transaction
```

### Local Clients

```bash
# PostgreSQL (use .pgpass file for secure password handling)
# Create ~/.pgpass with: localhost:5701:*:midaz:<password>
# Then: chmod 600 ~/.pgpass
psql -h localhost -p 5701 -U midaz -d onboarding

# MongoDB
mongosh --host localhost --port 5703 onboarding
```

## Alert Thresholds

| Check | Warning | Critical |
|-------|---------|----------|
| Balance discrepancies | > 0 | > 10 |
| Unbalanced transactions | > 0 | Any |
| Orphan records | > 0 | > 100 |
| Stale balances (> 5 min) | > 0 | > 5 |
| Version mismatches | > 0 | > 10 |

## Scheduling Recommendations

| Check | Frequency | When |
|-------|-----------|------|
| Balance consistency | Daily | Off-peak hours |
| Double-entry validation | Daily | After batch processing |
| Referential integrity | Weekly | Maintenance window |
| Redis sync validation | Hourly | Continuous |
| Full reconciliation | Weekly | Weekend maintenance |

## Troubleshooting

### Common Issues

1. **Balance discrepancy found**
   - Check if operations have `balance_affected = false`
   - Verify operation status (some may be PENDING)
   - Check for recent DLQ processing

2. **Stale balance detected**
   - Verify BalanceSyncWorker is running
   - Check Redis connectivity
   - Review balance_sync_schedule in Redis

3. **Orphan metadata in MongoDB**
   - Check if PostgreSQL entity was hard-deleted
   - Verify MongoDB collection indexes
   - Review soft-delete consistency

4. **Version gaps detected**
   - Check for failed transactions
   - Review optimistic locking conflicts
   - Verify DLQ consumer is processing

## Contributing

When adding new reconciliation checks:

1. Create SQL/JS script with telemetry output
2. Add to `run_reconciliation.sh`
3. Document thresholds and expected results
4. Add Go worker integration example
