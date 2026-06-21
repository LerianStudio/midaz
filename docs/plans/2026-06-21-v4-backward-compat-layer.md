# v4 Backward-Compatibility Layer Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: dispatch each
> wave — a phase or one epic, your choice — as a workflow → review → user
> checkpoint → detail the next phase against the real code → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Convert the v4 release from a hard cut into a graceful, sunset-dated deprecation window so existing v3 external REST clients (CRM holders/instruments, fees packages/billing/estimates) and operators keep working against the v4 unified ledger with zero client-side changes during the window.

**Architecture:** A thin, flag-gated, deletable compat layer in the ledger HTTP `in` package. (1) A shared `LegacyOrgHeaderToLocals` Fiber middleware maps the v3 `X-Organization-Id` header into the `organization_id` Fiber Local that the v4 handlers already read via `http.GetUUIDFromLocals(c, "organization_id")` — so the old flat header-scoped routes reuse the **exact** v4 handlers, no handler duplication. (2) Legacy route registrars mount the v3 paths (`/v1/holders*`, `/v1/aliases*`, `/v1/packages*`, `/v1/estimates`, `/v1/billing*`) behind feature flags, emitting RFC 8594 `Deprecation`/`Sunset` headers. (3) A dual-grant auth wrapper on the canonical CRM routes accepts either the legacy `plugin-crm:{holders,aliases}:*` or the new `midaz:{holders,instruments}:*` grants during the window, decoupling each tenant's RBAC migration from the code deploy. (4) Config-loader fallbacks read the old `PLUGIN_CRM_*` env names when the new `CRM_*` ones are unset. Compat routes are published in the OpenAPI spec as `deprecated: true` so the F5 spec-vs-routes gate stays green and clients get machine-readable deprecation. At sunset, flipping the flags off and deleting the compat files is a single isolated commit.

**Tech Stack:** Go 1.26.4, Fiber v2, `github.com/LerianStudio/lib-auth/v2/auth/middleware`, viper config, midaz v4 module (`github.com/LerianStudio/midaz/v4`).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | A v3-shaped CRM request (flat path + `X-Organization-Id` header + v3 body) succeeds against v4 via the real v4 handlers, behind a flag, with deprecation headers | 1.1, 1.2 | Detailed |
| 2 | A tenant holding only legacy `plugin-crm:*` grants authorizes on CRM routes during the window (dual-grant), independent of code deploy timing | 2.1 | Epic-level |
| 3 | A v3 fees REST client (flat path + header) works unchanged against v4, reusing the Phase 1 middleware; the `/estimates` response-shape compat decision is resolved here | 3.1, 3.2 | Epic-level |
| 4 | Operators deploy with old `PLUGIN_CRM_*` env names; deprecation comms ship (migration guide, CHANGELOG BREAKING note, OpenAPI `deprecated` marking, sunset wiring) | 4.1, 4.2 | Epic-level |

---

## Context the implementer needs (applies to all phases)

