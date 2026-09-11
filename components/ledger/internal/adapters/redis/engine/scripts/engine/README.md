# Engine Lua fragments

The Go adapter embeds these fragments in the order declared in `script.go` and
concatenates them with `scripts/engine.lua`. Redis receives the resulting source
as one script, so all validations, calculations, and writes remain within one
atomic execution.

| Fragment | Responsibility |
| --- | --- |
| `decimal.lua` | Exact decimal comparison, addition, and subtraction without converting monetary values to Lua numbers. |
| `json.lua` | Exact JSON parsing and deterministic serialization, including preservation of large numeric tokens. |
| `protocol.lua` | Shared protocol primitives and validation of text, UUIDs, integers, references, and money. |
| `balance_cache.lua` | Compatibility decoding, validation, and encoding of live balance-cache records. |
| `request.lua` | Redis key-type checks and validation of the complete execution request. |
| `receipt.lua` | Validation and replay of engine execution receipts; this closes the lost-response window independently of HTTP idempotency. |
| `posting_algebra.lua` | Monetary meaning of each supported posting type. |
| `execution.lua` | Protection checks, live-state loading, in-memory application, write preparation, and commit. |
| `../engine.lua` | Entrypoint and closed error-protocol translation. |

The fragments are not standalone Redis scripts and must not use `require`,
`dofile`, or filesystem access. Keep their dependency order explicit in
`script.go`. Apply the shared cache policy only after concatenation.

All predictable failures and serialization work must remain before
`commitPreparedExecution`. Redis writes belong only in that function. Once
`commitStarted` is true, any failure is indeterminate because earlier Redis
commands are not rolled back.

## Runtime call chain

The entrypoint tells one ordered story:

1. `main` validates the three ARGV values, decodes the request, and calls
   `execute`.
2. `prepareExecutionProtection` calls `storedReceipt` first. A valid receipt
   returns the exact prior response without loading balances. A new execution
   validates shared key types, lifecycle guards, and recovery conflicts, then
   prepares protection coordinators in memory.
3. `loadBalancePool` reads live cache values. A valid Redis value is
   authoritative; a request snapshot is only an in-memory seed for a cache miss.
   Noncanonical legacy limits request a separate precommit repair.
4. `validateLiveBalanceAvailability` checks deletion markers for declared
   requirements and postings. Generated companions are checked again at their
   exact mutation site.
5. `applyTransactionsInMemory` validates live asset/permission requirements,
   runs the closed `postingAlgebra`, resolves real overdraft draws or repayments,
   and builds truthful movements and version chains without writing Redis.
6. `prepareExecutionWrites` serializes the response, changed balance blobs,
   recovery records, receipt, guards, and protection data while enforcing the
   total prepared-byte ceiling.
7. `commitPreparedExecution` is the only publication phase. It writes changed
   balances, synchronization schedule members, recovery records, guards,
   protection coordinators, and finally the receipt.

The receipt is written last deliberately: its presence means the complete
prepared command sequence returned through the final write. Recovery records are
written before it so a failure after a monetary write retains as much completion
evidence as possible. A recovery consumer may complete SQL/MongoDB projection and
acknowledge that evidence; it must never call this script to reapply accounting.

## Maintenance rules

- Keep every live balance-dependent decision inside this one Redis execution.
  Go may supply seeds and declarative postings but must not pre-approve funds,
  calculate the authoritative overdraft split, or retry on a snapshot version.
- Add route/version behavior by changing Go posting composition when the monetary
  algebra is unchanged. Do not fork the Lua engine merely to mirror API versions.
- Perform predictable validation, calculation, JSON encoding, and size checks
  before `commitPreparedExecution`. No business refusal belongs after writes
  begin.
- Monetary values remain canonical decimal strings. Do not use Lua `tonumber`
  for amounts; it is reserved for small bounded protocol values and Redis time.
- Keep fragment dependencies explicit in `script.go`. A fragment may call only
  functions defined by an earlier fragment or by the final entrypoint.
- Treat a timeout, transport failure, invalid response, or post-commit runtime
  failure as potentially applied. Only a validated receipt replay, confirmed
  NOSCRIPT fallback, precommit normalization pass, or projection recovery may be
  repeated automatically.
