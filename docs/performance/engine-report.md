# Engine performance report

This report records a local integration benchmark of the internal adapter. It does not
claim a public HTTP before/after comparison; the measurements below isolate the adapter
and applied-transaction completion even though engine-backed transaction paths are active.

## Scope

The benchmark covers 2, 10, and 50 postings with a touched pool and a pool twice that
size. The payload metric is the prepared wire JSON sent to the Lua script; it excludes
Redis keys and RESP framing. Recovery completion has a separate deterministic benchmark
for decoding, projection, cloning, and metadata verification without database or network I/O.

## Reproduction

```text
ALLOW_INSECURE_TLS=true go test -tags=integration \
  ./components/ledger/internal/adapters/redis/engine \
  -run '^$' -bench '^BenchmarkAdapterExecute$' -benchmem \
  -benchtime=200ms -count=3
go test ./components/ledger/internal/bootstrap \
  -run 'Recovery|recovery' -count=1
go test ./components/ledger/internal/services/command \
  -run '^$' -bench '^BenchmarkTransactionCompletionService$' \
  -benchmem -benchtime=500ms -count=3
```

`BenchmarkAdapterExecute` uses deterministic input shapes and covers postings 2/10/50
with `pool_touched` and `pool_larger`. It requires the local Valkey test dependency.
The bootstrap recovery command is a correctness check, not a latency result. The
completion benchmark excludes PostgreSQL, MongoDB, Valkey, queue consumption, and
event publication, so it is an in-process cost baseline rather than end-to-end recovery
latency.

## Results

Collected on 2026-09-08 with Go 1.27, Darwin/arm64, Apple M4 Max, and a local Valkey 8
container. Values are medians of three runs. The short collection is suitable for local
characterization, not an SLO or a promotion decision.

| Scenario | Pool shape | ns/op | B/op | allocs/op | prepared wire bytes |
|---|---|---:|---:|---:|---:|
| 2 postings | touched | 2,572,392 | 111,693 | 1,402 | 4,821 |
| 2 postings | larger | 2,719,951 | 116,819 | 1,451 | 5,807 |
| 10 postings | touched | 8,775,805 | 499,764 | 5,712 | 17,669 |
| 10 postings | larger | 7,749,006 | 538,621 | 5,960 | 22,599 |
| 50 postings | touched | 30,927,185 | 2,398,204 | 27,017 | 81,915 |
| 50 postings | larger | 37,463,368 | 2,658,336 | 28,248 | 106,665 |

The `prepared wire bytes` value is the production `prepareExecution` payload sent to
the Lua script. Values reflect the one-byte internal field rename from
`recoveryPayload` to `completionPlan`; timing and allocation columns remain the original
run. The payload is distinct from the serialized `EngineExecution` input object. The
10-posting timing inversion is within this short run's variance and is not evidence that
the larger pool is faster.

Recovery completion was measured separately with `-benchtime=500ms -count=3` on the
same host. The median was 54,830 ns/op, 80,077 B/op, and 1,052 allocs/op. This is the
in-process completion cost described above, not consumer or persistence latency.

## Public k6 status

No public k6 before/after result is recorded here. A public result needs a documented
HTTP scenario and comparable baseline that exercises the active engine-backed paths;
the adapter benchmark above is not a substitute. Existing public k6 suites measure
other API paths and likewise cannot be presented as this comparison.

## Atomic transaction batch release gate

The atomic batch matrix uses the production Redis adapter and assembled Lua against
Valkey 8. It records the indivisible EVAL duration separately from adapter
end-to-end time. The gate was collected on 2026-09-16 with Go 1.27, Darwin/arm64,
Apple M4 Max, 20 measured executions per run, and three runs per scenario. These
local results are release evidence for the fixed admission envelope, not a public
HTTP SLO.