- **The v3 → v4 break being papered over.** v3 served CRM on a standalone service (`:4003`, authz namespace `plugin-crm`) and fees on the standalone `plugin-fees` service (`:4002`, authz namespace `plugin-fees`). Both carried `organization_id` in the **`X-Organization-Id` header**. v4 folds both into the ledger binary on `:3002`, moves `organization_id` into a **path param** (`/v1/organizations/:organization_id/...`), renames the CRM `aliases` resource to `instruments` (path + authz resource), and flips the CRM authz namespace to `midaz`. Fees authz namespace is **preserved** (`plugin-fees`) — fees auth does NOT break.
- **Why the middleware approach works.** The v4 handlers do not care where `organization_id` came from — they read `http.GetUUIDFromLocals(c, "organization_id")` (`components/ledger/internal/adapters/http/in/holder.go:61,138`; fees handlers `fees_package_handler.go:78`, `fees_handler.go:62`). `ParseUUIDPathParameters` populates that Local from the route's `:organization_id` segment. The compat layer sets the same Local from the header instead, then dispatches to the identical handler.
- **JSON bodies are unchanged.** `CreateHolderInput`/`UpdateHolderInput` (now in `pkg/mmodel/holder.go`) and the instrument input structs have the same json field names v3 used; fees `CreatePackageInput`, `BillingCalculateRequest/Response` are field-identical. No request-body translation is needed. The only response-shape delta is fees `/estimates` (Phase 3).
- **Registration entry points.** Canonical CRM routes register via `RegisterCRMRoutesToApp(...)` (called at `crm_routes.go:62`); canonical fees via `RegisterFeesRoutesToApp(...)` (called at `fees_routes.go:75`). The compat registrars are mounted alongside these, behind their flags.
- **Route chain helper.** `http.ProtectedRouteChain(authHandler fiber.Handler, options *ProtectedRouteOptions, handlers ...fiber.Handler) []fiber.Handler` (`pkg/net/http/protected_routes.go:24`). `auth.Authorize(...)` returns a `fiber.Handler` and is the `authHandler` arg. `WithBody` and `ParseUUIDPathParameters` are at `pkg/net/http/withBody.go:119,232`.
- **Error sentinels.** `ErrInvalidHeaderValue` (`pkg/constant/errors.go:558`, code `CRM-0022`) exists for a malformed header. A "header required" sentinel must be reused if present or added per the numeric-registry rule (`pkg/constant/errors.go`). The v3 pattern to mirror is plugin-fees' `ParseHeaderParameters` (`../plugin-fees/internal/http/in/middlewares.go:38-57`): required → business error; non-UUID → business error; else set Local.
- **Telemetry/error/logging conventions** are governed by `docs/standards/telemetry.md` and `docs/standards/error-handling.md` and the rules in `CLAUDE.md`. Compat code follows them like any other route code.

---

### Epic 1.1: Shared legacy compat middleware + config flags

**Goal:** A reusable `LegacyOrgHeaderToLocals` middleware and a `DeprecationHeaders` middleware exist and are unit-tested; the `LEGACY_CRM_COMPAT_ENABLED` / `LEGACY_FEES_COMPAT_ENABLED` flags and `LEGACY_COMPAT_SUNSET_DATE` are loaded into ledger config.
**Scope:** `components/ledger/internal/adapters/http/in/` (new middleware file), `components/ledger/internal/bootstrap/config.go`.
**Dependencies:** none
**Done when:** middleware unit tests pass for present/missing/malformed header and for header emission; config struct carries the flags with documented defaults.
**Status:** Pending

#### Task 1.1.1: Implement the legacy compat middlewares

- [ ] Done

**Context:** v3 CRM/fees clients send `organization_id` in the `X-Organization-Id` header; v4 handlers read it from the `organization_id` Fiber Local. We need a middleware that bridges the two, plus a middleware that stamps RFC 8594 deprecation headers on every compat response. The canonical v3 pattern to mirror is `../plugin-fees/internal/http/in/middlewares.go:38-57` (`ParseHeaderParameters`).

**Implementation vision:** In a new file, implement two `fiber.Handler`s.
- `LegacyOrgHeaderToLocals(c *fiber.Ctx) error`: read `c.Get("X-Organization-Id")`. If empty → return `http.WithError(c, pkg.ValidateBusinessError(<header-required sentinel>, "", "X-Organization-Id"))`. If non-empty but not a UUID → `http.WithError(c, pkg.ValidateBusinessError(constant.ErrInvalidHeaderValue, "", "X-Organization-Id"))`. On success, `c.Locals("organization_id", parsedUUID)` (key string must exactly match what `http.GetUUIDFromLocals(c, "organization_id")` reads — verify against `pkg/net/http/httputils.go:564`), then `c.Next()`. Set the span attribute `app.request.organization_id` per the telemetry standard (mirror `holder.go:68`). Do NOT log the header value beyond the standard request attribute.
- `DeprecationHeaders(sunsetHTTPDate string) fiber.Handler`: a closure that, before `c.Next()`, sets `Deprecation: true`, `Sunset: <sunsetHTTPDate>`, and `Link: <migration-guide-url>; rel="deprecation"`. The migration-guide URL is a constant placeholder for now (`https://docs.lerian.studio/midaz/v4/migration`); Phase 4 finalizes it.
- For the "header required" sentinel: reuse an existing one in `pkg/constant/errors.go` if a header-required code exists; otherwise add `ErrHeaderParameterRequired` following the numeric/`CRM-` registry convention already used by `ErrInvalidHeaderValue` (`errors.go:558`), with its factory wiring in `pkg/errors.go`. Decide by grepping `pkg/constant/errors.go` for `Header`; do not invent a duplicate.

