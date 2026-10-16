# Balance-engine bench environment

Disposable local Postgres HA stack for the balance-engine bench (Compose project
`midaz-bench-engine`). Run only one instance per host; other worktrees share these fixed names:

| Service | Role | Host bind (override) |
|---|---|---|
| `bench-pg-primary` | PG17 primary, `cluster_name=bench-pg-primary` | `127.0.0.1:15432` (`BENCH_PG_PRIMARY_PORT`) |
| `bench-pg-standby` | PG17 synchronous standby `standby1`, `cluster_name=bench-pg-standby` | `127.0.0.1:15435` (`BENCH_PG_STANDBY_PORT`) |
| `bench-toxiproxy` | toxiproxy 2.12.0: `pg-client` proxy and API | `127.0.0.1:15434` (`BENCH_PG_CLIENT_PROXY_PORT`), API `127.0.0.1:18474` (`BENCH_TOXIPROXY_PORT`) |

Both proxies point at the primary. `pg-repl` (container port 5433) carries the standby's
replication and is not published on the host; `pg-client` carries client SQL, so client
faults and replication faults can be injected separately through the toxiproxy API.

> **Warning:** `bench`/`bench` are disposable bench credentials for this local stack only.
> The database `bench_transaction` carries the comment `midaz-bench-engine:disposable`
> on both nodes; destructive bench tooling should check that marker and `cluster_name`
> before touching a node.

## Commands

Run from the repository root. Pull once (toxiproxy 2.12.0 is not a common local image),
then the stack starts without network access:

```bash
docker compose -f tests/bench/engine/docker-compose.yml pull
docker compose -f tests/bench/engine/docker-compose.yml up -d --wait --wait-timeout 120
```

DSNs (use `127.0.0.1`; Docker binds IPv4 loopback only):

```text
primary   postgresql://bench:bench@127.0.0.1:15432/bench_transaction
standby   postgresql://bench:bench@127.0.0.1:15435/bench_transaction
pg-client postgresql://bench:bench@127.0.0.1:15434/bench_transaction
toxiproxy http://127.0.0.1:18474
```

Post-up check (requires host `psql` and `timeout`). The healthchecks prove the standby is
in recovery with a streaming WAL receiver; the primary must show exactly one synchronous `standby1`:

```bash
P="postgresql://bench:bench@127.0.0.1:${BENCH_PG_PRIMARY_PORT:-15432}/bench_transaction?connect_timeout=5"
timeout 30 sh -c "until [ \"\$(psql $P -tAXc \"select count(*) filter (where application_name = 'standby1' and state = 'streaming' and sync_state = 'sync'), count(*) from pg_stat_replication\")\" = '1|1' ]; do sleep 1; done"
```

Stop, keeping data: `docker compose -f tests/bench/engine/docker-compose.yml down`.
Remove the bench volumes too: add `-v`. Both commands touch only this project.

## Schema bootstrap

`bootstrap.go` rebuilds the `public` schema of `bench_transaction`: ledger migrations
000000–000046 through the lib-commons Migrator (one file per Exec, so migration 46's
`CREATE INDEX CONCURRENTLY` runs outside a transaction), then the lib-commons `outbox_events`
schema from the module version `go.mod` pins, then `schema_bench.sql`. Before any DDL it refuses
unless:

- the four `BENCH_*` variables resolve to loopback IP literals, user `bench` and database
  `bench_transaction`, after query overrides, fallback hosts and `PG*` defaults;
- primary and client proxy report `cluster_name=bench-pg-primary`, no recovery and
  `synchronous_commit=on`; the standby reports `bench-pg-standby` in recovery; all three carry the
  marker and one `system_identifier`;
- `standby1` is the only replica, `streaming | sync`, and the standby's WAL receiver streams.

Limits: the guard waits at most 120 s for `standby1`. `lock_timeout=30s` turns a lock held by
another session into a failure; no session is terminated. The Migrator stops watching the context
once it starts, so `go test -timeout` bounds the migrations. Every commit waits for `standby1` and
nothing downgrades `synchronous_commit`: a lost standby stalls the run until that timeout fails it.

