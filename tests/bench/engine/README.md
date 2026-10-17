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
go test -count=1 -tags bench ./tests/bench/engine/ -run 'TestWorkloadKeysUnique|TestOpenLoopSchedule|TestPercentiles|TestWorkloadNullWriter|TestClosedLoopWindow' -timeout 5m -v
```

The closed loop admits until its count n or its window, whichever comes first. Each attempt reads the clock before
claiming an index; that one instant decides admission and becomes `scheduledAt`. Ending admission cancels no context:
the join waits for answers in flight, and the caller's context stays the abort. A timed caller must require fewer than
n sent, since reaching n cuts the window short. Conservation keeps 200,000 with a 5 min window, equal to its work
budget; the null count run has 1 min. The open loop stays count-bounded on its absolute schedule.

`newRecorder(n)` preallocates five latency series before the run starts, 40 B per request of capacity n (576 MB for
14.4 M). The summary logs n as `capacity`, beside the actual dispatched and answered counts and their cohort rates,
which are not window rates. It also logs the window and counts approvals by `dbAckAt`, the host's observation after
COMMIT returned: before stop, at or after it, and without one. Non-approvals count in none. A commit whose caller
answered unknown is absent from these counts; only the post-run checker sees it, without a time. Late open-loop
dispatch can also produce after-stop approvals, so that count is not only drain. The summary prints no window rate: a run
that ends on its count leaves the window a ceiling. First-touch page faults can still occur during the run; their
cost is not measured. The conservation results below predate the window and this presize: their runs ended on the count.

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

Each worker records every batch it takes, before running it, failed batches included: the take instant as an offset
from the writer's start, and the attempts. That is 16 B per batch on amd64 plus slice growth, not measured. After
Close, `occupancy(from, to)` gives the batches taken in `[from, to)`, their attempts and the largest. It counts
dequeued work, not approvals or busy time; a batch taken before `from` and still running stays outside. The
conservation gate logs it from the run start to Close's return instead of the per-worker records; no 200k run has
logged it yet.

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
stops the expansion; do not raise budgets or reduce N. Add `-skip '^TestOwner(less)?WriterConservation$'` to ordinary
regression and race commands so they select neither writer's volume.
No matrix, full-queue, owner or failover result is implied.

## Owner fence primitives

`writer_owner.go` provides acquisition and renewal of the bench tenant's SQL fence, the paged reload below and the
owner writer after it.
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

Limits of this unit: no renewal loop, cache, queue integration or financial owner run; the owner writer below adds
them. No failover proof. The ownerless financial behavior is unchanged. Its control scan now also reads organization
and ledger IDs for the shared scanner. The earlier throughput observations predate that change; this unit does not re-measure them.

## Owner paged reload

`reload` reads every live balance by `id`, then every control by `(organization_id, ledger_id, account_id)`, in keyset
pages of autocommit statements on the owner's session. A pending tick renews the fence before a page; one renewal after
the last page always runs, and only its success returns the maps. A read, cancellation or renewal error returns
neither map. The final renewal proves no takeover since acquisition, not a live lease after return. Consistent maps
also require excluding writers that ignore the fence. The lock path and reload share column lists and scanners.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 660 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerReload' -timeout 10m -v
```

The paging test uses pages of 3 over the seed plus a second organization and ledger, a live balance, a soft-deleted
balance on the same key and a control: 9 live balances in pages 3/3/3/0 and 8 controls in 3/3/2. It compares keys and
values rendered independently by PostgreSQL. Table locks stretch the reload beyond its initial lease; tick renewals
preserve ownership. Each release leaves about 1.5 s until the next renewal must run, so host load can exhaust that margin.

Two other tests use 50,000-row pages over the small seed. A successor taking over during the last page makes the final
renewal refuse. Cancelling the control read returns `context.Canceled` and discards the balances already read. Both
failures return nil maps. Scratch mutants proved the final-renewal, pagination, soft-delete and partial-return checks.
Removing both tick renewals fails; removing just one loop's renewal survives this short fixture. Other survivors
include partial returns on tick errors and cursor variants the fixture does not distinguish. The code keeps strict
cursors, matching ORDER BY and nil maps on every error; mutant survival is a coverage limit, not proof of equivalence.

