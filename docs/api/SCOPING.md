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
- **`ledger_id` is not a scope on this surface.** It is a create-body field
  (`CreatePackageInput.LedgerID`, `FeeEstimate.LedgerID`, `BillingCalculateRequest.LedgerID`) and
  an optional list filter (`?ledgerId=`) on the package/billing-package lists. The ledger-scoped
  surface below is a second contract, not a replacement of this one.
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
- **A body that names a different ledger is refused.** The ledger stays a required body field
  (the models are shared with `/v1` and with the in-process fee seam, so it cannot be dropped for
  one caller), but a disagreement with the path is a `400` (`0234`), not a silent path-wins.
- **`?ledgerId=` is refused on the two listings** (`400`, `0235`) — the only ledger-scoped
  operations that read a query at all. It can only restate the path or contradict it, and its empty
  value means "every ledger of the organization" — the one scope a ledger-scoped listing must not
  be able to express.

Authz is unchanged again: the `plugin-fees` namespace and the same `(resource, verb)` tuples, so
no new policy surface accompanies the second contract.

## Summary

One rule, no exceptions: **every organization-scoped surface in the unified binary — ledger,
routing, CRM, composition, fees/billing — scopes through the URL path hierarchy**, UUID-validated
by the protected-route chain. `X-Organization-Id` and `X-Ledger-Id` no longer exist in any API
contract. Clients integrate one convention.

Where a surface is served at two scopes — fees and billing, organization-scoped on `/v1` and
ledger-scoped on `/v2` — the deeper scope is expressed by a deeper path, not by a header or a
query parameter. The convention does not change; only how much of the hierarchy the path names.
