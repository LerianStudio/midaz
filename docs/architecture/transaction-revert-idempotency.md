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
Revert eligibility, child discovery, claim reads, and replay resolution are all
served from PostgreSQL primary; replica lag is not allowed to decide whether
money can move.

## Barriers and acquisition order

The bridge release acquires barriers in this order:

1. PostgreSQL primary claim for organization + ledger + origin.
2. Legacy Redis payload-hash barrier, shared with old pods.
3. Redis origin barrier, shared with bridge and final pods.
4. Atomic Redis balance Lua, which also writes the transaction backup outcome.

The money mutation is unreachable until all barriers required by the active
release are held. The claim reserves the ID used by both the balance outcome and
the persisted reverse, so recovery cannot mint a replacement ID.

The two Redis idempotency keys intentionally have different Cluster hash tags.
They are acquired by separate single-key `SET NX` operations; they are never
combined in one Lua invocation or multi-key operation. Combining them would
produce `CROSSSLOT`. The balance Lua uses only the existing `{transactions}`
slot for its queue, transaction outcome, schedule, and balance keys.

## Claim states and recovery

| State | Meaning | Retry behavior |
|---|---|---|
| `CLAIMED` | Reverse ID reserved; no durable proof of completion yet | Same origin cannot create another reverse |
| `MUTATED` | Balance Lua returned success | Reconcile or finish persistence with the reserved ID |
| `COMPLETED` | Reverse transaction and operations are durable | Return the exact persisted reverse as an idempotent replay |
| `RECONCILIATION_REQUIRED` | Lua result was ambiguous or persistence failed after movement | Return error `0501`; never release the claim or Redis fences |

The balance Lua changes balances and writes the transaction backup marker in
one atomic operation. This closes the crash window between Redis movement and a
separate outcome write: if the client loses the Lua response, reconciliation can
inspect the reserved reverse ID's backup marker. A bridge consumer also adopts
old-pod backup records into the PostgreSQL claim before marking them complete.
An existing claim is never overwritten when its reserved reverse ID differs
from a backup or persisted child.

## Failure policy

| Failure point | Proof available | Action |
|---|---|---|
| Before balance Lua dispatch | No movement was attempted | Delete the origin and legacy Redis barriers acquired by this request; release the still-`CLAIMED` PostgreSQL claim; retry is allowed |
| Lua-declared validation rejection | Lua rolled back the complete batch | Release all barriers and the claim; retry is allowed after the business condition changes |
| Transport error after Lua dispatch | Commit outcome is ambiguous | Preserve PostgreSQL, legacy, origin, backup, and tracer-reservation fences; mark reconciliation required |
| Error after Lua success | Funds moved and backup outcome exists | Preserve every fence; mark reconciliation required |
| Crash before PostgreSQL transaction persistence | Reserved ID and Redis backup remain | Backup consumer persists exactly that reverse and completes the claim |

Only the first two rows authorize release. Redis timeouts, connection resets,
context cancellation after dispatch, and PostgreSQL failures after movement do
not authorize a second mutation.

## Rollout: old to bridge to final

### Release A: bridge

1. Apply migration `000036_create_revert_claim` before starting bridge pods.
2. Deploy with `REVERT_IDEMPOTENCY_MODE=bridge`.
3. Keep bridge mode while old pods exist. Both generations serialize on the
   legacy payload-hash barrier; bridge/final generations additionally serialize
   by origin through PostgreSQL and the origin Redis barrier.
4. Stop routing new work to old pods, terminate every old pod, and drain all
   in-flight old requests.
5. Drain or reconcile every transaction backup created by old pods. Bridge
   consumers adopt old reverse IDs into claims. Do not advance while a backup
   can still represent an unclaimed balance movement.

Bridge never returns a cached reverse whose parent differs from the requested
origin. A legacy collision is a conflict, not a replay. The bridge overlap must
remain in place until the old generation and its in-flight work are proven zero;
changing only the environment flag while old code still serves traffic is not a
rollout.

### Release B: final

After all Release A exit criteria are satisfied, set
`REVERT_IDEMPOTENCY_MODE=final`. Final mode uses the PostgreSQL claim and origin
Redis barrier as authorities. It still reads the legacy barrier as a rollout
fence and can adopt an exact same-origin old result, but a value belonging to a
different origin is never returned and no longer blocks the independent origin.

Final-to-final and bridge-to-final races share the PostgreSQL claim and origin
barrier. Multiple callers may receive the same completed replay; only one
reverse ID, persisted transaction, and balance movement can exist.

## Rollback and migration down

Switching `final` back to `bridge` is safe while the schema remains present.
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
  code is the client/operator reconciliation signal even when production 5xx
  detail is redacted by the canonical error renderer.
- Every persisted reverse must have `parent_transaction_id` equal to its claim's
  origin. Any mismatch is reconciliation-required and is never replayed.
