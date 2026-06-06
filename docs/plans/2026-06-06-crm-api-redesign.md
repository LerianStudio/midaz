# CRM API Redesign — Path-Scoped Organization Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Move the CRM (holders/instruments) and holder-account composition surface from header-scoped organization (`X-Organization-Id`) to path-scoped organization (`/v1/organizations/{organization_id}/...`), killing the unvalidated-string-into-collection-name class of bug and making CRM consistent with every native ledger entity.

**Architecture:** Clean break, pre-GA, no dual-routing. Organization reaches handlers only as a path-validated `uuid.UUID` via `ParseUUIDPathParameters`; its `.String()` form continues to drive the unchanged Mongo collection partition (`holders_<org>`, `aliases_<org>`). The service layer keeps its `organizationID string` signatures — only the *source* and *validation* of the value move. `X-Ledger-Id` dies entirely: the one route that legitimately touches a ledger account (composition account-open) moves under `/v1/organizations/{organization_id}/ledgers/{ledger_id}/...`; `ledger_id` stays a create-body field + optional list filter on instruments and is never a scoping input for pure-CRM routes.

**Tech Stack:** Go 1.26, Fiber v2, `ParseUUIDPathParameters`/`ProtectedRouteChain` (`pkg/net/http`), MongoDB (org-partitioned collections), swaggo/swag (generated OpenAPI), Node postman generator (`postman/generator/`), lib-auth v2 (authz, unchanged namespace/resource keys).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | All CRM + composition routes are path-scoped on org; handlers read org as a validated path UUID; `X-Ledger-Id` removed; first-party tests pass against the new shapes; binary builds and serves the new routes | 1.1, 1.2, 1.3 | Detailed |
| 2 | Generated artifacts (Swagger/OpenAPI + Postman) reflect the new path-scoped contract; Postman base URL fixed to unified `:3002` | 2.1, 2.2 | Epic-level |
| 3 | Prose docs reversed and aligned: `SCOPING.md` (R22 reversal, fees named as remaining exception), `llms-full.txt` route inventory, `RBAC-NAMESPACES.md` cross-refs | 3.1 | Epic-level |
| 4 | Hardening follow-ons, each gated on a named decision: org-bound authz (X1), idempotency keys on creates, instrument-create referential validation | 4.1, 4.2, 4.3 | Epic-level |

---

## Ground Truth (verified against the repo on 2026-06-06)

The locked-decision text and the spec summary both contain stale assumptions. The implementer must work from the route inventory below, not from the spec's prose:

- **Instruments are ALREADY nested under holders today.** The only top-level instrument route is the LIST (`GET /v1/instruments`). The by-id routes live under `/v1/holders/:holder_id/instruments/:instrument_id`. There is no top-level `/v1/instruments/{instrument_id}` to move. (`crm_routes.go:48-53`)
- **A `related-parties` DELETE route exists today** at `/v1/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id`. (`crm_routes.go:53`, `instrument.go:385`)
- **`organization_id` and `ledger_id` are already members of `pkg/constant/http.go` `UUIDPathParameters`** (lines 9-10) — so `ParseUUIDPathParameters` will UUID-validate them automatically once they appear as path params. **No constant change is needed for org/ledger.**
- **Latent gap found:** `instrument_id` is NOT in `UUIDPathParameters` (only the legacy `alias_id` is, line 24). Handlers call `GetUUIDFromLocals(c, "instrument_id")` (`instrument.go:106,166,241,398`), which type-asserts a `uuid.UUID` from locals. `ParseUUIDPathParameters` stores non-allowlisted params as raw strings, so on the real router `instrument_id` is currently delivered as a string and `GetUUIDFromLocals` would return `ErrInvalidPathParameter`. The unit tests hide this because they bypass `ParseUUIDPathParameters` and seed `c.Locals("instrument_id", instrumentID)` with a real UUID inline (`instrument_test.go:501,759,880,989`). Phase 1 must add `instrument_id` to the allowlist and prove the by-id instrument routes work through the real chain (Task 1.1.1).

### Current → Target route map (authoritative)

| Method | Current path | Target path |
|--------|--------------|-------------|
| POST | `/v1/holders` | `/v1/organizations/:organization_id/holders` |
| GET | `/v1/holders` (list) | `/v1/organizations/:organization_id/holders` |
| GET | `/v1/holders/:id` | `/v1/organizations/:organization_id/holders/:id` |
| PATCH | `/v1/holders/:id` | `/v1/organizations/:organization_id/holders/:id` |
| DELETE | `/v1/holders/:id` | `/v1/organizations/:organization_id/holders/:id` |
| GET | `/v1/holders/:id/accounts` | `/v1/organizations/:organization_id/holders/:id/accounts` |
| GET | `/v1/instruments` (list) | `/v1/organizations/:organization_id/instruments` |
| POST | `/v1/holders/:holder_id/instruments` | `/v1/organizations/:organization_id/holders/:holder_id/instruments` |
| GET | `/v1/holders/:holder_id/instruments/:instrument_id` | `/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id` |
| PATCH | `/v1/holders/:holder_id/instruments/:instrument_id` | `/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id` |
| DELETE | `/v1/holders/:holder_id/instruments/:instrument_id` | `/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id` |
| DELETE | `/v1/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id` | `/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id/related-parties/:related_party_id` |
| POST (composition) | `/v1/holders/:id/accounts` | `/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts` |

