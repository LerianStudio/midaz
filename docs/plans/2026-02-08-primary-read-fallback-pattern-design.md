# Primary-Read Fallback Pattern: Resolving Replication Lag in State-Change Operations

## Overview

**Problem**: When clients create a transaction and immediately try to cancel it, the cancel operation fails with a 404 because the read replica hasn't yet replicated the newly created transaction from the primary database.

**Solution**: Implement a transparent primary-read fallback pattern at the repository layer. When a state-change operation (cancel, commit) reads from the replica and encounters a 404, it automatically retries the read from the primary database.

**Scope**: Transaction Service (Go); applies to cancel and commit operations initially; pattern can be extended to other read-before-write operations.

---

## Architecture

### Core Concept

```
Operation (Cancel/Commit) needs to read transaction:
  1. Call repository.GetTransactionWithFallback(id)
  2. Repository tries: Read from REPLICA
  3. If NOT FOUND in replica:
     - Automatically retry: Read from PRIMARY
     - If found in primary, return it
  4. If not found in either, return 404
  5. Caller proceeds with write to PRIMARY
```

### Design Principles

- **Non-invasive**: Existing queries remain unchanged
- **Opt-in**: Only operations that need fallback use `WithFallback` variants
- **Observable**: Metrics and logs track when fallback is triggered
- **Efficient**: Primary read only triggered on replica 404 (not double-read)
- **Safe**: No new concurrency issues introduced

---

## Components

### Repository Layer

Add new `WithFallback` method variants to the transaction repository:

```go
// GetTransactionByIDWithFallback tries replica first, falls back to primary on NotFound
func (repo *TransactionRepository) GetTransactionByIDWithFallback(
    ctx context.Context,
    orgID, ledgerID, txnID uuid.UUID,
) (*Transaction, error) {
    // Step 1: Attempt read from REPLICA
    tx, err := repo.getFromReplica(ctx, orgID, ledgerID, txnID)

    // Step 2: If found, return immediately
    if err == nil {
        return tx, nil
    }

    // Step 3: If NOT FOUND (404), try PRIMARY
    if isNotFoundError(err) {
        metrics.recordFallbackTriggered(ctx, "transaction_read")
        logger.Infof("Replica miss, falling back to primary for txn %s", txnID)

        tx, err := repo.getFromPrimary(ctx, orgID, ledgerID, txnID)
        return tx, err // Return whatever primary returns (found or not found)
    }

    // Step 4: Other errors (connection issues) return error as-is
    return nil, err
}
```

Similar method variants needed:
- `GetTransactionWithOperationsByIDWithFallback()`
- Other transaction queries used in cancel/commit paths

### Query Service Layer

Add `WithFallback` variants to Query service use cases:

```go
func (uc *QueryUseCase) GetTransactionWithOperationsByIDWithFallback(
    ctx context.Context,
    orgID, ledgerID, txnID uuid.UUID,
) (*Transaction, error) {
    // Delegates to repository.GetTransactionWithOperationsByIDWithFallback()
}
```

### Handler Layer

Update cancel and commit handlers to use the fallback variants:

**Cancel Transaction Handler:**
```go
// Before: Query.GetTransactionWithOperationsByID()
// After:  Query.GetTransactionWithOperationsByIDWithFallback()
```

**Commit Transaction Handler:**
```go
// Same change: use WithFallback variant
```

### Metrics & Observability

Track:
- `fallback_triggered_count` (per operation type: cancel, commit, etc.)
- `fallback_success_rate` (% of fallbacks where transaction found in primary)
- `fallback_latency_ms` (additional latency from fallback read)
- Alert if `fallback_triggered_count` exceeds threshold (indicates persistent replication lag)

Log events:
- Fallback trigger: `"Replica miss, falling back to primary for txn {id}"`
- With transaction ID for debugging and tracing

---

## Data Flow

### Cancel Transaction with Fallback

