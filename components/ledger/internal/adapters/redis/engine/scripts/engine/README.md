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
| `receipt.lua` | Validation and replay of stored idempotency receipts. |
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
