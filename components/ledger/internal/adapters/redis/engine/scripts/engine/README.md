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
| `request.lua` | Redis key-type checks and validation of the protocol-v3 execution request, including per-balance and per-transaction scope. |
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

1. `main` validates the six ARGV values (payload, byte budgets, and trusted
   transaction/posting/balance limits), decodes the request, and calls
   `execute`.
2. `prepareExecutionProtection` calls `storedReceipt` first. A valid receipt
   returns the exact prior response without loading balances. A new execution
   validates shared key types, lifecycle guards, recovery conflicts, and bounded
   causal references against the current transaction-state index, then prepares
   protection coordinators in memory.
3. `loadBalancePool` reads live cache values. A valid Redis value is
   authoritative; a request snapshot is only an in-memory seed for a cache miss.
   Noncanonical legacy limits request a separate precommit repair.
4. `loadAccountProtection` reads the closing controls of every account of the
   pool, and `validateAccountClosingAvailability` refuses every requirement and
   posting whose account is closing or closed, and every used balance this
   execution would seed without a confirmed admission (`admissionConfirmed`). The
   check is unconditional: no skip, permission, cancellation, or account-block
   exception exempts it, and a companion that only sits in the pool is untouched.
   An unused marker pair is the normal state of an open account; a marker that
   exists but carries no value refuses technically.
   `loadSeedAdmission` reads the administrative ownership key by its type: absent
   holds nothing; a string is an exclusive owner (closing, balance creation or
   deletion) and never confirms a seed, even when it carries the request's token;
   a sorted set holds the shared seed admissions, and a seed is confirmed only
   when the request's token is a live member (`ZSCORE`). A blank string owner or
   any other type refuses with `account_protection_unreadable`. The engine only
   reads that key; the balance loads that took the admissions release them.
5. `validateAccountBlockExceptions` re-reads every presented single-use grant,
   compares its alias and amount with the transaction's bound primary outflow,
   and prepares a transaction-local exemption for that primary balance and its
   overdraft companion. Missing, expired, consumed, malformed, or mismatched
   grants refuse before any write.
6. `validateLiveBalanceAvailability` checks both deletion-marker namespaces
   and the live account-block control for declared requirements and postings.
   A valid grant bypasses only the block and sending controls for its bound
   account pair; deletion markers and every other balance remain enforced.
   Cancellation explicitly disables the block control; generated companions
   repeat both protections at their exact mutation site.
7. `applyTransactionsInMemory` validates live asset/permission requirements,
   runs the closed `postingAlgebra`, resolves real overdraft draws or repayments,
   and builds truthful movements and version chains without writing Redis.
8. `selectCompanionCacheWrites` picks, for every account whose balances this
   execution writes, the overdraft companion that must stay cached beside them:
   an unmoved cached companion gets its expiry refreshed, and an unmoved seeded
   companion is published when `admissionConfirmed` proves its account's
   admission, the account is neither closing nor closed, and neither deletion
   marker is set. Without that proof the companion stays uncached and the
   execution is not refused. A published companion keeps its seed's amounts and
   version but takes the account-level `blocked` flag from a balance of its
   account that the execution writes and read from Redis: an account PATCH
   rewrites that flag only on cached balances, so the seed may carry a block
   state the account no longer has. When every written balance of the account
   was seeded too, the companion keeps its seed's flag. A refreshed companion
   keeps its cached value, flag included.
9. `prepareExecutionWrites` serializes the response, including the execution's
   single `appliedAtUnixMicro` value, changed balance blobs, published companion
   blobs, versioned write-behind evidence, transaction-state index entries,
   recovery records, receipt, guards, and protection data while enforcing the
   total prepared-byte ceiling. An execution without movements prepares none of
   them, so refusals and no-ops publish no companion.
10. `commitPreparedExecution` is the only publication phase. It receives the
    score derived from the same Redis `TIME` read and writes changed and
    published companion balances, synchronization schedule members, refreshed
    companion expiries, recovery records, guards, protection coordinators, and
    index entries, deletes consumed grant keys, and finally writes the receipt.

A published companion carries no movement and no version increment and appears
in neither the response nor the recovery evidence. Its synchronization is
version-guarded: it leaves PostgreSQL unchanged for a seed read from the current
row, and updates the row only when the seed was rebuilt ahead of it.

The receipt is written last deliberately: its presence means the complete
prepared command sequence returned through the final write. Recovery records are
written before it so a failure after a monetary write retains as much completion
evidence as possible. A recovery consumer may complete SQL/MongoDB projection and
acknowledge that evidence; it must never call this script to reapply accounting.

`execute` reads Redis `TIME` once after the replay short-circuit. The decimal
microsecond timestamp is assembled as a string before `numberToken` emits it,
avoiding floating-point precision loss; only the scheduling score uses numeric
seconds plus fractional microseconds.

## Declared key layout

`KEYS[1..7]` are the schedule, recovery, receipt, guard, protection,
transaction-index, and evidence keys for the execution's primary scope. The
receipt remains in that scope. Each transaction's guard, protection, index,
and evidence live in the scope of that transaction. Its index records the
scope of the receipt that wrote it. A dependency is accepted only while the
index still names the referenced execution and both evidence and receipt
remain present.
Each balance then contributes one ordered triplet: live balance, dedicated
deletion marker, and compatibility deletion marker. After all balance triplets,
each transaction that presents an account-block exception contributes exactly
one grant key, in transaction order. The request carries the corresponding
one-based key index; Lua verifies the tail position and exception-ID suffix.
Balance triplets and grant keys are derived from their balance or transaction scope;
logical balance references are indexed by organization, ledger, and reference
inside Lua so equal aliases in different ledgers cannot collide.

The account block has one triplet per account of the balance pool, in
ascending account order: the account-closing marker, the account-closed marker,
and the administrative ownership key. Every one of them carries the complete
organization, ledger, and account scope, and Lua verifies that each ends in its
own account identifier. The request declares those accounts in the same order,
each with the administrative admission token its caller holds — empty when the
caller owns none. The token is private to that boundary and reaches no receipt,
recovery record, or public contract.

For multi-scope executions, five keys per additional transaction scope follow
the account block: receipt, guard, protection, transaction index, and evidence.
They are ordered by organization and ledger ID. The receipt key for an
additional scope is available to validate a dependency on an earlier execution
whose primary scope was that ledger; the new execution still writes one receipt
at `KEYS[3]`. A single-scope request has no added keys or `scopeKeys` field.

A valid stored receipt is checked before the live grant. Replaying the same
execution therefore returns its prior result after the grant has been consumed;
a new execution cannot reuse that now-missing grant.

## Maintenance rules

- Keep every live balance-dependent decision inside this one Redis execution.
  Go may supply seeds and declarative postings but must not pre-approve funds,
  approve a cached account-block exception, calculate the authoritative
  overdraft split, or retry on a snapshot version.
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