```bash
env BENCH_PG_PRIMARY_DSN='postgresql://bench:bench@127.0.0.1:15432/bench_transaction?sslmode=disable' \
  BENCH_PG_CLIENT_PROXY_DSN='postgresql://bench:bench@127.0.0.1:15434/bench_transaction?sslmode=disable' \
  BENCH_PG_STANDBY_DSN='postgresql://bench:bench@127.0.0.1:15435/bench_transaction?sslmode=disable' \
  BENCH_TOXIPROXY_URL='http://127.0.0.1:18474' \
  go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestBenchEnv|TestBootstrapSchema|TestBootstrapRejects' -timeout 10m -v
```

Without the four variables the database tests skip; a partial set fails them. Each database test
sets `ALLOW_INSECURE_TLS=true` in the test process for its own duration, as the Migrator requires
on this `sslmode=disable` stack. The refusal test changes the marker only after the guard proves
the stack, and restores it on cleanup. A run killed before cleanup fails closed: every later
bootstrap refuses the stack until it is recreated with `down -v` and `up`.

## Seed fixtures

`seed.go` re-runs the guard, then in one synchronous transaction streams N ordinary balances plus
hot, fee and two multikey balances (N+4) and their controls (N+3: multikey's two keys share one
account) with `COPY`, and inserts an ownerless fence whose lease already expired. IDs derive from
class, index and key, so every run writes the same rows. `fillfactor=70` on `balance` and
`account_control` is an untested HOT-update hypothesis; `VACUUM (ANALYZE)` follows the commit.
`TestBootstrap` seeds 10k twice (rows must hash alike) and 1M once, with 60 s and 10 min seed
deadlines. At 1M it checks counts, sums, constant columns and named rows, not every row's IDs:

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBootstrap$' -timeout 30m -v
```

## Obligation catalog

`obligations.go` builds what completion owes after one approved v2 create of
`testdata/create_v2_body.json` (610 B; seeded aliases, org and ledger), with one known Tracer
reservation. Only `metadata` keeps the body's client metadata; `exact2leg` and `feeLeg` post the body
without it, so the three inputs differ. Events come from the public `pkg/streaming/events` builders; the
operation inside `transaction.posted` mirrors the ledger's internal `operation.Operation` keys, which
`tests/bench` cannot import, and takes the transaction description as the ledger does for legs without
one. `buildObligations` maps an explicit request id, the transaction (`events.TransactionSource`) and its
already-decided operations; the fixtures wrap it with legs on seeded balances. It emits `fee_charge.applied`
only when the transaction metadata carries `feeApplied` = `"true"` and a `packageAppliedID`, for that package,
as `send_transaction_events.go` does. Event and private row IDs
derive from request, kind and ordinal. Timestamps carry nanoseconds, as product values do; builders that
format RFC3339 keep doing so.

| Fixture | posted | balance.changed | fee_charge.applied | metadata | Tracer confirm | Outbox rows |
|---|---|---|---|---|---|---|
| `exact2leg` | 1 | 2 | 0 | 0 | 1 | 4 |
| `metadata` (body metadata) | 1 | 2 | 0 | 1 | 1 | 5 |
| `feeLeg` (deductible fee 2: credit 98 + fee 2) | 1 | 3 | 1 | 2 | 1 | 8 |

Events go through the lib-streaming producer's `EmitBatch` with `WithOutboxTx`; the lib writes the row
ID (UUIDv7) and derives `aggregate_id` from the partition key, and the bench sets `EventID`. Metadata
(`ledger.metadata.upsert`) and Tracer (`tracer.reservation.confirm`) rows go through the lib-commons
`CreateManyWithTx` in the same `*sql.Tx`, with `aggregate_id` = request id. The producer's only
transport counts and refuses `Publish`: the test proves zero external delivery.

Bench-defined parts: the metadata row freezes the fields `frozenMetadataRecords` sets (entity, id, data,
`createdAt` = `updatedAt` = the transaction's creation, a v2 create's `TransactionDate`; Mongo generates
`_id`); the Tracer row is the known reservation's correlation with its transaction, not the `Confirm` RPC
body. Its reservation id is derived locally from the request for one representative obligation: no Tracer
call, reservation or skip flow runs. The `feeLeg` fee is an assumed deductible, non-deferrable fee without
accounting route: no fee configuration, fee engine or financial writer runs here.

```bash
go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestObligation|TestBuildObligations' -v
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestOutboxOnlyEmitBatch$' -timeout 10m -v
```

The live test rebuilds the schema (dropping any seeded balances: it needs no seed and leaves the database
unseeded with eight outbox rows), commits one `feeLeg` request and rolls back another. It logs each
payload, the stored `payload::text` and `pg_column_size` for this fixture; these are not general product
sizes. Trace-carrier and multi-tenant variants, relay and Kafka/CloudEvents record bytes are not measured.

## Workload and metrics

`writer.go`, `workload.go` and `metrics.go` hold the request contract, the deterministic workload, the open and
closed loops and the recorder. The generator tests drive a null writer with simulated approvals; the worker tests
exercise the SQL writer below. Open-loop rates report the offered rate sustained with the logged dispatch lag,
never financial TPS. A workload owns the process-clock function; real SQL runs pass that same function to the
batchers, while deterministic fixtures inject their controlled clock. No test reads the wall clock directly.

```bash
go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles|TestWorkloadNullWriter' -timeout 5m -v
```

Limits: runs are count-bounded and end on the count or the caller's context. A time-bounded closed-loop cell
needs a graceful stop separate from abort, which a later task adds. The recorder preallocates five latency series,
40 B per scheduled request (576 MB for 14.4 M requests).

## SQL session

`sql_session.go` is one writer's only connection, used by one goroutine at a time: a one-connection `*sql.DB`
over pgx `stdlib`, which runs COMMIT under the `BeginTx` context. `commit` returns nil only when COMMIT returned
no error and no notice; anything else is an unknown outcome and nothing retries it. The session pins
`client_min_messages=warning`, overriding the tested `options`/`PGOPTIONS` setting that suppresses warnings.
`TestSQLSessionCommit` rebuilds the schema, which drops seeded balances:

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 360 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestSQLSessionCommit$' -timeout 5m -v
```

