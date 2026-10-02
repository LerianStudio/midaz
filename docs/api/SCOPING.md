# API Scoping Conventions (R22 — reversed, now exception-free)

The unified ledger binary (`:3002`) scopes the organization a request applies to through the
**URL path hierarchy** — on every surface. Ledger, routing, CRM (holders / instruments), the
holder-account composition route, and fees/billing are all path-scoped on the organization.
`X-Organization-Id` is no longer part of any API contract in this binary. Fees and billing are
additionally served **ledger-scoped** on the independent `/v2` contract; both surfaces are live.

> **R22 is reversed.** This document previously locked CRM to header-based organization scoping
> (`X-Organization-Id`) as an intentional, documented inconsistency. That convention is gone. CRM
> and composition moved to path-based org scoping pre-GA (2026-06-06), and fees/billing followed
> (2026-06-07) — both as **clean breaks with no dual-routing**: there is no header fallback and no
> transitional period. The substance below is the inverse of the original R22 record.

## The path-scoping convention

Every organization-scoped endpoint carries the organization (and, where a real ledger account is
involved, the ledger) in the URL path hierarchy:

```
GET  /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json

POST /v1/organizations/{organization_id}/holders
GET  /v1/organizations/{organization_id}/holders/{holder_id}
POST /v1/organizations/{organization_id}/holders/{holder_id}/instruments

POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/holders/{holder_id}/accounts

POST /v1/organizations/{organization_id}/packages
POST /v1/organizations/{organization_id}/estimates
POST /v1/organizations/{organization_id}/billing/calculate

POST /v2/organizations/{organization_id}/ledgers/{ledger_id}/packages
POST /v2/organizations/{organization_id}/ledgers/{ledger_id}/estimates
POST /v2/organizations/{organization_id}/ledgers/{ledger_id}/billing/calculate
```

The `:organization_id` (and `:ledger_id`) path parameters are parsed and UUID-validated by the
protected-route chain via `ParseUUIDPathParameters` before any handler runs. A non-UUID
`organization_id` segment is rejected with `400` (`ErrInvalidPathParameter`). The validated
`uuid.UUID` reaches the handler through request locals (`http.GetUUIDFromLocals`) — the
organization never enters a handler as an unvalidated string. This is the convention for the
entire ledger surface, with no exceptions.

A genuinely **missing** organization is not expressible in this convention: the route simply does
not match and Fiber returns `404`. The former "missing scoping header" error class is gone.

## What changed for CRM (2026-06-06)

- **Organization is a path-validated UUID.** CRM handlers read it from locals
  (`http.GetUUIDFromLocals(c, "organization_id")`) instead of the former
  `c.Get("X-Organization-Id")`. This kills the unvalidated-string-into-collection-name class of bug:
  the org value that partitions the CRM Mongo collections (`holders_<org>`, `aliases_<org>`) is now
  a validated UUID rather than a raw header string.
- **`X-Ledger-Id` was removed entirely.** It is no longer a live contract on any CRM or composition
  route. The single route that legitimately needs a ledger — composition account-open — now carries
  `:ledger_id` in its path (`/v2/organizations/{organization_id}/ledgers/{ledger_id}/holders/{holder_id}/accounts`),
  because it creates a real ledger account.
- **`ledger_id` keeps two non-scoping roles.** It remains a **create-body field** on instrument
  creation, and an **optional list filter** (`?ledger_id=`) on `GET .../instruments` and on
  `GET .../holders/{holder_id}/accounts`. In neither role is it a scoping input for pure-CRM routes.

  `GET /v2/organizations/{organization_id}/holders/{holder_id}/accounts` is org-scoped by its path, and
  holder ownership is org-global, so the listing spans **every ledger of the organization**;
  `?ledger_id=` narrows it to one. A malformed value is `0082` / 400, not a 404: it is a
  query-parameter format error, not a missing ledger. Because the read touches the onboarding
  stores rather than the CRM ones, the route carries its own `holder-accounts` route options
  instead of the CRM ones — see `components/ledger/internal/bootstrap/config.go`.

The service layer keeps its `organizationID string` signatures; only the source and validation of
the value moved (path UUID → `.String()`), so the Mongo partition is unchanged.

## What changed for fees / billing (2026-06-07)

- **Organization is a path-validated UUID.** All 12 fee/billing routes (`packages`, `estimates`,
  `billing-packages`, `billing/calculate`) moved under `/v1/organizations/{organization_id}/...`.
  Handlers read org (and the resource `id`) from locals via `http.GetUUIDFromLocals`, replacing the
  former `X-Organization-Id` header reads.
