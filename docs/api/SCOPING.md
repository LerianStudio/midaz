# API Scoping Conventions (R22 — reversed)

The unified ledger binary (`:3002`) scopes the organization a request applies to through the
**URL path hierarchy**. CRM (holders / instruments) and the holder-account composition surface are
now path-scoped on the organization, consistent with every native ledger entity. The one remaining
exception is fees/billing, which still scopes through an HTTP header and is tracked separately.

> **R22 is reversed.** This document previously locked CRM to header-based organization scoping
> (`X-Organization-Id`) as an intentional, documented inconsistency. That convention is gone. CRM
> and composition moved to path-based org scoping pre-GA as a **clean break with no dual-routing** —
> there is no header fallback and no transitional period. The substance below is the inverse of the
> original R22 record.

## The path-scoping convention

### Ledger, routing, CRM, composition

Onboarding, transaction, routing, CRM (holders / instruments), and the holder-account composition
endpoints scope the organization (and, where a real ledger account is involved, the ledger) through
the URL path hierarchy:

```
GET  /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json

POST /v1/organizations/{organization_id}/holders
GET  /v1/organizations/{organization_id}/holders/{id}
POST /v1/organizations/{organization_id}/holders/{holder_id}/instruments

POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/holders/{id}/accounts
```

The `:organization_id` (and `:ledger_id`) path parameters are parsed and UUID-validated by the
protected-route chain via `ParseUUIDPathParameters` before any handler runs. A non-UUID
`organization_id` segment is rejected with `400` (`ErrInvalidPathParameter`); a malformed
`ledger_id` segment on the composition route is rejected the same way. The validated `uuid.UUID`
reaches the handler through request locals — the organization never enters a handler as an
unvalidated string. This is the convention for the entire ledger surface.

### What changed for CRM

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

## The single remaining header-scoped exception: fees / billing

Fee and billing endpoints still scope the organization through the `X-Organization-Id` HTTP header.
This is now the **only** header-scoped surface in the binary. The shared constant lives at
`components/ledger/internal/adapters/http/in/fees_middlewares.go:19`
(`feeOrgIDHeaderParameter = "X-Organization-Id"`), read by `parseFeeHeaderParameters`.

```
POST /v1/packages
X-Organization-Id: 0192f5a1-...-...
```

Harmonizing fees onto path-based scoping is a breaking change with client-migration cost and is
**out of scope here**; its hardening is owned by the auth/X1 plan. Clients integrating fee/billing
endpoints must still send `X-Organization-Id`; clients integrating every other surface — ledger,
routing, CRM, composition — must use the path hierarchy.

## Summary

- ledger / routing / CRM / composition: **path-based** org (and ledger where applicable) scoping,
  UUID-validated by the protected-route chain.
- fees / billing: **header-based** scoping via `X-Organization-Id` — the sole remaining exception,
  tracked under the auth/X1 plan.
