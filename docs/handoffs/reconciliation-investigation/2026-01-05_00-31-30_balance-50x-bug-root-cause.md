---
date: 2026-01-05T00:31:30Z
session_name: reconciliation-investigation
git_commit: 6af6cf590543490c1e6d869c0fc19aa7e26d9feb
branch: fix/fred-several-ones-dec-13-2025
repository: LerianStudio/midaz
topic: "Balance 50x Multiplication Bug - Root Cause Identified"
tags: [reconciliation, balance-bug, redis, rabbitmq, prefetch, double-counting]
status: in_progress
outcome: ROOT_CAUSE_FOUND
---

# Handoff: Balance 50x Multiplication Bug - Root Cause Analysis Complete

## Task Summary

**Context**: Resumed from previous session that fixed 3 balance discrepancy bugs. After running 20K transactions, discovered a new CRITICAL bug where balances are being multiplied by ~50x.

**Status**: Root cause IDENTIFIED. Fix NOT yet implemented.

**Key Finding**: The bug is in `refreshBalancesFromCache()` which RE-APPLIES transaction amounts to Redis balances that ALREADY contain those amounts from the Lua script execution.

## Problem Statement

### Symptoms Observed

After running 20K transactions via load test:

| Metric | Expected | Actual | Issue |
|--------|----------|--------|-------|
| Unsettled transactions | ~0 | 19,042 | Messages failing in consumer |
| Balance discrepancy % | 0% | 95.28% | 404 of 424 balances wrong |
| Total discrepancy | $0 | $956,686.85 | Massive over-crediting |
| Queue backlog | 20,000 | 552,218 | 27x message multiplication |

### The Pattern Discovered

**Version = Operations × 50 + 1** (where 50 = RabbitMQ prefetch count)

| Balance Type | Operations | Version | Formula |
|--------------|------------|---------|---------|
| External account | 100 | 5001 | 100 × 50 + 1 |
| Regular account | 1 | 51 | 1 × 50 + 1 |
| Regular account | 48 | 51 | (capped) |

**Concrete Example**:
- Balance ID: `019b884d-a6ad-7590-a09e-8f2cd853a2ab`
- Operation recorded: 1 CREDIT of $26.75, version 0→1
- Actual balance: version=51, available=$1420.33
- Expected: version=2, available=$26.75
- Over-credited by: $1393.58 (~52× the transaction amount)

## Root Cause Analysis

### The Bug Location

**File**: `components/transaction/internal/services/command/update-balance.go`
**Function**: `refreshBalancesFromCache()` (lines 294-369)
**Specific Issue**: Lines 350-365

```go
// Line 351 - THIS IS THE BUG
calculatedBalance, err := pkgTransaction.OperateBalancesWithContext(
    ctx, transactionID, amount, *cachedBalance.ToTransactionBalance())
```

### Why It's Wrong

The flow when multiple HTTP requests hit the same balance:

```
50 HTTP Requests (concurrent, same external account):
┌──────────────────────────────────────────────────────────────────────┐
│ Request 1:  Lua script → Redis v=0→1,  amount=0→26.75               │
│ Request 2:  Lua script → Redis v=1→2,  amount=26.75→53.50           │
│ Request 3:  Lua script → Redis v=2→3,  amount=53.50→80.25           │
│ ...                                                                  │
│ Request 50: Lua script → Redis v=49→50, amount=...→1337.50          │
│                                                                      │
│ Each request publishes 1 message to RabbitMQ with its snapshot      │
└──────────────────────────────────────────────────────────────────────┘

Consumer processes message 1 (stale - Redis v=50, message v=1):
┌──────────────────────────────────────────────────────────────────────┐
│ 1. calculateNewBalances: version = 1+1 = 2                          │
│ 2. filterStaleBalances: Redis v=50 > msg v=2 → STALE!               │
│ 3. refreshBalancesFromCache:                                        │
│    - Reads Redis: v=50, amount=1337.50 (ALREADY has tx 1's amount!) │
│    - RE-APPLIES tx 1 amount: +26.75  ← BUG! Double-counting!        │
│    - Result: v=51, amount=1364.25                                   │
│ 4. BalancesUpdate: PostgreSQL v=0 → v=51, amount=1364.25            │
└──────────────────────────────────────────────────────────────────────┘
```