13 routes total (12 CRM in `crm_routes.go` + 1 composition in `composition_routes.go`). The composition route is the only one that gains BOTH `:organization_id` and `:ledger_id`, because it creates a real ledger account.

---

## Phase 1 — Route + handler migration (Detailed)

**Phase exit criteria:** `go build ./...` succeeds; `go test ./components/ledger/internal/adapters/http/in/...` is green; every route in the target map resolves; org and (for composition) ledger arrive in handlers as path-validated UUIDs; zero `c.Get("X-Organization-Id")` / `c.Get("X-Ledger-Id")` reads remain in the four CRM/composition handler files; the `uuidFromHeader` helper and the `ledgerIDHeader`/`organizationIDHeader` constants are deleted.

### Epic 1.1: Route definitions + path-param plumbing

**Goal:** `crm_routes.go` and `composition_routes.go` register the target paths, with `ParseUUIDPathParameters` validating `organization_id` (and `ledger_id` for composition) as UUIDs on every route.
**Scope:** `components/ledger/internal/adapters/http/in/crm_routes.go`, `components/ledger/internal/adapters/http/in/composition_routes.go`, `pkg/constant/http.go`.
**Dependencies:** none.
**Done when:** the binary builds; a route-level test driving the real `ParseUUIDPathParameters` chain returns 400 for a non-UUID `organization_id` and reaches the handler for a valid one, on at least one holder route and the by-id instrument route.

#### Task 1.1.1: Add `instrument_id` to the UUID path-parameter allowlist

- [ ] Done

**Context:** `ParseUUIDPathParameters` (`pkg/net/http/withBody.go:229-250`) only UUID-parses params whose name is in `cn.UUIDPathParameters` (`pkg/constant/http.go:7-26`); non-listed params are stored as raw strings (`withBody.go:233`). `instrument_id` is absent (only the legacy `alias_id` is listed at `http.go:24`). The instrument by-id handlers call `http.GetUUIDFromLocals(c, "instrument_id")` (`instrument.go:106,166,241,398`), which type-asserts `uuid.UUID` (`pkg/net/http/httputils.go:569-572`) and would fail on a raw string. Unit tests mask this by seeding locals directly (`instrument_test.go:501`). Once Phase 1 routes go through the real chain, this latent gap becomes a live 400 on every instrument-by-id request unless fixed first.

**Implementation vision:** Add the string `"instrument_id"` to the `UUIDPathParameters` slice in `pkg/constant/http.go`. Leave `alias_id` in place (it is a separate legacy param name still referenced elsewhere; removing it is out of scope and risks unrelated breakage). Do not reorder existing entries. This is a one-line additive change; no other allowlist member changes because `organization_id` and `ledger_id` are already present (lines 9-10) and `holder_id`/`related_party_id` already present (lines 23,25).

**Files:**
- Modify: `pkg/constant/http.go:7-26`

**Verification** (run from repo root): `go build ./...` succeeds. `grep -n instrument_id pkg/constant/http.go` shows the new entry.

**Done when:** `instrument_id` is a member of `UUIDPathParameters`; build is green.

#### Task 1.1.2: Rewrite `crm_routes.go` to path-scope organization

- [ ] Done

**Context:** `RegisterCRMRoutesToApp` (`crm_routes.go:34-54`) registers 12 routes flat under `/v1/...`. Each uses `http.ProtectedRouteChain(auth.Authorize(ApplicationName, <resource>, <verb>), routeOptions, [ParseUUIDPathParameters(...)], [WithBody(...)], handler)`. `ApplicationName = "midaz"` (line 20) is the authz namespace and MUST NOT change (X1 owns the policy keys). The list routes (POST/GET `/v1/holders`, GET `/v1/instruments`) currently have NO `ParseUUIDPathParameters` call because they had no path UUIDs.

