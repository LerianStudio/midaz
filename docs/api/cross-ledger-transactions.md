# Cross-ledger transactions

`POST /v2/transactions/direct` and `POST /v2/transactions/hold` accept debit
and credit legs from different ledgers when every participating ledger has
`settings.crossLedger.enabled=true`. The request uses the existing v2 leg-level
`organizationId` and `ledgerId` fields; there is no separate endpoint.

`POST /v2/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert`
also understands these groups. When the selected transaction has a `groupId`,
the operation reverses the complete group rather than one ledger-local part.

The ledger resolves `share` and `remaining` expressions once, groups the legs by
ledger, and creates one balanced transaction per participating ledger. Each
per-ledger transaction uses that ledger's `@external/<asset>` account for only
the net amount crossing its boundary. If a ledger appears on both sides, its
original internal legs remain together and only their difference reaches the
bridge.

Direct parts execute in one accounting-engine invocation and therefore succeed
or fail together. A cross-ledger hold initially executes only the origin parts;
its later commit executes all origin transitions and destination creates in one
invocation. Successful HTTP 201 responses contain `groupId` and an ordered
`transactions[]` array. The same `groupId` is persisted on every materialized
part and included in transaction lifecycle events.

List transactions belonging to a group with
`GET /v2/organizations/{organization_id}/ledgers/{ledger_id}/transactions?groupId={uuid}`.
The listing remains ledger-scoped; callers query each participating ledger they
are authorized to read.

## Gates and limits

- Only the v2 `direct` and `hold` actions create mixed-ledger groups. Cross-ledger
  `block` and `unblock` remain unsupported, and `POST /v2/transactions/batch`
  still rejects a batch item whose hold itself spans ledgers.
- Every part uses the same request asset. Mixed assets return `0250` (HTTP 422).
- A participating ledger with cross-ledger disabled returns `0249` (HTTP 422).
- Synthetic bridge legs have no accounting route. If any participant enables
  `accounting.validateRoutes`, the request returns `0251` (HTTP 422).
- Fees, Tracer, skip permissions, balance rules, and limits are evaluated with
  each part's own ledger settings.
- One idempotency key protects the full request. An identical replay returns the
  original group and sets `X-Idempotency-Replayed: true`; changing any leg while
  reusing the key conflicts with `0084`.
- Cross-tenant requests are not possible: tenant scope still comes from the
  authenticated connection.

## Hold, commit, and cancel

A cross-ledger hold persists the normalized group intent, including destination
parts, before accounting. It creates only origin transactions: each origin
debits its source into that ledger's `@external/<asset>` bridge and remains
PENDING. No destination transaction or destination balance change exists until
commit. The response contains only those PENDING origins.

Commit or cancel may be addressed to any pending origin through the existing v2
lifecycle routes. Commit changes every origin from PENDING to APPROVED and
creates every destination as APPROVED in the same accounting-engine invocation.
Cancel changes every origin to CANCELED and creates no destination. Both return
the group envelope under the original `groupId`; a singular pending transaction
retains the historical singular response shape.

The group record is compared from PENDING to the terminal status after member
completion. Internal keys `group-commit:{groupId}` and
`group-cancel:{groupId}` fence publication and retain an uncertain result for
reconciliation; the engine is never retried after an unknown outcome. A second
terminal action returns `0254` (HTTP 422) and identifies the current group
status. Missing or inconsistent intent/members return `0253` (HTTP 422).

Origin fees are frozen into the hold. Destination fees are evaluated when the
commit runs, so a package change between hold and commit can affect destination
parts. Origin Tracer reservations are confirmed on commit and released on
cancel; destination reservations are created only for commit. `/v1` commit,
cancel, or revert cannot return a group and rejects a group member with `0252`
(HTTP 422).

## Revert

A v2 revert addressed to any member loads every transaction with the same
`groupId` inside the authenticated tenant, validates each member in its own
organization and ledger, and submits every reversal in one accounting-engine
invocation. Any ineligible or missing member refuses the operation before
accounting. Reversals are returned in the reverse order of the original parts.

The HTTP 201 response uses the grouped create shape. `groupId` identifies the
new reversal group, `revertedGroupId` identifies the original group, and every
entry in `transactions[]` has a `parentTransactionId` pointing to its matching
origin. The reversal group has a new identifier so reads by group never mix an
original movement with its reversal.

Fees are not recalculated because the original fee legs are reversed as
persisted. Tracer capacity is reserved independently for every reversal part.
The account-block exception supplied on the request applies only to the member
named by the path. A completed second revert returns `0087`; an incomplete group
returns `0253` (HTTP 422). `/v1` cannot return a group response and rejects a
group member with `0252` (HTTP 422).

Authorization is evaluated by the existing route against the organization and
ledger in the path. As with cross-ledger create, the resulting atomic operation
may include other enabled ledgers or organizations in the same tenant.

This contract does not provide exchange rates or an aggregate group endpoint.

## Observability

**Events.** Every part keeps its per-ledger `transaction.*` event, now carrying
`groupRole` (`origin` or `destination`) beside `groupId`. On top of those, each
group operation publishes one `transaction_group.*` fact on the ledger stream:
`posted` for a direct create, `committed` and `canceled` for the lifecycle, and
`reverted` for a whole-group revert (its `groupId` is the new group and
`revertedGroupId` the reversed one). A hold and a replayed request publish no
group fact. The payload lists the materialized parts with ledger, role, and
status; see `docs/streaming/ledger-events.md`.

**Traces.** The coordinators open `command.create_cross_ledger_transaction_v2`,
`command.create_cross_ledger_hold_v2`, `command.transition_cross_ledger_group_v2`
(commit and cancel) and `command.revert_cross_ledger_group`, each carrying
`app.request.action`, `app.request.part_count`, `app.request.ledger_count`, and
the group identifiers (`app.request.group_id` for the group acted on,
`app.response.group_id` for a group the operation creates). A business rejection
keeps the span green and is logged once, at Warn, by the coordinator.

**Metrics.** `domain_operations_total` / `domain_operation_duration_ms` cover
`create_cross_ledger_transaction`, `create_cross_ledger_hold`,
`commit_cross_ledger_group`, `cancel_cross_ledger_group`, and
`revert_cross_ledger_group`. `atomic_transaction_batches_total` and
`atomic_transaction_batch_duration_ms` carry `scope=single|cross_ledger`.
`cross_ledger_group_ledgers` observes distinct ledgers per applied group
operation, by action. No metric carries a group, ledger, or account identifier.

**Group reconciliation.** The group row only labels its members; the members are
the truth. Once per recovery cycle, the leader pod reads PENDING groups older than
five minutes whose members have also been at rest that long. When every part is
APPROVED, or every origin is CANCELED, it moves the row to that status and
publishes the group fact the coordinator could not. A group still held is left
alone. Approved origins whose destinations are not projected yet are treated the
same way until a day has passed, since that is what a commit looks like while
its destination projection waits in recovery. An intent that never produced a
member is deleted only after a day, well beyond any deferred projection, and
only if no member row exists at the moment of the delete. Members that agree on no single state are logged
at Error and counted as `inconsistent`, and are never written: reconciliation
never moves a balance. Results are counted in
`cross_ledger_group_reconcile_total{result}`.