- **Both bespoke fee middlewares were deleted.** `parseFeeHeaderParameters` and
  `parseFeePathParameters` are gone; the standard `ParseUUIDPathParameters` validates org and the
  resource `id` in one pass, like every other route in the binary.
- **Path-validation errors normalized to canonical codes.** A malformed UUID segment now returns
  the canonical midaz `ErrInvalidPathParameter` envelope instead of the FEE-shim codes. `FEE-0020`
  ("missing header") had no remaining semantics and was deleted. The rest of the `FEE-` prefixed
  family has since been retired too: fee **business** errors now use the canonical numeric registry
  in `pkg/constant/errors.go`, and no `FEE-` code is emitted on the wire. The literal still appears
  in test fixtures and comments that document the old-to-new mapping.
- **`ledger_id` is not a scope on this surface.** It was historically a create-body field
  (`CreatePackageInput.LedgerID`, `FeeEstimate.LedgerID`, `BillingCalculateRequest.LedgerID`) and
  an optional list filter (`?ledgerId=`) on the package/billing-package lists. Those body fields
  were removed from the shared `feeshared/model` request structs when `ledgerId` was dropped from
  the fee create/estimate/calculate request contract (see the `/v2` section below), so `ledger_id`
  is no longer a request-body field on either scope; the org-scoped `/v1` fee routes are not
  currently mounted in the binary. The ledger-scoped surface below is a second contract, not a
  replacement of this one.
- **Authz keys unchanged.** The `plugin-fees` namespace and every `Authorize(...)` triple are
  byte-identical (R9) — route shape moved, policy keys did not. See `docs/auth/RBAC-NAMESPACES.md`.

Fees Mongo storage is org-filtered (`organization_id` field), not org-partitioned, so no storage
change accompanied the route reshape.

## The ledger-scoped fee / billing surface (v2, 2026-08-01)

The same twelve fee and billing operations are **also** served ledger-scoped on the independent
`/v2` contract. Both surfaces are live and neither supersedes the other: `/v1` reaches a resource
on whichever ledger of the organization owns it, `/v2` reaches only what the named ledger owns.

```
/v1/organizations/{organization_id}/packages[/{package_id}]           organization-scoped
/v1/organizations/{organization_id}/estimates
/v1/organizations/{organization_id}/billing-packages[/{billing_package_id}]
/v1/organizations/{organization_id}/billing/calculate

/v2/organizations/{organization_id}/ledgers/{ledger_id}/packages[/{package_id}]          ledger-scoped
/v2/organizations/{organization_id}/ledgers/{ledger_id}/estimates
/v2/organizations/{organization_id}/ledgers/{ledger_id}/billing-packages[/{billing_package_id}]
/v2/organizations/{organization_id}/ledgers/{ledger_id}/billing/calculate
```

On the ledger-scoped surface the path is the sole authority on which ledger a request acts within:

- **The nil ledger is refused as a path value.** It is a syntactically valid UUID, so
  `ParseUUIDPathParameters` admits it, and both fee repositories read it as "no ledger requested" —
  which would widen a ledger-scoped read back to the whole organization.
- **The request body no longer carries a ledger.** `ledgerId` was removed from every `/v2` fee and
  billing create/estimate/calculate request body (`packages`, `estimates`, `billing-packages`,
  `billing/calculate`); the billing-package create request has its own `CreateBillingPackageInput`
  so the response model can keep `ledgerId` while the request does not. The path is the sole ledger
  input — a body that still sends `ledgerId` is rejected as an unknown field (`400`). The former
  body-versus-path mismatch guard and its `0234` code are retired.
- **`0236` (duplicate fee key) is retired.** Fee keys are stored verbatim, so two never collide.
- **`?ledgerId=` is refused on the two listings** (`400`, `0235`) — the only ledger-scoped
  operations that read a query at all. It can only restate the path or contradict it, and its empty
  value means "every ledger of the organization" — the one scope a ledger-scoped listing must not
  be able to express.

Authz is unchanged again: the `plugin-fees` namespace and the same `(resource, verb)` tuples, so
no new policy surface accompanies the second contract.

### The admin surface is not the transaction seam

Everything above describes the fee **administrative** surface — packages, estimates, billing
packages, billing calculation — which is served at both scopes. It says nothing about where fees
are *applied to a transaction*, and conflating the two is what makes the boundary non-obvious.

The transaction fee seam is **`/v2`-only**. A `/v1` transaction create — `json`, `inflow`,
`outflow`, `annotation`, `block`, `unblock` — never reaches the fee engine: no package lookup, no
tenant fee-database resolution, no fee legs. It posts exactly as authored. `/v1` shipped before
the fee engine existed, and a client integrated against it must not acquire fee legs from a
version upgrade it never asked for.

