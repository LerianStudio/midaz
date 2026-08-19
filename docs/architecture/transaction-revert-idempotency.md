# Transaction revert idempotency

> **Status:** canonical money-path and rollout contract for transaction reversals.
> Revert correctness takes precedence over availability: when the system cannot
> prove that Redis did not move funds, it fences the origin for reconciliation.

## Product contract

A transaction origin can produce at most one economic reversal inside an
organization and ledger. Two origins with identical amounts, accounts, routes,
description, and metadata are still different origins and therefore receive
different reverse transactions. A repeat request for one origin returns the
same reserved reverse ID and never moves balances again.

The durable authority is PostgreSQL primary, keyed by:

`(organization_id, ledger_id, origin_transaction_id)`

The claim reserves the reverse transaction ID before any balance mutation.
Every phase-zero-capable pod in `prepared` or `active`, and every bridge pod,
persists the exact legacy payload-hash fence key and its
reserved-reverse owner before that fence is acquired. Recovery uses these
immutable values and never
recalculates the key or owner from mutable transaction fields.
Revert eligibility, child discovery, claim reads, and replay resolution are all
served from PostgreSQL primary; replica lag is not allowed to decide whether
money can move.

## Barriers and acquisition order

Phase-zero-capable and bridge requests acquire barriers in this order:

1. Validate the configured financial-dataset generation against both the
   `{transactions}` witness and the rollout-slot witness. Neither is created by
   a serving pod.
2. Short-lived Redis economic execution attempt plus its reserved-reverse owner
   in the shared transaction slot.
3. PostgreSQL primary claim for organization + ledger + origin, including the
   exact legacy key and reserved-reverse owner.
4. Legacy Redis payload-hash barrier, shared with genuinely old pods during
   the Release 0 replacement and with every phase-zero-capable request.
5. Redis origin barrier plus its reserved-reverse owner companion, shared with
   bridge and final pods.
6. Recoverable queue seed carrying the exact origin, attempt owner, expected
   outcome, immutable transaction input, and the versioned plan for the final
   resolved balance batch.
7. PostgreSQL primary compare-and-set from `CLAIMED` to `ARMED`, after the
   exact Redis attempt owner and seed exist.
8. Atomic Redis balance Lua, which checks the exact attempt owner and persisted
   plan digest before its first write, then writes an immutable economic outcome
   bound to that same plan in the same command.

The attempt is reserved before the claim is published, so recovery can never
observe a live claimant without the Redis owner check that fences a stale
winner. A caller that loses the PostgreSQL claim owner-releases only its own
attempt. The money mutation remains unreachable until all barriers required by
the active release are held. The claim reserves the ID used by both the balance
outcome and the persisted reverse, so recovery cannot mint a replacement ID.

The two Redis idempotency keys intentionally have different Cluster hash tags.
The legacy fence is acquired with a same-slot Lua operation that stores the
old-compatible empty value plus an owner token in a companion key. The origin
fence and its reserved reverse ID owner are acquired atomically by a second
same-slot Lua operation before validation, queue seeding, or balance Lua.
Legacy and origin keys are never combined in one Lua invocation or multi-key
operation; doing so would produce `CROSSSLOT`. Each companion owner key appends
a suffix outside its barrier's hash tag, so Redis Cluster places each pair in
one slot. While a phase-zero-capable or bridge request is in flight, both
legacy keys are persistent: an old request cannot regain the barrier
merely because a request lasts longer than a TTL. No persistent H1 is ever
created without the durable claim already naming its exact origin, key, and
owner. A proven
pre-movement failure owner-releases the pair; success atomically replaces the
empty fence with the finite-TTL replay and removes the owner key. The balance
Lua uses only the existing `{transactions}` slot for its queue, economic
outcome, schedule, balance keys, and execution attempt. Both attempt records
are checked and consumed in the same Redis command as the movement; the
protocol adds no multi-slot assumption.

## Claim states and recovery

| State | Meaning | Retry behavior |
|---|---|---|
| `CLAIMED` | Reverse ID reserved; balance execution has not been armed | Same origin cannot create another reverse; exact pre-arm evidence can be cleaned and retried |
| `ARMED` | PostgreSQL primary authorized the exact owned attempt before balance Lua | Never return to automatic retry; missing or divergent per-transaction Redis evidence requires manual reconciliation |
| `RECOVERING` | One process holds a 30-second cleanup lease for a proven pre-arm claim | Every other caller remains fenced; a crashed cleanup owner is re-electable after its lease expires |
| `MUTATED` | Balance Lua returned success | Reconcile or finish persistence with the reserved ID |
| `COMPLETED` | Reverse transaction and operations are durable | Return the exact persisted reverse; a losing retry cannot downgrade this terminal state |
| `RECONCILIATION_REQUIRED` | Lua result was ambiguous or persistence failed after movement | Return error `0501`; never release the claim or Redis fences |

A competing same-origin request that observes the winner's committed outcome
before PostgreSQL persistence returns `0501` without changing the shared claim.
Only the process carrying the authoritative Lua result may promote
`ARMED -> MUTATED`; otherwise a loser could poison a live winner and turn a
correctly serialized race into zero successful reversals. If the winner really
crashed, `ARMED` itself remains the durable no-retry reconciliation fence and
the backup consumer may still complete the reserved reverse.

Every durable claim also stores the exact financial-dataset generation read
before claim creation. Seed, balance mutation, evidence reads, and pre-movement
cleanup compare that generation in the `{transactions}` slot. A missing or
different witness is a trust-boundary failure, never a pre-movement fact.

The balance Lua changes balances, updates the transaction backup, and writes a
separate immutable economic outcome in one atomic command. The shared outcome
shape is `{identity, outcome, owner, economic_plan_version,
economic_plan_digest, before, after}`. `outcome` is `COMMITTED`
or `ABORTED`; the same identity and outcome replays the exact original balance
snapshots, while a different identity or opposite terminal outcome conflicts
before any movement. Commit, cancel, and revert use this same primitive. It is
also the boundary the later Ledger-to-Tracer dispatcher will consume; this P0
does not wire that dispatcher.