The original maximum candidate failed. The disjoint, cold-cache, fee-expanded
N=50 profile contained 200 postings and 400 balance snapshots; its isolated Lua
p99 values were 140.1–144.3 ms (143.4 ms median), above the required 100 ms.
The interim 100-posting/200-balance candidate stayed below the target, but its
combined N=50 maximum produced an 87.6 ms worst-run Lua p99. The balance ceiling
was lowered once more to retain more operational margin. The final release limits
are 100 expanded postings and 150 execution balances.

| Final admitted profile | Traffic | Lua p99 median | End-to-end p99 median | Unrelated p99 median | Engine input | Prepared wire | Prepared response | Recovery payloads |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| N=50, 100 postings, 150 balances | isolated | 77.1 ms | 88.5 ms | — | 221,653 B | 251,653 B | 147,150 B | 125,060 B |
| N=50, 100 postings, 150 balances | concurrent singular | 77.1 ms | 89.1 ms | 2.6 ms | 221,653 B | 251,653 B | 147,150 B | 125,060 B |

Each row simultaneously exercises the maximum public cardinality and both final
derived-work ceilings. Across the six final-limit runs, the worst recorded Lua p99
was 81.4 ms, leaving 18.6 ms (18.6%) below the 100 ms gate. The matrix reports Valkey
`used_memory`, Go `B/op`, and allocations for every run; those process-wide memory
samples are useful for comparisons within a collection but are not presented as
per-request memory.

The matching batch command limits serialized intermediate forms before Tracer or
accounting: 256 KiB for completion plans, 256 KiB for the accounting request,
1 MiB for the prepared execution, 512 KiB for recovery, and 1 MiB for the cached
terminal response. They leave serialization headroom over the measured maximum
while preventing the former 32/64 MiB compatibility ceilings of the general engine
adapter from becoming batch admission limits. The general engine ceilings remain
unchanged for existing singular v1/v2 paths.

Reproduce the rejected candidate from commit `95e88edf04a3992076f39d6a8aa419b8caf8d27a`
and the final maximum from the current tree with:

```text
go test -tags=integration \
  ./components/ledger/internal/adapters/redis/engine \
  -run '^$' \
  -bench='^BenchmarkAtomicTransactionBatchMatrix$/^n_(25|50)$/^disjoint$/^cold$/^fee_max$/^single_tenant$/^(isolated|concurrent_singular)$' \
  -benchmem -benchtime=20x -count=3
```

The complete matrix additionally varies direct, hold, and mixed item profiles at
N=1/10/25/50, shared balances, warm cache, tenant mix, base work, and unrelated
singular traffic. The retained replay envelope is measured as
`replay_response_bytes`, separately from the mutable completion payload.

On 2026-09-16, the revised N=50 disjoint/cold/fee-expanded single-tenant profiles
were sampled on the same host and Valkey 8 with 20 executions per profile. Hold Lua
p99 was 78.36 ms isolated and 77.70 ms with concurrent singular traffic; mixed Lua
p99 was 77.84 ms isolated and 78.49 ms concurrent. All four are below 100 ms. This
is targeted regression evidence for the new action profiles, not a replacement for
a full multi-run matrix collection.

The complete matrix additionally varies N=1/10, shared balances, warm cache,
tenant mix, base work, and unrelated singular traffic. Short local samples can
understate tail latency; production rollout must continue to monitor Lua duration,
rejection dimensions, recovery backlog, Redis memory, and unrelated-request
latency. Raising any fixed ceiling requires new benchmark evidence and review.

## Write-behind full-stack harness

`BenchmarkEngineWriteBehind` is a reproducible HTTP-to-persistence benchmark
under the integration build tag. One fixture runs Fiber/Huma and the production
command, engine, completion, and RabbitMQ dispatch paths against PostgreSQL 17,
MongoDB 8, Valkey 8 with the financial persistence profile, and RabbitMQ 4.1.
The singular matrix covers v1/v2, ST/MT, warm/cold state, the historical direct
completion boundary, corrected dependency-aware synchronous completion, and
confirmed asynchronous admission. The v2 atomic batch matrix covers asynchronous
bulk projection at N=1/10/25/50 across ST/MT and warm/cold state.