The two facts are independent: an organization can administer packages over `/v1` and still have
those packages apply only to the transactions it posts on `/v2`.

### The tracer reservation is a `/v2` contract too

The same boundary governs the tracer. The reservation lifecycle is **`/v2`-only** across all three
of its seams: the reserve anchor on create and revert, and the by-transaction confirm/release on
commit and cancel. A `/v1` route never reaches the tracer — no reserve request is built, no
connection is dialled, and a `/v1` create can never answer `0177` (reservation denied by a limit),
`0535` (denied by a rule), `0531` (review), `0178` (reservation unavailable) or `0536` (seam
credential rejected). Like fees, `/v1`
shipped before the tracer existed, and the per-ledger `tracer.mode` setting is an operator's choice that must not retroactively gate a contract the
client integrated against.

On every transaction path the version is the method name, not a runtime value:
`CreateTransactionV1` and `RevertTransactionV1` name neither the fee engine nor the tracer
reservation; `CreateTransactionV2` names both and `RevertTransactionV2` names the tracer only —
a revert already carries the reversed fee legs, but limits measure GROSS activity, so the reversal
reserves capacity of its own. The PENDING state transition follows the same rule:
`CommitTransactionV1` / `CancelTransactionV1` run `transitionPendingV1`, which names neither
by-transaction seam, while `CommitTransactionV2` / `CancelTransactionV2` run `transitionPendingV2`,
which confirms on APPROVED and releases on CANCELED, both after the balance commit. Structural
gates assert all of it: `create_transaction_version_gates_test.go` (a `/v1` create pipeline names
no versioned seam, a `/v2` one names them in order),
`transaction_reservation_anchor_structure_test.go` (the same for the two state pipelines) and, on
the transport side, `transaction_fee_seam_structure_test.go` and
`transaction_route_version_structure_test.go` (every route binds the use case matching its
version).

#### What a `/v2` reserve sends and how its answer gates the transaction

The reserve request carries the fee-inclusive `amount` and `asset`, the transaction date as
`transactionTimestamp`, and the scope of the first internal source leg:

- `account.accountId` and `account.type` — the source account's id and its ledger account type,
  verbatim (free-form; the tracer applies only a 256-character bound). An external-only source
  sends an account with neither, and the tracer matches only limits that are not account-scoped.
- `metadata` — the transaction metadata, filtered to what the tracer accepts: keys matching
  `^[a-zA-Z0-9_]+$` and at most 64 characters, scalar values rendered as strings (numbers in plain
  decimal notation), at most 50 entries taken in lexicographic key order. Every other entry is
  dropped, never sent, and counted on the span (`app.tracer.metadata_dropped`); metadata that
  filters to nothing is omitted. Because every value arrives as a string, a CEL rule evaluated on
  the reserve path must compare strings: `metadata.priority == "3"` matches a numeric `3`, while
  `metadata.priority > 2` does not evaluate (see the rule evaluation error row below).
- `revert` — `true` when the reservation is for a `/v2` revert (singular or an atomic batch item
  whose action is `revert`), omitted otherwise. The tracer skips its CEL rules for a revert and
  still reserves its limits: limits measure gross activity, and a rule that could refuse a revert
  would leave an applied movement impossible to correct.

The asset follows the ledger's asset code grammar exactly: uppercase Unicode letters, at most 100
characters. An asset the ledger accepts is never refused by the tracer for its shape.

`tracer.timeoutMs` (range `1..30000`) is the deadline of each reserve call, and the per-ledger
value is always the one applied: a ledger that never stored it reads the default `250`, so
raising `TRACER_TIMEOUT_MS` alone does not lengthen reserve calls on any ledger. The client
timeout (`TRACER_TIMEOUT_MS`) is only a ceiling — a per-call deadline can tighten it, never extend
it — so the effective reserve deadline is the shorter of the two. Confirm and release run under
the client timeout only.

The tracer evaluates its CEL rules first and its limits second, and answers `decision`
(`ALLOW`, `DENY` or `REVIEW`) beside the `denied` flag. On the reserve path only a MATCHED rule
refuses: when no rule matches, the tracer's `DEFAULT_DECISION_WHEN_NO_MATCH` is ignored and the
reserve continues to the limits. A matched rule's `DENY` or `REVIEW` refuses the reserve before any
limit counter is touched. A rule the tracer cannot evaluate for the transaction is a refusal too,
answered as `decision=REVIEW` with `reason=rule_evaluation_error`: that covers every rule-evaluation
class — a syntax or compile error, a program build error, a cost-estimation failure and a runtime
error such as a type mismatch against a string metadata value. Evaluation continues past a rule
that cannot be evaluated: a matched `DENY` from another rule still decides, and only without one
does the failure answer `REVIEW`. The tracer's `POST /v1/validations` answers a rule it cannot
evaluate the same way: HTTP 200 with `decision=REVIEW` and
`reason=rule_evaluation_error`. The ledger maps the reserve outcome as follows:

