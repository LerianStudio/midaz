# AGENTS.md — Ledger Async Transaction Persistence

## Scope

These instructions supplement the root `AGENTS.md` for accounting engine
write-behind: preparation, responses/replay, individual reads, persistence,
transport, recovery, and cleanup. Synchronous completion and fallback share
these rules. Unrelated CRM, fees, and onboarding jobs are outside this scope.

This file lives in `internal/` because the workflow spans services, adapters,
and bootstrap. Paths below are relative to this directory. Run commands from
the repository root. Keep all `AGENTS.md` content in English.

## Read First

- [Engine architecture](../../../docs/architecture/engine.md): accounting boundary, receipts, protection, and rollout.
- [Recovery inventory](../../../docs/runbooks/transaction-recovery-inventory.md): record families, quarantine, and reprocessing.
- [Batch contract](../../../docs/api/atomic-transaction-batch.md): identity, order, limits, and replay.
- [Telemetry](../../../docs/standards/telemetry.md) and [error handling](../../../docs/standards/error-handling.md): binding standards.
- [Performance report](../../../docs/performance/engine-report.md): measurements and evidence limitations.

For contract changes, inspect the active spec at the root resolved by OpenSpec.
Expose discrepancies between code, documentation, and specs; do not silently
turn incidental behavior into a new guarantee.

## Responsibility Map

| Area | Files |
| --- | --- |
| Response and persistence policy | `services/command/create_transaction_engine.go`, `services/command/transition_pending_engine.go`, `services/command/revert_transaction.go`, `services/command/create_atomic_transaction_batch_completion.go` |
| Evidence and causal validation | `services/command/transaction_write_behind_envelope.go`, `services/command/engine_write_behind_evidence_codec.go`, `services/command/dependency_aware_transaction_completion.go` |
| Shared projection and events | `services/command/transaction_completion_service.go`, `services/command/transaction_write_set.go` |
| SQL transactions, locks, duplicate equivalence, and bulk | `adapters/postgres/completion/completion.postgresql.go` |
| Accounting publication | `adapters/redis/engine/wire.go`, `adapters/redis/engine/scripts/engine/` |
| Lookup and retention | `adapters/redis/transaction/engine_write_behind.go`, `adapters/redis/transaction/atomic_batch_recovery_ack.go`, `adapters/redis/transaction/recovery_cleanup.go`, and associated scripts |
| Individual reads | `services/query/resolve_engine_write_behind_transaction.go`, `adapters/http/in/transaction_core.go` |
| Transport | `adapters/rabbitmq/engine_write_behind_producer.go`, `bootstrap/engine_write_behind_dispatcher.go`, `bootstrap/rabbitmq_engine_write_behind_dispatch.go` |
| Recovery and wiring | `bootstrap/redis.consumer_recovery.go`, `bootstrap/redis_recovery_consumers.go`, `bootstrap/applied_transaction_completion.go`, `bootstrap/config.rabbitmq.go` |

## Accounting Boundary and Success

1. Go normalizes, validates, and prepares postings and completion context.
   Live balance approval, overdraft arithmetic, and versions remain in atomic Lua.
2. The engine publishes movements and recoverable evidence, writing the receipt
   last. Failure after writes begin may leave applied state: Redis script
   atomicity does not roll back earlier commands.
3. Confirmed accounting with intact evidence permits response and replay
   composition. SQL/Mongo are subsequent projections of that decision.
4. Consumers, fallback, and recovery complete that evidence through the shared
   completer. None may recalculate or reapply accounting.

- Distinguish pre-write refusal, confirmed application, and unknown outcome.
  Timeout or invalid response does not authorize another execution identity,
  claim release, or automatic balance compensation.
- Resolve unknown outcomes through the original identity and evidence. Do not
  introduce generic accounting retries in HTTP, consumers, or recovery.
- Projection failure after confirmed accounting must not become financial
  refusal. Retain evidence and report deferred work at the responsible boundary.
- Public response, replay availability, durable projection, and terminal
  lifecycle are distinct states. Public `CREATED` does not prove terminality;
  a durable `PENDING` hold can still require protected guards/receipts.

## Sync, Async, Transport, and Projection

- `RABBITMQ_TRANSACTION_ASYNC` selects async dispatch or synchronous completion.
  Failed, rejected, or uncertain Rabbit confirmation requires shared projection
  fallback. Preserve the specified materialized-cache-failure fallback contract
  when changing that integration.
- A successful socket write is not delivery confirmation. Preserve publisher
  confirms, mandatory/returns, persistent messages, correlation, and timeouts in
  ST and MT. Isolate confirms from incompatible legacy producers.
- Fallback, redelivery, and recovery can race. Equivalent duplicates converge;
  genuine conflicts must be reported without overwriting valid projections.
- SQL committed with Mongo pending is not complete projection. Retries verify
  SQL and complete/verify frozen metadata before acknowledging durability.
- Publish projection events after SQL and metadata confirmation, using the
  durable lifecycle phase. Preserve the existing best-effort contract; do not
  promise exactly-once delivery or publish early merely because HTTP returned 201.
- Bulk uses grouped SQL persistence, row limits, and ordered locks. Preserve
  causal order and result correlation even if internal ordering differs from
  input. Process every wrapper entry, not just the first.
- Do not acknowledge messages containing incomplete units. Engine envelopes
  must not enter the legacy balance-mutation consumer path.