Limits: a row visible on the primary is a local commit, never a replicated one. A run killed before cleanup can
leave standby replay paused: `remote_apply` commits then wait and standby reads go stale, while ordinary
`synchronous_commit=on` commits still succeed. Resume with `SELECT pg_wal_replay_resume()` on the bench standby only.

## Direct financial batch

`batch.go` and `batch_sql.go` run one batch on one SQL session. Each pass claims request identities, touches stored
outcomes to write WAL, locks balances and shared account controls, then commits decisions, transactions, operations,
balances, controls and outbox rows together. Postings use decimal balance arithmetic; a refusal discards every leg
of that request. Replays do not move money. A new decision crossing its admission cutoff before COMMIT rolls back
the whole pass, including decisions funded by that pass, and recomputes from fresh rows.

Each run has a 10 s cap; a pass also uses the earliest future response deadline in the batch. A pre-COMMIT deadlock
allows one retry. Any error or notice after invoking COMMIT makes every answer unknown, without retrying money.
The worker drain sets the batch size, up to the configured experimental cap of 256.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 960 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestBatch' -timeout 15m -v
```

The live tests rebuild the guarded schema. They cover mixed decisions and copies, expiry before and after a real
money-lock wait, the earliest response deadline, whole-pass rollback, a cancelled SyncRep wait and the replay's own
WAL barrier, eight concurrent sessions exhausting one shared limit, and a real deadlock with one retry. The replay
test keeps standby replay paused until the retry itself waits in SyncRep, then resumes it before checking the result.

The ledger checker counts each owed outbox obligation by identity and compared fields. It rejects missing, duplicate
and miscorrelated rows independently, in linear time over the committed operations and outbox. It still holds maps
proportional to those rows; the first outflow pilot measured whole-process memory below, not isolated checker memory.

Limits: no owner cache/fence, failover or capacity measurement. The earliest future deadline can turn a whole
backlog batch unknown; that cost remains unmeasured. Two empty-array lock queries still run on replay-only batches.
Eight concurrent sessions prove that interleaving, not freedom from all deadlocks.

## Ownerless queue and drain

`writer_ownerless.go` starts one worker per prepared SQL batcher over a fixed 8,192-attempt queue. Each caller has
its own buffered reply. Workers take only attempts already queued, up to the configured batch cap, without waiting
to coalesce. `Submit` counts late entry and limits the caller's wait to the remaining response budget. A caller
leaving after enqueue gets unknown; accepted work may still commit, but the financial admission cutoff never moves.

Call `Close` once, after all `Submit` calls return. Normal Close drains accepted work and joins workers before the
harness closes sessions or outbox resources. The writer lifetime or Close's own deadline can abort SQL. Close reports
each worker's first batch error after the join; a worker's batch error never cancels another worker's drain.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 960 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerlessWriter(Workers|Drain|ErrorIsolation|CallerLeaves|CloseDeadline|ExpiredEntry)$' -timeout 15m -v
```

