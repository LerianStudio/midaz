# Account closing performance report

This report records what the account-closing protection costs, measured on a local
Testcontainers environment. It answers two questions the design left open: whether the
controls the engine now reads on every movement are cheap enough to stay on the hot
path, and where the cost of one closing actually goes.

It is a local characterization, not an SLO and not a promotion decision. Absolute
values belong to the host below; the ratios between series are what the conclusions
rest on.

## Scope

Two measurement surfaces, both driven by the same code paths production uses:

- **Engine side** — the declared execution submitted through `EVALSHA` to the real Lua
  script, over a Valkey 8 container. It includes `prepareExecution` in Go and the
  atomic execution in Lua, so each sample is one whole movement from the adapter's
  point of view. The controls (`closing`, `closed` and the administrative ownership of
  each protected account) are read inside that same execution, never as an extra round
  trip.
- **Closing side** — `UseCase.CloseAccount` against real PostgreSQL (onboarding and
  transaction schemas, separate containers) and real Valkey: the balance read, the live
  cache read, the recovery walk, the persistence cross-check, the pending query, the
  conditional write and the finalization.

A third, isolated series measures only the cache reads of the controls themselves, as
the floor under the engine numbers.

## Reproduction

```text
ALLOW_INSECURE_TLS=true go test -tags=integration -count=1 -v \
  -run 'TestIntegrationAccountClosingCostProfile|TestIntegrationAccountClosingMarkerReadCost' \
  ./components/ledger/internal/adapters/redis/engine

ALLOW_INSECURE_TLS=true go test -tags=integration -count=1 -v \
  -run 'TestIntegrationAccountClosingClosureCostProfile' \
  ./components/ledger/internal/services/command
```

Both tests also run under `make test-integration PKG=<package> RUN=AccountClosing`; the
explicit `go test -v` form above is what prints the tables, because the measurements are
logged rather than asserted. The only assertions they carry are shape assertions — a
refusal costs the order of an execution, and the recovery walk grows with the backlog —
so a slower machine does not turn the suite red.

Each engine series pays 25 untimed warm-up iterations, and the whole engine test pays a
further 200 untimed executions before the first sample, so container start-up, the
connection pool and the script cache are not charged to whichever series runs first.

## Environment

Collected on 2026-09-18 with Go 1.27.1, Linux x86_64 (kernel 7.0.0), Intel Core 7 150U,
12 logical CPUs, 31 GiB RAM, Docker 29.5.2, Valkey 8, PostgreSQL 17 and MongoDB 8
containers. The host was running other work at the same time, which is visible in the
p95 of the widest series; medians are the values to read.

## Engine side

150 samples per series, one protected account unless stated.

| Series | mean | p50 | p95 |
|---|---:|---:|---:|
| 1 protected account, warm cache | 5.36 ms | 3.09 ms | 5.96 ms |
| 20 protected accounts, warm cache | 28.65 ms | 14.31 ms | 70.25 ms |
| 1 protected account, cold cache (seed admitted) | 2.94 ms | 1.99 ms | 3.62 ms |
| 1 protected account, refused as closed | 2.03 ms | 1.03 ms | 2.35 ms |

Derived: the median cost of one additional protected account in the pool — its resolved
keys, its control reads and its pool entry — is **591 µs**.

Isolated cache cost of the controls, same host and same Valkey:

| Series | mean | p50 | p95 |
|---|---:|---:|---:|
| 3 control reads (1 account, all absent) | 71 µs | 76 µs | 99 µs |
| 60 control reads (20 accounts, all absent) | 136 µs | 141 µs | 194 µs |

57 additional control reads cost 65 µs, i.e. **≈1.1 µs per key** and **≈3.4 µs per
protected account**. That is 0.6 % of the 591 µs an additional protected account costs
end to end: what the extra account really pays for is its resolved keys, its wire entry
and its pool parsing, not the protection.

Two further observations:

- A refusal is **cheaper** than an execution (1.03 ms against 3.09 ms at p50). The
  controls are read in the preflight, so a closed or closing account is answered before
  any accounting work — the protection does not add a slow path, it short-circuits one.
- The cold-cache series is not slower than the warm one. Admitting a seed under an
  administrative ownership costs no extra round trip inside the execution: the ownership
  is validated from a key the same Lua call already reads.

The cold and warm series are within noise of each other on this host, and the warm
series carries the wider tail because it ran first. Neither difference is evidence about
cache admission; the claim they support is the negative one — no series shows the
controls adding a term that scales with anything the hot path does.

## Closing side

8 closings per series; every sample closes a freshly prepared account, because a
closing happens once.

| Series | mean | p50 | p95 |
|---|---:|---:|---:|
| 1 balance, cold cache, empty backlog | 11.11 ms | 8.21 ms | 9.94 ms |
| 20 balances, cold cache, empty backlog | 16.84 ms | 16.41 ms | 17.46 ms |
| 20 balances, warm cache, empty backlog | 13.44 ms | 12.39 ms | 16.71 ms |
| 20 balances, warm cache, 2 000-record backlog | 97.67 ms | 95.49 ms | 105.24 ms |
| 20 balances, warm cache, 8 000-record backlog | 331.51 ms | 331.46 ms | 347.99 ms |

Derived: the median cost of one additional recovery record on the closing walk is
**39 µs**. An earlier run of the same test with 5 000- and 20 000-record backlogs
measured 202 ms and 758 ms, i.e. 37 µs per record — the series were shortened so the
suite stays fast under the race detector, and the per-record figure is unchanged.

Reading the table:

- **Balances are cheap and linear.** Nineteen additional balances add ~8 ms, about
  430 µs each: one live cache read per balance plus its share of the row listing and the
  eviction. A warm cache is slightly faster than a cold one, because the persistence
  cross-check is answered from the cached state instead of the operation high-water-mark
  query.
- **The recovery walk dominates everything else.** With an empty backlog the whole
  closing is ~12 ms; with 8 000 records it is ~331 ms, and the growth is linear at
  ~39 µs per record — HSCAN pages plus one JSON decode and scope comparison per record.

## What this justifies

- **No new index, and no global lock.** D3 and D4 put the scan cost on the closing rather
  than on every movement, and the numbers confirm the trade is the right way round: the
  backlog term that costs ~39 µs per record is paid once per closing, by an operation
  that already takes ~12 ms, while the movement path pays ~3.4 µs per account for the
  controls. Moving that cost onto the hot path — a per-account recovery index maintained
  at `commitPreparedExecution` — would charge every transaction to make a rare operation
  faster.
- **The controls stay in the preflight.** They cost about a microsecond per key, are read
  inside an execution that was already reading Redis, and they make refusals cheaper
  rather than more expensive.

## Boundary to watch

The one term that is not bounded by the account is the tenant's recovery backlog. At
~39 µs per record, a backlog of 100 000 records puts one closing at ~3.9 s and a backlog
of 400 000 at ~16 s, which is where a request deadline starts to decide the outcome
instead of the evidence. The walk already refuses rather than passing when its
16 MiB byte budget runs out before the cursors terminate, so the failure mode is a
temporary refusal, never a closing granted on an unfinished scan.

If a deployment is observed sustaining a backlog of that size, the answer is to
investigate why completions are not draining — the backlog is work the completer still
owes — before considering a per-account recovery index, which the current numbers do not
justify.

## Not covered

No public HTTP or k6 result is recorded here. These measurements isolate the adapter and
the use case; an end-to-end result would need a documented HTTP scenario and a comparable
baseline, which is outside this report. The closing series also exclude the streaming
emission, which is best-effort and bounded by its own timeout.