**Files:**
- Create: `components/ledger/internal/adapters/http/in/legacy_compat_middleware.go`
- Modify (only if no header-required sentinel exists): `pkg/constant/errors.go`, `pkg/errors.go`
- Test: `components/ledger/internal/adapters/http/in/legacy_compat_middleware_test.go`

**Verification:** `go test ./components/ledger/internal/adapters/http/in/ -run TestLegacyOrgHeaderToLocals -v` and `-run TestDeprecationHeaders -v` — present-UUID sets the Local; missing header → business error (assert the sentinel via `errors.Is`); non-UUID header → `ErrInvalidHeaderValue`; deprecation middleware sets all three headers.

**Done when:** both middlewares behave as specified with passing unit tests; the org Local key provably matches `GetUUIDFromLocals`'s read key.

#### Task 1.1.2: Add legacy-compat config flags to ledger bootstrap

- [ ] Done

**Context:** The compat surface must be switchable and carry a sunset date for the `Sunset` header. `config.go` already has precedent for compat/legacy config (`LegacyBalanceSyncDrainer`, `config.go:1067-1087`) and is the composition root per `CLAUDE.md`.

**Implementation vision:** Add to the ledger `Config` struct the env-bound fields: `LegacyCRMCompatEnabled bool` (`LEGACY_CRM_COMPAT_ENABLED`, default `true`), `LegacyFeesCompatEnabled bool` (`LEGACY_FEES_COMPAT_ENABLED`, default `true`), `LegacyCompatSunsetDate string` (`LEGACY_COMPAT_SUNSET_DATE`, default a placeholder HTTP-date ~2 release cycles out, e.g. `Wed, 31 Dec 2026 23:59:59 GMT`). Defaults make the window ON out-of-the-box so an operator who does nothing keeps clients working. Follow the exact env-binding idiom already used in `config.go` (do not hand-roll `os.Getenv` if the file uses a struct-tag/viper loader). Validate the sunset date parses as an HTTP-date at load; on parse failure log Warn and fall back to the default (degraded-but-recoverable per T7), do not fail boot. Expose a small accessor the route wiring reads.

**Files:**
- Modify: `components/ledger/internal/bootstrap/config.go`
- Modify: `components/ledger/.env.example` (document the three vars + defaults)
- Test: `components/ledger/internal/bootstrap/config_test.go` (if the file has a config-parsing test; otherwise a focused new test asserting defaults + sunset-parse fallback)

**Verification:** `go test ./components/ledger/internal/bootstrap/ -run TestConfig -v` — defaults resolve to enabled + the placeholder sunset; an unparseable `LEGACY_COMPAT_SUNSET_DATE` falls back without boot failure.

**Done when:** config exposes the three fields with the documented defaults and the sunset-parse fallback, and `.env.example` documents them.

---

### Epic 1.2: CRM legacy route registrar

**Goal:** With `LEGACY_CRM_COMPAT_ENABLED=true`, the v3 flat CRM paths are mounted on the ledger and serve via the canonical v4 handlers; an integration test drives a v3-shaped request through to a v4 handler.
**Scope:** `components/ledger/internal/adapters/http/in/` (new registrar file + wiring in the CRM route registration path).
**Dependencies:** Epic 1.1
**Done when:** a v3-style `POST /v1/holders` (header + v3 body) and a v3-style `POST /v1/holders/:holder_id/aliases` reach the v4 `CreateHolder`/`CreateInstrument` handlers and return the v4 response with deprecation headers; with the flag off, the flat routes 404.
**Status:** Pending

#### Task 1.2.1: Implement RegisterLegacyCRMRoutesToApp

- [ ] Done

**Context:** Canonical CRM routes mount via `RegisterCRMRoutesToApp(f, auth, hh, ah, hah, routeOptions)` (`crm_routes.go:36-54`), each as `ProtectedRouteChain(auth.Authorize("midaz", resource, action), routeOptions, ParseUUIDPathParameters(entity), [WithBody(...)], handler)`. The compat layer must mount the v3 flat paths (`crm_routes.go` v3 reference: `git show origin/develop:components/crm/internal/adapters/http/in/routes.go:68-80`) to the SAME handler instances.

