# Balance engine

## Status and scope

The ledger defines a storage-independent accounting contract in
`components/ledger/internal/engine`. The existing Redis transaction adapter remains
the active execution path. Canonical overdraft-limit serialization and conditional
warm-cache repair protect that path independently of the new contract.

The posting Lua implementation and Redis adapter, dual-format cache codec, and
typed version-2 recovery payload/projector are implemented and tested foundations.
They are not connected to transaction execution, bootstrap activation, or a
compatible production recovery consumer. Existing writers have not all migrated
to the dual codec. The adapter currently supports standalone Redis clients only;
other client topologies fail before accounting is sent.

No activation, request-limit defaults, or receipt/guard retention policy is
implicitly supplied by these foundations. Consumer-first integration, measured
limits, observability, persistence, and retention gates below remain mandatory.
The current work does not switch transaction execution or change the HTTP API.

The boundary separates transaction processing from balance arithmetic while
preserving the observable rows, amounts, versions, and public errors of valid
transaction flows. Explicit integrity corrections are described separately.

## Ownership and package boundaries

| Responsibility | Owner |
| --- | --- |
| Authentication, tenant context, organization and ledger scoping | Go request/use-case layer |
| Input targeting, cardinality, asset and sending/receiving validation | Go use cases over explicit transaction legs |
| Fees, tracer, HTTP idempotency, transaction lifecycle and events | Go use cases |
| Ordered posting composition and route draw policy | Go command layer |
| Live balance arithmetic, overdraft split/repayment, movement versions | Accounting engine |
| Physical keys, cache codec, script transport, execution receipts and guards | Redis engine adapter |
| Accounting rows, metadata, route attribution and historical row compatibility | Go projection shared by normal finalization and recovery |

The neutral `engine` package must not import commands, adapters, or bootstrap.
The command layer owns the `BalanceEngine` port; bootstrap selects its
implementation. The Redis implementation belongs under
`components/ledger/internal/adapters/redis/engine`. Imports that need both packages
use `engine` for the contract and `redisengine` for the adapter. Tests importing
the contract belong under `components/ledger`; root-level shared test utilities
must remain independent of it. Existing `pkg` packages are not relocated.

The engine receives neither transaction status nor route/DSL interpretation
rules. Those determine postings and frozen projection context in Go. The adapter
may preserve recovery payloads opaquely without interpreting them in accounting
arithmetic.

## Request, movement, and execution identity

`engine.Request` contains nonzero UUIDs for `OrganizationID`, `LedgerID`, and
`ExecutionID`, ordered transactions, and a pool of `BalanceSnapshot` values.
Transactions contain a nonzero UUID and ordered postings. Each posting has a
transaction-unique `Ref`, a logical `BalanceRef` (`alias#key`), a supported type,
positive decimal `Amount`, `DrawPolicy`, and nonnegative `OverdraftAmount`.
No domain input may supply physical Redis keys.

All input belongs to one authenticated tenant and ledger. The adapter resolves
physical identity using validated organization, ledger, account, key, and alias
data; alias alone is insufficient. IDs in snapshots are UUIDs, not unvalidated
strings. Conflicting snapshots for the same physical balance are rejected.

Three balance sets must remain distinct:

- Explicit legs are the balances targeted by the transaction input. Existing
  targeting, cardinality, asset, and permission rules apply to this set.
- The snapshot pool contains available scoped seeds, including internal overdraft
  companions that might become necessary after a live-settings change or retry.
- The touched set contains effective postings and companions actually used.
  Only touched balances receive CAS, monetary writes, and synchronization work.

An unused internal companion in the pool is not an explicit user target and must
not cause rejection. A user explicitly targeting an internal balance remains
invalid. A missing companion fails only when a real draw or repayment requires it.

Each `Movement` carries a deterministic unique `Ref`, `TransactionID`, parent
`PostingRef`, role (`primary` or `overdraft_companion`), balance reference, type,
amount, overdraft delta, and truthful `Before`/`After` monetary state. Companion
`PostingRef` always identifies its originating posting. Reference construction
uses transaction ID, posting reference, role, and a stable ordinal; map iteration
must not determine ordering.

