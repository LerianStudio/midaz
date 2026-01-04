# Handoff: E2E Test Fix - Async Transaction Consistency Discussion

## Metadata
- **Created**: 2026-01-04T04:14:32Z
- **Branch**: fix/fred-several-ones-dec-13-2025
- **Commit**: 8290805aa68f543a4474b744ccd9e5842614b08c
- **Repository**: https://github.com/LerianStudio/midaz.git
- **Status**: IN_PROGRESS (architectural decision needed)

---

## Task Summary

### Original Problem
E2E tests failing with multiple issues:
1. **Share transaction precision errors** - Division producing exponents like -32/-34
2. **Duplicate outbox entry errors** - RabbitMQ retries causing insert conflicts
3. **Orphan transactions** - Transactions created without operations/balances
4. **Atomicity violations** - `handleBalanceUpdateError` returning success when balances weren't persisted

### Fixes Applied (3 of 4)

| Fix | File | Status |
|-----|------|--------|
| Share precision `.Truncate(18)` | `pkg/transaction/validations.go:447` | ✅ Applied |
| RELEASE/DEBIT idempotency | `pkg/transaction/validations.go:316-360` | ✅ Applied (prev session) |
| Outbox duplicate handling | `create-balance-transaction-operations-async.go:197-227` | ✅ Applied |
| Atomicity fix (return error) | `update-balance.go:178-186` | ⚠️ REVERTED - architectural decision needed |

### Current Blocker: Architectural Decision

The atomicity fix was **reverted** because of a deeper consistency question:

**The Problem:**
```
Client → POST /transaction → 201 returned (from Redis cache)
                                    ↓
                            RabbitMQ async processing
                                    ↓
                            Balance update FAILS (version conflict)
                                    ↓
                            What happens to the 201 promise?
```

**Two Competing Requirements:**
1. **Atomicity**: Transaction + Operations + Balances must all succeed or all fail
2. **201 Guarantee**: Once client gets 201, data MUST eventually be persisted

**The Conflict:**
- Returning error → rollback → RabbitMQ retries → eventually DLQ → nothing persisted
- Returning success → orphan transaction (balance not updated) → data corruption

---

## Critical References

Files that MUST be read to understand this problem:

| File | Purpose |
|------|---------|
| `components/transaction/internal/services/command/update-balance.go:167-190` | `handleBalanceUpdateError` - the decision point |
| `components/transaction/internal/services/command/create-balance-transaction-operations-async.go:52-79` | Atomic transaction wrapper |
| `components/transaction/internal/bootstrap/rabbitmq.server.go:229-264` | RabbitMQ consumer + retry logic |
| `components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go` | Retry config (5 attempts → DLQ) |

---

## Recent Changes (This Session)

### 1. `pkg/transaction/validations.go`
- **Line 447**: Added `.Truncate(18)` to share calculation
- **Lines 316-360**: Linter added `OperateBalancesWithContext` with metrics (from prev session)

### 2. `create-balance-transaction-operations-async.go`
- **Lines 197-201, 221-225**: Added `ErrDuplicateOutboxEntry` handling for idempotency

### 3. `update-balance.go`
- **Lines 178-186**: Atomicity fix was applied then REVERTED
- Current state: Returns `nil` when `usedCache=TRUE` (original behavior)

---

## Learnings

### What Worked
1. Share precision fix - `.Truncate(18)` prevents exponent overflow
2. Outbox idempotency - `errors.Is(err, ErrDuplicateOutboxEntry)` pattern
3. RELEASE/DEBIT idempotency - checking `onHold.IsZero()` before operations

### Key Insights Discovered
1. **Version conflicts are expected** under concurrent load - multiple workers racing on same balance
2. **Cache refresh doesn't solve conflicts** - other workers may update between refresh and write
3. **The 201 guarantee creates a consistency contract** - can't just rollback after client sees success
4. **DLQ is not a solution** - if retries exhaust, client has 201 but DB has nothing

### Decisions Pending

**User's requirement**: "After we gave a 201 to the client, we MUST ensure the transaction, operation and balance will go to the database, no matter what."

**Options discussed:**

| Option | Description | Trade-off |
|--------|-------------|-----------|
| A. Pessimistic Lock | `SELECT FOR UPDATE` on balances | Lower throughput |
| B. Saga Pattern | Create tx first, update balances separately, compensation on failure | Complex state machine |
| C. Infinite Retry | Remove retry limit, keep trying forever | Unpredictable latency |
| D. Single Writer | Partition by balance, one worker per balance | Load balancing complexity |
| E. Hybrid | Saga + higher retry + status tracking | Recommended |

---

## Action Items

### Immediate (Requires Decision)
- [ ] **DECIDE**: Which consistency approach to implement
- [ ] Apply chosen approach to `update-balance.go` and related files
- [ ] Update tests to reflect new behavior

### If Saga Pattern Chosen
- [ ] Add `BALANCE_PENDING` status to transaction states
- [ ] Separate balance update into its own queue/retry mechanism
- [ ] Add `BALANCE_FAILED` status for DLQ cases
- [ ] Update client-facing API to expose balance status

### If Infinite Retry Chosen
- [ ] Set `MaxRetries: 0` in RabbitMQ consumer config
- [ ] Add circuit breaker to prevent resource exhaustion
- [ ] Add alerting for long-running retries

### After Decision
- [ ] Run full E2E test suite
- [ ] Run atomicity tests specifically
- [ ] Commit all changes

---

## Test Commands

```bash
# Run unit tests for modified packages
go test ./pkg/transaction/... -v -run "TestApply|TestValidate"
go test ./components/transaction/internal/services/command/... -v -run "Stale"

# Check for errors in transaction service logs
docker logs midaz-transaction-dev --since 5m 2>&1 | grep -iE "error|panic|assertion"

# Run E2E tests (user's command)
# [Insert your E2E test command here]
```

---

## Resume Instructions

To continue this work in a new session:

```bash
/resume-handoff docs/handoffs/e2e-test-fix/2026-01-04_04-14-32_async-transaction-consistency.md
```

Key context to provide:
1. Which consistency approach was chosen (Saga, Infinite Retry, etc.)
2. Any new test failures observed
3. Whether the architectural decision has been finalized

---

## Architecture Diagram (For Reference)

```
┌─────────────────────────────────────────────────────────────────┐
│                     CURRENT FLOW                                │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  Client                                                         │
│    │                                                            │
│    ▼                                                            │
│  POST /transaction                                              │
│    │                                                            │
│    ├──► Idempotency Check (Redis)                              │
│    │                                                            │
│    ├──► Publish to RabbitMQ ◄──────────────────────────┐       │
│    │                                                    │       │
│    ▼                                                    │       │
│  201 Response ◄─── Client sees SUCCESS                 │       │
│                                                         │       │
│  ════════════════════════════════════════════════════  │       │
│                                                         │       │
│  RabbitMQ Consumer (async)                             │       │
│    │                                                    │       │
│    ├──► Read balances from cache                       │       │
│    │                                                    │       │
│    ├──► Apply transaction amounts                      │       │
│    │                                                    │       │
│    ├──► Update balances (version check)                │       │
│    │         │                                          │       │
│    │         ├── SUCCESS ──► Create Tx + Ops ──► DONE  │       │
│    │         │                                          │       │
│    │         └── VERSION CONFLICT                       │       │
│    │                   │                                │       │
│    │                   ├──► Refresh cache               │       │
│    │                   │                                │       │
│    │                   └──► Retry (up to 5x) ──────────┘       │
│    │                                                            │
│    └──► After 5 retries: DLQ (nothing persisted!)              │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```