**Implementation vision:** New `RegisterLegacyCRMRoutesToApp(f fiber.Router, auth *middleware.AuthClient, hh *HolderHandler, ah *InstrumentHandler, sunset string, routeOptions *http.ProtectedRouteOptions)`. For each v3 route, build the chain as `ProtectedRouteChain(auth.Authorize(ApplicationName, resource, action), routeOptions, DeprecationHeaders(sunset), LegacyOrgHeaderToLocals, ParseUUIDPathParameters(entity), [WithBody(...)], handler)`. Ordering is load-bearing: `LegacyOrgHeaderToLocals` MUST run before any handler (and before `auth.Authorize` if the authz check is org-scoped — verify and place it first in the chain if so). Routes to mount, each → the existing v4 handler:
- `POST /v1/holders` → `hh.CreateHolder`; `GET /v1/holders/:id` → `hh.GetHolderByID`; `PATCH /v1/holders/:id` → `hh.UpdateHolder`; `DELETE /v1/holders/:id` → `hh.DeleteHolderByID`; `GET /v1/holders` → `hh.GetAllHolders`.
- `GET /v1/aliases` → `ah.GetAllInstruments`; `POST /v1/holders/:holder_id/aliases` → `ah.CreateInstrument`; `GET /v1/holders/:holder_id/aliases/:alias_id` → `ah.GetInstrumentByID`; `PATCH .../aliases/:alias_id` → `ah.UpdateInstrument`; `DELETE .../aliases/:alias_id` → `ah.DeleteInstrumentByID`; `DELETE .../aliases/:alias_id/related-parties/:related_party_id` → `ah.DeleteRelatedParty`.
- For the alias routes, the handlers read instrument IDs from Locals via `ParseUUIDPathParameters`. The v3 path uses `:alias_id`; the handler reads whatever key the v4 route used (`:instrument_id`). Resolve the key mismatch in the registrar: name the compat path param the same key the handler reads (e.g. mount as `/v1/holders/:holder_id/aliases/:instrument_id`) so `GetUUIDFromLocals` finds it — the external path still reads `/aliases/<uuid>`, only the internal param name matches. Verify the exact Local keys each instrument handler reads before finalizing.
- Authz resource names on compat routes: holders → `"holders"`; alias routes → `"instruments"` (the v4 resource). Dual-grant for tenants still on `plugin-crm:aliases:*` is Phase 2; this task uses the canonical `midaz` check so it composes with Phase 2's wrapper.

**Files:**
- Create: `components/ledger/internal/adapters/http/in/crm_legacy_routes.go`
- Test: `components/ledger/internal/adapters/http/in/crm_legacy_routes_test.go` (route-presence/handler-wiring unit test using a Fiber test app with auth + tenant middleware stubbed, mirroring the existing `holder_test.go` setup)

**Verification:** `go test ./components/ledger/internal/adapters/http/in/ -run TestLegacyCRMRoutes -v` — a flat `POST /v1/holders` with `X-Organization-Id` header reaches `CreateHolder` and the org Local is populated from the header; `POST /v1/holders/<h>/aliases` reaches `CreateInstrument`.

**Done when:** all eleven v3 CRM routes mount and dispatch to the correct v4 handler with the org Local sourced from the header and deprecation headers on the response.

#### Task 1.2.2: Wire the legacy CRM registrar into the server behind the flag

- [ ] Done

**Context:** The canonical registrar is invoked at `crm_routes.go:62` inside the outer `RegisterCRMRoutes`-style function the server calls. The compat registrar must mount only when `LegacyCRMCompatEnabled` is true, using the configured sunset date.