Before the queue seed or Lua dispatch, the command resolves `remaining`,
expands fees, adds overdraft companion legs, and applies accounting routing.
It then seals that exact final batch as `expected_economic_plan` version 1.
Each leg carries a unique positional identity plus balance ID/key/account,
internal Redis key, accounting direction, movement type, asset, exact decimal
amount, source/destination side, primary/fee/companion role, and expected
persisted operation type. Canonical sorting makes the digest independent of
batch order while occurrence identities preserve duplicate legs. Two fees on
one balance, identical repeated aliases, and primary-plus-fee movements are
therefore separate economic facts rather than map collisions.

The plan is built from the final balance batch, never reconstructed from the
validation alias maps. Those maps intentionally collapse aliases and cannot
represent repeated legs, fee expansion, or overdraft companions. Every new
outcome-backed balance invocation requires all operations to carry the same
plan, and Lua requires the queue seed to contain the identical version and
digest before it may seed or change a balance. A mismatch leaves no outcome and
performs no movement. Consumer enrichment and terminal replay compare the
materialized operation multiset against the persisted plan and the
Lua-authored before/after snapshots; a candidate operation can never define
both sides of its own proof.

The backup consumer accepts every owner-and-terminal-outcome envelope through
the same economic preflight, even when the envelope predates financial-dataset
generation tagging. A generation, when present, adds a witness check; its
absence never bypasses owner, outcome, identity, operation, or balance proof.
Both balance sets must match the authoritative queue envelope as exact,
order-independent multisets. Repeated touches of one balance remain repeated;
deduplicating by balance identity would erase part of the economic effect.
The operation coverage is phase-aware: a PENDING hold and its CANCELED release
move only the source side, while direct, commit, and revert outcomes cover both
source and destination. A declared-but-not-yet-moved destination is never
invented as an operation, and an economically active leg cannot be omitted by
truncating both its operation and balance snapshot.
Comparison is order-independent and includes balance ID and key, account,
alias, asset, available, on-hold, version, account type, send/receive flags,
direction, overdraft used and policy, limit, and balance scope. Decimal spelling
differences are normalized; an omitted economic field is not silently filled
from current balance state.

Every new backup and Rabbit persistence event carries
`effect_mode_version=1` plus exactly one mode: `BALANCE_MUTATION` or
`ANNOTATION_ONLY`. The producer writes this discriminator before enqueue; a
partially present, unknown, or status-incompatible mode fails closed.
`operation_type_override` is a separate queue/event field because the public
transaction input deliberately excludes the internal BLOCK/UNBLOCK marker.
Recovery restores typed operations only from that durable field, never from a
candidate operation or a mutable transaction row. A balance mutation accepts
only an empty override, `BLOCK`, or `UNBLOCK`; annotations require an empty
override because `NOTED` is derived from the immutable transaction status.
Every other value fails before Redis or PostgreSQL can be changed. Payloads predating the
discriminator follow one explicit compatibility rule: `NOTED` identifies an
annotation, while every other status remains a balance mutation and must pass
the full snapshot proof. Missing balances alone never identifies an
annotation.

`ANNOTATION_ONLY` is not an economic-outcome shortcut. It may contain only
NOTED audit rows whose transaction, tenant, alias/key, asset, type and
direction match the immutable input. The transaction-level informational
amount remains nonzero and must equal the immutable input exactly.
`balanceAffected` is false; each operation's economic amount, before/after
available and on-hold, versions, and overdraft snapshots are all zero. The
envelope and event carry no balance snapshots, attempt owner,
or terminal outcome. Go proves those facts before an exact-raw CAS chooses the
annotation row IDs under the separate
`midaz:transaction-annotation-effect:v1` digest domain. A lost CAS response or
redelivery adopts the same rows. Annotation handling never updates balances,
creates a financial tombstone, or invokes an economic cleanup path.

The complete operation-and-balance multiset is sealed in Go as a
`midaz:transaction-economic-effect:v1` SHA-256 digest. Decimal normalization
uses the ledger decimal library, never Lua numbers or `float64`; operation and
balance entries are sorted while duplicates remain in the digest input. Lua
treats the digest as an opaque exact string. Transaction identity is included
in every operation entry. The transaction-level amount and asset are also
sealed from the immutable input in both the economic and annotation digest
domains; a persistence candidate cannot supply both sides of that comparison.
Attempt owner, terminal outcome, and dataset
generation remain explicit envelope fields and are compared alongside the
digest in the same bind and finalization commands.

The immutable outcome survives the five-minute request lease and asynchronous
Rabbit/backup delays, crosses the durable-write payload, and is deleted only
after the transaction, every operation, and any revert claim terminal state are
durable. After Lua, the queue envelope is never replaced. A same-slot read-only
command returns the exact raw envelope, immutable outcome, and Lua-authored
snapshots. Go decodes those bytes without `float64`, reconstructs the expected
economic effect from the immutable transaction input plus before/after
snapshots, and rejects any candidate amount, direction, type, asset, balance,
parent, status, action, or tenant that is not independently implied by that
evidence. Only then may an exact-raw CAS single-assign operation IDs and the
digest. A CAS loser rereads the authoritative bytes and adopts the winner only
after repeating the full proof; it never overwrites or trusts its own candidate.
Every PostgreSQL writer uses the returned operation set rather than its locally
generated candidate IDs. Two consumers and a restart after a lost CAS response
therefore converge on one durable operation identity set.

Synchronous writes, the individual Rabbit consumer (including async publish
fallback), and the default bulk Rabbit consumer all enter one terminal
persistence handoff. That handoff performs the same ordered proof every time:

1. Read the transaction and its complete operation set from PostgreSQL primary.
2. Verify transaction ID, origin, terminal status, transaction amount and asset,
   exact operation-ID multiset,
   and every operation's complete economic effect against the immutable payload:
   balance ID/key, direction, type, asset, amount, balance-affected flag, and
   before/after available, on-hold, version, and overdraft values.
3. Complete the durable revert claim.
4. Publish or verify the exact origin and persisted H1 replays.
5. Atomically publish an append-only terminal persistence receipt and then
   clean the owner/outcome-matched Redis backup and outcome in the same Lua
   command.