**Implementation vision:** Prefix all 12 route patterns with `/v1/organizations/:organization_id`. Per the target map, the new patterns are: `/v1/organizations/:organization_id/holders` (POST, GET list), `/v1/organizations/:organization_id/holders/:id` (GET, PATCH, DELETE), `/v1/organizations/:organization_id/holders/:id/accounts` (GET), `/v1/organizations/:organization_id/instruments` (GET list), `/v1/organizations/:organization_id/holders/:holder_id/instruments` (POST), `/v1/organizations/:organization_id/holders/:holder_id/instruments/:instrument_id` (GET, PATCH, DELETE), and `.../instruments/:instrument_id/related-parties/:related_party_id` (DELETE). Add `http.ParseUUIDPathParameters(<entityName>)` to the THREE routes that lacked it: `POST /v1/holders` (create, line 36), `GET /v1/holders` (list, line 45), and `GET /v1/instruments` (list, line 48) — each now carries `:organization_id` and must validate it. Note that `POST /v1/holders` is a CREATE, not a list; it had no `ParseUUIDPathParameters` because it previously had no path UUID, and omitting it now would let `organization_id` arrive as an unvalidated raw string into the `holders_<org>` Mongo partition — the exact bug this plan exists to kill. For routes that already call `ParseUUIDPathParameters`, the same single call now also validates `organization_id` (it iterates all path params). Keep the existing `entityName` arguments (`"holder"`, `"instruments"`, `"related-parties"`) — `entityName` only affects span attribute prefixes, not validation, and changing them is cosmetic and out of scope. Keep authz namespace/resource/action triples byte-for-byte. Do not change `routeOptions` wiring or the `hah != nil` guard for the accounts route.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/crm_routes.go:34-54`

**Verification** (run from repo root): `go build ./...` succeeds. `grep -c "/v1/organizations/:organization_id" components/ledger/internal/adapters/http/in/crm_routes.go` returns 12. `grep -c "ParseUUIDPathParameters" components/ledger/internal/adapters/http/in/crm_routes.go` returns 12 — i.e. the current 9 calls plus the 3 newly added (POST holders create, GET holders list, GET instruments list), so every route now validates at least `organization_id`.

**Done when:** all 12 CRM routes carry the org path prefix and a `ParseUUIDPathParameters` call; authz triples unchanged; build green.

#### Task 1.1.3: Rewrite `composition_routes.go` to path-scope organization and ledger

- [ ] Done

**Context:** `RegisterCompositionRoutesToApp` (`composition_routes.go:24-31`) registers POST `/v1/holders/:id/accounts` with `auth.Authorize(midazName, "accounts", "post")`, `routeOptions`, `ParseUUIDPathParameters("holder")`, `WithBody(new(mmodel.CreateHolderAccountInput), ch.CreateHolderAccount)`. The handler currently reads org and ledger from headers via `uuidFromHeader` (`composition.go:77-85`). This is the one route that legitimately needs a ledger because it creates a real ledger account.

**Implementation vision:** Change the path to `/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts`. The single `ParseUUIDPathParameters` call now validates `organization_id`, `ledger_id`, and `id` (holder) — all three are in the allowlist after Task 1.1.1 (org/ledger were already present). Keep `auth.Authorize(midazName, "accounts", "post")` unchanged. Keep `WithBody` and the handler reference unchanged. The handler rewrite (reading these from locals instead of headers) is Task 1.2.4.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/composition_routes.go:24-31`

**Verification** (run from repo root): `go build ./...` succeeds. `grep -n "/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts" components/ledger/internal/adapters/http/in/composition_routes.go` matches.

**Done when:** the composition route carries org + ledger + holder path segments; authz triple unchanged; build green.

### Epic 1.2: Handler reads org/ledger from validated path locals

**Goal:** All 12 `c.Get("X-Organization-Id")` reads and the composition header reads are replaced by `GetUUIDFromLocals`, passing org to the service as `organizationID.String()` (the service layer keeps its `string` signature; the Mongo partition is unchanged). `@Param` doc-comments are updated so generated specs are correct upstream.
**Scope:** `components/ledger/internal/adapters/http/in/holder.go`, `instrument.go`, `holder_accounts.go`, `composition.go`.
**Dependencies:** Epic 1.1 (routes must carry the path params before handlers can read them from locals).
**Done when:** `grep -rn 'c.Get("X-Organization-Id")\|c.Get("X-Ledger-Id")\|c.Get(organizationIDHeader)\|c.Get(ledgerIDHeader)' holder.go instrument.go holder_accounts.go composition.go` returns nothing; `go build ./...` green; the four files' handlers source org from locals.

#### Task 1.2.1: Migrate `holder.go` handlers to path-scoped org

- [ ] Done

**Context:** `holder.go` reads `organizationID := c.Get("X-Organization-Id")` (string) at lines 54, 101, 154, 222, 289, then passes it straight to the service (e.g. `handler.Service.CreateHolder(ctx, organizationID, payload)` at line 61). The service signature is `organizationID string` (`components/crm/services/create-holder.go:22`, `get-id-holder.go:20`, `get-all-holders.go:19`, `update-holder.go:20`, `delete-holder.go:22`). Each handler already sets `app.request.organization_id` as a span attribute from that string. The PATCH/GET/DELETE-by-id handlers already call `http.GetUUIDFromLocals(c, "id")` for the holder id — mirror that exact pattern for org.