### Root Cause Summary

1. **Lua script** executes during HTTP request and updates Redis with transaction amount
2. **RabbitMQ message** contains the balance snapshot from AFTER Lua execution
3. When consumer detects **stale message** (Redis version > message version), it refreshes from cache
4. **`refreshBalancesFromCache` RE-APPLIES** the transaction amount to a Redis balance that ALREADY includes it
5. This causes **double-counting** of the transaction amount

### Configuration Contributing to Issue

```go
// consumer.rabbitmq.go:946
NumbersOfPrefetch: numbersOfWorkers * numbersOfPrefetch  // 5 * 10 = 50
```

The prefetch count of 50 allows 50 messages to be "in flight" simultaneously. When 50+ HTTP requests hit the same balance before any consumer processes a message, all messages become stale, triggering the buggy refresh logic.

## Critical References

### Files to Read (in order)

1. **`components/transaction/internal/services/command/update-balance.go`** (CRITICAL)
   - Lines 294-369: `refreshBalancesFromCache()` - THE BUG IS HERE
   - Lines 167-235: `handleBalanceUpdateError()` and `retryBalanceUpdateWithCacheRefresh()`
   - Lines 240-292: `filterStaleBalances()` - detects stale messages

2. **`components/transaction/internal/adapters/redis/scripts/add_sub.lua`**
   - Lines 1200-1230: Where Lua script updates Redis balance and increments version

3. **`components/transaction/internal/services/query/get-balances.go`**
   - Lines 84-100: Where HTTP request calls `GetAccountAndLock` → Lua script

4. **`components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`**
   - Lines 940-950: Prefetch configuration
   - Lines 1040-1046: Worker spawning

### Database Evidence Queries

```sql
-- Verify the pattern: version = ops × 50 + 1
SELECT
    balance_id, COUNT(*) as op_count,
    MAX(b.version) as version,
    MAX(b.version)::float / NULLIF(COUNT(*), 0) as version_per_op
FROM operation o JOIN balance b ON o.balance_id = b.id
GROUP BY balance_id ORDER BY op_count DESC LIMIT 10;

-- Check operation vs balance version mismatch
SELECT o.balance_version_before, o.balance_version_after, b.version
FROM operation o JOIN balance b ON o.balance_id = b.id
WHERE b.version > o.balance_version_after + 1 LIMIT 5;
```

## Learnings

### What Worked
- **Parallel codebase-explorer agents** - Quickly identified prefetch=50 correlation
- **Database queries via MCP dbhub** - Direct evidence gathering
- **Docker logs analysis** - Confirmed consumer failures ("newer versions exist")

### What Failed
- **Initial assumption** that consumer was the sole culprit - issue is in refresh logic
- **Misunderstanding of message content** - Messages contain post-Lua balance snapshots, not pre-Lua

### Key Decisions Made

1. **Root cause is double-counting in `refreshBalancesFromCache`**
   - Alternatives considered: Consumer retry loop, RabbitMQ redelivery, BalanceSyncWorker
   - Evidence: Operation table shows correct version (0→1), balance shows 51

2. **The comment at line 328-330 is misleading**
   - Says "re-apply amounts because cache doesn't have them"
   - WRONG: Cache DOES have them from Lua script execution
   - This comment should be corrected or the logic changed

## Proposed Solutions

### Option 1: Don't Re-Apply Amounts for Stale Messages (Recommended)

When `filterStaleBalances` detects a stale message (Redis version > message version), the Redis cache ALREADY contains this transaction's effects. The fix:

```go
// In refreshBalancesFromCache, check if this is a stale refresh
// If stale, use cached values directly WITHOUT re-applying amounts
refreshed = append(refreshed, &mmodel.Balance{
    ID:        balanceID,
    Alias:     refreshAlias,
    Available: cachedBalance.Available,  // Already has tx effect
    OnHold:    cachedBalance.OnHold,     // Already has tx effect
    Version:   cachedBalance.Version + 1, // Just increment for DB write
})
```

**Pros**: Simple, minimal code change
**Cons**: Need to verify edge cases where cache legitimately doesn't have the transaction