- Propagate cancellation, deadlines, and tenant context into SQL, Mongo, and
  broker calls. Do not launch unmanaged goroutines to bypass cancellation.

## Lifecycle, Fees, Lookup, and Replay

- Commit/cancel may precede SQL hold creation; revert may precede origin
  projection. Resolve authenticated evidence and predecessors through bounded
  causal completion, rejecting cycles and invalid references.
- Redelivered holds must not regress durable commit/cancel or remove operations.
  Preserve each new action's validations, including Tracer and skip policy.
- Preserve each prepared fee leg's IDs, account, balance key, side, RouteID,
  materialized decimal amount, description, chart of accounts, metadata, and fee
  marker. Repeated accounts or routes do not authorize merging operations.
- Completion must not reload packages/routes to repeat validation, recalculate
  fees or Share/Remaining, redo routeFrom inheritance, or replace RouteID with
  legacy Route. Later configuration changes do not alter the original projection.
- Individual GET reconstructs transaction, operations, and metadata from
  evidence when needed. Materialized cache is an execution-fenced accelerator.
  After cleanup, use the designated primary read path to avoid replica-lag
  regression. Lists and metadata searches remain eventually consistent.
- For batches, capture/finalize the original response before slow projection
  and the last ACK. Replay preserves original IDs, order, status, and content
  after individual lifecycle changes. Never reconstruct creation from current
  state alone.
- Preserve distinct v1/v2 contracts, scoping/auth, two-key skip gates, and
  annotation/NOTED. Write-behind does not expand fee or lifecycle eligibility.

## ACK, Protection, and Recovery

- Broker ACK, recovery ACK, and protection cleanup are distinct operations.
  Recovery ACK failure after durable projection may leave repeatable work;
  it does not justify undoing accounting or forcibly deleting evidence.
- Deletion must verify expected identity/version and value. A delayed consumer
  must not delete a newer execution's cache or a concurrently replaced record.
- Retain evidence referenced by predecessors, replay, and open holds. Cache or
  replay TTL must not erase the only pending projection evidence or permit
  protected executions to be accounted again.
- Keep legacy and engine record families separate, including attempt counters.
  Do not deduplicate across hashes solely by transaction/execution field.
- Recovery uses bounded, cancelable retry/backoff. Permanent/exhausted failures
  require durable quarantine/isolation; preserve exact payload before conditional
  deletion. Quarantine failure or concurrent replacement retains original evidence.
- Reprocessing enters a compatible dispatcher/completer, never the engine.
  Validate tenant, organization, ledger, transaction, execution, and integrity.
- Deploy compatible readers/consumers/ACKs before new writers. Rollback requires
  demonstrated drain or continued compatible consumers. An empty inventory page
  does not prove all families and tenants have drained.

## Changes and Verification

- For format changes, inspect Go codecs, wire/Lua, readers, consumers, ACK,
  cleanup, and legacy fixtures together. Follow the README in
  `adapters/redis/engine/scripts/engine/` and composition order in `script.go`.
- Count evidence, indexes, dependencies, fees, and captures in their corresponding
  serialized budgets before accounting. Do not raise budgets or change error
  codes merely to accommodate implementation.
- Use bounded metric labels for publication, fallback, completion, backlog, age,
  and recovery. Never expose payloads, metadata, monetary amounts, or IDs in
  labels; follow the root logging and error standards.
- Cover affected failures: lost confirms, SQL success/Mongo failure, out-of-order
  delivery, concurrent duplicates, missing/corrupt cache, TTL, delayed ACK,
  partial capture, frozen fees, and ST/MT isolation. Tests must be deterministic;
  do not use `time.Now()` or sleeps as proof.

Reference commands; select affected packages and cases:

```sh
rtk go test ./components/ledger/internal/services/command ./components/ledger/internal/services/query -run 'EngineWriteBehind|Completion|AtomicTransactionBatch|Pending|Revert' -count=1
rtk go test ./components/ledger/internal/bootstrap ./components/ledger/internal/adapters/rabbitmq -run 'EngineWriteBehind|EngineRecovery|BTO|Bulk' -count=1
rtk go test -tags=integration ./components/ledger/internal/adapters/postgres/completion -run 'EngineWriteBehind|Completion' -count=1
rtk go test -tags=integration ./components/ledger/internal/adapters/redis/engine ./components/ledger/internal/adapters/redis/transaction ./components/ledger/internal/adapters/rabbitmq ./components/ledger/internal/bootstrap -run 'EngineWriteBehind|Compatibility|EngineRecovery|AtomicBatch' -count=1
rtk go test -tags=integration ./components/ledger/internal/adapters/http/in -run 'EngineWriteBehind|DirectV2FeeAccountingRoutes' -count=1
```

Integration tests require infrastructure described by their harnesses. Skipped
tests are not passing evidence. For broad changes, run root gates:
`rtk make test-unit`, `rtk make lint`, `rtk make check-telemetry`,
`rtk make test-openapi-locks`, `rtk make check-docs`, and `rtk git diff --check`.

A dispatch benchmark with an in-memory completer does not measure sustainable
throughput with persistence. Performance claims require admitted/projected TPS,
stable backlog, drain time, and HTTP/Lua latency under comparable load, including
ST/MT, warm/cold, and v1/v2. Check the performance report before declaring this
gate complete.
