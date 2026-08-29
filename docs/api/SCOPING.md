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
GET  /v1/organizations/{organization_id}/holders/{id}
POST /v1/organizations/{organization_id}/holders/{holder_id}/instruments

POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts

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
  `:ledger_id` in its path (`/v1/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts`),
  because it creates a real ledger account.
- **`ledger_id` keeps two non-scoping roles.** It remains a **create-body field** on instrument
  creation, and an **optional list filter** (`?ledger_id=`) on `GET .../instruments`. In neither
  role is it a scoping input for pure-CRM routes.

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
/v1/organizations/{organization_id}/packages[/{id}]           organization-scoped
/v1/organizations/{organization_id}/estimates
/v1/organizations/{organization_id}/billing-packages[/{id}]
/v1/organizations/{organization_id}/billing/calculate

/v2/organizations/{organization_id}/ledgers/{ledger_id}/packages[/{id}]          ledger-scoped
/v2/organizations/{organization_id}/ledgers/{ledger_id}/estimates
/v2/organizations/{organization_id}/ledgers/{ledger_id}/billing-packages[/{id}]
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
connection is dialled, and a `/v1` create can never answer `0177` (reservation denied) or `0178`
(reservation unavailable). Like fees, `/v1` shipped before the tracer existed, and the per-ledger
`tracer.mode` setting is an operator's choice that must not retroactively gate a contract the
client integrated against.

Both seams read one signal — `routeVersionPolicy` (`routeV1`/`routeV2`,
`transaction_route_version.go`), threaded from the transport shell because the cores are
transport-agnostic and cannot read the request path. Each seam decides for itself what the version
means. Structural gates in `transaction_fee_seam_structure_test.go` and
`transaction_route_version_structure_test.go` assert every route names its policy and that the
route gate is the first statement of each tracer seam.

**Mixing mounts across one transaction lifecycle is not supported.** A by-transaction
confirm/release cannot tell whether the transaction holds reservations, so gating it on the route
version means a PENDING created on `/v2` and committed through `/v1` never receives its confirm:
the reservation stays RESERVED until the TTL reaper releases it, and the committed amount is never
counted against the usage limit. Commit and cancel a transaction on the same contract that created
it. Closing this needs create-time reservation state persisted on the transaction row for the gate
to read instead of the route version.
## The holder seam is `/v2`-only

The same contract-versus-scope split applies to accounts. The **holder seam** on account create —
the `accounting.requireHolder` gate, the two-key `skip.holder` control, and the deterministic
self-holder default that materialises `account.holder_id` — is **`/v2`-only**.

The signal is `command.RouteHolderPolicy` (`HolderOffV1` / `HolderOnV2`), threaded from the transport
shell for the same reason the transaction cores thread `routeVersionPolicy`: the use case is
transport-agnostic and cannot read the request path. The two are siblings at different layers, not
duplicates — the fee and tracer seams sit in the transaction handler, the holder seam in the account
use case, and a `command` type cannot be the unexported `in` one without inverting the dependency
direction.

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
holder-account **composition** route (`POST /v2/.../ledgers/{ledger_id}/holders/{id}/accounts`) are
served on `/v2` only and are unaffected: composition exists to link a holder, so it contracts the
seam in full.

Two account-adjacent write paths are **outside** the seam on both contracts, and stay that way. The
implicit **external account** that asset creation opens is built and persisted directly through
`AccountRepo`, bypassing the account-create use case, so it carries no holder — which is also what
the seam would resolve for an external account. And the account **update** path cannot touch
ownership: `holderId` is immutable (it is not a field on the update input, and an unknown body field
is a `400`), and neither holder column appears in the update statement.

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
answer the second (`/v2` only — the first two driven by `routeVersionPolicy` in the transaction
handler, the third by `command.RouteHolderPolicy` in the account use case). A surface being
reachable at a scope says nothing about which contract applies it.
