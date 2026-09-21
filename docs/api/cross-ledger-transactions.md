# Cross-ledger transactions

`POST /v2/transactions/direct` accepts debit and credit legs from different
ledgers when every participating ledger has `settings.crossLedger.enabled=true`.
The request uses the existing v2 leg-level `organizationId` and `ledgerId`
fields; there is no separate endpoint.

`POST /v2/organizations/{organization_id}/ledgers/{ledger_id}/transactions/{transaction_id}/revert`
also understands these groups. When the selected transaction has a `groupId`,
the operation reverses the complete group rather than one ledger-local part.

The ledger resolves `share` and `remaining` expressions once, groups the legs by
ledger, and creates one balanced transaction per participating ledger. Each
per-ledger transaction uses that ledger's `@external/<asset>` account for only
the net amount crossing its boundary. If a ledger appears on both sides, its
original internal legs remain together and only their difference reaches the
bridge.

All parts execute in one accounting-engine invocation and therefore succeed or
fail together. The successful HTTP 201 response contains `groupId` and an
ordered `transactions[]` array. The same `groupId` is persisted on every part
and included in transaction lifecycle events.

List transactions belonging to a group with
`GET /v2/organizations/{organization_id}/ledgers/{ledger_id}/transactions?groupId={uuid}`.
The listing remains ledger-scoped; callers query each participating ledger they
are authorized to read.

## Gates and limits

- Only the v2 `direct` action creates mixed-ledger groups. Cross-ledger `hold`,
  `block`, `unblock`, commit, and cancel remain unsupported. The existing v2
  revert route is the sole grouped lifecycle operation.
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

This milestone does not provide exchange rates, cross-ledger hold/commit/cancel,
or an aggregate group endpoint.
