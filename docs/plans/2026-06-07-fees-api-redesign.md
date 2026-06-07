# Fees API Redesign — Path-Scoped Organization Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Move the fees/billing surface (packages, estimates, billing-packages, billing-calculate) from header-scoped organization (`X-Organization-Id`) to path-scoped organization (`/v1/organizations/{organization_id}/...`), retiring the last header-scoped surface in the unified binary. This is the fees analogue of the CRM redesign (`docs/plans/2026-06-06-crm-api-redesign.md`), which explicitly named fees as the remaining exception. Unlike CRM, fees has no unvalidated-string-into-collection-name bug (org is a UUID-validated field filter, not a collection partition) — the motivation here is API consistency, killing the bespoke fee middlewares in favor of the standard `ParseUUIDPathParameters` chain, and closing `SCOPING.md` with zero header-scoped exceptions.

**Architecture:** Clean break, pre-GA, no dual-routing. Organization reaches handlers only as a path-validated `uuid.UUID` via the standard `http.ParseUUIDPathParameters` middleware, read with `http.GetUUIDFromLocals(c, "organization_id")` — the exact pattern CRM and every native ledger entity use. Both bespoke fee middlewares (`parseFeeHeaderParameters`, `parseFeePathParameters`) die: a single `ParseUUIDPathParameters` call validates org AND the resource `:id` together. Ledger stays where it is today — a create-body field (`CreatePackageInput.LedgerID`, `FeeEstimate.LedgerID`, `BillingCalculateRequest.LedgerID`) and an optional list filter (`ledgerId` query) — never a path scope, matching the CRM precedent (ledger appears in the path only where a real ledger resource is touched; no fee route does). Authz namespace `plugin-fees` and all `Authorize(...)` triples stay byte-identical (R9: tenant-manager RBAC policies key on these strings).

**Tech Stack:** Go 1.26, Fiber v2, `ParseUUIDPathParameters`/`ProtectedRouteChain`/`GetUUIDFromLocals` (`pkg/net/http`), MongoDB (field-filtered `package`/`billing_package` collections, tenant-first DB resolution), swaggo/swag (generated OpenAPI), Node postman generator (`postman/generator/`), lib-auth v2 (authz, unchanged namespace/resource keys).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | All 12 fee/billing routes are path-scoped on org; both bespoke fee middlewares deleted; handlers read org via `GetUUIDFromLocals`; dead FEE-0020 sentinel removed; route tests pass against the new shapes; binary builds and serves the new routes | 1.1, 1.2, 1.3 | ✅ Done |
| 2 | Generated artifacts (Swagger/OpenAPI + Postman) reflect the path-scoped contract — **executes inside the Phase 1 commit** (see Known Constraints) | 2.1, 2.2 | ✅ Done (rode in the Phase 1 commit as planned) |
| 3 | Prose docs aligned: `SCOPING.md` rewritten to zero header-scoped exceptions, `llms-full.txt` fee route inventory, `RBAC-NAMESPACES.md` cross-refs (keys unchanged) | 3.1 | Pending |
| 4 | Hardening follow-ons gated on named decisions: org-bound authz (shared decision with CRM Epic 4.1) | 4.1 | Parked on decision point |

### Execution Notes (2026-06-07)

- **`:id` reads could not stay literally untouched.** The ground truth said handler `:id` reads keep working unchanged, but they referenced the `feeUUIDPathParameter` constant that dies with `fees_middlewares.go`. All `:id` reads were normalized to `http.GetUUIDFromLocals(c, "id")` — same guard pattern as org, which also removed the unchecked-assert panic risk on the package by-id handlers and the FEE-coded checked `id` blocks on the billing by-id handlers (a read style the ground truth had catalogued only for org).
- **One sentinel straggler outside the plan's grep:** `feeshared/errors_test.go:517` referenced FEE-0020 (the ground-truth sweep excluded `_test.go`). The test-table entry was removed with the sentinel.
- **Postman `{id}` heuristic struck exactly as predicted** (risk register item 1): `/packages/{id}` and `/billing-packages/{id}` rendered `{{organizationId}}`. Fixed at both mapping sites in `postman/generator/convert-openapi.js` (`createUrl` first-match-returns → new checks BEFORE the `/organizations/` fallback; `addParameters` last-sequential-if-wins → new checks AFTER `/holders/`). `{{packageId}}`/`{{billingPackageId}}` follow the `{{holderId}}` precedent: collection-level variables, not added to the environment template.
- **Auth-protection test simplified:** the per-route `if path == ...` ID substitutions became a generic `concreteFeePath` helper replacing `:organization_id`/`:id` with fixed UUID literals.
- **Swagger diff noise note:** the regenerated spec re-sorts paths, so the raw diff pairs unrelated lines (it can look like tag/description changes). Verified: `BillingPackages` tag intact, Authorization descriptions byte-identical in HEAD vs regenerated, all 6 fee path keys org-prefixed.