No consumer owns a partial version of this sequence. Bulk duplicates,
individual redelivery, fallback after publish failure, a crash after the
PostgreSQL commit, and a lost Rabbit acknowledgement all re-enter the same
handoff. A lost replay-publication response is accepted only when one same-slot
read observes both the exact reserved replay and the absence of its owner
companion; an exact replay with a surviving owner remains reconciliation work.
Cleanup is another same-slot owner/outcome-checked Lua command that compares the
complete economic operation and balance multisets, writes the non-expiring terminal receipt, and
only then removes the backup and outcome atomically. The receipt binds the
dataset generation, transaction identity, exact transaction amount and asset,
owner, terminal outcome, action,
canonical operation IDs and full economic operation bodies, and Lua-authored
balance snapshot multiset. The multiset keeps repeated touches of one balance
(for example principal plus fee settlement) and compares their exact count and
economic bodies order-independently. It is append-only: retrying the exact cleanup is idempotent,
while an opposite outcome or different economic body is a conflict. Commit and
cancel prove every operation in their terminal attempt while preserving the
older PENDING hold operations as durable history; a reverse has no older
history and therefore requires exact equality. A mismatched owner or operation
set removes nothing. A transport timeout is
therefore never interpreted as proof that funds did not move. TTL expiration
can retire only a transient execution or recovery lease. It never erases an
immutable outcome, durable claim, or persistent legacy/origin fence, and it is
never evidence that movement did not happen. The queue seed
exists before Lua dispatch, but it has no `balancesAfter`; the consumer never
persists that seed as a completed reverse. While a `CLAIMED` winner is live,
its owned execution attempt prevents a retry from declaring that seed
abandoned. If the exact attempt expires before the claim is armed, PostgreSQL
recovery wins the compare-and-set to `RECOVERING`; the stale writer can no
longer arm and therefore cannot reach balance Lua. Absence is considered only
together with a `CLAIMED` phase, the immutable outcome, and exact reserved seed
facts; elapsed time contributes no safety proof. A retry then verifies either
that the reserved backup is absent (crash before seed) or that the seed carries
the exact claim origin,
atomically elects one PostgreSQL recovery owner, removes the seed first, then
owner-releases the origin and legacy fences, releases PostgreSQL last, and re-enters the
normal claim path. Cleanup is idempotent, and a `RECOVERING` owner that crashes
can be re-elected after 30 seconds. If any cleanup step is uncertain, the claim
stays fenced and moves to reconciliation instead. Origin, legacy, and execution
attempt cleanup are compare-and-delete by reserved reverse ID. The queue seed is
also owner-fenced: a delayed writer can seed only while its exact execution
attempt is live, and it cannot replace either a successor's seed or a terminal
Lua envelope. Pre-movement queue cleanup compares the seed status, attempt
owner, and expected outcome in one Lua command; if a terminal envelope wins the
race, cleanup removes nothing and PostgreSQL remains fenced. A stale request or expired
`RECOVERING` owner therefore cannot delete a successor's fence even if it
resumes after that successor completes.

`ARMED` is intentionally asymmetric. Once primary commits `ARMED`, an expired
attempt plus a missing backup/outcome may be either a crash before Lua or
selective loss of Redis evidence after Lua moved funds. The system never
guesses between them: it preserves the PostgreSQL claim and all surviving
fences, returns `0501`, and requires manual reconciliation. A global dataset
generation proves which Redis dataset is being read; it cannot prove that one
transaction's keys were never lost.

Transaction-backup deletion has no key-only API. Every economic envelope
removal belongs to one of four explicit proof classes:

| Cleanup class | Atomic deletion proof |
|---|---|
| Proven pre-movement failure | Status, attempt owner, expected outcome, and absence of `balancesAfter` all match in one Lua command |
| Outcome-backed durable persistence | Transaction identity, immutable owner/outcome, terminal backup, operation identities and complete economic operation and balance bodies match; backup and outcome are removed together |
| Drained old-compatible persistence | Reverse ID, origin ID, terminal status, complete persisted operation identities, and complete economic operation and balance bodies are present; an outcome-backed or incomplete envelope is rejected; the compatibility receipt is written before the backup is removed. This is the only explicit planless compatibility rule. |
| Durable quarantine | The raw bytes copied into PostgreSQL still exactly equal the Redis field; a successor value under the same key is preserved |

The retry-attempt counter is non-economic bookkeeping and is the only direct Go
hash deletion. A permanent structural test inventories these classes, rejects
any key-only backup deletion API, and fails when a new backup `HDEL` appears
without an explicit atomic proof classification.

A bridge consumer also adopts parent-less bridge backup records only when a
PostgreSQL claim already maps their reverse ID to an origin. Backups created by
genuinely old pods before Release 0 have no pre-movement claim but may carry the
exact origin; the consumer accepts one only when the same backup contains Lua's
atomic `balancesAfter` outcome, then creates and completes the claim after the
child and every operation are durable. That adoption derives the released
payload hash from the immutable backup input and persists its exact legacy
fence key with a deliberately null owner classification; later retries never
derive it from the mutable origin row. Phase-zero-capable and bridge/final
backups carry both the origin and their pre-movement claim, and the
consumer verifies the pair. A truly old backup with neither a
claim nor an explicit origin has no trustworthy parent and is quarantined for
operator reconciliation; the consumer never guesses an origin from amount,
accounts, or other economic payload fields. An existing claim is never
overwritten when its reserved reverse ID differs from a backup or persisted
child.

Only a genuinely old pod can create the finite-TTL, unowned H1 accepted during
Release 0 compatibility. After PostgreSQL primary proves an adopted reverse and
every operation durable, the terminal handoff may replace that H1 only through
a same-slot CAS that requires the main value still be empty and the owner
companion still be absent. A foreign replay or any owner makes the CAS fail
closed. A phase-zero-capable pod uses the durable protocol before and after
marker activation: the claim records origin, exact H1 key, and owner before a
persistent H1 pair is created. Terminal redelivery chooses owned versus
unowned CAS solely from that durable owner classification, never from process
memory or which request first observed the claim. Recovery never uses a blind
`SET`.