### Option 2: Add Transaction-Level Idempotency to Balance Updates

Track which transactions have been applied to each balance using a separate idempotency table or Redis set.

```sql
CREATE TABLE balance_transaction_idempotency (
    balance_id UUID,
    transaction_id UUID,
    applied_at TIMESTAMPTZ,
    PRIMARY KEY (balance_id, transaction_id)
);
```

**Pros**: Robust, handles all edge cases
**Cons**: More complex, additional DB/Redis overhead

### Option 3: Skip Stale Messages Entirely

If Redis version > message version, the message is outdated. A newer message with the correct cumulative balance will be processed.

```go
// In filterStaleBalances, return empty slice if ALL messages are stale
// This will trigger the assert.Never() panic, but that's wrong behavior
```

**Pros**: Very simple
**Cons**: May cause legitimate transactions to be lost if messages arrive out of order

## Action Items & Next Steps

### Immediate (Next Session)

- [ ] **Implement Option 1 fix** in `refreshBalancesFromCache`
- [ ] **Add unit tests** for stale message scenarios
- [ ] **Run load test again** to verify fix
- [ ] **Check reconciliation report** - discrepancies should drop to 0%

### Before PR

- [ ] **Run full test suite** including integration tests
- [ ] **Code review** with parallel reviewers
- [ ] **Document the fix** with clear explanation of why re-apply was wrong

### Future Considerations

- [ ] Consider Option 2 (idempotency table) for belt-and-suspenders protection
- [ ] Monitor RabbitMQ queue depth in production
- [ ] Add alerting for high message backlog

## Useful Commands

```bash
# Check current reconciliation status
curl -s http://127.0.0.1:3005/reconciliation/report | jq .

# Check RabbitMQ queue depth
docker exec midaz-rabbitmq rabbitmqctl list_queues name messages consumers

# Check balance vs operation mismatch
docker exec -it midaz-transaction-db psql -U postgres -d transaction -c "
SELECT b.id, b.version, b.available, COUNT(o.id) as ops,
       b.version::float / NULLIF(COUNT(o.id), 0) as ver_per_op
FROM balance b LEFT JOIN operation o ON o.balance_id = b.id
GROUP BY b.id ORDER BY ver_per_op DESC NULLS LAST LIMIT 10;"

# View consumer errors
docker logs midaz-transaction-dev 2>&1 | grep -E "newer versions|STALE|Failed to update" | tail -20
```

## Environment State

- **Docker containers**: midaz-transaction-dev, midaz-rabbitmq running
- **Queue backlog**: 552,218 messages pending
- **Balance sync worker**: DISABLED (BALANCE_SYNC_WORKER_ENABLED=false)
- **All code changes**: None - investigation only, fix pending

## Diagram: The Bug Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          NORMAL FLOW (Single Request)                       │
├─────────────────────────────────────────────────────────────────────────────┤
│ HTTP → Lua(Redis v=0→1, amt=26.75) → Msg(v=1) → Consumer → PG(v=2, amt=26.75)│
│                                                                             │
│ Result: Correct balance                                                     │
└─────────────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────────────┐
│                     BUGGY FLOW (50 Concurrent Requests)                     │
├─────────────────────────────────────────────────────────────────────────────┤
│ HTTP×50 → Lua×50(Redis v=50, amt=1337.50) → Msg×50(v=1..50)                │
│                                                                             │
│ Consumer gets Msg(v=1):                                                     │
│   calculateNew: v=2                                                         │
│   filterStale: Redis v=50 > msg v=2 → STALE!                               │
│   refreshFromCache:                                                         │
│     Read Redis: v=50, amt=1337.50 (INCLUDES tx!)                           │
│     RE-APPLY tx: +26.75  ← BUG!                                            │
│     Result: v=51, amt=1364.25                                              │
│   BalancesUpdate: PG(v=51, amt=1364.25)                                    │
│                                                                             │
│ Result: Balance over-credited by 26.75 (double-counted)                    │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Session Metrics

- **Investigation duration**: ~1 hour
- **Agents spawned**: 3 parallel codebase-explorer agents
- **Database queries**: ~15 SQL queries via MCP
- **Files analyzed**: 25+ files
- **Root cause confidence**: HIGH (95%+)