### Known Constraints (lessons inherited from the CRM execution)

- **Phase 2 rides in the Phase 1 commit.** `TestContractSpecMatchesRoutes` makes the router and `swagger.json` one atomic contract; a green Phase 1 commit requires the regenerated artifacts. Run `make generate-docs` before committing Phase 1.
- **`@Router` doc-comments must flip together with `@Param`.** swag reads both; flipping only `@Param` regenerates a spec contradicting the live router. All 12 operations across 4 handler files carry both.
- **Postman generator heuristics need inspection.** The contextual `{id}` mapping bit CRM (`/holders/{{organizationId}}`). Verify the regenerated fee requests render `{organization_id}` → `{{organizationId}}` and a sane variable for `{id}` on `/packages/{id}` and `/billing-packages/{id}`; fix mapping sites in `postman/generator/` (not the output) if not.
- **Error-contract normalization is intentional.** Path validation through the standard chain returns the canonical midaz `ErrInvalidPathParameter` envelope, replacing the fee-shim codes (`FEE-0019` invalid header, `FEE-0020` missing header, FEE-coded invalid `:id`). This is the same normalization the CRM collapse locked in (`validation_error_returns_canonical_midaz_code_not_crm_shim`). Business errors from fee use cases keep their FEE codes — only the scoping/path-validation layer normalizes.

---

## Ground Truth (verified against the repo on 2026-06-07)

- **Fees routes already use `http.ProtectedRouteChain`** (`fees_routes.go:40-57`) — unlike pre-redesign CRM, no chain-shape migration is needed; the change is swapping `parseFeeHeaderParameters` (+ `parseFeePathParameters` on by-id routes) for one `http.ParseUUIDPathParameters(<entity>)` call per route.
- **`organization_id` and `id` are already in `UUIDPathParameters`** (`pkg/constant/http.go:8-9`) — no allowlist change needed (CRM needed `instrument_id`; fees needs nothing).
- **Two bespoke middlewares own scoping today** (`fees_middlewares.go`): `parseFeeHeaderParameters` (lines 43-58) validates `X-Organization-Id` as UUID and stores it in locals under the literal header name `"X-Organization-Id"`; `parseFeePathParameters` (lines 24-39) validates `:id` and stores under `"id"`. Both return FEE-coded error envelopes via `feehttp.WithError`. The file contains nothing else but these two functions and the two constants (`feeUUIDPathParameter`, `feeOrgIDHeaderParameter`) — it dies entirely.
- **Handler org reads come in two styles, 12 sites total:**
  - *Unchecked type asserts* (panic if middleware absent): `fees_package_handler.go:77,167,232,281,366`, `fees_handler.go:62`.
  - *Checked asserts re-returning FEE-0020*: `billing_package_handler.go:67,133,231,291,368`, `billing_calculate_handler.go:64`.
  - All 12 become the standard `http.GetUUIDFromLocals(c, "organization_id")` + error guard, which both fixes the panic risk and deletes the re-check blocks.