For an incomplete claim, a persisted reverse is replayable only when its full
operation-ID set matches the atomic Redis backup. Every new reverse derives
operation IDs from the reserved reverse ID, and operation materialization is a
required owner/outcome-checked enrichment before PostgreSQL persistence.
Replay after partial PostgreSQL persistence therefore reuses the exact IDs and
must match every operation's balance ID/key, direction, type, asset, amount,
balance-affected flag, and before/after available, on-hold, version, and
overdraft values.
Merely finding the child row or one operation cannot prove that
persistence finished. Rebuilds use Lua's `balancesAfter` snapshot, including
direction, overdraft-used before/after, balance settings, and versions, instead
of recomputing the audit trail. A reverse created by a genuinely old pod is
adoptable only while its authoritative backup retains enough immutable input to
derive the exact original H1 and parent. A missing or insufficient old backup is
quarantined even when the child and operations already exist on primary: their
existence does not identify the released payload-hash fence, and the current
mutable origin can never be used to recalculate it. If a drained old-compatible
backup remains after those PostgreSQL facts and the claim are terminal, final
mode removes it only through an exact Lua proof of the reserved reverse ID,
parent origin, compatibility status, full operation-ID set, and field-complete
economic operation and balance snapshots. An incomplete legacy envelope is
preserved for reconciliation; it is never converted into a terminal receipt or
treated as proof of successful cleanup. That
compatibility cleanup never touches an outcome-backed envelope.

## Failure policy

| Failure point | Proof available | Action |
|---|---|---|
| Before the primary claim is `ARMED`, with a confirmed failure response | No movement was attempted and no seed write is ambiguous | Prove removal of the reserved backup seed, owner-release the origin and legacy barriers, then release the still-`CLAIMED` PostgreSQL claim last; retry is allowed |
| Queue seed response is lost | The seed may exist, but movement was not dispatched | Preserve claim, execution attempt, origin, legacy, and possible seed; mark reconciliation required |
| Lua-declared validation rejection | Lua rolled back the complete batch | Release all barriers and the claim; retry is allowed after the business condition changes |
| Transport error after Lua dispatch | Commit outcome is ambiguous to the caller | Read the immutable Redis outcome; exact outcome recovers, unreadable outcome preserves every barrier and requires reconciliation |
| Error after Lua success | Funds moved and immutable outcome exists | Preserve every fence; mark reconciliation required |
| Crash after Lua and before PostgreSQL persistence | Reserved ID, immutable economic outcome, and atomic `balancesAfter` backup remain | Backup consumer persists exactly that reverse and completes the claim |
| Lost response after operation-ID enrichment | PostgreSQL status may already be terminal; the exact post-Lua envelope and outcome remain | Preserve the request fence; the delayed consumer persists the same operation IDs, then atomically removes backup and outcome |
| Crash after PostgreSQL persistence and before origin replay completion | Exact child and operations exist on PostgreSQL primary; owned origin fence remains | Retry completes or safely rematerializes the exact reserved replay, removes the owner, and returns it |
| Crash after old-pod PostgreSQL persistence and before H1 replay publication | Exact child, complete operation set, and terminal adopted claim exist; H1 is still the unowned empty compatibility fence | Retry replaces only that empty/no-owner H1 with the exact reserved replay; a foreign value or owner is preserved and fails closed |
| Crash after bridge child persistence and before H1 completion | Durable claim retains the original H1 key; child and every operation exist on primary | Final adoption completes only the persisted H1 key, marks the claim terminal, and owner-checks outcome cleanup; it never recalculates H1 from the origin |
| Lost response after rollout-generation completion | Durable claim retains the exact `legacy` or `bridge` generation and deterministic origin token; transaction, operations, claim, replays, and Redis economic cleanup are already proven terminal | HTTP or consumer redelivery repeats the same generation seal idempotently; it never releases a generation inferred from the current pod mode |
| Final adoption sees a foreign H1 collision | The durable claim and child prove this origin; the legacy key explicitly belongs to another owner or replay | Preserve the foreign H1 unchanged, finish the origin-scoped replay, and clean only this reverse's exact outcome/backup |
| Crash after an old-compatible child is durable but before backup cleanup | Child, all operations, and completed adopted claim exist on PostgreSQL primary; legacy backup has no owner/outcome envelope | Compare reverse, parent, status, every operation ID, and require field-complete economic operation and balance bodies in one Lua command; incomplete evidence stays quarantined; only exact evidence publishes a compatibility receipt and removes the backup |
| Crash before queue seed | `CLAIMED` names the exact origin, reverse, H1 key, owner, and current dataset generation; one atomic generation-bound read proves backup, execution attempt, and immutable outcome absent | Elect one `RECOVERING` owner, generation-check and owner-release the exact barriers, release PostgreSQL last, and retry; the stale writer cannot arm after recovery wins the primary CAS |
| Crash after seed but before arm | A valid exact-origin queue seed exists without `balancesAfter` or immutable outcome, the claim remains `CLAIMED`, the exact execution attempt is absent, and the configured/claim/Redis generation still agrees | Elect one `RECOVERING` owner, generation-check and clear Redis barriers/seed, release PostgreSQL last, and retry; the stale writer cannot arm after recovery wins the primary CAS |
| Crash after `ARMED` but before Lua, or loss of all per-transaction Redis keys | Primary proves the attempt crossed the durable point of no automatic return, but Redis cannot prove whether movement occurred | Preserve claim and every surviving barrier, return `0501`, and require manual reconciliation; the global dataset witness alone cannot authorize retry |
| Pre-movement cleanup races a terminal Lua envelope | Status/owner/outcome no longer match the exact seed selected for cleanup | Atomic cleanup removes nothing; preserve all barriers and require reconciliation |
| Crash while cleaning `RECOVERING` | PostgreSQL retains the cleanup state and timestamp | Re-elect after 30 seconds and resume idempotent cleanup; PostgreSQL remains the last record released |

Only proof that dispatch cannot happen (the configured and claimed generation
matches both Redis witnesses, financial durability remains healthy, and one same-slot Redis read proves the
exact backup, outcome, execution attempt, and attempt owner are all absent,
together with an absent compatible PostgreSQL claim or the valid exact-origin
seed-only crash case) or a Lua-declared rollback
authorizes release. Redis timeouts, connection resets,
context cancellation after dispatch, and PostgreSQL failures after movement do
not authorize a second mutation. Legacy and origin cleanup use compare-and-delete
Lua operations. Their in-flight fences do not expire, and a stale owner can
neither delete nor replace a fence owned by another reverse ID. The ambiguous and
post-movement branches never call cleanup, so neither a lost Lua response nor a
later persistence failure can release the legacy/origin barriers or the
durable claim.

