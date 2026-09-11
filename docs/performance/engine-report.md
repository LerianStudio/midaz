# Engine performance report

This report records a local integration benchmark of the internal adapter. It does not
claim a public HTTP before/after comparison; the measurements below isolate the adapter
and recovery projector even though engine-backed transaction paths are active.

## Scope

The benchmark covers 2, 10, and 50 postings with a touched pool and a pool twice that
size. The payload metric is the prepared wire JSON sent to the Lua script; it excludes
Redis keys and RESP framing. Recovery finalization has a separate deterministic benchmark
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
finalization benchmark excludes PostgreSQL, MongoDB, Valkey, queue consumption, and
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

Recovery finalization was measured separately with `-benchtime=500ms -count=3` on the
same host. The median was 54,830 ns/op, 80,077 B/op, and 1,052 allocs/op. This is the
in-process finalization cost described above, not consumer or persistence latency.

## Public k6 status

No public k6 before/after result is recorded here. A public result needs a documented
HTTP scenario and comparable baseline that exercises the active engine-backed paths;
the adapter benchmark above is not a substitute. Existing public k6 suites measure
other API paths and likewise cannot be presented as this comparison.