- **`:id` reads keep working without edits.** `ParseUUIDPathParameters` stores `"id"` in locals as `uuid.UUID` — the same key and type `parseFeePathParameters` used. Handler `:id` reads are untouched (smallest correct change); only the org local key changes (`"X-Organization-Id"` → `"organization_id"`).
- **`CalculateBilling` injects org into the payload** (`billing_calculate_handler.go:77`: `payload.OrganizationID = organizationID.String()`) before `validateBillingCalculateRequest` checks it (`:112`). Mechanics unchanged — only the source of `organizationID` flips to the path local. The FEE-0019 use at `:113` is body-field validation (effectively unreachable since the handler injects), NOT a header read — leave it.
- **Sentinel afterlife:** after Phase 1, `ErrHeaderParameterRequired` (FEE-0020, `feeshared/constant/errors.go:32`) has zero references → delete it and its `feeshared/errors.go:399-403` mapping entry. `ErrInvalidHeaderParameter` (FEE-0019) keeps one reference (`billing_calculate_handler.go:113`) → stays.
- **Authz:** `feesApplicationName = "plugin-fees"` (`fees_routes.go:19`, R9 — MUST NOT be renamed). All 12 triples (`packages`/`estimates`/`billing-packages`/`billing-calculate` × verbs) stay byte-identical. `RBAC-NAMESPACES.md:203` records `plugin-fees:*` as intentionally unmigrated — route shape change does not touch policy keys, so there is NO X1-style release gate for fees.
- **No `X-Ledger-Id` anywhere in fees** — ledger is already body/filter only. Nothing to delete on that axis.
- **Mongo is field-filtered, not partitioned:** collections `package`/`billing_package` (`feeshared/constant/mongo.go`), filter `queryFilter["organization_id"] = organizationID` (`mongodb/fees/pack/find.go:76`, `billing_package/find.go:113`). DB resolution is tenant-first (`tmcore.GetMBContext`). No adapter change in this plan.
- **In-process seam is unaffected:** the transaction fee engine is called via the `FeeApplier` port (`transaction_fee_application.go:54-83`), never over HTTP. `POST /v1/fees` is intentionally not mounted. The 8 `transaction_fee_*` test files exercise the seam, not the routes — untouched.
- **Test surface is one file:** `fees_routes_test.go` (route table at lines 21-37; registration tests at 43, 63, 78; auth-protection sweep at 106-147). Zero fee tests set `X-Organization-Id`; there are no fee handler unit-test files. No `tests/`, k6, or chaos suite calls fee endpoints.
- **Generated-artifact footprint:** 12 `X-Organization-Id` header params in each of `components/ledger/api/{swagger.yaml,swagger.json,openapi.yaml,docs.go}` and mirrors under `postman/specs/ledger/`; 12+ header rows in `postman/MIDAZ.postman_collection.json` (Packages, BillingPackages, BillingCalculate folders).

### Current → Target route map (authoritative)

| Method | Current path | Target path |
|--------|--------------|-------------|
| POST | `/v1/packages` | `/v1/organizations/:organization_id/packages` |
| GET | `/v1/packages` (list) | `/v1/organizations/:organization_id/packages` |
| GET | `/v1/packages/:id` | `/v1/organizations/:organization_id/packages/:id` |
| PATCH | `/v1/packages/:id` | `/v1/organizations/:organization_id/packages/:id` |
| DELETE | `/v1/packages/:id` | `/v1/organizations/:organization_id/packages/:id` |
| POST | `/v1/estimates` | `/v1/organizations/:organization_id/estimates` |
| POST | `/v1/billing-packages` | `/v1/organizations/:organization_id/billing-packages` |
| GET | `/v1/billing-packages` (list) | `/v1/organizations/:organization_id/billing-packages` |
| GET | `/v1/billing-packages/:id` | `/v1/organizations/:organization_id/billing-packages/:id` |
| PATCH | `/v1/billing-packages/:id` | `/v1/organizations/:organization_id/billing-packages/:id` |
| DELETE | `/v1/billing-packages/:id` | `/v1/organizations/:organization_id/billing-packages/:id` |
| POST | `/v1/billing/calculate` | `/v1/organizations/:organization_id/billing/calculate` |

12 routes total, all in `fees_routes.go`. No route gains `:ledger_id` — no fee route touches a ledger resource directly (ledger remains a body field on creates/estimates/calculate and a `ledgerId` query filter on lists).

---

## Phase 1 — Route + middleware + handler migration (Detailed)

**Phase exit criteria:** `go build ./...` succeeds; `go test ./components/ledger/internal/adapters/http/in/...` is green; every route in the target map resolves; org arrives in handlers as a path-validated UUID via `GetUUIDFromLocals`; `fees_middlewares.go` is deleted; `grep -rn "X-Organization-Id" components/ledger/internal/adapters/http/in/` returns zero hits in fee files; FEE-0020 sentinel and mapping removed; `make generate-docs` artifacts regenerated (Phase 2 rides along — see Known Constraints).