If automatic pre-movement cleanup stops in `RECOVERING` or
`RECONCILIATION_REQUIRED`, an operator must first verify on Redis primary that
the reserved backup has no `balancesAfter`, verify on PostgreSQL primary that no
child exists for the origin or reserved reverse ID, and stop revert traffic for
that origin. Only then may the operator delete the exact origin barrier and
backup, owner-release the exact legacy fence using the reserved reverse ID, and
delete the matching claim last. A backup with `balancesAfter`, an unreadable
backup, a child, or an ownership mismatch is never eligible for release.

## Commit and cancel terminal outcomes

Commit and cancel serialize one PENDING transaction across the complete money
path. The command acquires the transaction's Redis mutation lock, reads from
PostgreSQL primary, locks the row, and holds that row lock across the Redis
balance command and the PostgreSQL terminal write. The terminal write is a CAS
from `PENDING`: `COMMITTED` promotes to `APPROVED`, `ABORTED` promotes to
`CANCELED`, an exact repeat is idempotent, and the opposite terminal can never
overwrite the winner.

Redis is the authoritative fact for an ambiguous balance response. A lost
`COMMITTED` response followed by cancel replays the committed snapshots,
persists `APPROVED`, and rejects cancel without a second movement. A lost
`ABORTED` response followed by commit symmetrically persists `CANCELED` and
rejects commit. This remains true after the five-minute request lease expires
and while the durable consumer is delayed. If Redis outcome resolution is
unavailable, the API returns reconciliation error `0503`; it never guesses from
the PostgreSQL status or moves funds again.

The Lua-authored backup remains the authoritative handoff after movement.
Commit/cancel never reseeds or replaces it after the terminal PostgreSQL CAS;
the exact operation IDs are added through the same owner/outcome CAS used by
reverts. A consumer delayed beyond the request lease can therefore persist the
winner without reconstructing identity, while an opposite HTTP retry observes
the terminal row and cannot move balances. Backup and outcome are removed
together only after every operation is durable.

PATCH participates in the same transaction serialization. Its final response
is loaded from PostgreSQL primary, so a lagging replica cannot return stale
status or mutable fields after the write.

If the PostgreSQL commit response is lost, PATCH performs a bounded primary
reread using transaction identity, organization, ledger, pre-update status,
the exact desired description, and a strictly monotonic update version. The
metadata snapshot is also part of the payload proof because it lives in MongoDB.
An exact applied PostgreSQL row after a confirmed metadata write returns success
and releases the APPROVED-update token. An exact pre-update PostgreSQL row proves
rollback only when metadata is also unchanged; then the token is released and
the commit error allows retry. A missing, unreadable, half-applied, or divergent
payload retains the token and returns reconciliation; it is never treated as
rollback.

## Rollout: old to phase zero to bridge to final

The legacy payload hash includes mutable transaction fields. Allowing an
APPROVED origin to change while old and bridge algorithms coexist would let the
same origin acquire two different legacy barriers. The rollout therefore uses
one deployment-wide Redis state at
`rollout:{transaction-revert-rollout:v1}:state` and binds it to the immutable
UUID configured as `REVERT_REDIS_DATASET_GENERATION`. Initialization writes the
same UUID to a rollout-slot witness and to
`financial:{transactions}:dataset-generation`. Before either Redis write, the
initializer atomically inserts a singleton `PREPARING` birth certificate on the
deployment transaction PostgreSQL primary. The row binds the generation to a
second one-shot UUID, `REVERT_ROLLOUT_INITIALIZATION_ID`. After Redis contains
the exact generation and `prepared` marker, PostgreSQL promotes the exact row
to `PREPARED`. Serving targets compare both Redis witnesses and the PostgreSQL
birth certificate; they never create any of them:

| Value | Phase-zero readiness/revert | APPROVED updates | Bridge readiness/revert | Final readiness/revert |
|---|---|---|---|---|
| absent | Released old algorithm plus initialization drain lease; rollout targets rejected | Allowed only with empty target | Rejected with `0502` | Rejected with `0502` |
| `prepared` | Allowed with target `prepared` | Allowed and durably counted | Rejected with `0502` | Rejected with `0502` |
| `active` | Allowed | Rejected with `0008` on phase zero, bridge, and final | Allowed | Rejected with `0502` |
| `phase-zero-drained` | Rejected with `0502` | Rejected with `0008` on bridge and final | Allowed | Allowed |
| `finalized` | Rejected with `0502` | Allowed on final | Rejected with `0502` | Allowed |

PENDING transactions remain mutable in every state because they are not
eligible for revert. The marker is global rather than tenant-prefixed, so every
pod and every tenant observes one barrier. Persistent in-flight structures use
the same Redis Cluster hash tag: one set for APPROVED updates, one active-origin
set per revert generation, and one attempt-ID hash per active origin. Request
admission reads the marker and adds a `pod-hostname:request-uuid` token for an
APPROVED update or idempotently inserts a fresh HTTP attempt ID under
`SHA-256(organization:ledger:origin)` for a revert in one Lua command. Retrying
the same admission after a lost response cannot add a second field. A normal
return deletes only its exact attempt field; the origin leaves the active set
only when its attempt hash is empty. The unified terminal handoff first seals
the origin as completed and then removes its complete attempt hash after
PostgreSQL, claim, both replays, and Redis cleanup agree. A delayed admission or
release becomes an idempotent no-op and cannot recreate the origin. Marker
transitions run in the same slot and refuse to advance while the generation
they retire has any active origin.

Completed-origin sets are append-only tombstones scoped by rollout generation.
Marker transitions never delete them. No retention period is currently defined;
deleting these tombstones would weaken lost-response and delayed-redelivery
proof and therefore requires a separate product contract and archival design.

An HTTP return may remove its exact attempt only after one same-slot transaction
read proves backup, outcome, execution attempt, and attempt owner absent; the
PostgreSQL primary claim names the same reverse and is `COMPLETED`; and one
same-slot rollout read proves the persisted generation tombstone exists while
its attempt hash and active-origin membership are absent. A missing, unreadable,
or internally inconsistent proof keeps the token and requires reconciliation.
In particular, successful economic cleanup is not permission to release the
last rollout attempt when generation sealing returned an ambiguous error.

