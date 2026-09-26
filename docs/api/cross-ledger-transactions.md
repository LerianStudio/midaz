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
- Participants that enable `accounting.validateRoutes` are supported when the
  group stays in one organization; see [Accounting routes](#accounting-routes).
  A group that spans more than one organization while any participant validates
  routes returns `0251` (HTTP 422), because accounting routes belong to one
  organization. Cross-organization groups without route validation are accepted.
- Fees, Tracer, skip permissions, balance rules, and limits are evaluated with
  each part's own ledger settings.
- One idempotency key protects the full request. An identical replay returns the
  original group and sets `X-Idempotency-Replayed: true`; changing any leg while
  reusing the key conflicts with `0084`.
- Cross-tenant requests are not possible: tenant scope still comes from the
  authenticated connection.

## Accounting routes

Accounting routes belong to the organization, so one transaction route (the
request's `routeId`) covers every part of a group. Each client leg names its
operation route (`operationRouteId`, per leg or inherited from the request). The
client never names a route for the bridge legs:

- In a participant that validates routes, the synthetic bridge leg takes the
  bridge route of the transaction route: the one bidirectional operation route
  carrying a `crossLedger` accounting entry, and no other entry. Its rubric
  follows the posted direction: `crossLedger.credit` where value leaves the
  ledger, `crossLedger.debit` where it arrives (swapped in a revert). A
  transaction route without such an operation route returns `0255` (HTTP 422);
  more than one, or a bridge route with other entries, returns `0256`.
- Participants that do not validate routes keep unrouted bridge legs.

Validation runs in two steps for every operation that changes balances:

- **Per part**, in each ledger that validates routes, exactly as for a single
  transaction except the route count: every client leg names a route of the
  phase's template on a side it accepts (`0117`), directions and account rules
  hold, and overdraft and bridge legs carry their rubrics. Bridge legs never
  count in a template.
- **Over the group**, once per phase: the client legs of every part together
  must use exactly the phase's template (`0116`), and a bidirectional route used
  on both sides needs a debit and a credit somewhere in the group (`0151`). The
  union counts every client leg that names a route, including legs in parts
  whose ledger does not validate routes; a leg without a route in such a part
  counts for nothing. So a transfer from a validating ledger to one that does not
  validate still names the destination route to complete a
  `{source, destination}` template.

Each phase uses its own template:

| Phase | Parts in the union | Template |
| --- | --- | --- |
| direct | every part | `direct` |
| hold | the origins created now, plus the destination parts persisted in the intent (by route ID only; their account rules are checked at commit) | `hold` for sources, `commit` for destinations |
| commit | the origin transitions and the destinations created now | `commit` (destinations validate and take rubrics as `commit`, although they post as direct transactions) |
| cancel | the origins | `cancel`, source side only, with no group-wide rule |
| revert | every reversal | `revert`; every routed operation must be bidirectional |

A transaction route for a full lifecycle therefore gives its source routes
`hold`, `commit`, `cancel` and `direct` entries as needed, its destination routes
`commit` (for holds) and `direct` entries, and bidirectional client routes with
`revert` entries when groups are reverted, plus the `crossLedger` bridge route.

Settings are read again at every step. A group held while a ledger did not
validate routes, and committed after it started to, is validated at commit, as a
single-ledger pending transaction is. The same holds for `0251`: a
cross-organization group is refused at commit or cancel once any participant
validates routes.

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

One internal idempotency claim protects the whole group regardless of which
pending origin is addressed. Its scope is the lexicographically smallest
organization/ledger pair among the participating parts; the engine receipt
remains in the execution's primary scope.

Origin fees are frozen into the hold. Destination fees are evaluated when the
commit runs, so a package change between hold and commit can affect destination
parts. Origin Tracer reservations are confirmed on commit and released on
cancel; destination reservations are created only for commit. `/v1` commit,
cancel, or revert cannot return a group and rejects a group member with `0252`
(HTTP 422).

## Member resolution

Commit, cancel, and revert answer the same way right after the create as they
do once the members are persisted, including when the ledger projects
transactions asynchronously. Every grouped accounting execution records the
transactions it applied, each with its organization and ledger, and the
engine index of each of those transactions points at its latest execution.
The lifecycle reads that list from the addressed member and resolves each
listed transaction through the engine index, falling back to the PostgreSQL
primary for a member that is no longer indexed:

- after a hold, an origin lists every origin, which commit and cancel need;
- after a direct create or a commit, any member lists the whole approved
  group, which revert needs.

When the addressed member has no index, or its execution recorded no list, the
members are read from the PostgreSQL primary by `groupId`. A listed member
found in neither source returns `0253` (HTTP 422). Roles still come from the
persisted intent, never from the list.

## Revert

A v2 revert addressed to any member loads every member of the group (see
[Member resolution](#member-resolution)) inside the authenticated tenant,
validates each member in its own organization and ledger, and submits every
reversal in one accounting-engine invocation. Any ineligible or missing member refuses the operation before
accounting. Reversals are returned in the reverse order of the original parts.

The HTTP 201 response uses the grouped create shape. `groupId` identifies the
new reversal group, `revertedGroupId` identifies the original group, and every
entry in `transactions[]` has a `parentTransactionId` pointing to its matching
origin. The reversal group has a new identifier so reads by group never mix an
original movement with its reversal.

Fees are not recalculated because the original fee legs are reversed as
persisted. Tracer capacity is reserved independently for every reversal part.
The account-block exception supplied on the request applies only to the member
named by the path. Reverts addressed to different members of the same group
share one claim. A concurrent duplicate returns `0084` (HTTP 409) or replays
the same reversal group; a completed second revert returns `0087` (HTTP 409); an incomplete group
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