A movement exists when Available, OnHold, or OverdraftUsed changes. Each applied
primary or companion movement increments its balance version once. `Amount=0`
does not suppress a debt-only movement. Unchanged state produces no movement,
version increment, or schedule update. `Result.Final` contains one final snapshot
per touched physical balance in deterministic order, excluding unused seeds.

Execution identity is distinct from transaction identity: pending creation,
commitment, and cancellation share a transaction ID but require different
execution IDs. Retries of one logical action preserve its execution ID.

## Posting arithmetic

Let `A` denote Available, `H` OnHold, `U` OverdraftUsed, and `x` the positive
posting amount. Empty legacy direction uses credit-direction arithmetic, but
does not grant new overdraft draw eligibility. Directions are lowercase strings.

| Posting | Credit direction / legacy empty | Debit direction |
| --- | --- | --- |
| `debit` | Decrease A; an eligible deficit becomes debt and leaves A at zero | Increase A |
| `credit` | Repay eligible existing debt first; add the remainder to A | Decrease A and enforce the floor |
| `reserve` | Increase H only; do not check A again | Increase H only |
| `unreserve` | Decrease H only | Decrease H only |
| `hold` | Decrease A and increase H in one movement, without new debt | Increase A and H in one movement |
| `release` | Decrease H and restore A, except explicitly capped legacy repayment | Decrease H and A; enforce the directional floor |

External accounts retain their arithmetic floor exemption. The prohibition on
external pending sources remains a Go targeting rule, not a new engine status
branch. `unreserve` and `release` must reject `H < x` before any write.

### Draw and repayment

New debt requires direction exactly `credit`, a non-external account, live
`AllowOverdraft`, and `DrawAllowed`. The engine combines path policy with live
settings; Go must not bake snapshot `AllowOverdraft` or limit into the policy.
Settings updates do not increment the monetary version.

`DrawForbidden` never creates debt, including every pending debit. A hold is not
allowed to draw debt even when account settings permit it. `DrawRouteDenied`
retains the distinction between route denial and account/path ineligibility.
For a permitted draw, add the deficit to U and compare against the live limit;
equality succeeds. Generate a debit on the same account's `overdraft` companion.
Resolve that companion by account identity and key, not by concatenating onto an
already-qualified alias. Do not recursively generate companions of companions.

For non-external credit/empty-direction `credit`, repayment is `min(x, U)`.
When `OverdraftAmount > 0`, additionally cap repayment at that value. Add only the
remainder to A and generate a companion credit for the real repayment. Existing
debt can be repaid even when future draws have been disabled.

`release` is not a general credit operation. With a zero override, it restores A
and decreases H without repaying debt. Only a positive override on a non-external
credit-direction release permits repayment, bounded by the override, x, and U.
This difference must remain visible in separate arithmetic functions.

## Go path composition and row projection

In this table, ON/OFF means route validation enabled/disabled.

| Path | Source postings | Destination postings | Row compatibility |
| --- | --- | --- | --- |
| Direct / revert | Conclusive `debit` | `credit` | Apply route draw policy; use live splits |
| Pending OFF | `hold` | None | One source version increment |
| Pending ON | `debit(DrawForbidden)`, `reserve` | None | Two source increments |
| Commit OFF | `unreserve` | `credit` | Source row remains DEBIT |
| Commit ON | `unreserve` | `credit` | Source row remains ON_HOLD |
| Cancel OFF | `release` | None | No general repayment; preserve legacy override |
| Cancel ON | `unreserve`, `credit` | None | Credit may repay debt; historical projection is separate |
| NOTED / annotation | Do not call engine | Do not call engine | Produce annotation rows with BalanceAffected=false |

Deferred destinations still undergo Go input validation even though they produce
no posting, row, or version increment at that point. A validated hold of 60 from
A=100 ends at A=40/H=60 with two increments; holding 100 succeeds. A hold of 101
fails without drawing debt. `reserve` must not check the already-debited A again.

Posting type does not uniquely determine accounting row type. Projection retains
the origin side, lifecycle action, expected row type, direction, RouteID/rubrics,
aliases, snapshot information, and relevant metadata. Companions inherit the
originating RouteID but preserve empty Metadata and ChartOfAccounts unless a
separate business change explicitly changes that behavior.

Validated cancellation with repayment has a historical row projection that can
differ from the actual Lua monetary state. Preserve its observable Amount and
BalanceAfter in a named Go compatibility projection; never overwrite truthful
movement state to imitate a row. Normal finalization and recovery use the same
projector and must produce identical operation IDs, rows, and versions.

