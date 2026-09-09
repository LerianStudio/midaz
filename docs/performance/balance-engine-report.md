# Balance-engine performance report

This report records a local integration benchmark of the internal adapter. It does not
claim a public before/after comparison while the engine execution port is unset.

## Scope

The benchmark covers 2, 10, and 50 postings with a touched pool and a pool twice that
size. The payload metric is the prepared wire JSON sent to the Lua script; it excludes
Redis keys and RESP framing. Recovery finalization remains a separate correctness check
because there is no stable recovery benchmark fixture.

## Reproduction

```text
ALLOW_INSECURE_TLS=true go test -tags=integration \
  ./components/ledger/internal/adapters/redis/engine \
  -run '^$' -bench '^BenchmarkAdapterExecute$' -benchmem \
  -benchtime=200ms -count=3
go test ./components/ledger/internal/bootstrap \
  -run 'Recovery|recovery' -count=1
```

`BenchmarkAdapterExecute` uses deterministic input shapes and covers postings 2/10/50
with `pool_touched` and `pool_larger`. It requires the local Valkey test dependency.
The recovery command is a correctness check, not a latency result. Add a dedicated
recovery benchmark only when the consumer seam has a stable benchmark fixture.

## Results

Collected on 2026-09-08 with Go 1.27, Darwin/arm64, Apple M4 Max, and a local Valkey 8
container. Values are medians of three runs. The short collection is suitable for local
characterization, not an SLO or a promotion decision.

| Scenario | Pool shape | ns/op | B/op | allocs/op | prepared wire bytes |
|---|---|---:|---:|---:|---:|
| 2 postings | touched | 2,572,392 | 111,693 | 1,402 | 4,822 |
| 2 postings | larger | 2,719,951 | 116,819 | 1,451 | 5,808 |
| 10 postings | touched | 8,775,805 | 499,764 | 5,712 | 17,670 |
| 10 postings | larger | 7,749,006 | 538,621 | 5,960 | 22,600 |
| 50 postings | touched | 30,927,185 | 2,398,204 | 27,017 | 81,916 |
| 50 postings | larger | 37,463,368 | 2,658,336 | 28,248 | 106,666 |

The `prepared wire bytes` value is the production `prepareExecution` payload sent to
the Lua script. It is distinct from the serialized `EngineExecution` input object. The
10-posting timing inversion is within this short run's variance and is not evidence that
the larger pool is faster.

Recovery outcome and duration are intentionally absent from the table: the correctness
suite exercises recovery, but the repository does not yet contain a benchmark that can
produce a comparable duration measurement.

## Public k6 status

Public k6 before/after execution is blocked: the engine is an internal adapter and its
execution port is not enabled, so no public HTTP route reaches it. Do not add a public
k6 result until a local opt-in exists and the opt-in is explicitly documented. Existing
public k6 suites measure other API paths and are not a substitute for this engine report.