```
Client: POST /cancel/{transactionID}
  ↓
CancelTransactionHandler
  ↓
Query.GetTransactionWithOperationsByIDWithFallback()
  ├─ Try: Read REPLICA
  ├─ Miss → Fallback: Read PRIMARY
  └─ Return transaction object
  ↓
Validate transaction state (can be cancelled?)
  ↓
Command.CreateRevertTransaction() [writes to PRIMARY]
  ↓
Return success to client
```

### Commit Transaction with Fallback

```
Client: POST /commit/{transactionID}
  ↓
CommitTransactionHandler
  ├─ Query.GetTransactionWithOperationsByIDWithFallback()
  │  └─ Same replica-with-fallback logic
  ├─ Validate state
  └─ Command.UpdateTransactionStatus() [writes to PRIMARY]
  ↓
Return updated transaction to client
```

---

## Error Handling

### Error Classification

| Scenario | Action | Outcome | Log Level |
|----------|--------|---------|-----------|
| Replica returns 404 | Fallback to primary | Transaction found or legitimately missing | Debug |
| Primary returns 404 after replica miss | Return 404 to client | Client receives accurate error | Info |
| Replica connection error (not 404) | Return error immediately | Client receives 500 | Error |
| Primary connection error after replica miss | Return connection error | Client receives 500 | Error |
| Transaction deleted between read and write | Write operation fails (existing validation) | Returned as conflict/invalid state | Info |

### Race Conditions

**Race Condition: Transaction deleted after read, before write**
- Scenario: Transaction read successfully, but deleted before the write attempt
- Mitigation: Existing transaction state validation and optimistic locking catch this
- No new issues introduced

**Race Condition: Primary ahead of replica with other changes**
- Impact: Non-issue—primary has authoritative state; operations use current truth
- Outcome: Correct behavior

---

## Testing Strategy

### Unit Tests (Repository Layer)

```go
// Test 1: Replica success - no fallback needed
TestGetTransactionWithFallback_ReplicaHit
  - Mock replica returns transaction
  - Assert: Primary not called, transaction returned

// Test 2: Replica miss - fallback to primary
TestGetTransactionWithFallback_ReplicaMiss_PrimaryHit
  - Mock replica returns NotFound
  - Mock primary returns transaction
  - Assert: Transaction returned, fallback triggered

// Test 3: Both replica and primary miss
TestGetTransactionWithFallback_BothMiss
  - Mock both return NotFound
  - Assert: 404 returned, fallback triggered

// Test 4: Replica error (not 404) - no fallback
TestGetTransactionWithFallback_ReplicaConnectionError
  - Mock replica returns connection error
  - Assert: Error returned immediately, primary not called
```

### Integration Tests (Handler Level)

```go
// Test 1: Cancel after rapid create (simulating replication lag)
// 1. Create transaction
// 2. Immediately cancel (replica lag simulated by mock)
// 3. Assert: Cancel succeeds (fallback works)

// Test 2: Commit after rapid create
// Same pattern for commit operation
```

### Performance Tests

- Baseline: Normal read latency from replica
- With fallback: Latency when fallback triggers (should be negligible)
- Load test: Verify fallback doesn't create primary bottleneck

### Observability Tests

- Verify metrics recorded when fallback triggers
- Verify logs contain transaction ID
- Verify alert thresholds work

---

## Backwards Compatibility

- Original methods (`GetTransactionByID`) remain unchanged
- New methods (`GetTransactionByIDWithFallback`) are additive
- No breaking changes to API or handler contracts

---

## Future Considerations

This pattern can be extended to:
- Other operations with read-before-write patterns (e.g., account balance updates)
- Multiple fallback layers (if needed in extreme scenarios)
- Configurable fallback timeout (currently immediate)

---

## Decision Log

- **Approach Selected**: Repository layer fallback (Option A)
- **Rationale**: Single source of truth, reusable, easy to audit, prevents future inconsistencies
- **Alternative Considered**: Explicit fallback in handlers (Option C) - rejected due to higher maintenance burden
- **Alternative Considered**: Query wrapper pattern (Option B) - rejected in favor of direct method simplicity
