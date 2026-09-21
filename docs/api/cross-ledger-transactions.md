# Cross-ledger transactions

`POST /v2/transactions/direct` accepts debit and credit legs from different
ledgers when every participating ledger has `settings.crossLedger.enabled=true`.
The request uses the existing v2 leg-level `organizationId` and `ledgerId`
fields; there is no separate endpoint.

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

- Only the v2 `direct` action supports mixed leg scopes. Cross-ledger `hold`,
  `block`, `unblock`, lifecycle actions, and `/v1` remain unsupported.
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

This milestone does not provide exchange rates, cross-ledger hold/commit/cancel,
cross-ledger revert, or an aggregate group endpoint.