### Epic 1.1: Route definitions

**Goal:** `fees_routes.go` registers the 12 target paths with `http.ParseUUIDPathParameters` validating `organization_id` (and `id` where present) on every route; both bespoke middlewares are gone from every chain.
**Scope:** `components/ledger/internal/adapters/http/in/fees_routes.go`.
**Dependencies:** none (allowlist already covers `organization_id` and `id`).
**Done when:** the binary builds; a route-level test driving the real chain returns 400 (canonical `ErrInvalidPathParameter`) for a non-UUID `organization_id` and reaches the handler for a valid one, on at least one package route and one billing route.

#### Task 1.1.1: Rewrite `fees_routes.go` to path-scope organization

- [x] Done

**Context:** `RegisterFeesRoutesToApp` (`fees_routes.go:30-58`) registers 12 routes flat under `/v1/...` via `http.ProtectedRouteChain(auth.Authorize(feesApplicationName, <resource>, <verb>), routeOptions, parseFeeHeaderParameters, [parseFeePathParameters,] [feehttp.WithBodyTracing(...),] handler)`. `feesApplicationName = "plugin-fees"` (line 19) MUST NOT change (R9). The by-id routes carry both bespoke middlewares; list/create routes carry only the header one.

**Implementation vision:** Prefix all 12 route patterns with `/v1/organizations/:organization_id` per the target map. In every chain, replace `parseFeeHeaderParameters` (and `parseFeePathParameters` where present) with a single `http.ParseUUIDPathParameters(<entityName>)` — it validates ALL path params present (org on every route; org+id on by-id routes) and stores them in locals (`organization_id` and `id`, both already allowlisted). Use entity names `"packages"`, `"estimates"`, `"billing-packages"`, `"billing-calculate"` per resource group (entityName only affects span attribute prefixes, not validation). Keep `feehttp.WithBodyTracing(...)` body binding exactly as-is (converging body binding onto `http.WithBody` is out of scope — it would flip body-validation error envelopes, a separate contract decision). Keep authz triples byte-for-byte. Keep `routeOptions` wiring and the registrar untouched. Update the `RegisterFeesRoutesToApp` doc comment: it currently describes the header-scoped shape.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/fees_routes.go:21-58`

**Verification** (run from repo root): `go build ./...` succeeds. `grep -c "/v1/organizations/:organization_id" components/ledger/internal/adapters/http/in/fees_routes.go` returns 12. `grep -c "ParseUUIDPathParameters" components/ledger/internal/adapters/http/in/fees_routes.go` returns 12. `grep -c "parseFee" components/ledger/internal/adapters/http/in/fees_routes.go` returns 0. `grep -c 'feesApplicationName = "plugin-fees"' components/ledger/internal/adapters/http/in/fees_routes.go` returns 1.

**Done when:** all 12 fee routes carry the org path prefix and one `ParseUUIDPathParameters` call each; no bespoke middleware references remain; authz triples unchanged; build green.

### Epic 1.2: Handlers read org from validated path locals; bespoke middlewares die

**Goal:** All 12 org-read sites use `http.GetUUIDFromLocals(c, "organization_id")` + error guard; the checked-assert FEE-0020 blocks in billing handlers are deleted; `fees_middlewares.go` is deleted; the dead FEE-0020 sentinel and its mapping are removed; `@Param`/`@Router` doc-comments flip to the path-scoped contract.
**Scope:** `components/ledger/internal/adapters/http/in/{fees_package_handler.go, fees_handler.go, billing_package_handler.go, billing_calculate_handler.go, fees_middlewares.go}`, `components/ledger/pkg/feeshared/constant/errors.go`, `components/ledger/pkg/feeshared/errors.go`.
**Dependencies:** Epic 1.1 (routes must carry the path param before handlers read it from locals).
**Done when:** `grep -rn "feeOrgIDHeaderParameter\|X-Organization-Id" components/ledger/internal/adapters/http/in/fees_package_handler.go components/ledger/internal/adapters/http/in/fees_handler.go components/ledger/internal/adapters/http/in/billing_package_handler.go components/ledger/internal/adapters/http/in/billing_calculate_handler.go` returns nothing; `fees_middlewares.go` does not exist; build green.

#### Task 1.2.1: Migrate `fees_package_handler.go` (5 handlers)

- [x] Done

**Context:** `CreatePackage` (:77), `GetAllPackages` (:167), `GetPackageByID` (:232), `UpdatePackageByID` (:281), `DeletePackageByID` (:366) each read `organizationID := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID)` — an UNCHECKED assert that panics if the local is absent. Service methods take `organizationID uuid.UUID` (unchanged). `@Param X-Organization-Id header` lines at 52, 145, 215, 263, 349; `@Router` lines at 62 (`/v1/packages [post]`), 158, 223, 272, 357. `GetAllPackages` keeps `segmentId`/`ledgerId` query filters (lines 146-147) — they do not move.

**Implementation vision:** Replace each unchecked assert with:
```go
organizationID, err := http.GetUUIDFromLocals(c, "organization_id")
if err != nil {
    return http.WithError(c, err)
}
```
Mind `err` scoping per handler (`:=` where fresh, `=` where an earlier read declared it). The `:id` reads stay as they are (same local key/type as before). Span attributes keep `organizationID.String()`. Flip the five `@Param X-Organization-Id header string true` lines to `@Param organization_id path string true "The unique identifier of the Organization."` and prefix the five `@Router` paths with `/v1/organizations/{organization_id}`. Leave the `ledgerId`/`segmentId` query `@Param` lines untouched. Add the `pkg/net/http` import if not already present (it may collide with the `feehttp` alias — both stay; the canonical package keeps its conventional name).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/fees_package_handler.go`