The historical baseline is reproduced by a benchmark-only dispatcher that calls
the durable completer and recovery ACK directly before HTTP returns, matching the
pre-write-behind persistence boundary without maintaining a second production
implementation. Corrected sync uses the current shared fallback/completion path.
Async measures HTTP through broker confirmation, then drains the real messages
through the production individual or bulk dispatcher. Cold cases create uncached
balances outside the timer; warm cases reuse initialized balances. The producer
is reused as a singleton, matching bootstrap ownership of RabbitMQ confirmation
subscriptions.

The MT rows exercise trusted tenant context, tenant-bearing engine keys, and a
per-publication RabbitMQ channel, but intentionally reuse the fixture's physical
PostgreSQL, MongoDB, and Valkey instances. They characterize the MT code path;
they are not a cross-tenant infrastructure-isolation benchmark.

Reproduce the harness with:

```text
ALLOW_INSECURE_TLS=true go test -tags=integration \
  ./components/ledger/internal/bootstrap -run '^$' \
  -bench '^BenchmarkEngineWriteBehind$' -benchmem -count=1
```

A local run on 2026-09-18 (Go 1.27, Darwin/arm64, Apple M4 Max) completed the
exact command above in 77.6 seconds. Each sub-benchmark reports admitted TPS from
the timed HTTP interval and projected TPS from HTTP plus drain. The Lua hook is
attached only to the engine's Valkey client, so idempotency, materialization, and
recovery scripts do not contaminate the Lua percentiles. `rows_per_commit` is the
observed transaction plus operation row delta per request/commit. Redis memory is
reported both as the process-wide `used_memory` snapshot and as delta per request;
the table below uses the latter because one fixture intentionally accumulates all
matrix cases.

### Singular summary

Ranges below include all eight API/tenancy/cache combinations for each profile.
All cases wrote three SQL rows per commit. Async cases reached a one-message peak
and ended at zero backlog.

| Profile | Admitted TPS | Projected TPS | HTTP p50/p95/p99 (ms) | Lua p50/p95/p99 (ms) | Drain p50/p95/p99 (ms) | Redis delta/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Historical sync, v1/v2, ST/MT, cold/warm | 84.76–98.70 | 84.76–98.70 | 9.48–12.05 / 12.72–16.97 / 13.46–18.31 | 3.13–3.48 / 3.32–4.06 / 3.38–4.83 | — | 15.8–19.0 KiB |
| Corrected sync, v1/v2, ST/MT, cold/warm | 80.48–88.89 | 80.48–88.89 | 10.59–12.30 / 13.97–16.19 / 15.25–20.54 | 3.15–3.47 / 3.40–3.92 / 3.62–4.87 | — | 15.8–19.4 KiB |
| Async single, v1/v2, ST/MT, cold/warm | 112.4–135.6 | 66.75–76.76 | 6.77–8.44 / 9.51–11.55 / 10.20–13.43 | 3.14–3.50 / 3.47–3.97 / 4.10–4.75 | 4.75–5.56 / 6.78–8.57 / 7.52–9.85 | 15.5–19.4 KiB |

### Atomic batch results

Every row ended at zero backlog; the peak equaled N. SQL rows per commit were
3/30/75/150 for N=1/10/25/50. Values are from the same exact-command run.