| Tracer outcome | `mode=enforce` | `mode=advisory` |
|---|---|---|
| `ALLOW` | proceeds; the reservation handle is kept for confirm/release | proceeds |
| `DENY` by a limit (`reason=limit_exceeded`) | rejects with `0177` (422) before the balance commit | proceeds, logs a warning |
| `DENY` by a matched rule | rejects with `0535` (422) before the balance commit; the rule's reason is logged, never returned | proceeds, logs a warning |
| `REVIEW` (a matched rule, or `reason=rule_evaluation_error`) | rejects with `0531` (422) before the balance commit | proceeds, logs a warning |
| request refused (gRPC `InvalidArgument`/`FailedPrecondition`, including a reserve replayed onto a transaction whose reservation is already released, expired or confirmed — tracer code `0533`) | rejects with `0532` (422) whatever `failPosture` says: the tracer answered | proceeds, logs a warning |
| unavailable (timeout, connection failure, open breaker, a tenant whose rule cache is not loaded yet, the tracer's per-tenant worker cap reached — gRPC `Unavailable` with code `0445` —, a tenant the tracer holds as not provisioned or not active — gRPC `Unavailable` with code `0534` —, a client certificate outside the tracer's `TRACER_TLS_CLIENT_ALLOWED_NAMES`, an Access Manager the tracer cannot reach, a seam credential the ledger cannot obtain — counted as `tracer_reservation_credential_rejected_total{operation,reason="not_sent"}` and logged once at Error —, any other tracer error) | `failPosture=open` proceeds with a SKIPPED audit; `failPosture=closed` rejects with `0178` (503) | proceeds, logs a warning |
| credential rejected (gRPC `Unauthenticated`: missing or invalid token, missing or wrong API key; gRPC `PermissionDenied`: the Access Manager denies `tracer/reservations`, or the tracer's guard refuses the token) | `failPosture=open` proceeds with the reservation skipped; `failPosture=closed` rejects with `0536` (503). Either way the ledger logs at Error and counts `tracer_reservation_credential_rejected_total{operation,reason="rejected"}` | proceeds, logs at Error |

`mode=off`, an unset `TRACER_BASE_URL` and an honored `skip.tracer` build no request at all. A
denied or refused result holds no capacity, so none of those rejections leaves a reservation to
release. An unanswered reserve is different: it was sent and then timed out or was cancelled before
the tracer answered (gRPC `DeadlineExceeded`/`Canceled`), and the tracer may still have reserved
capacity for the transaction. The ledger then settles by transaction, in every mode and posture,
once the accounting outcome is known — it confirms by transaction when the movement applied and the
transaction is not PENDING (a PENDING transaction is settled by its commit or cancel), and releases
by transaction on a `0178` rejection or when the engine aborted. A reserve the tracer answered with
an error (including `Unavailable`, such as `0534`) or one that was never sent (connection failure)
held nothing and is not settled. The settle never runs on the request path and never shares the
retrier that redelivers by-id confirms: a dedicated queue of 64 waits `3s` (the tracer's reserve lock
wait) plus `TRACER_TIMEOUT_MS` before each attempt, so the unanswered reserve has finished before the
settle lands. A confirm that fails or settles nothing is offered once more, a release that fails is
offered once more, and what is still unsettled is logged as a warning with the transaction id only.
A full queue drops the settle with the same warning.
An atomic batch or a cross-ledger v2 commit whose execution hand-off fails releases the
reservations held for its items (for the commit, its destinations), because no movement follows.
These settles are best-effort: they never block or change the response.

Under `mode=advisory` a transaction the tracer denied, flagged or refused still commits, but nothing was reserved for it, so its spend is never counted against any limit.
Infrastructure failures inside the tracer (its database or cache) stay errors and follow the
unavailable row; only a rule evaluation error is a refusal. The ledger records a refusal as a
business event on a span that is not marked as an error, and never forwards the tracer's response
body to the client. A tracer that predates `decision` answers a `REVIEW` as a plain `denied=true`,
which the ledger reads as `0177`.

The ledger reaches the tracer only over the gRPC reservation seam (`TRACER_BASE_URL` is its
`host:port`, default tracer port `:4021`); the tracer's HTTP API has no reservation route. A confirm
reports `already_released`, the rows of the transaction an explicit release (a cancel) had already
moved to RELEASED before the confirm arrived, and whose spend the tracer therefore never counts. Its
TTL reaper does not produce `already_released`: it marks an unsettled row EXPIRED and returns its
capacity, and a later confirm still settles that EXPIRED row and counts its spend. The ledger does not fail the commit on
them: it logs a Warn, adds the span event `tracer.reservation.confirm_already_released` and increments
`tracer_reservation_confirm_already_released_total{operation}`.

#### Seam identity

The ledger presents at most one credential on every seam call. With `PLUGIN_AUTH_ENABLED=true` it
is an Access Manager application token in gRPC metadata `authorization: Bearer <token>`, minted with
a credential dedicated to the tracer: the static `TRACER_M2M_CLIENT_ID`/`TRACER_M2M_CLIENT_SECRET`
pair in single-tenant mode, and in multi-tenant mode the calling tenant's own credential, read from
`tenants/{ENV_NAME}/{tenant UUID without dashes}/ledger/m2m/tracer/credentials` on the backend
`M2M_SECRETS_BACKEND` selects (`aws`, the default: AWS Secrets Manager; `vault`: HashiCorp Vault KV v2 under `M2M_VAULT_MOUNT`, connected through `VAULT_ADDR`/`VAULT_TOKEN`/`VAULT_CACERT`/`VAULT_NAMESPACE`), so the token carries the tenant's `tenantId` claim. Without plugin auth it is `TRACER_API_KEY` as `x-api-key`; with
neither, nothing is sent and identity is left to the transport (`mtls` or a mesh). Setting both
refuses boot.

The tracer enforces the first identity its configuration enables: the token
(`PLUGIN_AUTH_ENABLED=true`, authorized as `tracer/reservations:post`, only for application tokens,
whose `azp` (client id) or `sub` must be in `TRACER_SEAM_ALLOWED_CLIENTS` in single-tenant mode, and whose `name` claim must be
the ledger→tracer client of the tenant its own `tenantId` claim names (`ledger-m2m-tracer-{tenant}`,
tenants compared canonically) in multi-tenant mode; `AUTH_M2M_INVERSION_ENABLED=true` is
recommended — without it the tracer boots with a Warn and the Access Manager authorizes application
tokens under a shared editor role, so only the allowlist or the ledger client name binding restricts
who may reserve — and it refuses to boot under `DEPLOYMENT_MODE=saas` — Warns
elsewhere — unless `AUTH_CACHE_TTL` is greater than zero), the API key (`API_KEY_ENABLED=true`), the transport, or none.
Under the token the tenant is the token's `tenantId` claim and `x-tenant-id` is only cross-checked;
under every other identity, all single-tenant, `x-tenant-id` carries it. A seam without identity
boots with one Warn in BYOC and `local`, and refuses to boot under `DEPLOYMENT_MODE=saas` and in
multi-tenant mode. The ledger caches each token and refreshes it ahead of expiry, serving the still-valid
token while one background mint runs, and schedules each token's refresh, retried with backoff while
the token is valid, so an idle tenant normally finds a valid token. Consecutive failed mints widen a
per-tenant failure window (1s, doubling, capped at 15s, jittered) that every caller and the scheduled
refresh honour; past 30 idle minutes the token is
no longer refreshed but stays served until it expires. On `Unauthenticated` it invalidates only the rejected token and retries the call once
with a fresh one, unless that token is younger than 5 seconds, never on `PermissionDenied` and never for
an API key. A missing tenant secret is cached for 5 seconds and a malformed one for 30. A caller whose
deadline strikes while it waits for a token sent nothing and takes the ordinary unavailable path. A credential the ledger cannot obtain is never sent:
the call fails as unavailable (the `0178` row above), logged once at Error and counted in
`tracer_reservation_credential_rejected_total{operation,reason="not_sent"}`.

**Upgrade order.** Provision the ledger's credential first (`TRACER_M2M_CLIENT_ID`/`TRACER_M2M_CLIENT_SECRET`
in single-tenant mode; the tenant-manager's ledger→tracer credential for every tenant in multi-tenant
mode) and deploy the ledger, then enable the tracer's token identity (`PLUGIN_AUTH_ENABLED=true`,
`AUTH_M2M_INVERSION_ENABLED=true` (recommended), `TRACER_SEAM_ALLOWED_CLIENTS` in single-tenant
mode, and `AUTH_CACHE_TTL`). A tracer that enables token identity before the ledger sends tokens answers
`Unauthenticated`: the ledger rejects with `0536` under `enforce` + `closed` and proceeds without a
reservation under `open` or `advisory`. The full posture matrix, the transport modes
(`mtls`, `server`, `mesh`) and the operator checklist are in
[Ledger / Tracer topology §5](../architecture/ledger-tracer-topology.md#5-seam-identity-and-transport-security).

### Cross-ledger enablement is a `/v2` contract

`crossLedger.enabled` is an operator's per-ledger opt-in. The policy resolver accepts only
ledgers that explicitly enable it and returns `0249` (HTTP 422) when one is disabled. This
setting does not retroactively change `/v1`: `/v2/transactions/direct` and
`/v2/transactions/hold` consume the policy when debit and credit legs name multiple ledgers.
Direct decomposes and executes every part atomically under a shared `groupId`. Hold creates
only PENDING origin parts; v2 commit creates destinations while approving all origins in one
engine execution, and v2 cancel releases the origins without creating destinations. V2 revert
continues to reverse every APPROVED member in one atomic execution and returns a new group plus
`revertedGroupId`. Cross-ledger block and unblock remain unsupported. Authorization is checked
against the organization and ledger in the lifecycle route path; other group members may belong
to other enabled ledgers or organizations in the same authenticated tenant. `/v1` cannot express
the grouped response and rejects grouped commit, cancel, or revert with `0252` (HTTP 422). See
[Cross-ledger transactions](cross-ledger-transactions.md).

### Transaction skips are a `/v2` body field

The two per-call transaction controls — `skip.fees` and `skip.tracer` — exist only on the
`/v2` create input (`CreateTransactionV2Input`). They opt out of the fee engine and the tracer
reservation, and neither runs on `/v1`, so the field has nothing to mean there. A `/v1` create
body naming `skip` is rejected by the decoder as an unknown field: **HTTP 400**
(`ErrUnexpectedFieldsInTheRequest`), the same answer any other unknown field gets — not the
422 an unpermitted skip earns on `/v2`.

The consequence is durable, not just transport-level: `transaction.fees_skipped` and
`transaction.tracer_skipped` can only be `true` on a row created through `/v2`. On a `/v1`
row they are always `false`, and they stay distinguishable from `fees_route_eligible` /
`tracer_route_eligible`, the span attributes that say the control was never in play.

This differs from `skip.holder`, which remains a known — but inert — field on the `/v1`
account body.

**Mixing mounts across one transaction lifecycle is not supported.** A by-transaction
confirm/release cannot tell whether the transaction holds reservations, so a PENDING created on
`/v2` and committed through `/v1` never receives its confirm — `transitionPendingV1` names no
reservation seam: the reservation stays RESERVED until the TTL reaper marks it EXPIRED and
returns its capacity, and because no confirm ever arrives the committed amount is never counted
against the usage limit. Commit and cancel a transaction on the
same contract that created it. Closing this needs create-time reservation state persisted on the
transaction row for the `/v1` pipeline to read.

### Singular create idempotency applies to both contracts

A singular create on `/v1` or `/v2` stores a fingerprint of the request next to the
transaction in its idempotency slot. A later request that reuses the slot replays the stored
transaction (`X-Idempotency-Replayed: true`) only when its fingerprint matches; a different
request under the same key answers `0084` (HTTP 409) and posts nothing. The effective key of a
request without `X-Idempotency` is unchanged, so a retry that straddles a deploy still finds its
original slot, and a slot written before fingerprints existed keeps replaying until its TTL
(default 300 s, `X-TTL` up to 604800 s) runs out.

- **`/v2`** fingerprints the body canonically (whitespace and property order ignored) together
  with the action, so direct, hold, block, and unblock never replay one another.
- **`/v1`** fingerprints the canonical transaction together with its status and operation-type
  override. `json`, `annotation`, `block`, and `unblock` accept the same body, so a byte-identical
  body posted to two of them within the TTL answers `0084` even without a key.
- **`/v1` and `/v2`** fingerprints never match each other, so one key cannot cross versions.

`RevertTransactionV1` and `RevertTransactionV2` go through the same check. A revert sends no
idempotency key, so its slot is still keyed on the reversal hash and is not scoped by origin. Its
fingerprint is derived the way `/v1`'s is, from the reversal with status `CREATED` and no override.
A matching fingerprint replays the stored reversal, so two reverts that share a slot still replay
each other. A differing fingerprint answers `0084` (HTTP 409); that happens only when the slot was
created by another kind of request with the same serialized reversal, such as a `/v1` annotation.
The atomic batch and the cross-ledger request keep their own fingerprint rules; see
[Atomic transaction batch](atomic-transaction-batch.md) and
[Cross-ledger transactions](cross-ledger-transactions.md).

## The holder seam is `/v2`-only

The same contract-versus-scope split applies to accounts. The **holder seam** on account create —
the `accounting.requireHolder` gate, the two-key `skip.holder` control, and the deterministic
self-holder default that materialises `account.holder_id` — is **`/v2`-only**.

The signal is `command.RouteHolderPolicy` (`HolderOffV1` / `HolderOnV2`), threaded from the transport
shell because the use case is transport-agnostic and cannot read the request path. It is the one
place where the version travels as a runtime value rather than as a method name: the account create
path has a single `CreateAccount` use case, so the seam inside it has to be told which contract it
is serving. The transaction paths encode the version in the use-case name instead
(`CreateTransactionV1`/`V2`, `RevertTransactionV1`/`V2`, `CommitTransactionV1`/`V2`,
`CancelTransactionV1`/`V2`) and thread nothing.

A `/v1` account create never reaches it. It links no holder (the row persists `holder_id = NULL`
and `holder_check_skipped = false`), performs no holder settings read, and can be rejected by
neither the requireHolder gate (`ErrHolderRequired` / `ErrHolderNotFound`) nor an unpermitted skip
(`ErrSkipNotPermitted`). `holderId` and `skip.holder` in a `/v1` body are inert. `/v1` shipped
before the seam existed, and a client integrated against it must not acquire a holder link — or a
new rejection class — from a version upgrade it never asked for.

The independence is **physical, not only semantic**: the policy reaches the SQL, so a `/v1`
statement does not NAME `holder_id` or `holder_check_skipped` — with one exception, the holder
filter below. A create omits both columns — an account without a holder writes what they default
to, so the row is identical either way — and a `/v1` read projects `NULL::uuid AS holder_id` and
`FALSE AS holder_check_skipped`, which keeps the projection's arity and column order intact for the
positional scans. `/v1` therefore stays servable against a database that has not reached migrations
000017 and 000019, which matters because the schema is applied out of band and the runner is
tenant-agnostic: a tenant database can sit behind the binary. `/v2` names the real columns and, on
such a database, answers `0501` `ErrSchemaMigrationPending` / **503** — retryable, because the same
request succeeds once the migration runner reaches that database. The three `ListAccounts*` reads
that serve the transaction and asset paths read no holder at all, so they always project the
constants and are immune on both contracts.

The **exception is `GET /v1/.../accounts?holder_id=…`**. The list filter is applied on both
contracts whenever the parameter is present, so that one `/v1` statement does add a
`holder_id = ?` predicate and does fail on a pre-000017 database. This is a deliberate gap, not an
oversight: filtering by holder on a contract whose responses withhold `holderId` is already an
anomaly, and rejecting the parameter would hand `/v1` a new rejection class — the very thing this
seam exists to prevent. Holder filtering on `/v1` is therefore not expected to work before
migrations 000017 and 000019. Every other `/v1` read, and every `/v1` create, is unaffected.

Ordering matters on the write paths. A `/v1` update completes on such a database: both its
pre-update lookup and the update statement name no holder column. A `/v2` update fails at the
lookup, before the row is mutated and `account.updated` is emitted — the update statement itself
names no holder column, so a lookup that ignored the route version would let the mutation land and
answer 503 over a write that actually happened.

The withholding reaches the response too. Every `/v1` account response — create, list, get-by-id,
get-by-alias, get-external-by-code, update — answers with the projection that omits `holderId` and
`holderCheckSkipped`; the `/v2` twins answer with the full account. Both contracts publish the
projection they serve as a distinct component, and the `/v1` one keeps the canonical **`Account`**
name so generated v1 SDKs bind to the type they already have, which puts the holder-bearing shape
on **`AccountV2`**.

The **organization self-holder** is outside the seam on both contracts. Creating an organization
writes no CRM record on either `/v1` or `/v2`; the idempotent backfill runner
(`components/ledger/cmd/backfill`) is the only path by which an organization acquires its
deterministic self-holder — the `LEGAL_PERSON` holder whose ID is derived from the org ID via
UUIDv5, and the default owner a `/v2` account create resolves to. The derivation is pure, so an
account create materialises `holder_id` without consulting CRM; the referenced record exists once
the backfill has run. Nothing about the organization response is versioned: the organization wire
shape carries no holder field, so both contracts publish one schema and differ only in the
operation IDs they publish.

The **CRM holder surface itself** (`/v2/organizations/{organization_id}/holders...`) and the
holder-account **composition** route (`POST /v2/.../ledgers/{ledger_id}/holders/{holder_id}/accounts`) are
served on `/v2` only and are unaffected: composition exists to link a holder, so it contracts the
seam in full.

Two account-adjacent write paths are **outside** the seam on both contracts, and stay that way. The
implicit **external account** that asset creation opens is built and persisted directly through
`AccountRepo`, bypassing the account-create use case, so it carries no holder — which is also what
the seam would resolve for an external account. And the account **update** path cannot touch
ownership: `holderId` is immutable (it is not a field on the update input, and an unknown body field
is a `400`), and neither holder column appears in the update statement.

## Accounting routes: organization-owned, reachable from both scopes

Operation routes and transaction routes belong to the **organization**. A route is resolved,
updated and deleted by `(organization, id)`, a transaction route may link operation routes created
under different ledgers of the organization, and a route validates in every ledger of the
organization that sets `accounting.validateRoutes` (that setting stays per ledger).

Two path scopes serve the same routes:

| Scope | Paths | Contracts | `ledgerId` on create |
| --- | --- | --- | --- |
| Organization | `/organizations/{organization_id}/{operation,transaction}-routes[/{operation_route_id|transaction_route_id}]` | `/v2` only | absent — the route has no ledger |
| Ledger | `/organizations/{organization_id}/ledgers/{ledger_id}/{operation,transaction}-routes[/{operation_route_id|transaction_route_id}]` | `/v1` and `/v2` | the path ledger, recorded as provenance |

On the ledger paths the ledger is **provenance, not a filter**: list, get, patch and delete reach
every route of the organization, whichever ledger it was created under and including routes created
at organization level. `ledgerId` is omitted from the response (and from the six route events) for a
route that has no ledger; a route created under a ledger still carries it, so existing `/v1`
responses keep their shape.

Both scopes authorize with the same tuples — `("midaz","operation-routes",verb)` and
`("midaz","transaction-routes",verb)`, verbs `post`/`get`/`patch`/`delete` — and the same
transaction-module tenant chain. The organization paths add no permission name and need no
tenant-manager policy change.

**Rollout:** pods older than this change cannot see a route with no ledger. Do not create routes at
organization level until every pod runs a version that serves the organization paths.

Route cache entries never expire. Newer pods read `accounting_routes:{organization:route}` and clear the
older per-ledger key on every route write, so older pods reload fresh rules. An update or delete served
by an older pod clears only the per-ledger key and leaves the newer pods' entry stale. Hold route updates
and deletes until the rollout completes, or delete the two-segment `accounting_routes` keys once
afterwards.

## Metadata on a PATCH: `null` is a `/v2` no-op

Every PATCH whose body carries `metadata` applies it as an RFC 7396 merge patch: organization,
ledger, portfolio, segment, account, account type, asset, transaction, operation, operation route,
transaction route, holder and instrument. The contracts differ on one body only, an explicit
`"metadata": null`:

| Body | `/v1` | `/v2` |
| --- | --- | --- |
| no `metadata` key, or `"metadata": {}` | stored metadata left as it is | stored metadata left as it is |
| `"metadata": {"k": "v"}` | `k` added or replaced, every other key kept | same |
| `"metadata": {"k": null}` | `k` deleted, every other key kept | same |
| `"metadata": null` | every key the client wrote deleted; the ledger's reserved fee keys on a transaction or operation stay | stored metadata left as it is |

The other patched fields apply in every row. `/v2` clears metadata one key at a time, so a client
whose serializer writes an unset map as `null` cannot erase it by accident; `/v1` keeps the reading
it shipped with. Fee packages, billing packages and balances carry no `metadata` on their PATCH, and
the asset rate is a `/v1` `PUT`.

## Summary

One rule, no exceptions: **every organization-scoped surface in the unified binary — ledger,
routing, CRM, composition, fees/billing — scopes through the URL path hierarchy**, UUID-validated
by the protected-route chain. `X-Organization-Id` and `X-Ledger-Id` no longer exist in any API
contract. Clients integrate one convention.

Where a surface is served at two scopes — fees and billing, organization-scoped on `/v1` and
ledger-scoped on `/v2` — the deeper scope is expressed by a deeper path, not by a header or a
query parameter. The convention does not change; only how much of the hierarchy the path names.

Scope and contract are separate questions. The fee admin surface answers the first (two scopes,
both live); the transaction fee seam, the tracer reservation lifecycle and the account holder seam
answer the second (`/v2` only — the first two by the transaction create pipeline the route binds,
the third by `command.RouteHolderPolicy` in the account create use case). A surface being
reachable at a scope says nothing about which contract applies it.