**Verification** (run from repo root): `grep -c 'feeOrgIDHeaderParameter' components/ledger/internal/adapters/http/in/fees_package_handler.go` returns 0. `grep -c 'GetUUIDFromLocals(c, "organization_id")' components/ledger/internal/adapters/http/in/fees_package_handler.go` returns 5. `grep -c '@Router.*\/v1\/organizations\/{organization_id}\/packages' components/ledger/internal/adapters/http/in/fees_package_handler.go` returns 5. `go build ./...` green.

**Done when:** all five package handlers source org from validated path locals with checked errors; doc-comments match the live router.

#### Task 1.2.2: Migrate `fees_handler.go` (estimates)

- [x] Done

**Context:** `EstimateFeeCalculation` (:62) does the same unchecked assert. `@Param X-Organization-Id header` at line 44; `@Router /v1/estimates [post]` at line 53. `FeeEstimate.LedgerID` stays a body field.

**Implementation vision:** Same transform as Task 1.2.1 — one site. Flip `@Param` to `organization_id path` and `@Router` to `/v1/organizations/{organization_id}/estimates [post]`.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/fees_handler.go`

**Verification** (run from repo root): `grep -c 'feeOrgIDHeaderParameter' components/ledger/internal/adapters/http/in/fees_handler.go` returns 0. `go build ./...` green.

**Done when:** the estimate handler sources org from validated path locals; doc-comments match.

#### Task 1.2.3: Migrate `billing_package_handler.go` and `billing_calculate_handler.go` (6 handlers, checked asserts)

- [x] Done

**Context:** These handlers use CHECKED asserts returning FEE-0020 when the local is missing: `billing_package_handler.go:67-69, 133-135, 231-233, 291-293, 368-370` and `billing_calculate_handler.go:64-67`. Each block is `orgVal, orgOK := c.Locals(feeOrgIDHeaderParameter).(uuid.UUID); if !orgOK { return feehttp.WithError(c, feeerrors.ValidateBusinessError(feeconstant.ErrHeaderParameterRequired, "", feeOrgIDHeaderParameter)) }` followed by `organizationID := orgVal`. `CalculateBilling` then injects `payload.OrganizationID = organizationID.String()` (:77) — that line stays, only the source flips. `@Param X-Organization-Id header` at `billing_package_handler.go:50,114,214,272,351` and `billing_calculate_handler.go:46`; `@Router` at `billing_package_handler.go:58,124,222,282,359` and `billing_calculate_handler.go:55`. The FEE-0019 use inside `validateBillingCalculateRequest` (`billing_calculate_handler.go:113`) is body-field validation — leave it untouched.

**Implementation vision:** Replace each checked-assert block (assert + `if !orgOK` + `organizationID := orgVal`) with the standard `GetUUIDFromLocals(c, "organization_id")` + error guard — three lines replace five, and the missing-local error becomes the canonical envelope instead of FEE-0020. Flip the six `@Param` lines to `organization_id path` and prefix the six `@Router` paths (`/v1/organizations/{organization_id}/billing-packages...`, `/v1/organizations/{organization_id}/billing/calculate`). `GetAllBillingPackages` keeps its `ledgerId`/`type` query filters (lines 115-116) untouched. Remove `feeconstant`/`feeerrors` imports from these files ONLY if the compiler confirms nothing else uses them (`billing_calculate_handler.go` keeps both — `:113` and other business-error sites; check `billing_package_handler.go` per remaining usage).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/billing_package_handler.go`
- Modify: `components/ledger/internal/adapters/http/in/billing_calculate_handler.go`