## Precision and cache representation

Money crosses the wire as canonical decimal strings, produced with
`shopspring/decimal`. Do not use `float64` or Lua `tonumber` for money. Validate
amounts, overrides, directions, references, UUIDs, collection shapes, and numeric
bounds before sending a request. Empty collections encode as `[]`, not `{}`;
arbitrary objects are not accepted as singleton arrays.

Domain structs are not the Lua wire DTO. Protocol version 1 transports versions
as canonical decimal strings, including values above 2^53. The Lua JSON parser
preserves numeric tokens exactly; legacy cache Version and version-2 recovery
results retain exact numeric int64 literals. The lowerCamel cache version is a
string. Incrementing the maximum int64 version is rejected before writes.
Money remains textual throughout arithmetic and serialization.

The shared codec supports dual and new-only output. Decoding ignores unknown
cache extensions, so decode/encode alone does not preserve them. The Lua live
writer preserves unrelated fields from the original blob, including exact
numeric tokens. Payload limits still require explicit measured configuration.

The migration uses an explicit field map:

| Legacy field | New field |
| --- | --- |
| ID | id |
| AccountID | accountId |
| AccountType | accountType |
| AssetCode | assetCode |
| Alias | alias |
| Key | key |
| Direction | direction |
| BalanceScope | balanceScope |
| Available | available |
| OnHold | onHold |
| OverdraftUsed | overdraftUsed |
| Version | version |
| AllowSending | allowSending |
| AllowReceiving | allowReceiving |
| AllowOverdraft | allowOverdraft |
| OverdraftLimitEnabled | overdraftLimitEnabled |
| OverdraftLimit | overdraftLimit |

New identifiers, enums, and decimals are strings; new flags are booleans. Legacy
flags retain 0/1 and legacy Version retains its compatible numeric representation.
`SchemaVersion=2` is metadata, not an authority switch.

During dual writing, legacy CamelCase fields win whenever present, regardless of
SchemaVersion. New lowerCamel fields are authoritative only in a new-only blob.
Every dual writer updates both representations. An old writer may mutate only
the legacy fields while preserving stale lowerCamel fields, so selecting fields
by schema version would read stale money. Readers must also accept new-only
blobs; a dual writer reading one reconstructs both representations coherently.

The writer inventory includes cold seeds, accounting mutations, settings PATCH,
conditional limit repair, and any recovery rehydration. Settings writers change
only settings, preserving live money and Version. Cache-aside readers still need
asset, permission, alias, and key fields for Go validation. Cache TTL, deletion
marker construction, and hash tag must have a shared definition.

### Existing warm-cache normalization

Settings normalize before validation; malformed values remain malformed so
validation rejects them. Both Go serializers canonicalize without mutating the
caller's settings. The existing script performs a read-only whole-batch preflight
before any seed or monetary write and reports all noncanonical warm-cache limits.

Go parses those strings and conditionally replaces only the expected live limit.
The repair preserves money, Version, other settings, key absence, and remaining
TTL with `SET KEEPTTL`. Invalid JSON or decimal data is a technical failure, never
zero. A concurrent settings change causes re-evaluation, not an overwrite. There
are at most three repair passes for the whole batch; this is separate from CAS.
Transport errors cannot authorize a repair/re-execution loop.

The repair reply is an exact `BALANCE_LIMIT_NORMALIZATION_REQUIRED:` prefix plus
a JSON key array, optionally framed by one Redis `ERR ` prefix. Only an actual
Redis error with a valid reply and keys belonging to the current execution is
accepted. Similar text inside runtime or transport errors remains technical.

## Preflight, ordered execution, and commit

The inactive posting adapter sends one versioned envelope in `ARGV[1]` containing the
accounting DTO, opaque recovery payloads, and indices into `KEYS`. Every physical
balance, deletion marker, schedule, backup, receipt, and guard key must appear in
`KEYS`; hash field names belong in ARGV. Preserve the existing `{transactions}`
hash tag. Tenant namespacing comes only from authenticated context.

`ARGV[2]` and `ARGV[3]` carry trusted request-byte and total-prepared-byte bounds.
The latter covers the response, balance blobs, recovery records, receipt, and
prepared guard/hash-field data. These bounds are required inputs, not defaults.