Six live tests cover 32 named backends waiting concurrently, cap-8 drain of nine queued attempts, reply correlation,
error isolation during Close, caller cancellation and remaining budget, forced Close, expired entry and known replay.
Checks run after drain and owned-resource cleanup; goleak checks for remaining goroutines. The tests do not exercise
a full 8,192-attempt queue. The separate conservation gate below exercised 32 workers with cap 256 across all five
200k-request shapes; backlog still needs proof.

## Ownerless correctness gates

Four small gates repeat five reset rounds each. Identity races copies and divergent payloads over 32 workers,
then replays the stored winner after its original cutoff. A separate spent-credit case proves that replay cannot
fund a new debit. MultiKey exhausts one shared limit across two balance keys. Protection exhausts the hot balance
and checks sending, receiving and blocked-account refusals. Admission crosses a new refusal's cutoff before COMMIT
and verifies that the entire pass is discarded, including its approval.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 960 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerlessWriter(Identity|MultiKey|Protection|Admission)$' -timeout 15m -v
```

The verifier snapshots balances and controls once per fresh connection, after reset and fixture changes but before
any request. After all callers return and Close joins workers, it checks first-operation and final-state anchors,
per-posting floors and flags, shared limits and versions, captured answers and the exact winning posting multiset.
Expected transaction IDs come from captured requests, including when the caller got unknown. Unknown does not
prove rollback. `sql-approved/caller-unknown` counts SQL approvals without a new approved answer; a later replay
does not remove that count. The existing exact outbox checker runs once, with no per-row SQL.

These small gates do not establish volume or capacity. The conservation test below uses the workload's process clock,
a separate Close budget and a new baseline after each reset, before any request.

## Ownerless conservation

`TestOwnerlessWriterConservation` has five reset rounds: outflow, inflow, mixed, outflow_fee and mixed_fee.
Each uses 200,000 requests, 256 closed-loop producers, 32 workers, cap 256 and four ordinary counterparts.
The callers have a 5 min work budget; the writer stays alive for a separate 30 s Close after all callers return.
A Close error stops verification. Otherwise the baseline, ledger, answers and outbox are checked once, even if the
work budget expired. Every request must approve; unknown, replay, refusal, expiry, late entry or an error fails the run.

Run the single outflow pilot without race:

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 600 go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestOwnerlessWriterConservation$/^outflow$' -timeout 9m30s -v
```

The first pilot approved all 200,000 requests with no other outcome or error. Run: 195.791 s; checker: 22.104 s;
test: 218.38 s. The observed local rate was 1,021 durable approvals/s and answer p99 was 609.664 ms. This is below
the target, not a maximum-capacity or sustained result. The 35,584 batches averaged 5.62 requests, not the cap of 256.
Peak RSS was 909,460 KiB among the test binary and its reaped children, excluding Postgres; it is not a heap metric.
Lazy connection dials and prepares remained inside the first batches.

The complete gate at `d840eb78e` passed all five shapes and the four small gates: 5 tests, 26 subtests, no failures or
skips, 1282.347 s within the 30 min test limit. Each shape returned exactly 200,000 approvals, with no other outcome,
error, late entry, publish or deadlock. Each batch ran one pass. The run budget is 300 s; each request still has the
usual 30 s response and 28 s admission budgets.

