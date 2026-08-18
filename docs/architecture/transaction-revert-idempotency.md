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
After the rollout marker becomes `active`, both phase zero and bridge persist
the exact legacy payload-hash fence key and its reserved-reverse owner before
that fence is acquired. Recovery uses these immutable values and never
recalculates the key or owner from mutable transaction fields.
Revert eligibility, child discovery, claim reads, and replay resolution are all
served from PostgreSQL primary; replica lag is not allowed to decide whether
money can move.

## Barriers and acquisition order

Marker-active phase zero and bridge acquire barriers in this order:

1. Short-lived Redis economic execution attempt plus its reserved-reverse owner
   in the shared transaction slot.
2. PostgreSQL primary claim for organization + ledger + origin, including the
   exact legacy key and reserved-reverse owner.
3. Legacy Redis payload-hash barrier, shared with pre-activation phase-zero
   requests.
4. Redis origin barrier plus its reserved-reverse owner companion, shared with
   bridge and final pods.
5. Atomic Redis balance Lua, which checks the exact attempt owner before its
   first write and writes an immutable economic outcome in the same command.

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
one slot. While a marker-active phase-zero or bridge request is in flight, both
legacy keys are persistent: a pre-activation request cannot regain the barrier
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
| `CLAIMED` | Reverse ID reserved; no durable proof of completion yet | Same origin cannot create another reverse |
| `RECOVERING` | One process holds a 30-second cleanup lease for a proven pre-Lua seed | Every other caller remains fenced; a crashed cleanup owner is re-electable after its lease expires |
| `MUTATED` | Balance Lua returned success | Reconcile or finish persistence with the reserved ID |
| `COMPLETED` | Reverse transaction and operations are durable | Return the exact persisted reverse; a losing retry cannot downgrade this terminal state |
| `RECONCILIATION_REQUIRED` | Lua result was ambiguous or persistence failed after movement | Return error `0501`; never release the claim or Redis fences |

The balance Lua changes balances, updates the transaction backup, and writes a
separate immutable economic outcome in one atomic command. The shared outcome
shape is `{identity, outcome, owner, before, after}`. `outcome` is `COMMITTED`
or `ABORTED`; the same identity and outcome replays the exact original balance
snapshots, while a different identity or opposite terminal outcome conflicts
before any movement. Commit, cancel, and revert use this same primitive. It is
also the boundary the later Ledger-to-Tracer dispatcher will consume; this P0
does not wire that dispatcher.

The immutable outcome survives the five-minute request lease and asynchronous
Rabbit/backup delays, crosses the durable-write payload, and is deleted only
after the transaction, every operation, and any revert claim terminal state are
durable. After Lua, the queue envelope is never replaced: operation IDs and the
action are single-assigned by a same-slot CAS that verifies the exact transaction,
owner, immutable outcome, and Lua-authored `balancesAfter`. The CAS returns the
stored operation set whether this consumer won or lost, and every PostgreSQL
writer uses that returned set rather than its locally generated candidate IDs.
Two consumers and a restart after a lost CAS response therefore converge on
one durable operation identity set.

Synchronous writes, the individual Rabbit consumer (including async publish
fallback), and the default bulk Rabbit consumer all enter one terminal
persistence handoff. That handoff performs the same ordered proof every time:

1. Read the transaction and its complete operation set from PostgreSQL primary.
2. Verify transaction ID, origin, terminal status, exact operation-ID multiset,
   and every operation's complete economic effect against the immutable payload:
   balance ID/key, direction, type, asset, amount, balance-affected flag, and
   before/after available, on-hold, version, and overdraft values.
3. Complete the durable revert claim.
4. Publish or verify the exact origin and persisted H1 replays.
5. Atomically clean the owner/outcome-matched Redis backup and outcome.

No consumer owns a partial version of this sequence. Bulk duplicates,
individual redelivery, fallback after publish failure, a crash after the
PostgreSQL commit, and a lost Rabbit acknowledgement all re-enter the same
handoff. Cleanup is another
same-slot owner/outcome-checked Lua command that compares the complete
operation-ID multiset and removes the backup and outcome atomically. Commit and
cancel prove every operation in their terminal attempt while preserving the
older PENDING hold operations as durable history; a reverse has no older
history and therefore requires exact equality. A lost cleanup response is an
exact idempotent replay; a mismatched owner or operation set removes nothing. A transport timeout is
therefore never interpreted as proof that funds did not move. TTL expiration
can retire only a transient execution or recovery lease. It never erases an
immutable outcome, durable claim, or persistent legacy/origin fence, and it is
never evidence that movement did not happen. The queue seed
exists before Lua dispatch, but it has no `balancesAfter`; the consumer never
persists that seed as a completed reverse. While the winner is live, its
owned execution attempt prevents a retry from declaring that seed abandoned.
If the exact attempt is absent, the balance Lua itself prevents the old winner
from moving if it resumes because it checks the exact attempt key and owner
inside the script. Absence is considered only together with the immutable
outcome and exact reserved seed facts; elapsed time contributes no safety
proof. A retry then verifies either that the reserved backup is absent
(crash before seed) or that the seed carries the exact claim origin,
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