Limits of this unit: no ticker, cache, queue integration or owner financial run; the owner writer below adds them. No
1M reload or failover. Concurrent administrative writers are not excluded by the fence and stay out of the bench until
Epic 3.2.

## Owner writer

`newOwnerWriter` acquires the fence, then reloads every balance and control in pages of 50,000 under a 1 s ticker and
a 3 s lease. It then serves one worker on the ownerless queue, `Submit` and `Close`, on one session. Idle, the worker
renews on each tick, each renewal bounded by one lease. A financial pass renews and locks the fence as its first
statement after BEGIN, then claims and touches identities as before. It decides on copies of only the cached rows its
admitted postings name. A confirmed COMMIT stamps the acknowledgement, then publishes those rows to the cache, then
answers. Rollback, a cutoff rerun and an unknown COMMIT publish nothing. Any final batch error or failed tick renewal
ends the owner: it drops the cache, keeps the first cause and answers later attempts unknown without SQL until Close
reports that cause. Nothing reacquires, reloads or releases the fence. A COMMIT confirmed after the lease expired
stays an approval, because the pass held the fence row through it; the next renewal refuses.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 960 go test -count=1 -race -tags bench ./tests/bench/engine/ -run '^TestOwnerWriter(Lease|Successor|IdleLoss|Identity|MultiKey|Protection|Admission)$' -timeout 15m -v
```

`TestOwnerWriterLease` keeps an idle owner past the lease it started with on the database clock, refuses a rival and
approves a payment. A second payment parks on a held balance row past its in-pass lease, and a rival takeover waits
only on that pass. Its COMMIT approves, the rival then takes epoch 2. The next renewal refuses: the next attempt
answers unknown, no claim persists, and the one after runs no SQL. That a pass renews before it
claims is source order, not tested.

`TestOwnerWriterSuccessor` cancels a financial COMMIT in SyncRep once the owner is seen inside its pass. That COMMIT
answers unknown though it committed locally; a request on untouched balances then answers unknown without SQL. The
successor's constructor waits in SyncRep while the paused standby lacks the original. Once replay resumes, the
successor replays the original request past its cutoff with only a touch, and approves on the versions the original
left.

`TestOwnerWriterIdleLoss` pauses replay under an idle `remote_apply` owner that never receives a request. Its tick
renewal leaves SyncRep after its 3 s context expires while replay stays paused. After replay resumes, no later renewal
keeps the fence: the lease expires with the writer open, and Close reports `renew on tick`. The test observes that
server-side cleanup completes within its 30 s wait; the 3 s context is not a strict fence-lock release deadline.

The four small gates share their bodies with the ownerless ones:
`TestOwnerWriter(Identity|MultiKey|Protection|Admission)` run the same five reset rounds and expectations with one
owner worker instead of 32. Fixture changes precede the constructor's reload. Spent-credit needs the cache the
spend's COMMIT published. Admission needs its void pass to publish nothing. Its busy pass now waits on its own claim,
held in flight, instead of a balance row lock, for both writers: both claim before deciding, and an owner refuses
from its cache without touching a balance row.

Final supervisor verification: the owner and ownerless gates plus the three lifecycle tests passed with race and
shuffle, 11 tests and 42 subtests in 62.683 s. The complete package without conservation passed, 44 tests and 68
subtests in 101.449 s. The author's earlier complete race run passed in 283.618 s. All had zero failures or skips;
vet, gofumpt, pinned lint v2.14.0 and stack cleanup passed. Before the fix, IdleLoss failed because Close returned nil. Without
the 3 s bound, its renewal stayed in SyncRep through the 30 s wait. Two more scratch overlay mutants failed
behaviorally. Without publication after COMMIT, spent-credit's spend never reached its balance update, and an Identity
round turned unknown on `update balances: 3 rows, want 5`. Publishing before the cutoff check made Admission's rerun
hit `update balances: 0 rows, want 2`. Each survived the other gate.

`TestOwnerWriterConservation` runs the shared conservation body with one owner worker instead of 32: the same five
shapes of 200,000 requests, producers, cap, budgets, checker and assertions. The owner acquires and reloads after the
baseline, on the subtest context that stays its lifetime, so it has no startup deadline of its own: the command's
`-timeout 30m` and external `timeout 1860` bound it. Those bounds fail the gate; they do not promise every shape
finishes. A detected lease loss, an expired run budget or a Close error fails the shape, with no restart or budget change.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 1860 go test -count=1 -failfast -tags bench ./tests/bench/engine/ -run '^TestOwnerWriter(Identity|MultiKey|Protection|Admission|Conservation)$' -timeout 30m -v
```