**Implementation vision:** In each of the five handlers (`CreateHolder`, `GetHolderByID`, `UpdateHolder`, `DeleteHolderByID`, `GetAllHolders`), replace `organizationID := c.Get("X-Organization-Id")` with:
```go
organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
if err != nil {
    return http.WithError(c, err)
}
```
Then pass `organizationID.String()` to the service call (service stays `string`). Keep the span attribute as `attribute.String("app.request.organization_id", organizationID.String())`. For `CreateHolder` (line 41-71) note it currently has no `err` in scope before the org read — declare it via `:=`. For handlers that already have `err` in scope from `GetUUIDFromLocals(c, "id")` (GetHolderByID line 96, UpdateHolder line 149, DeleteHolderByID line 217), reuse `=` not `:=` to avoid shadowing, or keep `:=` only where `err` is fresh. `GetAllHolders` (line 266) has `err` from `ValidateParameters` at line 274 — place the org read after that block and use `=`. Update the `@Param X-Organization-Id header ... true` doc-comment line in each of the five blocks (lines 34, 80, 133, 201, 253) to an `@Param organization_id path string true "The unique identifier of the Organization."` line. Do NOT touch the Mongo adapter or service: the partition `holders_<organizationID>` is fed the same string value, now sourced from a validated UUID.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/holder.go` (handlers at 41, 88, 141, 209, 266; `@Param` lines at 34, 80, 133, 201, 253)

**Verification** (run from repo root): `grep -c 'c.Get("X-Organization-Id")' components/ledger/internal/adapters/http/in/holder.go` returns 0. `grep -c 'GetUUIDFromLocals(c, "organization_id")' components/ledger/internal/adapters/http/in/holder.go` returns 5. `go build ./...` green.

**Done when:** all five holder handlers source org from `organization_id` locals, pass `.String()` to the unchanged service, and carry path-param `@Param` doc-comments.

#### Task 1.2.2: Migrate `instrument.go` handlers to path-scoped org

- [ ] Done

**Context:** `instrument.go` reads `c.Get("X-Organization-Id")` at lines 62, 116, 176, 251, 339, 408 (6 sites across `CreateInstrument`, `GetInstrumentByID`, `UpdateInstrument`, `DeleteInstrumentByID`, `GetAllInstruments`, `DeleteRelatedParty`). The by-id handlers already read `holder_id` and `instrument_id` from locals via `GetUUIDFromLocals`. Service signatures take `organizationID string` (`create-instrument.go:21`, `get-id-instrument.go:20`, etc.). `GetAllInstruments` (line 304) keeps `ledger_id` as an optional query filter (`@Param ledger_id query` line 291) and `holder_id` as an optional query filter (line 284) — those are list filters and DO NOT move to the path; leave them exactly as-is.

**Implementation vision:** Same transform as Task 1.2.1: replace each `organizationID := c.Get("X-Organization-Id")` with `http.GetUUIDFromLocals(c, "organization_id")` + error guard, pass `organizationID.String()` to the service, keep span attribute via `.String()`. Mind `err` scoping per handler (most already have `err` from prior `GetUUIDFromLocals` calls — use `=`). Update the six `@Param X-Organization-Id header` doc-comments (lines 36, 89, 149, 224, 283, 376) to `@Param organization_id path string true`. Leave the `@Param ledger_id query` filter (line 291) and `@Param holder_id query` filter (line 284) on `GetAllInstruments` untouched — ledger is a filter, not a scope. Do not alter the body-field `ledger_id` validation on `CreateInstrumentInput`.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/instrument.go` (handlers at 44, 98, 158, 233, 304, 385; `@Param` org lines at 36, 89, 149, 224, 283, 376)

**Verification** (run from repo root): `grep -c 'c.Get("X-Organization-Id")' components/ledger/internal/adapters/http/in/instrument.go` returns 0. `grep -c 'GetUUIDFromLocals(c, "organization_id")' components/ledger/internal/adapters/http/in/instrument.go` returns 6. `grep -c '@Param.*ledger_id.*query' components/ledger/internal/adapters/http/in/instrument.go` still returns 1 (filter preserved). `go build ./...` green.

**Done when:** all six instrument handlers source org from locals; ledger remains a list-filter query param; build green.

#### Task 1.2.3: Migrate `holder_accounts.go` to path-scoped org

- [ ] Done

**Context:** `GetAccountsByHolder` (`holder_accounts.go:55-107`) reads `organizationID := c.Get("X-Organization-Id")` at line 77 and passes it as a string to `handler.Reader.ListAccountsByHolder(ctx, organizationID, holderID, *headerParams)`. The `HolderAccountsReader` interface (line 27-29) takes `organizationID string`. The handler already reads `holderID` from locals via `GetUUIDFromLocals(c, "id")` (line 63). The doc-comment block (lines 38-54) carries `@Param X-Organization-Id header` at line 45.