Marker transitions are executed by ledger startup through
`REVERT_ROLLOUT_TARGET`, not by an operator writing Redis directly. Accepted
targets are `initialize`, `prepared`, `active`, `phase-zero-drained`, and
`finalized`; an invalid target,
an out-of-order transition, or a transition with an in-flight retiring request
fails startup. The deployment controller still proves every individual pod's
readiness capability before changing the target, because Redis cannot infer
which application generation a cluster scheduler is still running.

`initialize` is a one-shot deployment action and never starts HTTP or background
consumers; the process exits successfully after initialization. It is the only
target authorized to create an uninitialized generation. The PostgreSQL state
machine is `absent -> PREPARING -> PREPARED`: a crash before the Redis writes
can resume only the exact PREPARING generation/request pair; a crash after the
Redis writes adopts only the exact witnesses before promoting PostgreSQL. An
exact retry after a lost PostgreSQL commit response rereads the primary and is
idempotent. A different generation or initialization request conflicts.
`PREPARED` plus any missing or divergent Redis witness fails closed forever; the
initializer cannot recreate it. After initialization,
the initializer is removed and phase zero is deployed with target `prepared`.
Restarts in `prepared`, `active`, `phase-zero-drained`, and `finalized` validate
the exact marker, both Redis generation witnesses, and the PostgreSQL primary
birth certificate without writing them. Loss after `prepared` therefore makes
readiness and money-path admission fail; losing all three Redis keys or only the
rollout-slot keys cannot be reclassified as first install.

The birth certificate is deployment-scoped, not tenant-scoped. In
multi-tenant mode it always uses the static `DB_TRANSACTION_*` primary as the
deployment control database and deliberately ignores a tenant database carried
in request context. One Redis rollout marker therefore has exactly one external
birth certificate across all tenants.

Target-empty phase-zero-capable pods do not cache the certificate's absence.
Every readiness and money-path admission reads the deployment PostgreSQL
primary; an absent replica is irrelevant and a primary read failure is
fail-closed. An admitted old-algorithm revert still registers its unique
attempt in the deployment-wide Redis drain set. It rereads the primary
certificate immediately before entering the balance path. If initialization
has inserted `PREPARING`, the request aborts before movement; if initialization
starts after that final read, the initializer cannot publish either Redis
witness until the exact old request leaves the drain set. This closes both
orders of the paused-request race without changing the released legacy
idempotency algorithm.

The tokens and attempt hashes have no TTL. A crashed request may block rollout
availability, but state can never expire underneath a still-running money-path
request and create a false drain proof. A phase-zero/bridge retry reuses the
same deterministic origin token while owning a distinct attempt ID; recovery
releases all attempts only after proving the origin terminal or pre-movement.
An operator never guesses that a timeout means the request stopped. An APPROVED update
cannot therefore be admitted before activation and persist afterward:
`prepared -> active` is serialized after every admitted update token. Phase-zero
and bridge reverts are similarly serialized against the transition that retires
their generation. All marker and lease operations are same-slot; none couples
a tenant's legacy and origin barriers or assumes multi-slot Lua.

If the APPROVED-update admission Lua commits but its response is lost, the
handler stops before its PostgreSQL write and removes only that request's unique
token. A revert admission response may also be lost, but re-executing the Lua
with the same attempt ID is idempotent and an exact release can remove only that
attempt. If release is itself ambiguous, the request returns an error and any
surviving attempt remains until same-origin recovery proves it safe to release.
A distinct same-origin caller has a different attempt ID and cannot be removed
by the loser. Availability may block; a false drain cannot be manufactured.

### Redis financial trust boundary

Between the atomic balance Lua and the PostgreSQL terminal handoff, Redis is the
authoritative economic record. Phase-zero activation and bridge/final readiness
therefore fail closed unless every reachable Redis primary or shard reports
`maxmemory-policy=noeviction`, AOF enabled, `appendfsync=always` or `everysec`,
and a healthy last AOF write. `CONFIG` and persistence `INFO` must be readable by
the ledger identity; a managed provider that hides them needs an equivalent
machine-verifiable attestation before it can run this rollout.

This gate proves that eviction is disabled and that the configured persistence
mechanism is healthy; it does not promise zero loss. `appendfsync=everysec` and
the provider's replication and backup policy define a non-zero RPO. Loss of the
complete Redis financial store beyond that RPO is a trust-boundary failure, not
evidence that a request crashed before seeding or movement. Automatic recovery
must stop and reconcile from external durability evidence; it may never release
a claim or admit a second mutation merely because backup and outcome disappeared
after a Redis data-loss event. TTL applies only to transient execution leases
where expiration is not used as economic proof; no economic outcome, persistent
fence, rollout attempt set, or drain fact depends on TTL expiry.

The supported automatic absence proof is deliberately bounded by that trust
contract: the PostgreSQL claim's generation must equal the prepared birth
certificate, Redis must still report the same generation and healthy persistence,
and backup/outcome/attempt/owner absence must be read atomically. This does not
detect silent partial corruption or a provider restore that preserves the
generation key while discarding newer data. Such an event is beyond the
configured Redis RPO and must be surfaced operationally as dataset loss, with
serving disabled and a new generation admitted only by a future explicit
reconciliation migration. Re-running `initialize` is never the recovery path.

The immutable configured UUID is the external deployment identity for one
financial dataset. The PostgreSQL birth certificate proves that the identity was
assigned once, the Redis rollout-slot witness binds that identity to the state
machine, and each PostgreSQL claim binds it to one economic attempt.
With any serving target from `prepared` onward, a missing or regressed marker,
a missing witness, or a different generation fails readiness, revert admission,
and APPROVED-update preflight. `legacy` cannot reinterpret total Redis loss as
the pre-rollout absent state. The rollout witness comparison is part of the
same Lua admission snapshot as the marker; the financial witness is read before
admission and checked again atomically by seed, balance, and cleanup Lua in the
`{transactions}` slot. No Lua spans those two Cluster slots.

Async, sync, bulk, and redelivered consumers classify economic evidence by its
attempt owner and terminal outcome, not by generation presence. Before the
first PostgreSQL transaction, metadata, operation, or balance write, they use
the same read-only Redis preflight, independent Go economic reconstruction, and
exact-raw CAS described above. When a generation is present, the read and CAS
additionally validate the current financial generation. A
consumer delayed beyond the request lease, or across a
dataset generation change, therefore preserves the backup and rollout token for
reconciliation instead of materializing an old generation in PostgreSQL.