The gate ran once on `0d5aa04fa` plus this shared entry, without race, retry, tuning or reduced N, and passed: 5 tests
and 26 subtests, no failures or skips, 762.832 s within the 30 min limit. Each shape returned exactly 200,000
approvals, with no other outcome, error, late entry or publish, and Close returned nil before the checker ran. The log
also shows one pass per batch and no deadlock; the test logs those counts but does not assert them.

| Shape | Run s | Checker s | Observed approvals/s | Answer p99 ms | Mean requests/batch |
|---|---:|---:|---:|---:|---:|
| outflow | 97.386 | 23.035 | 2053.7 | 200.838 | 134.68 |
| inflow | 100.166 | 22.416 | 1996.7 | 229.865 | 135.14 |
| mixed | 100.886 | 22.661 | 1982.4 | 219.737 | 135.50 |
| outflow_fee | 155.245 | 35.835 | 1288.3 | 345.466 | 137.84 |
| mixed_fee | 153.603 | 35.937 | 1302.1 | 349.310 | 138.12 |

Approvals/s is 200,000 over run seconds: the closed-loop rate of 256 producers, each with one request in flight, in
one run on a shared, loaded host. It is not a maximum, sustained, open-loop, repeated, RTT-controlled or capacity
result. Every rate is below the plan's 8k target and every p99 far above its 15 ms hypothesis; method and RTT differ,
so this gate neither accepts nor rejects either. Mean batch is 200,000 over batches; that run did not log the maximum
batch, and cap 256 is a limit, not occupancy. The ownerless table above is not a controlled comparison: those runs predate
the shared scanner, queue and batch changes and ran under different host conditions.

An independent read-only check of the retained mixed_fee state, on primary and standby, found 200,000 approved
requests and transactions, 600,000 operations, 1,600,000 pending outbox rows (8 per request), 8 balances and 7
controls. Balance, version, limit, cutoff and outbox checks and a match of every request against the workload
generator passed on both; row digests of all seven tables matched between them. The balance sum stayed
80,000,000,000. The final fence held owner `bench-conserve` at epoch 1, corroborating that the owner path ran. Resets
removed the four earlier states; their proof is each shape's checker and the raw log.

With the fence, reload, runtime and small-gate proofs above, this completes the owner writer's correctness gates
(plan Task 1.2.2). Volume conservation is not universal correctness.

Limits: one run; no 1M reload, failover or capacity result. Process and Postgres CPU and memory and WAL per request
were not measured; raw latency series were not retained. Close took microseconds because it closes the queue and
joins the idle worker. It does not release the lease; SQL row locks end with the transaction. The selection excludes Lease,
Successor and IdleLoss: no owner loss was detected, but the run adds no lease-loss evidence. Event delivery remains
untested. Owner drain, deadline Close and caller exits reuse the ownerless queue proofs above, not owner-specific
tests. An extra copy of the cache on the terminal path is not observable. After a pass outlasts its 3 s lease, the
next renewal ends the owner; a confirmed COMMIT stays approved. Close can finish before that renewal, so a last-pass
overrun alone need not fail the gate.

## Replication latency and SQL round trip

Status: accepted preparation (M3A); the quick cell below uses it.