| N | Tenancy | Cache | Admitted / projected TPS | HTTP p50/p95/p99 (ms) | Lua p50/p95/p99 (ms) | Drain p50/p95/p99 (ms) | Redis delta/op |
| ---: | --- | --- | ---: | ---: | ---: | ---: | ---: |
| 1 | ST | cold | 87.90 / 56.95 | 10.82 / 15.21 / 15.53 | 3.08 / 3.35 / 3.48 | 5.46 / 8.41 / 9.46 | 23.9 KiB |
| 1 | ST | warm | 87.42 / 56.94 | 10.86 / 13.89 / 14.70 | 3.45 / 3.80 / 4.62 | 5.31 / 8.32 / 8.80 | 20.9 KiB |
| 1 | MT | cold | 77.11 / 50.54 | 12.70 / 16.18 / 16.39 | 3.15 / 3.51 / 3.55 | 6.49 / 9.05 / 9.85 | 24.0 KiB |
| 1 | MT | warm | 77.29 / 50.58 | 12.51 / 15.26 / 18.48 | 3.49 / 3.82 / 4.72 | 6.26 / 10.28 / 11.41 | 21.0 KiB |
| 10 | ST | cold | 180.3 / 99.32 | 54.05 / 65.56 / 65.56 | 21.42 / 21.84 / 21.84 | 44.17 / 47.83 / 47.83 | 181.5 KiB |
| 10 | ST | warm | 186.3 / 97.91 | 52.82 / 55.77 / 55.77 | 21.02 / 21.59 / 21.59 | 47.53 / 51.86 / 51.86 | 177.7 KiB |
| 10 | MT | cold | 167.6 / 92.48 | 58.68 / 63.03 / 63.03 | 21.00 / 21.52 / 21.52 | 47.28 / 49.16 / 49.16 | 181.1 KiB |
| 10 | MT | warm | 165.2 / 87.46 | 59.59 / 64.24 / 64.24 | 21.67 / 22.81 / 22.81 | 48.47 / 56.20 / 56.20 | 178.0 KiB |
| 25 | ST | cold | 163.2 / 90.79 | 153.4 / 155.5 / 155.5 | 51.23 / 67.85 / 67.85 | 117.6 / 118.2 / 118.2 | 463.1 KiB |
| 25 | ST | warm | 135.6 / 80.10 | 151.5 / 200.3 / 200.3 | 67.62 / 88.42 / 88.42 | 125.6 / 126.0 / 126.0 | 471.1 KiB |
| 25 | MT | cold | 146.8 / 83.99 | 167.3 / 171.8 / 171.8 | 67.66 / 77.65 / 77.65 | 125.9 / 127.0 / 127.0 | 429.8 KiB |
| 25 | MT | warm | 128.9 / 77.63 | 147.6 / 234.6 / 234.6 | 51.58 / 51.64 / 51.64 | 127.0 / 127.6 / 127.6 | 474.9 KiB |
| 50 | ST | cold | 120.5 / 73.34 | 404.7 / 404.7 / 404.7 | 151.6 / 151.6 / 151.6 | 263.2 / 263.2 / 263.2 | 877.3 KiB |
| 50 | ST | warm | 126.6 / 74.93 | 383.5 / 383.5 / 383.5 | 108.4 / 108.4 / 108.4 | 259.1 / 259.1 / 259.1 | 874.5 KiB |
| 50 | MT | cold | 108.2 / 67.85 | 407.6 / 407.6 / 407.6 | 170.2 / 170.2 / 170.2 | 258.5 / 258.5 / 258.5 | 878.3 KiB |
| 50 | MT | warm | 110.7 / 68.70 | 450.5 / 450.5 / 450.5 | 148.9 / 148.9 / 148.9 | 271.5 / 271.5 / 271.5 | 870.8 KiB |

The async singular path improved admitted throughput and HTTP latency in this
local run, but projected throughput remained below the synchronous references
once durable drain was included. Backlog stability passed because every case
returned to zero. The full-stack N=50 Lua p99 was 108.4–170.2 ms, above the
required 100 ms gate, despite the isolated adapter release-gate results recorded
earlier in this report. Therefore this run does **not** support a sustainable
throughput-gain claim. The N=50 rows had only two timed iterations, so their
percentiles are characterization data rather than an SLO measurement; repeat
with a fixed longer benchtime before making capacity decisions.