**Implementation vision:** Replace line 77 with `http.GetUUIDFromLocals(c, "organization_id")` + error guard (`err` is already in scope from line 63; use `=`). Pass `organizationID.String()` to `ListAccountsByHolder`. Keep the `HolderAccountsReader` interface signature `string` — only the call-site source changes. Update the `@Param` at line 45 to `@Param organization_id path string true`. The interface doc-comment at lines 21-26 correctly states ownership is org-global; leave that prose intact (it remains true).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/holder_accounts.go:55-107` (org read at 77; `@Param` at 45)

**Verification** (run from repo root): `grep -c 'c.Get("X-Organization-Id")' components/ledger/internal/adapters/http/in/holder_accounts.go` returns 0. `go build ./...` green.

**Done when:** the holder-accounts handler sources org from locals and passes `.String()` to the reader.

#### Task 1.2.4: Migrate `composition.go` to path-scoped org + ledger; delete header helper and constants

- [ ] Done

**Context:** `composition.go` reads org and ledger via `uuidFromHeader(c, organizationIDHeader)` (line 77) and `uuidFromHeader(c, ledgerIDHeader)` (line 82). The constants `ledgerIDHeader = "X-Ledger-Id"` and `organizationIDHeader = "X-Organization-Id"` are defined at lines 26-27. `uuidFromHeader` (lines 115-127) is a private helper used ONLY in this file (confirmed: it is the composition-route header reader). The handler already reads holder via `GetUUIDFromLocals(c, "id")` (line 72). It returns typed business errors `ErrMissingFieldsInRequest` / `ErrInvalidPathParameter`. `ParseUUIDPathParameters` already returns `ErrInvalidPathParameter` for malformed path UUIDs, so the validation semantics are preserved by the route chain.

**Implementation vision:** Replace the two `uuidFromHeader` calls (lines 77-85) with:
```go
organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
if err != nil {
    return http.WithError(c, err)
}
ledgerID, err := http.GetUUIDFromLocals(c, "ledger_id")
if err != nil {
    return http.WithError(c, err)
}
```
(reuse `=`/`:=` per scope — `err` is fresh at this point since holder read used `:=` at line 72; use `:=` on the first and `=` on the second). Keep `holderID` from `GetUUIDFromLocals(c, "id")` and the `token := c.Get("Authorization")` read (line 94 — Authorization is a legitimate header, not a scope). Keep the service call signature `CreateHolderAccount(ctx, organizationID, ledgerID, holderID, payload, token)` (it takes UUIDs, unchanged). Delete the now-unused `uuidFromHeader` helper (lines 111-127) and the `ledgerIDHeader`/`organizationIDHeader` const block (lines 25-28). Remove the now-unused `uuid` and `pkg`/`constant` imports ONLY if nothing else in the file uses them — verify with the compiler; `mmodel`, `http`, observability imports stay. Update the `@Param X-Organization-Id header` (line 47) and `@Param X-Ledger-Id header` (line 48) doc-comments to `@Param organization_id path` and `@Param ledger_id path` respectively.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/composition.go` (handler 57-109; delete consts 25-28 and helper 111-127; `@Param` 47-48)

**Verification** (run from repo root): `grep -c 'c.Get("X-Ledger-Id")\|c.Get("X-Organization-Id")\|uuidFromHeader\|ledgerIDHeader\|organizationIDHeader' components/ledger/internal/adapters/http/in/composition.go` returns 0. `go build ./...` green (no unused-import or unused-function errors).

**Done when:** composition reads org+ledger+holder from validated locals; the header helper and header-name constants are gone; build green.

### Epic 1.3: First-party test rewrites

**Goal:** The six CRM/composition test files exercise the new path-scoped routes; header-behavior cases (`missing X-Ledger-Id returns 400`, `invalid X-Organization-Id returns 400`) become path-behavior cases (`non-UUID organization_id path returns 400`, etc.); `go test ./components/ledger/internal/adapters/http/in/...` is green.
**Scope:** `components/ledger/internal/adapters/http/in/{holder_test.go, instrument_test.go, holder_accounts_test.go, crm_error_contract_test.go, composition_test.go, composition_integration_test.go}`.
**Dependencies:** Epics 1.1, 1.2.
**Done when:** the package test suite passes; no test sets `X-Organization-Id` or `X-Ledger-Id` headers for scoping; the route-shape and path-validation cases assert the new contract.

#### Task 1.3.1: Rewrite `holder_test.go` and `holder_accounts_test.go`

- [ ] Done

**Context:** `holder_test.go` sets `X-Organization-Id` on 5 request blocks and registers routes with inline middleware seeding `c.Locals(...)` (pattern visible in `instrument_test.go:334-336`). `holder_accounts_test.go` sets it once. These tests bypass `ParseUUIDPathParameters` by seeding locals directly. The handlers now read `organization_id` from locals (Task 1.2.1/1.2.3).