`setReplLatency(env, ms)` in `toxiproxy.go` drives only `pg-repl` through the pinned Toxiproxy 2.12.0 SDK. It reads the
proxy and refuses it unless it listens on port 5433, points at `bench-pg-primary:5432` and is enabled. It removes
every existing `pg-repl` toxic, then adds one latency toxic, `pg-repl-latency`, on the downstream stream: primary to
standby, carrying WAL and keepalives. Toxicity is 1 and jitter 0; `ms` 0 adds nothing. It reads the toxics back,
and fails unless they are exactly the requested state. It never touches `pg-client` and never calls
`/reset` or PATCH. Each SDK call is bounded only by the client's fixed 30 s timeout and the server's 25 s handler
timeout; there is no context. After any error the toxic state is unknown: nothing retries, and the caller's cleanup
reads and removes once more. The delay is whole milliseconds per chunk Toxiproxy reads, in one direction, not an RTT.
Toxiproxy splices an add into the live replication link; a removal releases buffered WAL undelayed, so remove only
with no workload running.

`probeReplRoundtrip` observes `sql_roundtrip_pg_repl`. It runs one psql session inside `bench-pg-standby` through
`bench-toxiproxy:5433`, with `LC_ALL=C` and `-X -q -A -t -v ON_ERROR_STOP=1`, sends `\timing on` and 35 `SELECT 1`, and
keeps the last 32 samples after 3 warm-up queries. Process startup, connection and authentication finish before the
first query, so no sample contains them. An outer 60 s context kills the docker CLI; `timeout 50s` inside the container
ends psql, which the CLI's death would not. The parser accepts only `1` rows alternating with `Time: N.NNN ms` lines,
with or without psql's parenthesised copy from 1 s up. It requires exit 0 and exactly 35 pairs; anything else fails the
probe, and no sample is ever zero-filled. A sample spans both proxy directions, server execution and scheduling on
both ends: it is neither network RTT nor PG acknowledgement lag. This helper does not measure replication ACK lag.

```bash
env BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 300 go test -count=1 -race -shuffle=on -tags bench ./tests/bench/engine/ -run '^(TestReplLatency|TestParsePsqlTimings)$' -timeout 4m30s -v
```

`TestReplLatency` passes the identity guard, reads standby1's walsender pid and start time, registers cleanup, sets
20 ms and probes. It then removes the toxic and waits for standby1 streaming and synchronous. The walsender is the same
after the add and after the removal, so neither reconnected replication. Every sample is at least the configured
20 ms; no upper bound is asserted. `TestParsePsqlTimings` parses the preflight's captured output shape plus one line
over 1 s, and rejects an incomplete output and a comma decimal.

Five passing runs with 20 ms downstream, 32 samples each, on one shared, loaded host:
minimum 20.296-20.360 ms, lower median 20.356-20.411 ms, maximum 20.416-20.683 ms. These describe this probe on this
host, not a benchmark or a target. The non-volume suite passed 47 tests and 72 subtests in 102.107 s, with zero
failures or skips; vet, gofumpt, pinned lint v2.14.0 and stack cleanup passed.

An independent scratch check observed the existing replication link: one WAL-writing synchronous commit took
0.770 ms without a toxic and 20.869 ms with 20 ms downstream; the last standby message's receipt-minus-send time
changed from 0.203 ms to 20.341 ms, on the same walsender. These are single observations, not a benchmark.

Limits: the committed tests cannot distinguish a consistently wrong upstream setting; the scratch one-way check
caught that mutant. Outer CLI kill and SDK/server timeouts remain untested; the inner timeout and cleanup after a
probe failure were exercised. Process death skips test cleanup, so the next preflight must check for leftover toxics.
The helper does not inspect `pg-client`; the stack state check reports its toxics. Final supervisor tests passed
47 tests and 72 subtests in 103.629 s, with no failures or skips. The final cleanup saw one transient `engine.test`
process of unknown origin; a separate recheck found none and confirmed a clean stack. That unit ran no timed cell,
200k run or capacity measurement.

## First timed cell (quick)

Status: accepted (M3B). One canonical 60 s + 20 s run passed; its result below is one closed-loop observation, not a
capacity or sustained result. Plan Task 1.2.4 stays open.

