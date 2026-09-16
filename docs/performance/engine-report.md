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