After terminal cleanup, a redelivery can become a read-only acknowledgement
only when the same-slot Redis preflight finds the exact terminal persistence
receipt and no live backup or outcome, the current witness still names the
receipt generation, PostgreSQL primary contains the economically identical
transaction amount, asset, and complete canonical operation set, and a reverse claim is
`COMPLETED` for that exact origin and reverse. A missing receipt, partial Redis
restoration, opposite outcome, changed generation, claim mismatch, or economic
divergence is reconciliation; none can recreate operations or acknowledge a
foreign replay. Terminal receipts have no TTL and rollout transitions never
delete them. Their future retention requires a separate archival contract.

The sole compatibility exception is a reverse admitted before the one-shot
initialization certificate existed. Its terminal receipt and adopted claim both
carry the explicit pre-generation shape: empty generation, empty owner/outcome,
and exact reverse, parent, terminal status, operation bodies, and balance
snapshot. That pair cannot be confused with an outcome-backed receipt, and no
new target-empty request is admitted once `PREPARING` exists. A future dataset
generation rollover is unsupported and must reconcile or migrate these receipts
explicitly; an empty generation is never silently rebound to a new witness.

A PENDING update locks its transaction row on the PostgreSQL primary before it
checks status and holds that row lock through the PostgreSQL description and
MongoDB metadata writes. Every commit, cancel, backup consumer, and recovery
promotion updates the same row, so it waits for the serialized update and then
writes only the status transition. It cannot promote between the update's status
read and write, and a stale promotion snapshot cannot overwrite the completed
description. A rollback or lost database connection releases the row lock
automatically; no persistent Redis lease can strand future transitions. The
existing commit processing key remains independent, so its success TTL cannot
block an update after finalization restores APPROVED mutability. Description-only
updates also preserve the persisted accounting body; clearing that body would
make the later commit response claim APPROVED without promoting the durable
transaction.

### Release 0: freeze-capable legacy algorithm

1. Apply the published `000036_create_revert_claim` and
   `000037_add_revert_rollout_generation` unchanged, then apply
   `000038_create_revert_rollout_initialization` and
   `000039_arm_revert_claim`. `000037` is additive and idempotent:
   it adds rollout mode/token and financial generation to an already-migrated
   `000036` database without rewriting existing claims. `000038` adds the
   deployment-scoped birth certificate without reusing an already-recorded
   migration version. `000039` adds the monotonic `ARMED` boundary. Because an
   already-existing nonterminal claim cannot prove that balance Lua was never
   invoked, the first `000039` application conservatively promotes every old
   `CLAIMED` or `RECOVERING` row to `ARMED`; an idempotent rerun does not alter
   claims created afterward. Deploy every pod with
   `REVERT_IDEMPOTENCY_MODE=legacy` and an empty target. This is the released
   old algorithm plus rollout capability; it neither creates a witness nor
   claims durable phase-zero ownership yet.
2. The deployment controller must verify every individual pod's `/readyz`
   response contains `checks.revert_rollout_barrier.status=up`, not merely an
   aggregate service health result. A pre-phase-zero pod does not expose that
   check and therefore cannot satisfy the gate.
   Before initialization, released legacy readiness does not impose the new
   durability contract.
3. After the deployment controller proves zero pre-phase-zero binaries, choose
   one UUID and run exactly one unready instance with
   `REVERT_ROLLOUT_TARGET=initialize` and
    `REVERT_REDIS_DATASET_GENERATION=<uuid>` and
    `REVERT_ROLLOUT_INITIALIZATION_ID=<different-uuid>`. An exact retry uses
    both values; either value changing is a conflict. Successful startup proves
   noeviction, healthy AOF, and the exact `uninitialized -> prepared` witness.
   Stop the initializer. Deploy all phase-zero pods with target `prepared` and
   the same UUID. Target-empty pods become unready as soon as `prepared` exists;
   the deployment must use a controlled drain or blue-green cutover rather than
   route traffic through that boundary. Every prepared revert now reserves its
   PostgreSQL claim before persistent H1 creation.
4. After every phase-zero pod is ready on `prepared`, deploy
   `REVERT_ROLLOUT_TARGET=active` with the same UUID. Ledger startup atomically
   changes `prepared` to `active`. Activation refuses while any APPROVED update or
   phase-zero revert admitted before activation is still executing, so success
   proves both mutable origin writes and pre-activation money paths are drained.
   Phase zero continues the same durable claim + owned-H1 protocol while the
   active marker additionally freezes APPROVED updates for bridge coexistence.
   Activation is idempotent only while
   the state is prepared or already active with the exact UUID; a `finalized` rollout cannot be
   reopened.
5. Prove an APPROVED update is rejected on every pod before any bridge pod is
   admitted. The bridge's readiness and per-revert preflight independently
   enforce the same state, so a missing, corrupt, unreadable, or inactive marker
   cannot become a money-path request.

In prepared and active states, phase zero reserves one reverse per origin in
PostgreSQL before creating its persistent H1 and verifies the cached reverse
parent before returning it. Economically identical origins may still share the
old payload-hash slot during coexistence; that collision is a conflict, never a
response carrying another origin's reverse. A crash before seed leaves enough
claim, H1 owner, and deterministic rollout admission for a same-origin retry to
prove pre-movement cleanup and retry without timeout heuristics.

Activation before step 2 is unsafe: code older than phase zero does not honor
the marker. The readiness capability check makes that ordering machine
verifiable; it is not a human assertion hidden in a runbook.

### Release A: bridge

1. Deploy with `REVERT_IDEMPOTENCY_MODE=bridge` and
   `REVERT_ROLLOUT_TARGET=active`, preserving the exact
   `REVERT_REDIS_DATASET_GENERATION`. A pod remains unready and every
   revert returns `0502` unless the shared marker is `active` or
   `phase-zero-drained`; coexistence starts in `active`.