Transaction-backup deletion has no key-only API. Every economic envelope
removal belongs to one of four explicit proof classes:

| Cleanup class | Atomic deletion proof |
|---|---|
| Proven pre-movement failure | Status, attempt owner, expected outcome, and absence of `balancesAfter` all match in one Lua command |
| Outcome-backed durable persistence | Transaction identity, immutable owner/outcome, terminal backup, and complete operation-ID multiset match; backup and outcome are removed together |
| Drained phase-zero durable persistence | Reverse ID, origin ID, terminal status, and the complete persisted operation-ID multiset match; an outcome-backed envelope is rejected |
| Durable quarantine | The raw bytes copied into PostgreSQL still exactly equal the Redis field; a successor value under the same key is preserved |

The retry-attempt counter is non-economic bookkeeping and is the only direct Go
hash deletion. A permanent structural test inventories these classes, rejects
any key-only backup deletion API, and fails when a new backup `HDEL` appears
without an explicit atomic proof classification.

A bridge consumer also adopts parent-less bridge backup records only when a
PostgreSQL claim already maps their reverse ID to an origin. Phase-zero backups
created before marker activation have no pre-movement claim but do carry the
exact origin; the consumer accepts one only when the same backup contains Lua's
atomic `balancesAfter` outcome, then creates and completes the claim after the
child and every operation are durable. That adoption derives the released
payload hash from the immutable backup input and persists its exact legacy
fence key with a deliberately null owner classification; later retries never
derive it from the mutable origin row. Marker-active phase-zero and
bridge/final backups carry both the origin and their pre-movement claim, and the
consumer verifies the pair. A truly old backup with neither a
claim nor an explicit origin has no trustworthy parent and is quarantined for
operator reconciliation; the consumer never guesses an origin from amount,
accounts, or other economic payload fields. An existing claim is never
overwritten when its reserved reverse ID differs from a backup or persisted
child.

Before marker activation, the released phase-zero algorithm creates only a
finite-TTL, unowned H1. After PostgreSQL primary proves an adopted reverse and
every operation durable, the terminal handoff may replace that H1 only through
a same-slot CAS that requires the main value still be empty and the owner
companion still be absent. A foreign replay or any owner makes the CAS fail
closed. Once the marker is `active`, phase zero changes to the durable protocol:
the claim records origin, exact H1 key, and owner before a persistent H1 pair is
created. Terminal redelivery chooses owned versus unowned CAS solely from that
durable owner classification, never from process memory or which request first
observed the claim. Recovery never uses a blind `SET`.

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
of recomputing the audit trail. A completed old-pod
reverse has no claim-time backup to compare; it is adopted only after its
persisted child and operations are present on primary and the backup is absent.
If a drained phase-zero backup remains after those PostgreSQL facts and the
claim are terminal, final mode removes it only through an exact Lua proof of the
reserved reverse ID, parent origin, phase-zero status, and full operation-ID
set. That compatibility cleanup never touches an outcome-backed envelope.

## Failure policy