`TestBenchHotAccount` in `bench_test.go` runs the cell `BENCH_MATRIX` names. Unset, it skips, so ordinary regression
selects it and runs nothing; `quick` runs the cell below, `full` one block A cell (next section); anything else fails
before any database work. The cell: one owner writer on a fresh 10k-counterpart seed, outflow without fee, 256
closed-loop producers, batch cap 256, the usual 30 s response and 28 s admission budgets, and 1 ms downstream on
`pg-repl`. A 60 s warm-up and a 20 s measured phase,
each with its own workload label and recorder, share one process clock, one owner lease, one capture and one Close.

1. Guard, rebuild, seed and record the baseline under a 60 s setup clock. Register cleanup, configure the delay,
   probe idle and start the owner on its lifetime context. The owner's wait shares the setup clock with the delay
   and probe, whose calls have their own bounds. Migrations ignore the context once `Up` starts.
2. Allocate the capture for 960,000 + 320,000 calls and both recorders, then warm up. An invalid warm-up skips measurement.
3. Read S1, run the measured phase, read S2 on the verification connection (`measure.go`). Each read is bounded at 10 s.
4. Close once after every Submit returns, with a fresh 30 s bound. Probe, remove the delay and verify synchronous replication.
5. Run the checker over all captured calls under a 5 min bound. The final test result, including cleanup, validates the run.

The test fails, and prints no rate, on an invalid measurement:
- a phase with a run error (a late answer included), an abort of its own bound (window + 30 s + 2 s), or admission
  that ended on its count instead of its window;
- a missing boundary read, or standby1 not streaming sync at S1 or S2;
- a Close error, which also stops verification: an aborted backend may still commit.

It also fails on any failed ledger, baseline or answer check, any publish, and any replay, conflict or refusal: new
identities on funded, unblocked outflow produce none. Expired and unknown answers, and SQL approvals whose caller read
unknown, are reported, not failed. A rate below 8k/s or a p99 above 15 ms is a measured result, not a failure. The
window rate uses approvals with `dbAckAt` before stop over the 20 s window. A later cleanup failure invalidates a
printed rate. Measurement starts after the warm-up join, so pipeline refill is inside the measured window.

The caps are 16k attempts/s over each window, twice the target. They bound memory, not capacity: about 553 MB of
recorders, capture and postings at full caps, before the checker, GC, SQL and slack. Reaching one invalidates the
phase; nothing raises it or retries.

The log also carries: both probes; each phase's summary; the warm-up's unknown count, an upper bound on carried-over
attempts (zero proves none), and queue length at the join, with in-flight work unmeasured; the S1-to-S2 line; writer
passes and deadlocks over its lifetime, warm-up included; entry-late; publishes; measured-window occupancy; stage
timings and bounds; the checker's tally. The capture and in-process generator affect rate, CPU and GC; that effect is
unmeasured. The pointer-bearing capture reserves about 400 MB, and its capacity affects GC even at lower volume.
Postgres CPU and memory, cumulative lock wait, standby fsync, network RTT and in-run sampling remain unmeasured.
Process memory is the S2 line's lifetime peak RSS on Linux; the checker's peak is not recorded.

S1-to-S2 includes drain and background WAL. PG counter deltas are approximate because backends flush stats lazily;
a reset invalidates only the affected view's deltas. `walWritten` is not per-request WAL. `flushLag@end` is one
post-drain acknowledgement sample, not RTT. `checkpointerActivity=observed` means a wait other than idle
`CheckpointerMain` at either read, `running` included, or a rising counter: checkpointer activity, not necessarily a
checkpoint. `unobserved` never proves that no checkpoint overlapped.

The canonical cell runs once, after review, on a clean stack: no toxics on either proxy, standby1 streaming sync and
no foreign backends.

```bash
env BENCH_MATRIX=quick BENCH_PG_PRIMARY_DSN=... BENCH_PG_CLIENT_PROXY_DSN=... BENCH_PG_STANDBY_DSN=... BENCH_TOXIPROXY_URL=... \
  timeout 960 go test -count=1 -tags bench ./tests/bench/engine/ -run '^TestBenchHotAccount$' -timeout 15m -v
```