**Verification** (run from repo root): `grep -c 'feeOrgIDHeaderParameter\|ErrHeaderParameterRequired' components/ledger/internal/adapters/http/in/billing_package_handler.go components/ledger/internal/adapters/http/in/billing_calculate_handler.go` returns 0 each. `grep -c 'GetUUIDFromLocals(c, "organization_id")' components/ledger/internal/adapters/http/in/billing_package_handler.go` returns 5; same grep on `billing_calculate_handler.go` returns 1. `go build ./...` green.

**Done when:** all six billing handlers source org from validated path locals; the FEE-0020 re-check blocks are gone; payload org injection in `CalculateBilling` preserved.

#### Task 1.2.4: Delete `fees_middlewares.go`; remove the dead FEE-0020 sentinel

- [x] Done

**Context:** After Tasks 1.1.1-1.2.3, nothing references `parseFeeHeaderParameters`, `parseFeePathParameters`, `feeOrgIDHeaderParameter`, or `feeUUIDPathParameter` — the file's entire content. `ErrHeaderParameterRequired` (FEE-0020) then has zero references: sentinel at `components/ledger/pkg/feeshared/constant/errors.go:32`, mapping entry at `components/ledger/pkg/feeshared/errors.go:399-403`. `ErrInvalidHeaderParameter` (FEE-0019) keeps its `billing_calculate_handler.go:113` reference and STAYS (sentinel `errors.go:31`, mapping `errors.go:393-397`).

**Implementation vision:** Delete `components/ledger/internal/adapters/http/in/fees_middlewares.go`. Delete the FEE-0020 sentinel line and its `ValidationError` mapping entry. Do NOT renumber or touch any other FEE sentinel (codes are a wire contract). Confirm with the compiler and a repo-wide grep that no test or other package references the deleted symbols.

**Files:**
- Delete: `components/ledger/internal/adapters/http/in/fees_middlewares.go`
- Modify: `components/ledger/pkg/feeshared/constant/errors.go` (remove line 32)
- Modify: `components/ledger/pkg/feeshared/errors.go` (remove mapping entry at 399-403)

**Verification** (run from repo root): `grep -rn "parseFeeHeaderParameters\|parseFeePathParameters\|feeOrgIDHeaderParameter\|feeUUIDPathParameter\|ErrHeaderParameterRequired" --include="*.go" .` returns nothing. `go build ./...` and `go vet ./...` green.

**Done when:** the middleware file and the dead sentinel are gone; build green; FEE-0019 untouched.

### Epic 1.3: Test rewrites

**Goal:** `fees_routes_test.go` exercises the new path-scoped routes; new route-level cases prove the real `ParseUUIDPathParameters` chain rejects a non-UUID org with the canonical code and admits a valid one; the suite is green.
**Scope:** `components/ledger/internal/adapters/http/in/fees_routes_test.go`.
**Dependencies:** Epics 1.1, 1.2.
**Done when:** the package test suite passes; the route table matches the target map; path-validation cases assert the canonical contract.

#### Task 1.3.1: Rewrite `fees_routes_test.go` route table; add path-validation cases

- [x] Done

