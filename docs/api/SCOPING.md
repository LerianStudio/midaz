# API Scoping Conventions (R22)

The unified ledger binary (`:3002`) exposes two scoping conventions for the organization a
request applies to. They are **intentionally different** and both are accepted in the current
API surface. This is a known, documented inconsistency — not a shim and not a bug.

## Two mechanisms

### 1. Path-based scoping — ledger, routing, fees

Onboarding, transaction, routing, and fee/billing endpoints scope the organization (and ledger)
through the URL path hierarchy:

```
GET /v1/organizations/{organization_id}/ledgers/{ledger_id}/accounts/{account_id}
POST /v1/organizations/{organization_id}/ledgers/{ledger_id}/transactions/json
```

The `:organization_id` and `:ledger_id` path parameters are parsed and UUID-validated by the
protected-route chain. This is the convention for everything that descended from the original
midaz ledger.

### 2. Header-based scoping — CRM (holders / instruments)

CRM endpoints scope the organization through the `X-Organization-Id` **HTTP header**, not the
path. The holder/instrument routes carry no `:organization_id` path segment:

```
POST /v1/holders
X-Organization-Id: 0192f5a1-...-...

GET  /v1/holders/{id}
X-Organization-Id: 0192f5a1-...-...

POST /v1/holders/{holder_id}/instruments
X-Organization-Id: 0192f5a1-...-...
```

The header is read in the CRM handlers via `c.Get("X-Organization-Id")`
(`components/ledger/internal/adapters/http/in/holder.go`, `instrument.go`). This is **legacy**
behavior inherited from when CRM was a standalone service; the route merge did not rework it.

## Why it persists

Per the locked Phase 9 decision, this inconsistency is **documented, not reworked** (PD: document,
do not rework). Harmonizing the two conventions — either lifting CRM onto path-based scoping
(`/v1/organizations/{organization_id}/holders`) or moving ledger onto header-based scoping —
is a breaking API change with client-migration cost. It is **out of Phase 9 scope** and tracked
as risk **R22**.

For now:

- ledger / routing / fees: **path-based** org/ledger scoping.
- CRM (holders / instruments): **header-based** scoping via `X-Organization-Id`.
- Both are intentional. Clients integrating CRM endpoints must send `X-Organization-Id`; clients
  integrating ledger endpoints must use the path hierarchy.

No handler code is changed by this document.