**Implementation vision:** For each test's inline route-setup middleware, seed `c.Locals("organization_id", orgUUID)` with a real `uuid.UUID` (mirroring how `holder_id`/`id`/`instrument_id` are already seeded). Remove every `Header.Set("X-Organization-Id", ...)` call. Update the route registration path strings to the target patterns (e.g. `/v1/organizations/:organization_id/holders/:id`) so any test that drives the real chain stays accurate; if a test seeds locals directly it does not need the path prefix, but update it for documentation fidelity. Use fixed UUIDs from the existing test fixtures (do not call `time.Now()`; that rule is about timestamps, but reuse the suite's existing fixed-UUID helpers for org). Where a test asserted a missing/blank org returned an error via the handler, convert it to seed no `organization_id` local (so `GetUUIDFromLocals` returns `ErrInvalidPathParameter` → handled).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/holder_test.go`
- Modify: `components/ledger/internal/adapters/http/in/holder_accounts_test.go`

**Verification** (run from repo root): `go test ./components/ledger/internal/adapters/http/in/ -run 'TestHolder' -count=1` passes. `grep -c "X-Organization-Id" components/ledger/internal/adapters/http/in/holder_test.go components/ledger/internal/adapters/http/in/holder_accounts_test.go` returns 0 each.

**Done when:** both files seed org via locals, set no scoping headers, and pass.

#### Task 1.3.2: Rewrite `instrument_test.go`

- [ ] Done

**Context:** `instrument_test.go` sets `X-Organization-Id` on 12 sites and already seeds `holder_id`/`instrument_id`/`related_party_id` locals inline (lines 334-336, 498-505, 756-759, 877-880, 986-990, 1193). The instrument handlers now read `organization_id` from locals (Task 1.2.2). The `GetAllInstruments` test (line 1014, route at 1193) keeps `ledger_id`/`holder_id` as query filters.

**Implementation vision:** Add `c.Locals("organization_id", orgUUID)` to every inline route-setup middleware that previously relied on the header. Delete all 12 `Header.Set("X-Organization-Id", ...)` calls. Update route path strings to target patterns. For `GetAllInstruments`, keep exercising `ledger_id` and `holder_id` as query-string filters (they did not move) — assert the filter still flows to the service. Reuse fixed UUID fixtures.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/instrument_test.go`

**Verification** (run from repo root): `go test ./components/ledger/internal/adapters/http/in/ -run 'TestInstrument' -count=1` passes. `grep -c "X-Organization-Id" components/ledger/internal/adapters/http/in/instrument_test.go` returns 0.

**Done when:** all instrument tests seed org via locals, preserve ledger/holder as list filters, set no scoping headers, and pass.

#### Task 1.3.3: Rewrite `crm_error_contract_test.go`

- [ ] Done

**Context:** This is the dedicated CRM error-contract suite asserting canonical error codes (e.g. "missing required fields emits canonical 0009 not CRM-0003", names at lines 65, 74, 83, 99). It sets `X-Organization-Id` at lines 125 and 180 to satisfy the old header scope while exercising body/validation error paths.

**Implementation vision:** Replace the two `c.Request().Header.Set("X-Organization-Id", orgID)` calls with `c.Locals("organization_id", orgUUID)` seeding in the test's route-setup middleware (or seed via the request path if the test drives the real chain — match the file's existing pattern). The error-contract assertions themselves (canonical code mapping) are unaffected by the scope-source change and must stay byte-identical. Only the mechanism for supplying org changes.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/crm_error_contract_test.go` (lines 125, 180)

**Verification** (run from repo root): `go test ./components/ledger/internal/adapters/http/in/ -run 'CRMError\|ErrorContract' -count=1` passes. `grep -c "X-Organization-Id" components/ledger/internal/adapters/http/in/crm_error_contract_test.go` returns 0.

**Done when:** the error-contract suite supplies org via locals and all canonical-code assertions still pass.

#### Task 1.3.4: Rewrite `composition_test.go` and `composition_integration_test.go` — header cases become path cases

- [ ] Done

**Context:** `composition_test.go` bakes header behavior into named cases: `"missing X-Ledger-Id header returns 400"` (line 122, expectedStatus 400 line 128) and `"invalid X-Organization-Id header returns 400"` (line 136, expectedStatus 400 line 142). The test app registers `/v1/holders/:id/accounts` (line 158, 196) and issues requests to that path (line 168, 207). `composition_integration_test.go` sets both headers (around line 242). The composition handler now reads org+ledger+holder from path locals via the real `ParseUUIDPathParameters` chain (Task 1.1.3, 1.2.4).

**Implementation vision:** Rewrite the two named header cases as path cases:
- `"missing X-Ledger-Id header returns 400"` → `"non-UUID ledger_id path segment returns 400"`: issue a request to `/v1/organizations/<validOrg>/ledgers/not-a-uuid/holders/<holder>/accounts` and assert 400 (produced by `ParseUUIDPathParameters` → `ErrInvalidPathParameter`). A genuinely missing ledger segment is no longer expressible (the route won't match → 404), so the "missing" semantics are replaced by "malformed" — note this in a code comment so the intent is clear.
- `"invalid X-Organization-Id header returns 400"` → `"non-UUID organization_id path segment returns 400"`: request `/v1/organizations/not-a-uuid/ledgers/<validLedger>/holders/<holder>/accounts`, assert 400.
Update the route registration in the test app to the full target path `/v1/organizations/:organization_id/ledgers/:ledger_id/holders/:id/accounts` and chain the real `http.ParseUUIDPathParameters("holder")` so path validation is actually exercised (these cases must drive the real validator, not seeded locals — that is the point of the case). For the happy-path composition test and `composition_integration_test.go`, switch from header sets to building the full path with valid org/ledger UUIDs in the URL; remove both `Header.Set` calls. Keep the `Authorization` header where present (it is not a scope header).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/composition_test.go` (cases at 122-142; route reg 158, 196; request URLs 168, 207)
- Modify: `components/ledger/internal/adapters/http/in/composition_integration_test.go` (header sets ~242)