**Implementation vision:** At the canonical CRM registration call site (`crm_routes.go:56-62` region), after `RegisterCRMRoutesToApp(...)`, add: `if cfg.LegacyCRMCompatEnabled { RegisterLegacyCRMRoutesToApp(router, auth, hh, ah, cfg.LegacyCompatSunsetDate, routeOptions) }`. Thread the config flag + sunset into this function from the composition root; follow how other config-driven route toggles are passed (do not reach for a global). Log one Info milestone at boot when compat is enabled, naming the sunset date, so operators see the window is open (sparse-Info per T7).

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/crm_routes.go` (call site) and the composition-root caller that supplies `cfg` (grep `RegisterCRMRoutes` for the server wiring)
- Test: `components/ledger/internal/adapters/http/in/crm_legacy_routes_test.go` (flag-off case: assert flat routes return 404)

**Verification:** `go test ./components/ledger/internal/adapters/http/in/ -run TestLegacyCRMRoutes -v` (flag on: routes present; flag off: 404). Then `make test-unit` green.

**Done when:** flat CRM routes are served iff the flag is on; boot logs the open window with its sunset date.

---

### Epic 2.1: CRM dual-grant authorization window

**Goal:** During the window, a tenant whose `tenant-manager` policies still carry only `plugin-crm:{holders,aliases}:*` (the v3 keys) authorizes on the CRM routes (canonical and compat), removing the fail-closed 403 cliff and decoupling each tenant's RBAC migration from the v4 code deploy. After sunset, only `midaz:{holders,instruments}:*` is accepted.
**Scope:** `components/ledger/internal/adapters/http/in/` (auth wrapper), CRM route registration (canonical + compat), config.
**Dependencies:** Phase 1
**Done when:** with dual-grant enabled, a request bearing only a legacy `plugin-crm:holders:post` grant is authorized on `POST .../holders`; with it disabled, only the `midaz` grant authorizes; the resource rename (`aliases`→`instruments`) is handled so a legacy `plugin-crm:aliases:*` grant authorizes the instrument routes.
**Status:** Pending

> **Front-loaded design risk (resolve first during elaboration):** the graceful dual-grant depends on `lib-auth/v2`'s `AuthClient` supporting a *non-terminating* authorization probe — "check grant A; if denied, check grant B; only 403 if both fail" — without the first `auth.Authorize` writing a 403 to the response and ending the request. Before writing tasks, inspect `github.com/LerianStudio/lib-auth/v2/auth/middleware` for a check that returns a decision rather than a `fiber.Handler` that terminates. If none exists: (a) prefer a small upstream addition to lib-auth (a boolean/`error`-returning `Can(...)`), or (b) fall back to the tenant-manager **dual-provision** path from `docs/runbooks/v4-x1-rbac-migration-and-rollback.md` (issue both grant sets per tenant for the window) — which needs no midaz code but must confirm tenant-manager can hold both key sets (runbook Known Gap #2). The chosen mechanism determines whether Epic 2.1 is midaz code or a runbook/coordination epic. Record the decision before elaborating tasks.

---

### Epic 3.1: Fees legacy route registrar (reuse Phase 1 middleware)

**Goal:** With `LEGACY_FEES_COMPAT_ENABLED=true`, the v3 flat fees/billing/estimate paths mount on the ledger and serve via the canonical v4 fees handlers, reusing `LegacyOrgHeaderToLocals`. Fees authz namespace (`plugin-fees`) is preserved, so no dual-grant is needed.
**Scope:** `components/ledger/internal/adapters/http/in/` (new fees compat registrar + wiring at `fees_routes.go:75`).
**Dependencies:** Phase 1 (the shared middleware)
**Done when:** v3-shaped `POST /v1/packages`, `GET/PATCH/DELETE /v1/packages/:id`, `POST /v1/estimates`, `POST /v1/billing-packages`, `GET/PATCH/DELETE /v1/billing-packages/:id`, `POST /v1/billing/calculate` (all with `X-Organization-Id` header) reach the canonical v4 fees handlers with deprecation headers; flag-off → 404. `POST /v1/fees` is intentionally NOT mounted (confirmed: no external consumer).
**Status:** Pending

### Epic 3.2: Fees `/estimates` response-shape compat (gated, decided at execution)

**Goal:** Decide and, if required, implement response compatibility for `POST /v1/estimates`, whose v4 response projects `FeeCalculate` → `FeeEstimateResult` and drops the deprecated `route` field (adds `routeId`).
**Scope:** `components/ledger/internal/adapters/http/in/` (fees compat estimate handler), `components/ledger/pkg/feeshared` (engine carrier access).
**Dependencies:** Epic 3.1
**Done when:** EITHER (a) confirmed via client check/telemetry that no v3 estimate consumer reads `FeesApplied.Transaction.route`, so the projected v4 shape is served as-is on the compat route and the epic closes as a no-op with a recorded rationale; OR (b) the compat estimate route returns the un-projected `FeeCalculate` carrier (the engine still produces it internally; only the wire layer projects) so legacy `route`-reading clients keep working for the window. Choose (a) unless a real `route`-reading consumer is found.
**Status:** Pending

---

### Epic 4.1: Operator env-var fallbacks

**Goal:** An operator deploying v4 with the old `PLUGIN_CRM_*` env names keeps a working reporter/CRM config during the window; the new `CRM_*` names take precedence when set.
**Scope:** the config loaders that read the renamed vars (`components/reporter/internal/worker/bootstrap/config.go:63-64`, `components/ledger/internal/bootstrap/config.go`, and the `DATASOURCE_CRM_*` / `CRYPTO_*_SECRET_KEY_CRM` read sites).
**Dependencies:** none (independent of Phases 1–3)
**Done when:** for each renamed var, the loader reads the new name and falls back to the old `PLUGIN_CRM_*` name when the new one is unset, logging a one-time Warn naming the deprecated var; `.env.example` documents the fallback and sunset.
**Status:** Pending

### Epic 4.2: Deprecation communications + spec/gate alignment

**Goal:** The deprecation is discoverable and the contract gate stays green: a migration guide ships, the CHANGELOG carries the `BREAKING CHANGE` entry for v4, the compat routes are published in the OpenAPI as `deprecated: true` with sunset info, and the F5 spec-vs-routes gate is reconciled to expect the deprecated routes while the window is open.
**Scope:** `docs/` (migration guide), `CHANGELOG.md`, OpenAPI spec generation for the ledger, the F5 spec-vs-routes diff gate config, the `Link` rel="deprecation" URL constant from Task 1.1.1.
**Dependencies:** Phases 1–3 (the routes that get documented)
**Done when:** the migration guide maps every v3 surface → v4 surface (paths, header→path, namespace, resource rename, port, removed `POST /v1/fees`, module bump) with the sunset date; CHANGELOG has the v4 `BREAKING CHANGE` entry; compat routes appear in the served OpenAPI as `deprecated: true`; the F5 gate passes with compat enabled and will pass again when both spec and routes drop the compat surface at sunset.
**Status:** Pending

---

## Sunset / teardown (post-window, not a phase)

At the sunset date: flip `LEGACY_*_COMPAT_ENABLED` defaults to `false`, then delete `legacy_compat_middleware.go`, `crm_legacy_routes.go`, the fees compat registrar, the dual-grant wrapper, the env fallbacks, and the `deprecated: true` OpenAPI entries — one isolated commit. The X1 hard-cut in `docs/runbooks/v4-x1-rbac-migration-and-rollback.md` then applies as originally written, but now reached gracefully (every tenant has had the window to migrate grants).

---

## Self-Review

- **Spec coverage.** CRM REST break → Epics 1.1/1.2 (path) + 2.1 (auth) + 4.2 (rename/comms). Fees REST break → 3.1 (path) + 3.2 (response). Env break → 4.1. Module `/v3→/v4` → out of scope (audience handled: midaz-sdk-golang already on /v4); recorded in the migration guide (4.2). Host/port move → infra Service alias, recorded as coordination in 4.2, not midaz code. Data/streaming → confirmed safe in the analysis, no work. Gaps: none for the in-code compat surface.
- **Vagueness scan.** Phase 1 tasks name exact files, handlers, route lists, Local-key mismatch resolution, sentinel decision, and chain ordering. No "appropriate"/"TBD" in the detailed wave. Deferrals (2.1 lib-auth mechanism, 3.2 reshape decision) are in later-phase epics, which is allowed.
- **Contract consistency.** `LegacyOrgHeaderToLocals` sets `organization_id`; all handlers read `organization_id` via `GetUUIDFromLocals` — single key. Flags named consistently (`LEGACY_*_COMPAT_ENABLED`, `LEGACY_COMPAT_SUNSET_DATE`). Authz resources: `holders`/`instruments` on compat routes match canonical.
- **Phase boundaries.** P1 ends with a working v3 CRM client path (auth aside). P2 ends with grant-migration decoupled. P3 ends with a working v3 fees client. P4 ends with operator + comms compat. Each is independently verifiable.
- **Verification plausibility.** All `go test` targets hit real packages; `make test-unit` is the integrating check. The lib-auth probe risk is flagged for first-resolution in Phase 2, not assumed.