Before the first write, the engine must:

1. Validate protocol/schema, configured limits, references, amounts, snapshots,
   recovery correlation, receipt/guard state, and expected Redis key types.
2. Resolve touched balances and use live data when present; use cache-miss seeds
   only in working memory. Do not seed Redis early with `SET NX`.
3. Check deletion markers and initial versions for every touched physical
   balance, including companions discovered during calculation. An unused pool
   balance with a marker or stale version must not block the request.
4. Execute transactions and postings in stable order against working state.
   Later transactions observe earlier intermediate results. Compare each initial
   physical version once, not after every increment made by this same execution.
5. Serialize all final blobs, per-transaction recovery envelopes, receipts,
   guards, and the response, and prepare all command arguments.

Only then may the script publish prepared writes. Update each changed balance's
schedule score with overwrite semantics, retaining the worker's fractional-second
precision. Do not use `ZADD NX`. Refusals before commit leave key values, TTLs,
absence, schedule, backups, and guards unchanged, except separately executed
conditional normalization.

Lua execution is isolated, not rollback-capable. An arbitrary error after the
first write can leave partial state. Preflight must detect predictable WRONGTYPE
and serialization failures, but must not promise rollback for runtime/OOM errors.
Such failures are technical and potentially indeterminate; retain evidence and
do not replay accounting blindly. The existing script's per-balance writes and
manual rollback must not be mistaken for the target prepared-commit guarantee.

## CAS, transport, and error classification

The command layer retries only a confirmed pre-write `stale_version`, for at most
three total attempts. Each retry reloads live snapshots and rebuilds postings,
companion information, projection context, and all balance-dependent validations.
Preserve execution identity, original intent, dates, row identities, resolved
fees, tracer reservation, and HTTP idempotency work. Do not restart the whole
transaction workflow. Context cancellation stops additional attempts.

The standalone adapter uses a dedicated client with automatic retries disabled
(`MaxRetries=-1`), without changing the shared client's settings. `EVALSHA` to
`EVAL` fallback occurs only after confirmed NOSCRIPT, never after timeout or
connection loss. Lost-response integration tests verify a single application;
explicit replay uses the same execution receipt. Cluster and other client types
are rejected before execution until their transport guarantees are implemented
and verified. Standalone support is not permission to restrict deployed topology
during activation.

Structured refusals use exact `MIDAZ_ENGINE_V1 ` framing followed by
validated JSON. Accept at most one known Redis `ERR ` framing prefix before the
protocol prefix. Validate the code enum, transaction/posting index bounds, and
reference correlation; indices are zero-based, with -1 only when not applicable.
Do not classify errors by substring. The adapter returns `*engine.Failure` for
recognized refusals and preserves technical causes separately.

Technical replies use `MIDAZ_ENGINE_TECH_V1 ` and a validated technical code.
A malformed response, corrupt stored receipt, or transport failure can describe
an execution that already changed state and must not enter CAS retry. A deliberate
post-first-SET command denial is covered by integration tests: the balance write
survives while later schedule/backup/guard/receipt writes can be absent. The
adapter reports an indeterminate outcome, preserves evidence, and sends no blind
retry. Receipts prevent duplicate completed executions; they do not roll back or
automatically repair a partially executed commit.

| Failure | Public treatment |
| --- | --- |
| insufficient_funds | 0018, not 0025 |
| overdraft_limit_exceeded | 0167 |
| overdraft_not_eligible | 0492 only for eligible-account route denial; 0018 for forbidden/ineligible paths, preserving validation precedence |
| stale_version | Retry within the bound; then 0174 |
| balance_deleted | 0019 |
| balance_missing | 0139 for the corresponding retrieval failure |
| overdraft_companion_missing | Technical invariant failure, generic 0046 |
| onhold_underflow | Technical invariant failure, generic 0046; not external-hold code 0098 |
| Conflicting transition guard | 0486 while concurrent; 0099 when terminal state is confirmed; not CAS retry |
| Unknown code/version, malformed JSON/indices, runtime, transport | Technical; potentially indeterminate if execution may have occurred |