**Verification** (run from repo root): `go test ./components/ledger/internal/adapters/http/in/ -run 'Composition' -count=1` passes (integration test may require its testcontainer tag — run with the build tag the file declares). `grep -c "X-Ledger-Id\|X-Organization-Id" components/ledger/internal/adapters/http/in/composition_test.go components/ledger/internal/adapters/http/in/composition_integration_test.go` returns 0 each. The two converted cases assert 400 via path validation.

**Done when:** composition tests drive the path-scoped route; the former header-missing/invalid cases are path-malformed cases producing 400; both files set no scoping headers and pass.

---

## Phase 2 — Artifact regeneration (Epic-level)

**Phase exit criteria:** Generated OpenAPI/Swagger and the Postman collection reflect the path-scoped contract with zero `X-Organization-Id`/`X-Ledger-Id` scope parameters on CRM/composition operations; Postman CRM requests point at the unified `:3002` base, not the stale `{{onboardingUrl}}`. No generated file is hand-edited.

### Epic 2.1: Swagger / OpenAPI regeneration

**Goal:** `components/ledger/api/{swagger.yaml,swagger.json,openapi.yaml,docs.go}` regenerate from the updated `@Param` doc-comments (Phase 1), showing org as a path parameter on all 13 CRM/composition operations and `X-Ledger-Id` gone.
**Scope:** swag generation pipeline (`make generate-docs` / `postman/generator/generate-docs.sh`, which covers ledger), generated artifacts under `components/ledger/api/`.
**Dependencies:** Phase 1 (doc-comments are the upstream source).
**Done when:** running the generator produces a clean diff containing only the intended path-param flips; `grep -rc "X-Organization-Id" components/ledger/api/` shows the CRM/composition occurrences gone (fees occurrences, if co-located, remain — fees is out of scope); no manual edits to generated files; the diff is committed as a generation artifact.

### Epic 2.2: Postman collection + specs regeneration and base-URL fix

**Goal:** `postman/MIDAZ.postman_collection.json` and `postman/specs/ledger/*` regenerate so every CRM request uses path segments for org (and org+ledger for composition), drops the `X-Organization-Id`/`X-Ledger-Id` header rows, and uses the unified `:3002` base instead of `{{onboardingUrl}}`/`{{onboardingPort}}`.
**Scope:** `postman/generator/` (Node generator + config), `postman/MIDAZ.postman_collection.json`, `postman/MIDAZ.postman_environment.json`, `postman/specs/ledger/*`. `postman/backups/*` left untouched (historical snapshots).
**Dependencies:** Epic 2.1 (Postman generator consumes the regenerated OpenAPI spec).
**Done when:** the regenerated collection's CRM requests resolve against `:3002` with org in the URL; no CRM request carries a scoping header row; `postman/backups/*` is unchanged; the base-URL fix is verified by inspecting at least one holder and the composition request; generator config (not the output) is the only hand-edited surface if the base-URL var needs correcting.

---

## Phase 3 — Documentation reversal (Epic-level)

**Phase exit criteria:** Prose docs no longer describe header-scoped CRM as a locked convention; `SCOPING.md` explicitly reverses R22 and names fees as the single remaining tracked header-scoped exception; `llms-full.txt` and `RBAC-NAMESPACES.md` are consistent with the new route inventory.

### Epic 3.1: Rewrite SCOPING.md, llms-full.txt, RBAC cross-refs

**Goal:** `docs/api/SCOPING.md` is rewritten (not patched) to state CRM is now path-scoped on org, R22 reversed, with fees named as the remaining header-scoped exception (shared `X-Organization-Id` constant, tracked under the auth/X1 plan); `llms-full.txt` route inventory and architecture overview reflect the 13 new paths; `docs/auth/RBAC-NAMESPACES.md` cross-references are updated so the X1 policy-migration narrative still lines up (namespace/resource keys unchanged — only route shape changed).
**Scope:** `docs/api/SCOPING.md`, `llms-full.txt`, `docs/auth/RBAC-NAMESPACES.md`.
**Dependencies:** Phase 1 (route inventory is the source of truth); Phase 2 (artifacts are the published contract the docs point to).
**Done when:** `SCOPING.md` describes only one remaining header-scoped surface (fees) and names it explicitly; `llms-full.txt` lists the path-scoped CRM routes with `:organization_id`; `RBAC-NAMESPACES.md` cross-refs are accurate; no doc still asserts CRM header-scoping is the intended convention.