| Failure point | Proof available | Action |
|---|---|---|
| Before queue seed or balance Lua dispatch, with a confirmed failure response | No movement was attempted and no seed write is ambiguous | Prove removal of the reserved backup seed, owner-release the origin and legacy barriers, then release the still-`CLAIMED` PostgreSQL claim last; retry is allowed |
| Queue seed response is lost | The seed may exist, but movement was not dispatched | Preserve claim, execution attempt, origin, legacy, and possible seed; mark reconciliation required |
| Lua-declared validation rejection | Lua rolled back the complete batch | Release all barriers and the claim; retry is allowed after the business condition changes |
| Transport error after Lua dispatch | Commit outcome is ambiguous to the caller | Read the immutable Redis outcome; exact outcome recovers, unreadable outcome preserves every barrier and requires reconciliation |
| Error after Lua success | Funds moved and immutable outcome exists | Preserve every fence; mark reconciliation required |
| Crash after Lua and before PostgreSQL persistence | Reserved ID, immutable economic outcome, and atomic `balancesAfter` backup remain | Backup consumer persists exactly that reverse and completes the claim |
| Lost response after operation-ID enrichment | PostgreSQL status may already be terminal; the exact post-Lua envelope and outcome remain | Preserve the request fence; the delayed consumer persists the same operation IDs, then atomically removes backup and outcome |
| Crash after PostgreSQL persistence and before origin replay completion | Exact child and operations exist on PostgreSQL primary; owned origin fence remains | Retry completes or safely rematerializes the exact reserved replay, removes the owner, and returns it |
| Crash after phase-zero PostgreSQL persistence and before H1 replay publication | Exact child, complete operation set, and terminal claim exist; H1 is still the unowned empty phase-zero fence | Retry replaces only that empty/no-owner H1 with the exact reserved replay; a foreign value or owner is preserved and fails closed |
| Crash after bridge child persistence and before H1 completion | Durable claim retains the original H1 key; child and every operation exist on primary | Final adoption completes only the persisted H1 key, marks the claim terminal, and owner-checks outcome cleanup; it never recalculates H1 from the origin |
| Final adoption sees a foreign H1 collision | The durable claim and child prove this origin; the legacy key explicitly belongs to another owner or replay | Preserve the foreign H1 unchanged, finish the origin-scoped replay, and clean only this reverse's exact outcome/backup |
| Crash after a phase-zero child is durable but before backup cleanup | Child, all operations, and completed claim exist on PostgreSQL primary; legacy backup has no owner/outcome envelope | Compare reverse, parent, status, and every operation ID in one Lua command, then remove only that exact phase-zero backup |
| Crash before queue seed | Durable claim names the exact origin, reverse, H1 key, and owner; the reserved backup, execution attempt, and immutable outcome are absent on Redis primary | Elect one `RECOVERING` owner, owner-release the exact barriers, release PostgreSQL last, and retry; safety comes from Lua requiring the now-absent exact attempt, not from elapsed time |
| Crash before Lua dispatch | Valid exact-origin queue seed exists without `balancesAfter` or immutable outcome, and the exact execution attempt is absent | Elect one `RECOVERING` owner, clear Redis barriers/seed, release PostgreSQL last, and retry; the old winner is rejected inside Lua if it resumes |
| Pre-movement cleanup races a terminal Lua envelope | Status/owner/outcome no longer match the exact seed selected for cleanup | Atomic cleanup removes nothing; preserve all barriers and require reconciliation |
| Crash while cleaning `RECOVERING` | PostgreSQL retains the cleanup state and timestamp | Re-elect after 30 seconds and resume idempotent cleanup; PostgreSQL remains the last record released |

Only proof that dispatch cannot happen (the exact execution attempt and outcome
are absent, together with either an absent reserved backup or the valid
exact-origin seed-only crash case) or a Lua-declared rollback
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

## Rollout: old to phase zero to bridge to final

The legacy payload hash includes mutable transaction fields. Allowing an
APPROVED origin to change while old and bridge algorithms coexist would let the
same origin acquire two different legacy barriers. The rollout therefore uses
one deployment-wide Redis state at
`rollout:{transaction-revert-rollout:v1}:state`:

| Value | Phase-zero readiness/revert | APPROVED updates | Bridge readiness/revert | Final readiness/revert |
|---|---|---|---|---|
| absent | Allowed | Allowed on phase zero | Rejected with `0502` | Rejected with `0502` |
| `active` | Allowed | Rejected with `0008` on phase zero, bridge, and final | Allowed | Rejected with `0502` |
| `phase-zero-drained` | Rejected with `0502` | Rejected with `0008` on bridge and final | Allowed | Allowed |
| `finalized` | Rejected with `0502` | Allowed on final | Rejected with `0502` | Allowed |

PENDING transactions remain mutable in every state because they are not
eligible for revert. The marker is global rather than tenant-prefixed, so every
pod and every tenant observes one barrier. Three persistent in-flight sets use
the same Redis Cluster hash tag: APPROVED updates, phase-zero reverts, and
bridge reverts. Request admission reads the marker and adds a unique
`pod-hostname:request-uuid` token in one Lua command; the token is removed only
after the PostgreSQL write or revert request returns. Marker transitions run in
the same slot and refuse to advance while the generation they retire has any
token.

Marker transitions are executed by ledger startup through
`REVERT_ROLLOUT_TARGET`, not by an operator writing Redis directly. Accepted
targets are `active`, `phase-zero-drained`, and `finalized`; an invalid target,
an out-of-order transition, or a transition with an in-flight retiring request
fails startup. The deployment controller still proves every individual pod's
readiness capability before changing the target, because Redis cannot infer
which application generation a cluster scheduler is still running.