Classify before calling the public error factory, which matches exact sentinel
identity. Public errors do not necessarily preserve `Unwrap`. Preserve underlying
technical causes with `%w` and `errors.Is/As` internally. Backup retrieval, write,
and serialization retain 0139, 0128, and 0129 respectively where those existing
paths apply. Fingerprint reuse with conflicting intent is a protocol failure,
not a balance refusal and not a newly invented public numeric code.

The existing adapter's numeric replies remain an independent legacy protocol:
exact 0018, 0019, 0139, 0167, and 0174, optionally preceded by one `ERR ` prefix.
Code-like digits embedded in descriptive runtime errors remain technical.

## Execution guards, receipts, and recovery

The command-owned `EngineExecution` combines the accounting request with an
immutable intent fingerprint, one `ExecutionGuard` per transaction, and one
opaque `RecoveryIntent` per transaction. A guard contains transaction ID,
expected token, and next token; empty expected token requires an absent guard.
The accounting engine compares tokens without interpreting lifecycle status.

The fingerprint includes scope, action, normalized intention, and stable leg
references, but excludes snapshots and calculated splits that can change on CAS
retry. It also includes frozen primary row attribution, metadata, parent identity,
and skip-audit flags. Derived companion contexts are excluded, since their need
can change after a fresh balance read. Validation recomputes the fingerprint from
the ordered immutable payloads, rather than only comparing supplied hash strings.

The same execution ID and fingerprint returns its recorded result without
reapplying postings; reuse with different intent is rejected before writes.
Completed-receipt replay validates scope, posting correlation, movement identity,
version chains, and final snapshots before returning the stored response bytes.
Malformed stored receipts are technical indeterminate outcomes, not proof of
an unapplied execution.

Guard CAS prevents concurrent commit and cancel from both succeeding even after
a Go lock expires. For a legacy pending transaction without a guard, validate
persisted lifecycle state and conditionally create the guard; two transitions
must not both win an absent guard. Transaction ID alone is insufficient dedupe.

### Recovery envelope

The implemented outer envelope has `formatVersion=2`, tenant/organization/ledger scope,
ExecutionID, fingerprint, TransactionID, the opaque payload, and the real result
restricted to that transaction, including its intermediate before/after states.
Validate one-to-one correlation between request transactions and recovery intents
before EVAL. Use typed, versioned payloads rather than ad hoc maps.

The frozen Go payload preserves `header_id`, `transaction_id`, `organization_id`,
`ledger_id`, normalized `parserDSL` including resolved fees, `ttl`, `validate`,
`transaction_status`, `action`, `transaction_date`, and projection context keyed
by stable PostingRef. It also preserves `parentTransactionId`, `feesSkipped`, and
`tracerSkipped`. The payload is an opaque JSON string in the outer envelope;
strict decoding rejects duplicate keys, unknown fields, and scope drift.
Freeze route decisions and metadata required for replay;
do not make current route/settings lookups prerequisites for recovery. Do not
store a precomputed split as accounting authority.

Operation IDs use UUIDv5 namespace
`c102438e-88ba-5d08-b785-a699df083ecd`, with length-prefixed ExecutionID,
TransactionID, PostingRef, and role, followed by a stable ordinal. This namespace
and encoding are immutable replay contracts. Replay must not generate random IDs
or change already-materialized legacy IDs. Projection validates real debt deltas,
required companions, immutable balance identity, and per-transaction final state;
historical synthetic row states remain separate from truthful movements.

The script commits recovery data and receipts/guards in the same execution as
balance changes; a later Go backup update must not be required for recoverability.
Backup hash fields identify both transaction and execution. Cleanup checks both
identities so delayed cleanup cannot erase a later transition's backup.

Receipt and guard retention is not balance-cache TTL. They must survive pending
finalization and, after confirmed persistence, cover the retry/idempotency window
and terminal-state verification. The concrete retention policy must be defined
with recovery workers before activation; absence of that policy blocks enablement.

### Finalization outcomes

- Confirmed pre-write refusal: cleanup may remove only that execution's
  uncommitted recovery preparation.
- Indeterminate outcome: preserve receipts/backups, query recorded execution,
  and reconcile before any accounting replay.
- Confirmed balance application followed by projection/database/publication
  failure: finalize the same recorded result without reapplying postings.