| Shape | Run s | Checker s | Observed approvals/s | Answer p99 ms |
|---|---:|---:|---:|---:|
| outflow | 198.704* | 22.104 | 1006.5* | 771.248* |
| inflow | 183.231 | 22.186 | 1091.5 | 516.909 |
| mixed | 204.180 | 22.477 | 979.5 | 695.331 |
| outflow_fee | 274.877 | 36.133 | 727.6 | 837.249 |
| mixed_fee | 263.778 | 36.091 | 758.2 | 749.944 |

*The executor's wait command consumed a CPU core for about 100 s during outflow, violating the no-synthetic-load rule.
The process ended, and the supervisor removed an orphan pilot log monitor. Outflow's correctness result stands, but
its timing is excluded from comparison. No rerun hid the violation. The other rates describe one local closed-loop
run each, all below the target; they do not establish maximum or sustained capacity.

An independent read of the retained mixed_fee data on primary and standby found 200,000 approvals/transactions,
600,000 operations, 1,600,000 pending outbox rows, and an unchanged balance sum of 80,000,000,000. Previous shapes rely
on their own checks because each reset removes the preceding shape. Event delivery remains untested.

RSS readings disagreed: ru_maxrss reported 1,244,712 KiB; a hand-copied instantaneous sample reported 1,327,320 KiB.
The cause was not isolated, so the true peak is unknown. Both exclude Postgres. Only latency percentiles, not raw
series, were retained. WAL and spill deltas cover the whole command, not financial writes alone.

Run all five shapes and the small gates with `-failfast -timeout 30m` and an external `timeout 1860`. A failed shape
stops the expansion; do not raise budgets or reduce N. Add `-skip '^TestOwnerlessWriterConservation$'` to ordinary
regression and race commands so they do not select volume. No matrix, full-queue, owner or failover result is implied.

## Owner fence primitives

`writer_owner.go` provides only acquisition and renewal of the bench tenant's SQL fence. It is not yet an owner writer.
Acquisition increments the epoch only after the previous lease expires. Renewal requires the current owner, epoch
and a live lease. Both use `clock_timestamp()`, including after a lock wait, and the shared COMMIT notice classifier.
An error returns no usable acquisition token; a COMMIT error may coexist with a locally committed fence.

`renewFenceTx` renews and locks inside an existing transaction. The lock remains held through SyncRep. A competing
acquirer waits, then rechecks the committed renewal; the old lease it saw before waiting is not permission to take over.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 660 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerFence' -timeout 10m -v
```

Three live tests cover acquisition, renewal, expiry within a transaction begun earlier, stale epochs, both outcomes
of a lock recheck, and cancelled SyncRep for acquisition and renewal. The test proves the competing backend waits
on the renewing backend alone. Mutants removing expiry, epoch or notice classification, using transaction-start time,
or replacing renewal with a lock-only statement fail. Owner-only and tenant-only predicate mutants survive by
construction in this single-tenant fixture; no fabricated tokens were added to make them fail.

Limits: no renewal loop, paged reload, cache, owner queue integration, financial owner run or failover proof yet.
The ownerless baseline is unchanged apart from shared test-session setup and removal of redundant test cleanup.

## How it is built

- `init-primary.sql` runs once on a fresh primary volume. It writes the marker and
  persists the settings listed in that file with `ALTER SYSTEM`, without reload.
- The standby runs `pg_basebackup -R --no-slot --checkpoint=fast` through `pg-repl` with
  `application_name=standby1` on its first start, so it inherits the same
  `postgresql.auto.conf`. Only `cluster_name` differs, set on each command line.
- Either primary, including a promoted standby, requires `standby1` to confirm synchronous
  commits. Without that replica, commits wait.
- No service has a restart policy: a stopped primary stays stopped.
- `down` without `-v` keeps both volumes; the next `up` skips init and base backup, and
  both nodes keep the same system identifier.

## What this proves

Environment readiness, guarded schema rebuild, deterministic seed fixtures and the obligation
catalog written outbox-only through the libraries. The synchronous
standby streams through the proxy and inherits the primary's settings. Seed timing is setup cost,
not transaction throughput. Financial correctness, failover survival and unknown replay are untested.