---

## Phase 4 — Hardening follow-ons (Epic-level)

**Phase exit criteria:** Each follow-on is gated on a named decision point and does not block the Phase 1-3 clean break. None changes authz namespace/resource keys (X1 owns policy migration).

### Epic 4.1: Org-binding authz check (coordinated with X1)

**Goal:** Authorization for CRM routes binds the grant to the path `organization_id` so a principal authorized for `midaz:holders:*` cannot operate on an org outside its grant — closing the residual horizontal-privilege gap that path-validation alone does not fix.
**Scope:** authz middleware integration for CRM routes; coordination with the X1 tenant-manager policy migration (`plugin-crm:* → midaz:{holders,instruments}:*`).
**Dependencies:** Phase 1; the X1 policy migration (Fred-owned release gate); a decision point with plugin-auth on whether grants bind to org as a resource-instance dimension or via tenant scoping.
**Done when:** **Decision point** — plugin-auth confirms the org-binding mechanism (resource-instance authz vs per-org tenant grant). After that decision, a request whose path org differs from the principal's authorized org is rejected; covered by tests. Until the decision, this epic is parked.

### Epic 4.2: Idempotency keys on CreateHolder / CreateInstrument

**Goal:** `CreateHolder` and `CreateInstrument` accept an idempotency key (mirroring the transaction-create idempotency claim) so a client can safely retry on timeout without minting duplicates.
**Scope:** CRM create handlers + use cases (`components/crm/services/create-holder.go`, `create-instrument.go`), idempotency claim store.
**Dependencies:** Phase 1.
**Done when:** **Decision point** — choose the idempotency backing (reuse the transaction idempotency mechanism vs a CRM-local claim) and the key header name. After that, a retried create with the same key returns the original result without a duplicate; covered by tests.

### Epic 4.3: Instrument-create referential validation

**Goal:** `CreateInstrument` verifies the referenced `ledger_id`/`account_id` actually exist within the caller's org/ledger before writing the org-partitioned instrument record, instead of trusting body strings.
**Scope:** `components/crm/services/create-instrument.go` and the ledger/account lookup it would call.
**Dependencies:** Phase 1; Epic 4.1 (org binding makes the referential check meaningful).
**Done when:** **Decision point** — confirm the lookup path (cross-store read into the ledger account query) and the failure mode (422 vs 404). After that, creating an instrument against a non-existent ledger/account in the org is rejected; covered by tests.

---

## Out of Scope

- Fees/billing path reshape — shares the `X-Organization-Id` constant (`fees_middlewares.go:19`); tracked as the remaining header-scoped exception, hardening owned by the auth plan.
- RBAC namespace/resource renames — X1 owns policy migration; this plan changes route shape only, not authz keys (`ApplicationName = "midaz"` and all `Authorize(...)` triples stay byte-identical).
- `postman/backups/*` — left untouched (historical snapshots).
- Removing the legacy `alias_id` allowlist entry / renaming internal `alias` symbols to `instrument` — unrelated cleanup, not required for scoping.

---

## Self-Review

- **Spec coverage:** crm_design recommendations (Option A path-scope org, drop X-Ledger-Id from pure-CRM, validate org as UUID, keep ledger as body field/list filter) → Phase 1 (Epics 1.1-1.3). crm_blast_radius surfaces: handler source + @Param (Phase 1), generated Swagger/OpenAPI (Epic 2.1), Postman + base-URL (Epic 2.2), in-repo tests (Epic 1.3), prose docs (Phase 3). crm_design problems: tenant-isolation-on-unvalidated-header → Epic 1.1/1.2 (path UUID validation) + Epic 4.1 (authz binding); idempotency gap → Epic 4.2; instrument referential gap → Epic 4.3. Composition X-Ledger-Id subtlety → Tasks 1.1.3, 1.2.4, 1.3.4. No spec requirement left uncovered.
- **Ground-truth corrections baked in:** instruments already nested (not top-level by-id); `related-parties` route exists today; `org/ledger` already in `UUIDPathParameters`; `instrument_id` allowlist gap caught and given a prerequisite task (1.1.1).
- **Vagueness scan:** detailed tasks name exact files, line ranges, grep verifications, and per-handler `err`-scoping edge cases. No "appropriate"/"TBD"/unnamed edge cases in the detailed wave.
- **Contract consistency:** the target route map is the single contract; every Phase-1 task and Phase 2/3 epic references it. Service signatures stay `organizationID string`; handlers pass `.String()` — stated identically across Tasks 1.2.1-1.2.4. Authz triples explicitly held constant everywhere.
- **Phase boundaries:** Phase 1 ends with a building, test-green binary serving the new routes; Phase 2 ends with regenerated published artifacts; Phase 3 ends with consistent docs; Phase 4 epics are independently gated and parked on decision points.
- **Verification plausibility:** all commands target real paths verified in this repo (`go build ./...`, scoped `go test` runs, greps against named files).