2. Keep the marker active while phase-zero and bridge pods coexist. Both
   generations reject APPROVED updates and serialize reverts on the legacy
   payload-hash barrier. Its visible value remains empty while either generation
   owns it, preserving the old algorithm contract; owner metadata is stored only
   in the same-slot companion key. Both generations also reserve by origin in
   PostgreSQL and acquire the origin Redis barrier.
3. Stop routing new work to phase-zero pods and terminate every phase-zero pod.
   Deploy bridge with `REVERT_ROLLOUT_TARGET=phase-zero-drained`; startup
   atomically advances the marker from `active` to `phase-zero-drained`. The
   transition refuses while an admitted phase-zero revert token exists and
   prevents new phase-zero admission in the same Lua command, so success proves
   the request drain. The new state makes any surviving phase-zero pod unready
   while bridge/final pods keep APPROVED updates frozen.
4. Drain or reconcile every backup created by old or phase-zero pods. A
   phase-zero-capable backup always carries its explicit parent, pre-movement
   claim, the exact persisted H1 key and owner, the final economic plan, and the
   atomic balance outcome after movement. A
   parent-less backup is accepted only when its reverse ID already has a durable
   claim. A genuinely old backup with no trustworthy origin is quarantined. An
   old explicit-parent seed without an atomic outcome remains untouched before
   the drained marker; after the marker proves no old request can resume, the
   consumer first verifies no claim exists for the origin, then requires the
   backup's persisted H1 key to equal the key derived from that backup's own
   immutable input snapshot. Only then may it delete an empty H1 with no owner,
   and only afterward delete the exact seed. A missing or divergent persisted
   H1 witness is quarantined for reconciliation; the consumer never chooses a
   deletion target by hashing a mutable PostgreSQL transaction. A crash
   between those steps leaves the seed for idempotent redelivery rather than
   orphaning H1. Do not advance while any backup can
   represent an unclaimed balance movement.

Bridge never returns a cached reverse whose parent differs from the requested
origin. A legacy collision is a conflict, not a replay. Changing only the mode
while an older generation still serves traffic is not a rollout.

### Release B: final and unfreeze

After all Release A exit criteria are satisfied, deploy
`REVERT_IDEMPOTENCY_MODE=final` and
`REVERT_ROLLOUT_TARGET=phase-zero-drained`, preserving the exact
`REVERT_REDIS_DATASET_GENERATION`, while the marker remains
`phase-zero-drained`. Final mode
uses the PostgreSQL claim and origin Redis barrier as authorities. It still
reads the legacy barrier as a rollout fence and can adopt an exact same-origin
old result, but a value belonging to a different origin is never returned and
does not block the independent origin.

If final encounters a pre-movement claim created by bridge, recovery reads the
exact legacy fence key persisted in that claim and owner-releases it with the
claim's reserved reverse ID before releasing PostgreSQL. Recovery never derives
the key from the current origin payload. A foreign payload collision is left
untouched and does not block final recovery; final never deletes or replays a
legacy value owned by another reverse.

Every final pod must become ready before bridge pods are drained. Then stop and
terminate bridge pods and deploy final with `REVERT_ROLLOUT_TARGET=finalized`.
Startup atomically changes the marker from `phase-zero-drained` to `finalized`.
Finalization refuses while an admitted
bridge revert token exists and prevents new bridge admission in the same Lua
command, so success proves the request drain. This terminal state restores APPROVED
updates, keeps final pods restart-safe, and immediately makes any surviving
phase-zero or bridge pod unready while its per-request preflight rejects new reverts. A
missing or merely `active` marker cannot be finalized, so finalization cannot
skip the machine-verifiable phase-zero drain proof.

Final-to-final and bridge-to-final races share the PostgreSQL claim and origin
barrier. Multiple callers may receive the same completed replay; only one
reverse ID, persisted transaction, and balance movement can exist.
The claim also retains the exact retiring rollout generation and origin token.
Terminal sync, async, bulk, HTTP adoption, and redelivery all use that persisted
identity to seal the generation only after transaction, full operation set,
claim, both replays, and Redis economic cleanup are durably complete.

## Rollback and migration down

The current rollout is forward-only after `finalized`. A bridge binary remains
unready and its per-request preflight returns `0502`; configuration cannot
reopen the v1 marker. A future rollback would require a new marker version,
another Release 0 freeze/capability proof, and an explicit backfill of exact
legacy fence keys for claims created by final. Without all three, downgrade
stays fail-closed.
Rolling bridge code back to old code is a money-path operation: old code ignores
the durable claims, so it requires a traffic stop, zero in-flight bridge/final
requests, completed backup reconciliation, and verified PostgreSQL primary and
replica convergence before old code can serve reverts.

Migration down starts at `000039`: it takes an exclusive lock and proves the
claim table is empty. It never maps `ARMED` backward; rollback with any live or
historical claim is refused because erasing that phase would erase the fact
that automatic retry is forbidden. `000038` then takes its own exclusive lock
and refuses to remove the PostgreSQL rollout birth certificate while its
singleton row exists. Because that row is created before the first Redis
witness, migration down is available only before initialization; after
initialization the rollout is forward-only unless a future product migration
explicitly supersedes and archives the birth certificate. Only after both
guards pass may the published `000037` down remove its rollout/generation
columns and the unchanged `000036` down remove the claim table.
Removing any fence would silently erase the only barrier understood by
bridge/final pods. This is not a rolling rollback step.

## Client and operator surface

- Completed replay: HTTP 201 with `X-Idempotency-Replayed: true` and the original
  reserved reverse ID.
- Active same-origin attempt: duplicate-idempotency conflict; no money movement.
- Ambiguous or post-movement failure: HTTP 503 with stable code `0501`; the
  code is the revert client/operator reconciliation signal even when production
  5xx detail is redacted by the canonical error renderer.
- Commit/cancel outcome temporarily unreadable after an ambiguous Redis
  response: HTTP 503 with stable code `0503`; the opposite terminal remains
  fenced and no second movement is attempted.
- Bridge/final rollout precondition missing: HTTP 503 with stable code `0502`;
  the pod is also unready, and no balance mutation is attempted.
- APPROVED update while the freeze is active: HTTP 422 with stable code `0008`.
  PENDING updates remain available; `finalized` restores APPROVED updates.
- Every persisted reverse must have `parent_transaction_id` equal to its claim's
  origin. Any mismatch is reconciliation-required and is never replayed.