The run used no overlay, race, retry or tuning and passed in 325.69 s; source hashes matched before and after, and
the stack was clean afterwards. A prior launch failed before Go ran: the planned `/usr/bin/time -v` wrapper is absent
on the host, so it was dropped and the whole command's peak RSS is unmeasured. Settings: PG 17.11,
`synchronous_commit=on`, `synchronous_standby_names=FIRST 1 (standby1)`, `wal_sync_method=fdatasync`,
`full_page_writes=on`, `checkpoint_timeout=15min`, `max_wal_size=8GB`. The host was shared: 1 min load average 33.5
before, 57.3 after.

| Measure | Result |
|---|---|
| Answers, both phases | 148,702, all approved, no publish; the checker matched the ledger to every captured call |
| Warm-up, excluded | 106,642 approved from 60 s of admission plus drain; 256 ACKs after stop; unknown 0 and queue 0 at the join |
| Measured phase | 42,060 dispatched, answered and approved: 41,804 with `dbAckAt` before stop, 256 at or after it, 0 without |
| Window rate | 41,804 / 20 s = 2,090.2 approvals/s; the 256 drain approvals are excluded |
| Answer latency, measured | p50 119.441 ms, p99 209.937 ms, p99.9 286.555 ms, max 287.969 ms |
| Batches taken in the window | 312 with 41,995 attempts, mean 134.6, largest 256: attempts taken, not dispatches or approvals |
| Writer lifetime, warm-up included | 1,102 passes, 0 deadlocks, 0 entry-late; Close nil in 753.567 µs |
| SQL round trip across the delay | median(lower) 1.140 ms before, 1.139 ms after; 1 ms configured, not network RTT |
| S1 to S2, drain and background included | 20.059 s; write position +467,363,168 B; lazily flushed `pg_stat_wal` 482,472,321 B, 2,204,777 records, 5,687 FPI, 0 buffers full, 436 syncs in 278.119 ms |
| Checkpointer and standby, same reads | `CheckpointWriteDelay` at both, 12,093 buffers written, 0 timed or requested: activity observed, no checkpoint proved; standby1 streaming sync at both; `flushLag@end` 2.792 ms, one sample |
| Process, S1 to S2 | 14 GC cycles, 6,928,646,160 B heap allocated, 11.703 s user and 0.799 s system CPU; 622,472 KiB max RSS is the lifetime peak at S2 |
| Bounds | checker 244.363 s of its 5 min; test 325.69 s under the 15 min Go timeout and the 960 s external limit |

An independent SELECT-only check found the same 148,702 requests and transactions, 297,404 operations and 594,808
PENDING outbox rows on primary and standby. Balance/control totals and operation chains matched the seed and postings.
The in-memory request/answer payload checks remain evidence from the run; the independent read did not reproduce them.

One 20 s closed-loop observation: below the 8k/s target, p99 far above the 15 ms hypothesis. That is a result, not a
failure, and not a maximum, sustained, failover or capacity result. It is no controlled comparison with the
conservation tables above, which ended on their count, on four counterparts, without the injected delay. Process
figures cover the whole test process, generator, capture, writer, pgx and drain included, not the writer alone;
nothing here attributes checker time or allocation to a cause. Each bound's margin holds for this run only.

`TestRunPhase` drives the phase runner against a null writer on a controlled clock: a phase the window ended is valid,
a count-ended or aborted one is not.

Limits: the boundary query reads PG17 views only. The heap-allocation delta counts small allocations when per-P
caches flush, so it is approximate. An abort invalidates the phase; nothing proves it hit only late work. The probe's
minimum is logged, not asserted; the toxic's read-back is the check. Process death skips cleanup, so check toxics
before the next run. Lazy dials and prepares fall inside the warm-up.

## Cells, Make and reports

Status: M4 code accepted after independent reviews, contrarian and final verification. The final regression passed
53 tests and 84 subtests, with only the unselected cell skipped; vet, formatting and pinned lint passed. The default-path
Make pair passed at 1 s/2 s. No full cell has run; those smokes are not capacity.