The tokens have no TTL. A crashed request may block rollout availability, but a
token can never expire underneath a still-running money-path request and create
a false drain proof. A leaked token may be removed only after the owning pod is
terminated and its PostgreSQL connections are drained. An APPROVED update
cannot therefore be admitted before activation and persist afterward:
`absent -> active` is serialized after every admitted update token. Phase-zero
and bridge reverts are similarly serialized against the transition that retires
their generation. All marker and lease operations are same-slot; none couples
a tenant's legacy and origin barriers or assumes multi-slot Lua.

If the admission Lua commits but its response is lost, the handler stops before
the PostgreSQL or balance mutation and removes only its own unique token. A
different request's token is never removed. If that cleanup is itself
unavailable or ambiguous, the request returns an error; any surviving token
keeps the phase transition blocked until the terminated request is reconciled.

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

1. Apply migration `000036_create_revert_claim`, then deploy every pod with
   `REVERT_IDEMPOTENCY_MODE=legacy`. The schema must precede the binary because
   its backup consumer can inspect claims. While the marker is absent this
   release retains the old payload-scoped revert algorithm with a finite H1;
   it also adds the shared APPROVED update gate and the
   `revert_rollout_barrier` readiness check.
2. The deployment controller must verify every individual pod's `/readyz`
   response contains `checks.revert_rollout_barrier.status=up`, not merely an
   aggregate service health result. A pre-phase-zero pod does not expose that
   check and therefore cannot satisfy the gate.
3. After the deployment controller proves zero pre-phase-zero pods, deploy
   `REVERT_ROLLOUT_TARGET=active`. Ledger startup atomically changes the absent
   marker to `active`. Activation refuses while any APPROVED
   update admitted by phase zero is still executing, so success is the durable
   drain proof for mutable origin writes. The same phase-zero binary then
   switches reverts to the durable claim + owned-H1 protocol before bridge is
   introduced. Activation is idempotent only while
   the state is absent or already active; a `finalized` rollout cannot be
   reopened.
4. Prove an APPROVED update is rejected on every pod before any bridge pod is
   admitted. The bridge's readiness and per-revert preflight independently
   enforce the same state, so a missing, corrupt, unreadable, or inactive marker
   cannot become a money-path request.

Before activation, phase zero keeps valid same-origin replay behavior but
verifies the cached reverse parent before returning it. After activation,
phase zero reserves one reverse per origin in PostgreSQL before creating its
persistent H1. Economically identical origins may still share the released
payload-hash slot during coexistence; that collision is a conflict, never a
response carrying another origin's reverse.

Activation before step 2 is unsafe: code older than phase zero does not honor
the marker. The readiness capability check makes that ordering machine
verifiable; it is not a human assertion hidden in a runbook.

### Release A: bridge

1. Deploy with `REVERT_IDEMPOTENCY_MODE=bridge` and
   `REVERT_ROLLOUT_TARGET=active`. A pod remains unready and every
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
4. Drain or reconcile every transaction backup created by phase-zero pods.
   A phase-zero backup with an explicit parent and atomic balance outcome is
   persisted under that exact parent and adopts a durable claim. A parent-less
   backup is accepted only when its reverse ID already has a durable claim. A
   backup with no trustworthy origin is quarantined. An explicit-parent seed
   without an atomic outcome remains untouched before the drained marker; after
   the marker proves no phase-zero request can resume, the consumer first
   verifies no claim exists for the origin, then deletes only an empty H1 with
   no owner, and only afterward deletes the exact seed. A crash between those
   steps leaves the seed for idempotent redelivery rather than orphaning H1. Do
   not advance while any backup can
   represent an unclaimed balance movement.

Bridge never returns a cached reverse whose parent differs from the requested
origin. A legacy collision is a conflict, not a replay. Changing only the mode
while an older generation still serves traffic is not a rollout.

### Release B: final and unfreeze

After all Release A exit criteria are satisfied, deploy
`REVERT_IDEMPOTENCY_MODE=final` and
`REVERT_ROLLOUT_TARGET=phase-zero-drained` while the marker remains
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

The down migration intentionally refuses to drop the table while **any row**
exists, including completed rows. Dropping it would silently remove the only
barrier understood by bridge/final pods. The operator must first stop all
bridge/final pods, archive the claims, independently prove that every reverse is
durable and visible where the rollback binary reads it, explicitly clear the
table, and only then run the down migration. It is not a routine rolling
rollback step.

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