**Context:** The route table (lines 21-37) lists the 12 flat paths and drives `TestRegisterFeesRoutesToApp_RegistersEveryRoute` (:43), `TestRegisterFeesRoutesToApp_DoesNotMountFeeCalculate` (:63), `TestCreateFeesRouteRegistrar_RegistersEveryRoute` (:78), and `TestRegisterFeesRoutesToApp_RoutesAreAuthProtected` (:106-147). The auth-protection sweep expects 401 from the auth gate, which in `ProtectedRouteChain` runs BEFORE `ParseUUIDPathParameters` — so auth ordering is preserved regardless of segment validity, but use valid UUID segments in request paths anyway so each test asserts exactly one thing. No fee test sets `X-Organization-Id` today; none will after.

**Implementation vision:** Flip the 12 route-table entries to the target paths. Where tests issue requests against concrete paths, substitute fixed UUID literals for `:organization_id`/`:id` segments (follow the file's existing fixture style; no `uuid.New()` needed where determinism is cheaper). Add two new cases driving the REAL registered chain (auth stubbed/disabled per the file's existing pattern): (a) a non-UUID `organization_id` segment on one package route and one billing route returns 400 with the canonical `ErrInvalidPathParameter` code in the body — this pins the error-contract normalization (FEE-shim codes are dead for path validation); (b) a valid org segment reaches the handler (any terminal status that proves the chain passed validation). Mirror the CRM semantics note in a code comment: a genuinely MISSING org can no longer be expressed — the route does not match and Fiber returns 404 — so the former FEE-0020 "missing header" semantics become "malformed segment → canonical 400".

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/fees_routes_test.go`

**Verification** (run from repo root): `go test ./components/ledger/internal/adapters/http/in/ -run 'Fees' -count=1` passes. `grep -c "/v1/organizations/" components/ledger/internal/adapters/http/in/fees_routes_test.go` ≥ 12.

**Done when:** the route tests assert the path-scoped contract, including canonical 400 on malformed org; suite green.

---

## Phase 2 — Artifact regeneration (Epic-level; executes inside the Phase 1 commit)

**Phase exit criteria:** Generated OpenAPI/Swagger and the Postman collection reflect the path-scoped contract with zero `X-Organization-Id` parameters anywhere (fees was the last surface carrying it); Postman fee requests carry org in the URL. No generated file is hand-edited.

### Epic 2.1: Swagger / OpenAPI regeneration

**Goal:** `components/ledger/api/{swagger.yaml,swagger.json,openapi.yaml,docs.go}` regenerate from the updated doc-comments, showing org as a path parameter on all 12 fee operations and `X-Organization-Id` gone repo-wide.
**Scope:** swag pipeline (`make generate-docs`), generated artifacts under `components/ledger/api/`.
**Dependencies:** Phase 1 (doc-comments are the upstream source).
**Done when:** `grep -rc "X-Organization-Id" components/ledger/api/` returns 0 across all four files (CRM occurrences died in the previous redesign; fees was the remainder); `TestContractSpecMatchesRoutes` green; the diff contains only the intended flips.

### Epic 2.2: Postman collection + specs regeneration

**Goal:** `postman/MIDAZ.postman_collection.json` and `postman/specs/ledger/*` regenerate so every fee request uses the org path segment with no `X-Organization-Id` header row.
**Scope:** `postman/generator/`, `postman/MIDAZ.postman_collection.json`, `postman/specs/ledger/*`. `postman/backups/*` untouched.
**Dependencies:** Epic 2.1.
**Done when:** regenerated fee requests carry `{{organizationId}}` in the URL and no scoping header row; the `{id}` heuristic renders a sane variable for `/packages/{id}` and `/billing-packages/{id}` (inspect — the CRM execution found and fixed one such heuristic bug; fix mapping sites in the generator if fees hits another); `grep -c "X-Organization-Id" postman/MIDAZ.postman_collection.json` returns 0.

---

## Phase 3 — Documentation alignment (Epic-level)

**Phase exit criteria:** No prose doc describes any header-scoped surface in the unified binary; `SCOPING.md` records the fees reversal and closes with zero exceptions; `llms-full.txt` fee route inventory matches the live router; `RBAC-NAMESPACES.md` cross-refs accurate (policy keys unchanged).

### Epic 3.1: Rewrite SCOPING.md; update llms-full.txt and RBAC cross-refs

**Goal:** `docs/api/SCOPING.md` is rewritten so the "single remaining header-scoped exception: fees / billing" section (lines 58-81) becomes a closure record: every surface is path-scoped, `X-Organization-Id` no longer exists in the API contract. `llms-full.txt` "Embedded Fee Service" section (lines 355-374) lists the 12 path-scoped routes and drops the "fee-specific header parameter" scoping note. `docs/auth/RBAC-NAMESPACES.md` fee cross-refs stay accurate — `plugin-fees:*` keys explicitly unchanged (R9), only route shape moved; no policy migration is created by this plan. Historical plan docs (`2026-06-06-crm-api-redesign.md`, `2026-06-06-auth-stabilization.md`) are execution records — do NOT rewrite them.
**Scope:** `docs/api/SCOPING.md`, `llms-full.txt`, `docs/auth/RBAC-NAMESPACES.md`.
**Dependencies:** Phases 1-2.
**Done when:** `grep -rn "X-Organization-Id" docs/ llms-full.txt` returns only historical-plan and changelog references; `SCOPING.md` states zero header-scoped surfaces; `llms-full.txt` fee table shows the org-prefixed paths.

---

## Phase 4 — Hardening follow-ons (Epic-level, parked)

### Epic 4.1: Org-binding authz check (shared decision with CRM Epic 4.1)

**Goal:** A principal authorized for `plugin-fees:packages:*` cannot operate on an org outside its grant — the same residual horizontal-privilege gap CRM Epic 4.1 tracks. Path validation guarantees a well-formed org, not an authorized one; today the tenant (JWT) is the trust boundary (auth-stabilization decision, 2026-06-06) and org is taken as caller-asserted within the tenant.
**Dependencies:** the SAME plugin-auth decision point CRM Epic 4.1 is parked on (resource-instance authz vs per-org tenant grant). Whatever mechanism is chosen there applies to both surfaces in one stroke — do not design a fees-only variant.
**Done when:** **Decision point** — plugin-auth confirms the org-binding mechanism. Until then, parked.

---

## Out of Scope

- **Body-binding convergence** (`feehttp.WithBodyTracing` → `http.WithBody`) — flips body-validation error envelopes from FEE codes to canonical codes; a separate contract decision, not required for path scoping.
- **FEE error-code normalization beyond the scoping layer** — business errors from fee use cases keep FEE codes; only path/scoping validation normalizes (this plan) per the CRM-collapse precedent.
- **RBAC namespace/resource renames** — `plugin-fees` stays distinct by design (R9, `RBAC-NAMESPACES.md:203`); no policy migration, no X1-style gate.
- **Mongo layer** — field-filtered collections and tenant-first DB resolution unchanged; org reaches adapters as the same `uuid.UUID`, only its HTTP source moves.
- **Ledger in path** — no fee route touches a ledger resource; ledger stays a body field / list filter (CRM precedent).
- **`postman/backups/*`** — historical snapshots, untouched.
- **Historical plan docs** — `2026-06-06-*.md` record what was true at execution time; they are not updated to pretend fees was always path-scoped.

---

## Self-Review

- **Precedent fidelity:** mirrors the CRM plan's structure and decisions (org-only path scope, ledger stays body/filter, authz keys frozen, canonical error normalization at the scoping layer, clean break with no dual-routing) and bakes in all four CRM execution lessons (atomic artifacts, `@Router` flips, Postman heuristic check, missing→malformed semantics).
- **Ground-truth verified in-repo on 2026-06-07:** chain shape (already `ProtectedRouteChain`), allowlist coverage (no constant change), the two handler read styles with exact line refs, the FEE-0019/FEE-0020 afterlife split, the `CalculateBilling` payload injection, and the absence of external HTTP consumers.
- **Simplifications relative to CRM, each justified:** no allowlist task (params already listed), no service-signature concern (already `uuid.UUID` end-to-end), one test file instead of seven, no X1 policy gate (namespace unchanged by R9), middleware deletion instead of helper deletion.
- **Risk register:** (1) Postman generator `{id}` heuristic — explicit inspection step in Epic 2.2; (2) error-contract flip from FEE-shim to canonical on path validation — pinned by a dedicated test case in Task 1.3.1 so the change is asserted, not incidental; (3) unchecked-assert panic risk in package/estimate handlers — eliminated as a side effect of the standard locals read.
- **Verification plausibility:** every command targets real paths verified in this repo; greps are count-exact where the count is knowable.