`make test-bench-engine` runs cells of `TestBenchHotAccount` against the running stack, one process per cell. It never
starts, stops or resets containers. The four DSNs default to the loopback addresses above; values already set win.

```bash
make test-bench-engine BENCH_MATRIX=quick                                                    # the quick cell
make test-bench-engine BENCH_MATRIX=full BENCH_BLOCK=A                                       # block A, 18 cells
make test-bench-engine BENCH_MATRIX=full BENCH_BLOCK=A BENCH_DELAY_MS=1 BENCH_SHAPE=outflow  # one matched pair
```

Block A is closed loop: writer `owner` (one batcher) or `ownerless` (32), downstream delay 0, 1 or 10 ms on `pg-repl`,
shape `outflow`, `inflow` or `mixed`. Each cell keeps the quick's 10k counterparts, 256 producers, batch cap 256 and
usual budgets, with a 60 s warm-up and a 120 s measured window; the memory caps stay 16k attempts/s over each window,
960,000 and 1,920,000. Cells of one shape share the labels `A/<shape>/warmup|measured`, so every writer and delay
draws the same postings by index; transaction IDs and clocks are per run. `BENCH_WRITER`, `BENCH_DELAY_MS` and
`BENCH_SHAPE` each keep one value; Make runs owner then ownerless for each delay and shape. Effects of that fixed order remain unmeasured. `quick` takes none of
them. Any other matrix, block or value fails before a cell starts, and the test rejects it again before database
work. Direct `go test` needs `BENCH_MATRIX=full` with all four others set.

Ceilings per cell: quick 960 s external and 15 min Go, full 1140 s and 18 min. They are bounds, not predicted
runtimes. A timeout invalidates the cell and may skip cleanup.

Each invocation creates a new directory `reports/bench-engine/<matrix>-<UTC>-<random>/`; `TEST_REPORTS_DIR` moves its
root, and git ignores `reports/`. It holds `provenance.txt` and one full `<cell>.log` per cell. Provenance carries
the commit, `go version`, `GOFLAGS`, the cells and `git status --porcelain`, never the environment or a DSN. A smoke
passes its overlay in `GOFLAGS`, so provenance records it. The test writes its JSON from the first cleanup it
registers, which runs last, only when nothing failed or skipped. Make accepts it as `<cell>.json` only after the
process exits 0. A failure, timeout, skip or `tee` error removes that cell's pending JSON, keeps its log and stops the
run without retry; earlier accepted cells stay. Check the stack (toxics, standby1, foreign backends) before rerunning.

The JSON holds the cell's configuration, settings and both probes. Each phase carries dispatched, answered, outcomes,
late, approvals by `dbAckAt` against stop, and its window. Answer and dbAck p50/p99/p99.9/max come with their sample
count: `null` without samples, never zero. Also present: the warm-up join, measured-window occupancy, writer lifetime
counters, the checker tally, stage durations (checker included) and what stays unmeasured. The window rate is
`1e9 * measured.approvalsDbAckBeforeStop / measured.windowNS` in approvals/s. `summary`, `pgSettings` and `s1ToS2` copy log lines as text;
nothing parses them. `BENCH_REPORT` is the test-to-Make handoff, not an accepted-result path on its own.

The baseline now ends with `ANALYZE base_balance, base_control`, because autovacuum never analyzes temporary tables.
On TEMP tables rebuilt from the retained quick data, the protection check rejected 2,974,634,808 pairs at its join filter
in 278.571 s without statistics and 0.164 s with them. Every check stayed effective and no financial data changed.
That attributes the reproduced cost; it does not re-measure the Go checker. The quick's 244.363 s predates the
change, and a full cell's checker time is unmeasured. At full caps the capture and recorders take about 1.24 GB
before the checker, by arithmetic from the quick's 553 MB, not by measurement.

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
not transaction throughput. Financial correctness and unknown replay are proven only where the sections above say:
the batch and small gates for both writers, the owner lifecycle tests, the five 200k shapes of each writer and the
quick cell's 148,702 answers.
Failover survival, a 1M owner reload and capacity remain unproven.