The compatible consumer dispatches explicitly by format version. Legacy records
use materialized Operations when present and otherwise an isolated historical
projection fallback. Version 2 uses the frozen context and real result through
the shared projector, never EVAL or recalculation from current balances. Unknown
versions and malformed payloads follow technical retry/quarantine handling.
Acknowledge only after confirmed persistence, preserving tenant isolation.

## Compatibility changes and activation gates

Valid-flow state and row fixtures are compatibility requirements, not evidence
that every historical integrity behavior is desirable. Two intentional
fail-closed changes are required: a necessary missing companion and OnHold
underflow must reject before writes instead of continuing with incomplete or
corrupt state. Cover these separately from valid-flow compatibility fixtures.
Expanded CAS also changes concurrency detection while retaining final code 0174.
Never regenerate expected rows merely to make a regression pass.

Activation has a consumer-first sequence:

1. Deploy a reader-compatible release with support for legacy and version-2
   recovery while the existing accounting writer remains active. Verify every
   consumer is compatible.
2. Integrate the posting adapter, full snapshot-pool loading, shared projection,
   complete CAS retry, durable guards/receipts, and dual cache codec. Enable only
   after the verification below and explicit limits/retention configuration.
3. Keep compatible readers throughout rollback. Disable new executions and
   drain or recover version-2 records before any rollback to an incompatible
   consumer. Never send the same live posting to both engines for comparison.
4. Switch to new-only cache writing only after all dual writers are deployed,
   every reader accepts new-only blobs, all writers have been inventoried, and
   at least 24 hours have elapsed after the complete dual-writer rollout.
5. Remove legacy projection/override support only after open legacy pendings,
   backup queues, and quarantine are inventoried and drained. A 24-hour balance
   TTL is not proof that historical recovery records are gone.

Once new-only blobs are written, rollback requires a reader that accepts them.
The dual-compatible reader is the minimum cache rollback target; a precompatible
binary cannot safely resume against new-only data. Historical fixtures may remain
after production fallback removal.

## Verification and operational limits

Before activation, require evidence for:

- Valid-flow state, operation-row, amount, and version equivalence across direct,
  pending, commit, cancel, revert-shaped, NOTED, both directions, and external
  paths, including draw, repayment, exact limits, and legacy overrides.
- New-write → old-mutation → new-read interleaving, settings updates, stale
  lowerCamel fields, and dual readers over new-only blobs.
- Multiple transactions sharing a balance, initial CAS once per physical key,
  failure in the last posting/companion with no pre-commit writes, and accurate
  failure indices.
- Lost response after server execution, confirmed NOSCRIPT fallback, expired
  locks, commit/cancel races, fingerprint conflict, and delayed cleanup.
- Crash immediately after EVAL and failures during projection or persistence,
  producing identical normal/recovered rows and IDs without double application.
- WRONGTYPE schedule/backup/guard keys caught before writes; deliberate
  post-first-write failure classified as indeterminate rather than rolled back.
- Decimal magnitude/scale and version boundary round trips, empty collections,
  malformed wire, tenant separation, and unused versus touched pool entries.

Bound serialized bytes, transactions, postings, and snapshots before EVAL.
Choose explicit values from existing API maxima and measurements at 2, 10, and
50 postings; do not silently reject previously supported requests. Record the
chosen limits and measure full-pool loading separately from touched balances.

Observe request counts, posting types, closed failure enums, CAS attempts,
indeterminate outcomes, recovery, latency, and payload/pool sizes. Labels must
not contain money, aliases, metadata, or IDs. Limits and these signals are
enablement requirements, not follow-up hardening.

## Tentative alternative-engine mapping

TigerBeetle is a possible future adapter, not an implemented or validated mapping.
Candidate investigations include transfer pairs for debit/credit, pending/post/void
for hold/settlement/release, linked transfers for ordered atomic groups, fixed
per-ledger scaling into unsigned 128-bit amounts, and explicit transfers involving
the overdraft account. These are hypotheses requiring API and behavior validation.

The current Version contract, decimal range, direction behavior, legacy row
projection, guard/receipt semantics, and recovery guarantees may require additional
adapter logic or make a direct mapping unsuitable. No equivalence is promised.
Alternative adapters, a public multi-transaction endpoint, Valkey sharding/hash-tag
changes, and replacement of the existing overdraft representation are outside
this architecture's implementation scope.
