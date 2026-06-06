# Auth Stabilization Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Close the verified fail-open and header-trust authentication gaps across the four Midaz Go services (ledger, tracer, reporter-manager, reporter-worker) so that no service can boot into an unauthenticated production posture, no client-controlled header can spoof identity or audit evidence, and the cross-service authz/routing model is documented and aligned.

**Architecture:** Three rolling waves ordered by blast radius and risk. Phase 1 ships the two highest-leverage, lowest-regret changes — boot gates that make a fail-open production deployment impossible (tracer auth-presence gate; ledger single-tenant production auth gate). Phase 2 hardens the request-path header-trust seams (tracer XFF client-IP for audit records; ledger fees `X-Organization-Id`; deletes the dead reporter-worker header-trusting resolver). Phase 3 attacks cross-service consistency and the structural authz model (central route-chain helper for reporter-manager; remove ledger dead router; an RBAC-namespace decision document; lib-auth upstream issues). Every gate keeps the existing local/dev developer experience (Warn-and-continue) and only tightens non-local/production posture.

**Tech Stack:** Go 1.26 (module `github.com/LerianStudio/midaz/v4`); Fiber v2; lib-commons/v5 v5.4.1 (`SetConfigFromEnvVars`, tracking, HTTP toolkit) and lib-observability v1.0.1 — both MANDATORY (third rail; never fork); lib-auth/v2 v2.8.0 (auth middleware — fixes go upstream); testify + gomock + testcontainers.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | No service can boot into an unauthenticated production/non-local posture; local/dev DX unchanged | 1.1, 1.2 | ✅ Done (`b5d88d801`, `0a7742894`) |
| 2 | No client-controlled header can spoof audit IP, fee org-scope, or worker tenant; dead resolver removed | 2.1, 2.2, 2.3 | ✅ Done (2.1 `4b65d7354`; 2.2 resolved-no-action by owner decision; 2.3 `416909bd2`) |
| 3 | Cross-service route-chain/authz drift removed or documented; RBAC namespace decision recorded; lib-auth gaps filed upstream | 3.1, 3.2, 3.3, 3.4 | ✅ Done (3.1 `2f07d2be4`; 3.2 `731d087a8`; 3.3 `14296b4b5`; 3.4 lib-auth#106/#107/#108) |

---

## Preamble: Scope, Trust Model, and Accepted Risks

**The shared trust model (read before implementing).** All four services depend on `lib-auth/v2` v2.8.0. Two properties of that library drive this entire plan and are NOT changed here (they are deferred to upstream issues in Epic 3.4):

1. **Fail-open when disabled.** `lib-auth.Authorize` returns `c.Next()` with zero checks when plugin auth is disabled (`middleware.go:222-224`). Disabling auth is one missing env var away from serving every endpoint unauthenticated. Phase 1 makes that impossible to reach *by accident* in production via boot gates — it does NOT change the library default.
2. **No in-process JWT signature verification.** lib-auth uses `ParseUnverified` (`middleware.go:276`); ledger's tenant middleware and `MarkTrustedAuthAssertion` (`protected_routes.go:51`) do too. `CASDOOR_JWK_ADDRESS` is loaded but never consumed. The entire signature-trust model rests on the external auth service. This is a defense-in-depth gap filed upstream (Epic 3.4), not patched locally — forking lib-auth is a third-rail violation.

**Accepted risks (explicitly out of scope, do not plan work for them):**

- **Tracer `APIKeyOnlyValidation` tier** (`API_KEY_ENABLED_ONLY_VALIDATION`): lets `POST /v1/validations` run on API-key-only auth, a weaker tier than the rest of the API. This is a documented, deliberate operator choice for the highest-volume write path. Left as-is; not a gap to close.
- **Ledger metadata-index routes without `WithTenantDB`**: REFUTED by verification — the handler resolves the tenant DB itself from the ctx `tenantId` seeded by `MarkTrustedAuthAssertion` (`metadata.go:80-112`). Tenant-safe. No work.
- **Synchronous remote authz with no cache/breaker/retry**: real availability amplifier, but a lib-auth concern → filed upstream in Epic 3.4, not patched locally.
- **CRM fees path-shape redesign** (header-vs-path scoping): tracked as a separate v4 release-gated break coordinated with the X1 plugin-crm policy migration. Phase 2 only HARDENS the existing fees header in place (the principal-org cross-check; UUID-validation already exists at `fees_middlewares.go:43-58`); it does NOT move org from header to path. The path-shape redesign lives in the CRM plan.

**Posture invariant for every gate in this plan:** local/dev (the developer onboarding path) keeps today's Warn-and-continue behavior. Only non-local (tracer `DeploymentMode != "local"`) / production (ledger `ENV_NAME == "production"`) tightens to fail-boot. No developer running `make up` against an empty `.env` should hit a new boot error.

---

## Phase 1: Boot Gates — Fail-Closed in Production

**Milestone:** A deployment that forgets to enable auth in production fails to boot with a clear, actionable error, instead of silently serving every endpoint unauthenticated. Local/dev boots unchanged. Both services have unit tests proving the gate fires in production and stays quiet in local.

### Epic 1.1: Tracer auth-presence boot gate

**Goal:** Tracer refuses to boot when NO auth mechanism is enabled (`API_KEY_ENABLED=false` AND `PLUGIN_AUTH_ENABLED=false`) in any non-local deployment mode; local/dev keeps the existing Warn-and-continue.
**Scope:** `components/tracer/internal/bootstrap/` (new gate file + boot call site wiring; mirrors the existing `ValidateSaaSTLS` / `resolveDeploymentMode` pattern).
**Dependencies:** none.
**Done when:** with `DEPLOYMENT_MODE` unset/`local`, both flags off → boot succeeds with a Warn (today's behavior); with `DEPLOYMENT_MODE=saas` or `byoc` (the documented non-local modes per `config.go:53-57`), both flags off → boot fails with an error naming both env vars; with any non-local mode and at least one flag on → boot succeeds. The gate keys on "any mode that is not `local`", so any undocumented non-local string accepted by `resolveDeploymentMode`'s pass-through is also gated. Unit test covers all three branches.

#### Task 1.1.1: Add `ValidateAuthPresence` boot gate and wire it into tracer bootstrap

- [x] Done — commit `b5d88d801`

**Context:** Tracer already has a `DeploymentMode` config field (`config.go:57`, documented values per `config.go:53-57` are `"saas"`/`"byoc"`/`"local"` only — `local` is the default applied via `resolveDeploymentMode` in `deployment_mode.go:21`) used today ONLY to gate SaaS TLS enforcement (`tls_enforcement.go:35`, `ValidateSaaSTLS`). Note `resolveDeploymentMode` passes through any non-empty string unchanged (`deployment_mode_test.go:30` exercises an arbitrary value like `"onprem"`), so the gate logic below ("any mode that is not `local`") correctly catches both the documented non-local modes and any undocumented non-local string. The auth validators `ValidateAuthConfig` (`config.go:633-660`, signature `ValidateAuthConfig(ctx context.Context, cfg *Config, logger libLog.Logger) error`) and `ValidateAccessManagerConfig` (`config.go:665-676`, signature `ValidateAccessManagerConfig(ctx context.Context, cfg *Config, logger libLog.Logger) error`) currently return `nil` with only a `Warn` when their respective flag is off — there is NO cross-check forcing at least one mechanism on. With both off, `AuthGuard.Protect` returns `apiKeyAuth` (`middleware/auth_guard.go:85,98`), and `APIKeyAuth` calls `c.Next()` unconditionally when `!Enabled` (`middleware/apikey.go:93-94`), so every `/v1` route is unauthenticated and boot succeeds (verified: spec gap "Fail-open / missing boot gate", severity critical). Tracer has NO `ENV_NAME` field — `DeploymentMode` is the existing production signal, and the locked decision is to extend that same gate to auth presence. The three boot validators are called sequentially at `config.go:1472-1493` (after the logger is built, before telemetry/connections).

**Implementation vision:** Add a new `ValidateAuthPresence(ctx context.Context, cfg *Config, logger libLog.Logger) error` in a new file `auth_presence.go` alongside `tls_enforcement.go` — one function, one call site, matching the centralization convention documented in `tls_enforcement.go:14-17` AND the `(ctx, cfg, logger)` signature of the sibling auth validators `ValidateAuthConfig`/`ValidateAccessManagerConfig` (`config.go:633`, `config.go:665`). Logic:
- Compute `mode := resolveDeploymentMode(cfg)` (reuse the existing helper; never re-read the env or re-default inline).
- Treat any mode that is not `local` (case-insensitive, trimmed — mirror `ValidateSaaSTLS`'s `strings.EqualFold(strings.TrimSpace(...))` normalization so `"SaaS"` / `" onprem "` cannot slip the gate) as the gated tier.
- If `!cfg.APIKeyEnabled && !cfg.PluginAuthEnabled`:
  - When mode is `local`: `logger.Log(ctx, libLog.LevelWarn, ...)` that all auth is disabled (preserve today's DX) and return `nil` — pass `ctx` as the first argument, matching the sibling validators' `logger.Log(ctx, ...)` calls (`config.go:636`, `config.go:667`).
  - When mode is non-local: return `fmt.Errorf("DEPLOYMENT_MODE=%q requires at least one auth mechanism: set API_KEY_ENABLED=true or PLUGIN_AUTH_ENABLED=true; running non-local without authentication leaves every /v1 route open", mode)`.
- Otherwise (at least one mechanism on, any mode): return `nil`.

Logger CAN be used here (unlike `ValidateSaaSTLS`) because the existing auth validators already take and use a logger at this boot stage (`config.go:633`). Follow CLAUDE.md logging rules: structured fields via `libLog.String(...)`, never `fmt.Sprintf` inside the logger call; the Warn message names the config keys as fields, not interpolated. Wire the call as `ValidateAuthPresence(ctx, cfg, logger)` immediately after the `ValidateAccessManagerConfig` block (which ends at `config.go:1479`) — i.e. at `config.go:1481`, before the `ValidateMultiTenantConfig` block — so individual-flag misconfig errors surface first, then the cross-check, and `ctx` is passed exactly as the sibling call sites do (`config.go:1472`, `config.go:1477`). Wrap as `fmt.Errorf("auth presence: %w", err)` to match the sibling call sites' wrap style. Do NOT modify `ValidateAuthConfig`/`ValidateAccessManagerConfig` — their per-flag Warns stay; this gate is the cross-check they lack.

**Files:**
- Create: `components/tracer/internal/bootstrap/auth_presence.go`
- Modify: `components/tracer/internal/bootstrap/config.go:1481` (insert the `ValidateAuthPresence(ctx, cfg, logger)` call after the `ValidateAccessManagerConfig` block that ends at line 1479, before the `ValidateMultiTenantConfig` block)
- Test: `components/tracer/internal/bootstrap/auth_presence_test.go`

**Verification:** `go test ./components/tracer/internal/bootstrap/ -run TestValidateAuthPresence -v` — table-driven cases all pass: (local, both off) → nil; (saas, both off) → error mentioning both env vars; (byoc, both off) → error; (saas, api-key on only) → nil; (saas, plugin on only) → nil; (mixed-case " SaaS ", both off) → error; ("onprem", both off) → error — `"onprem"` is NOT a documented mode (`config.go:53-57` lists only `saas`/`byoc`/`local`); it is an arbitrary non-local string accepted by `resolveDeploymentMode`'s pass-through, included here specifically to exercise the "any non-local" branch rather than a supported mode. Then `make lint` from `components/tracer` passes (wsl_v5 whitespace, errorlint `%w`).

**Done when:** the gate fires for every non-local mode with both flags off, stays a Warn for local, and is wired into the boot sequence with a wrapped error; tests green; lint clean.

---

### Epic 1.2: Ledger single-tenant production auth boot gate

**Goal:** Ledger refuses to boot when `ENV_NAME=production` and `PLUGIN_AUTH_ENABLED=false`, closing the single-tenant production fail-open hole; non-production (local/dev/staging) keeps current behavior; the existing multi-tenant gate is untouched.
**Scope:** `components/ledger/internal/bootstrap/config.go` (extend the boot-time validation block already present at the top of `InitServersWithOptions`).
**Dependencies:** none (independent of Epic 1.1; can run in parallel).
**Done when:** `ENV_NAME=production` + `PLUGIN_AUTH_ENABLED=false` → boot fails with an actionable error; `ENV_NAME=production` + `PLUGIN_AUTH_ENABLED=true` → boots; `ENV_NAME` unset/`local`/`development` + auth off → boots (today's behavior); the existing MT gate (`MULTI_TENANT_ENABLED` + `!AuthEnabled`) still fires independently. Unit test covers production-gated and non-production-ungated branches.

#### Task 1.2.1: Add single-tenant production auth gate next to the existing MT gate

- [x] Done — commit `0a7742894` (extracted `validateBootAuthGates` per the testability note: no isolated test path existed; the MT gate gained coverage as a side effect)

**Context:** Ledger already hard-fails at boot for the multi-tenant case: `if cfg.MultiTenantEnabled && !cfg.AuthEnabled { return nil, fmt.Errorf(...) }` at `components/ledger/internal/bootstrap/config.go:334-337`, immediately after `applyConfigDefaults(cfg)` (`config.go:332`) and before the logger is built. But a SINGLE-tenant production deploy that forgets `PLUGIN_AUTH_ENABLED=true` serves every business endpoint unauthenticated, because lib-auth `Authorize` fail-opens (`middleware.go:222-224`) and `MULTI_TENANT_ENABLED` is the only structural guard (verified: spec gap "Fail-open single-tenant", severity medium; canonical weakness "Fail-open is the default-off posture"). The config already carries `EnvName string \`env:"ENV_NAME"\`` (`config.go:53`) and `AuthEnabled bool \`env:"PLUGIN_AUTH_ENABLED"\`` (`config.go:70`); `EnvName` is already used for logger environment (`config.go:347`) and insecure-MT-HTTP gating (`config.go:1158`). The locked decision is to MIRROR reporter-manager's `validateProductionConfig` posture, which keys on `c.EnvName != "production"` → skip, else require `AuthEnabled` (`components/reporter-manager/internal/bootstrap/config.go:289-302`, message `"PLUGIN_AUTH_ENABLED must be true in production"`).

**Implementation vision:** Add a second guard immediately AFTER the existing MT gate (`config.go:337`), so both gates are co-located and read as one "auth-presence at boot" block. Key the production signal on case-insensitive `EnvName == "production"`, normalized via `strings.ToLower(strings.TrimSpace(cfg.EnvName))` to match `resolveLoggerEnvironment`'s normalization style (`config.go:985`) and to avoid `" Production "` slipping the gate. Logic: `if strings.EqualFold(strings.TrimSpace(cfg.EnvName), "production") && !cfg.AuthEnabled { return nil, fmt.Errorf("ENV_NAME=production requires PLUGIN_AUTH_ENABLED=true; a single-tenant production deployment without authentication serves every endpoint unauthenticated") }`. This runs before the logger exists (same as the MT gate), so it returns an error rather than logging — correct, matching the sibling gate. Do NOT touch the MT gate; the two are independent (MT mode in non-production still fails via the existing gate, production single-tenant now fails via this one, production MT fails via both — all correct). `strings` is already imported in this file (used by `resolveLoggerEnvironment`). Scope discipline: only this gate is added; do not "improve" adjacent config loading.

**Files:**
- Modify: `components/ledger/internal/bootstrap/config.go:334-337` (insert the new guard immediately after the closing brace of the MT gate)
- Test: `components/ledger/internal/bootstrap/config_test.go` (or a focused new `config_authgate_test.go` if the existing file's boot harness is heavyweight — follow whatever pattern reporter-manager's `config_test.go:188-352` uses for `validateProductionConfig`)

**Implementation note on testability:** `InitServersWithOptions` opens real infrastructure, so a full boot test is not the right unit. If the existing `config_test.go` already exercises config validation in isolation, extend it. If not, extract the two guards into a small pure helper `validateBootAuthGates(cfg *Config) error` called from `InitServersWithOptions` right after `applyConfigDefaults`, and unit-test the helper directly (this also keeps the MT gate covered). Prefer extraction ONLY if no existing isolated test path exists — confirm by reading `config_test.go` first; do not refactor gratuitously.

**Verification:** `go test ./components/ledger/internal/bootstrap/ -run 'TestBootAuthGate|TestValidateBootAuthGates' -v` — cases: (production, auth off) → error mentioning `PLUGIN_AUTH_ENABLED`; (production, auth on) → nil; (local, auth off) → nil; (development, auth off) → nil; (mixed-case "Production", auth off) → error; (MT on, auth off, non-production) → still errors via the existing MT gate. Then `make lint` from `components/ledger` passes.

**Done when:** production single-tenant without auth fails boot with an actionable error; non-production is unaffected; the MT gate still fires independently; tests green; lint clean.

---

## Phase 2: Request-Path Header-Trust Hardening

**Milestone:** No client-controlled HTTP/AMQP header can spoof an identity-bearing value that reaches a security decision or a durable record. Tracer audit records carry a trustworthy client IP; ledger fee routes bind the (already UUID-validated) org header to the authenticated principal; the dead header-trusting tenant resolver in reporter-worker is gone. Every service still passes its full test suite and boots.

### Epic 2.1: Tracer trusted-proxy XFF handling for audit client IP

**Goal:** The client IP written into every audit-event record is derived safely: when `TRUSTED_PROXY_CIDRS` is configured, take the rightmost-untrusted X-Forwarded-For hop; when unset, use the socket `RemoteIP()` — NEVER the client-controlled leftmost XFF value.
**Scope:** `components/tracer/internal/adapters/http/in/middleware/client_ip.go` (replace the leftmost-XFF extraction); a new `TRUSTED_PROXY_CIDRS` config field + parse in `components/tracer/internal/bootstrap/config.go`; the audit write path is unchanged (`command/record_audit_event.go:69-75` reads `contextutil.GetClientIP(ctx)` — it just receives a trustworthy value now).
**Dependencies:** none.
**Done when:** with `TRUSTED_PROXY_CIDRS` unset, a forged `X-Forwarded-For` header is ignored and the socket peer IP is recorded; with `TRUSTED_PROXY_CIDRS` set and the request arriving through a trusted proxy, the rightmost hop NOT in the trusted set is recorded; an empty/garbage XFF never overrides the socket IP. Unit tests cover the trusted-set, untrusted-spoof, and unset-config branches; an integration test asserts the recorded audit-event IP for a spoofed header equals the socket IP, not the forged value.

- [x] **Done — commit `4b65d7354`.** Executed as four tasks: 2.1.1 middleware rewrite (`client_ip.go` — `ClientIPMiddlewareWithTrustedProxies([]*net.IPNet)`, right-to-left walk, socket-IP fallback; X-Real-IP handling REMOVED — equally client-controlled, honoring it would reintroduce the forge vector; dead `isValidIP` removed); 2.1.2 boot parse (`config.go` — `TrustedProxyCIDRs` env field + `parseTrustedProxyCIDRs`, boot fails on malformed CIDR naming the env var); 2.1.3 wiring (`routes.go` `RouteConfig.TrustedProxyCIDRs`, parsed in `initHTTPServer`); 2.1.4 tests + docs (table-driven unit tests incl. IPv6/garbage/all-trusted; integration case in `17_audit_actor_test.go` asserts forged XFF ≠ recorded actor IP; `.env.example` documents semantics). Unit tests use a real `fasthttp.RequestCtx` with `SetRemoteAddr` because Fiber's `app.Test()` cannot inject a socket peer IP.

### Epic 2.2: Ledger fees `X-Organization-Id` org-claim cross-check (in place)

**Goal:** When auth is enabled, the fees/billing middleware cross-checks the already-parsed `X-Organization-Id` against the authenticated principal's org claim — so a caller authorized for `plugin-fees:*` cannot point at an arbitrary org. This is hardening-in-place; the header-to-path redesign stays OUT OF SCOPE (CRM plan, X1-gated).

> **Scope correction (verified against code).** UUID-validation and empty-rejection of `X-Organization-Id` ALREADY exist today: `parseFeeHeaderParameters` (`components/ledger/internal/adapters/http/in/fees_middlewares.go:43-58`) returns `ErrHeaderParameterRequired` on empty (via `commons.IsNilOrEmpty`) and `ErrInvalidHeaderParameter` on a non-UUID value (via `uuid.Parse`), and stores the parsed `uuid.UUID` in `c.Locals(feeOrgIDHeaderParameter, ...)`. The 400-validation deliverable is therefore ALREADY SHIPPED — do NOT re-implement it. The "no UUID validation" wording in the audit spec targets the separate CRM holder/instrument routes (the `crm_design` section), not this fees middleware. The only genuinely missing piece is the principal-org-claim cross-check.

**Goal (work to do):** Add an org-claim cross-check to `parseFeeHeaderParameters`: when auth is enabled, compare the parsed `X-Organization-Id` (already a `uuid.UUID` in `c.Locals`) against the org claim seeded by `MarkTrustedAuthAssertion` (`pkg/net/http/protected_routes.go:40-69` sets `tenantId` on the request context), returning 403 on mismatch; no-op when auth is disabled.
**Scope:** `components/ledger/internal/adapters/http/in/fees_middlewares.go:43-58` (`parseFeeHeaderParameters`) — add the cross-check after the existing parse; the principal-org claim source is the same `tenantId`/org assertion the ledger already seeds via `MarkTrustedAuthAssertion` (`pkg/net/http/protected_routes.go:40-69`).
**Dependencies:** none (independent of 2.1 and 2.3). Cross-check behavior must be a no-op when auth is disabled (local/dev), preserving the auth-off path that works today via the raw header.
**Done when:** the existing 400 behavior for empty/non-UUID `X-Organization-Id` is preserved (already shipped — covered by existing tests, not a new deliverable); with auth enabled, an `X-Organization-Id` that does not match the principal's org claim is rejected (403); with auth disabled, the header is still honored (UUID-validated, as today) so local/dev keeps working; fee-route tests gain the new 403 mismatch case. NOTE: this does not change the Mongo collection partition or the path shape — only the authz-binding of the org value.

> **⛔ BLOCKED — premise refuted at implementation checkpoint (2026-06-06).** The "principal's org claim" does not exist. Evidence: `MarkTrustedAuthAssertion` (`pkg/net/http/protected_routes.go:61,68-69`) seeds only `user_id` and `tenantId`; `tenantId` is a tenant-DB selector (`metadata.go:81,96`), and one tenant holds many organizations. lib-auth v2.8.0 `Authorize` (`middleware.go:216-261`) is a global `(sub, resource, action)` RBAC check with no organization dimension. Comparing `X-Organization-Id` to `tenantId` would 403 every legitimate request. Implementation aborted per the dispatch's blocking checkpoint.
>
> **Revised options (decision pending):**
> 1. **Org-scoped authz (correct, cross-repo):** lib-auth `Authorize` gains a resource-instance dimension (grant on org X) — upstream lib-auth epic, X1-adjacent.
> 2. **Tenant-scoped org-existence guard (shippable in-repo):** verify the requested `X-Organization-Id` exists within the caller's tenant DB. Converts "any org in any tenant" → "any org the caller's tenant owns". Does NOT stop sibling-org targeting inside the same tenant.
> 3. **Defer with explicit risk acceptance:** document that fees authorization is tenant-bounded, not org-bounded; track option 1 upstream.
>
> Deciding question (owner): is intra-tenant org isolation a fees requirement? If yes → only option 1 satisfies; if no → option 2 or 3.
>
> **✅ RESOLVED — no action (owner decision, 2026-06-06):** "não existe risco. o tenant owner é responsável efetivamente por todas as orgs embaixo dele." Intra-tenant org targeting is by design: the tenant is the principal/trust boundary (enforced by DB isolation from the verified `tenantId`); organizations under a tenant share ownership. This matches the platform-wide model — midaz routes take `organization_id` from the path with the same tenant-bounded authz. No code change; no upstream org-dimension issue. The trust model is recorded in the Epic 3.3 decision document.

### Epic 2.3: Delete the dead header-trusting reporter-worker MultiTenantResolver

**Goal:** Remove `MultiTenantResolver` (`consumer.rabbitmq.go:42-56`), which derives tenant identity from the client-controllable `X-Tenant-ID` AMQP header with no vhost/JWT cross-check. Verified DEAD in both live wirings (single-tenant installs `NoOpTenantResolver`; multi-tenant uses the vhost-topology-derived `NewMultiQueueConsumerMultiTenant`, never `ConsumerRoutes`). Deletion over fixing.
**Scope:** `components/reporter-worker/internal/adapters/rabbitmq/consumer.rabbitmq.go:42-56` and any now-dead references (the `NoOpTenantResolver` wiring at `consumer.rabbitmq.go:99-103` stays — confirm what remains after removing the header-trusting implementation); associated tests.
**Dependencies:** none.
**Done when:** `MultiTenantResolver` and its header-reading path are removed; both live wirings (single-tenant nil-mongoManager and multi-tenant `NewMultiQueueConsumerMultiTenant`) still compile and behave identically; no remaining caller references the deleted type; `go build ./...` and the reporter-worker test suite pass. Verify-before-delete: grep the whole monorepo for the type name to confirm no live caller before removal.

- [x] **Done — commit `416909bd2`.** Verify-before-delete evidence recorded: single-tenant path always passes nil mongoManager (`config_multitenant.go:24-27`) so the resolver branch was unreachable; multi-tenant path never constructs `ConsumerRoutes`; `NewConsumerRoutesMultiTenant` had zero callers and was deleted too. Scope ripple (justified by `unparam`): dropped the dead `mongoManager`/`tenantMongoManager` params through `NewConsumerRoutes` → `initConsumerRoutes` → `initSingleTenantWorkerService`. `TenantResolver` seam and `NoOpTenantResolver` preserved. NOTE: 3 pre-existing failures in `internal/bootstrap/config_mt_test.go` (`MULTI_TENANT_SERVICE_API_KEY` validation) reproduce on the clean tree — develop-merge inheritance, out of this epic's scope, surfaced to owner.

---

## Phase 3: Cross-Service Consistency and Authz Model

**Milestone:** Route-chain middleware drift is removed or explicitly documented across deploy units; the ledger legacy dead router is confirmed-and-removed; the RBAC-namespace divergence has a recorded decision + migration sketch coordinated with the X1 plugin-crm gate; and the lib-auth structural gaps are filed as upstream issues. No silent authz footguns remain undocumented.

### Epic 3.1: Reporter-manager adopts a central ProtectedRouteChain-style helper

**Goal:** Replace reporter-manager's per-route hand-listed middleware (auth → tenant → UUID → body, inlined on every route, `routes.go:93-95`) with a single central chain helper mirroring ledger's `ProtectedRouteChain` (`pkg/net/http/protected_routes.go:24-35`), killing composition drift across deploy units.
**Scope:** `components/reporter-manager/internal/adapters/http/in/routes.go`; possibly a new helper in the reporter shared lib or the component's http package.
**Dependencies:** none. No behavior change — auth stays first, tenant second; this is a refactor to a single composition point that future routes inherit. Reporter-manager's confirmed current order is auth → `WhenEnabled(tenant)` → `ParsePathParametersUUID` → `WithBody` (UUID before body). Reconcile against ledger's order — but when this epic is detailed, VERIFY ledger's actual `ProtectedRouteChain` ordering at the chain level (`pkg/net/http/protected_routes.go:24`) before declaring it canonical; the audit spec asserts the divergence exists, not which order ledger truly uses. Pick ledger's order as canonical only after that verification, and unless a reporter-manager route depends on the inverse.
**Done when:** every reporter-manager protected route is registered through the central helper; the chain order matches ledger's documented order; existing route tests pass unchanged (no behavior change); adding a new route no longer requires hand-listing middleware.

- [x] **Done — commit `2f07d2be4`.** Chain-order verification (the plan's mandated first step): ledger's real order is `auth → PostAuthMiddlewares(MarkTrustedAuthAssertion, WithTenantDB) → UUID parse → body → handler`; reporter-manager's was `auth → WhenEnabled(tenant) → UUID/string parse → body → handler` — relative order IDENTICAL, so adoption was a pure refactor. Reused `pkg/net/http.ProtectedRouteChain` directly (no local helper; a 3-line `protected()` closure binds `auth.Authorize` + shared options). All 20 routes migrated. `MarkTrustedAuthAssertion` deliberately NOT adopted — reporter-manager never had it; adding it would be a behavior change outside this epic. New `TestProtectedRouteChain_Composition` locks auth short-circuit, order, and nil-tenant skip.

### Epic 3.2: Confirm-and-remove ledger legacy `in.NewRouter` dead path

**Goal:** Remove the legacy single-module router `in.NewRouter` (`routes.go:34-67`) that re-declares `/health`, `/version`, `/swagger`, and metadata routes with a different public-route set than the production `bootstrap.NewUnifiedServer` path — an audit/drift hazard.
**Scope:** `components/ledger/internal/adapters/http/in/routes.go:34-67`; confirm the production path is exclusively `bootstrap.NewUnifiedServer` (`unified-server.go:60-94`).
**Dependencies:** none. Verify-before-delete: grep for all callers of `NewRouter` across the monorepo (including tests and `cmd/`) and confirm none are live before removal. If a test depends on it, retarget the test to the unified server rather than keeping the dead path.
**Done when:** `in.NewRouter` and its now-unused helpers are removed; no live caller remains; `go build ./...` and the ledger test suite pass; the only route-registration path is `NewUnifiedServer`.

- [x] **Done — commit `731d087a8`.** Verify-before-delete: zero live callers — only 3 tests exercised the dead router itself (deleted) plus one stale comment (fixed). Production path confirmed exclusively `bootstrap.NewUnifiedServer` (`config.go:950`, six RouteRegistrars); metadata routes reach production via `CreateRouteRegistrar`, not `NewRouter`. Orphan cascade: `in.WithSwaggerEnvConfig` (`in/swagger.go`) was referenced only by `NewRouter` — file deleted (bootstrap has its own copy); 6 orphaned imports pruned.

### Epic 3.3: RBAC namespace strategy decision document + migration sketch

**Goal:** Produce a DECISION DOCUMENT (not a code change) capturing the cross-monorepo authz-namespace divergence and a migration sketch — coordinated with, not pre-empting, the X1 plugin-crm policy migration. This is the planning deliverable; a unilateral namespace change is explicitly NOT in scope.
**Scope:** `docs/auth/` (extend or sibling `RBAC-NAMESPACES.md`); analysis only, plus a sketch of the eventual code/policy changes.
**Dependencies:** none for the document; the actual migration is Fred-owned and X1-release-gated.
**Done when:** the document records: (1) the current state — ledger uses `midaz` + `routing` + `plugin-fees` (three namespaces in one binary; `routing` splits account-types/operation-routes/transaction-routes from their `midaz` siblings), tracer uses `"tracer"` (`pkg/constant/app.go:7`), reporter-manager uses `"reporter"` (`pkg/reporter/constant/app.go:7`); (2) the consequence — a `midaz:*` grant silently 403s `routing`, `plugin-fees`, `tracer`, `reporter`; (3) the decision options with a recommendation (unify routing under `midaz`? keep `plugin-fees` per R9? keep tracer/reporter?); (4) a migration sketch sequencing this WITH the X1 plugin-crm → midaz tenant-manager policy migration so integrators absorb one break. The document explicitly defers execution to the X1 gate.

- [x] **Done — commit `14296b4b5`.** `docs/auth/RBAC-NAMESPACES.md` EXTENDED (existing R9/X1 content preserved). All five namespace refs verified accurate against code. Recommendation: A1 fold `routing` into `midaz` at the X1 gate (same binary, same domain, least-defensible split); B1 keep `plugin-fees` (R9 closure, cited in-file — no standalone R9 doc exists); C1 keep `tracer`/`reporter` per-deploy-unit with a documented grant bundle. Net five → four namespaces, one integrator break. Also records the tenant-as-trust-boundary owner decision (2026-06-06) from Epic 2.2.

### Epic 3.4: File lib-auth upstream issues

**Goal:** File the structural lib-auth gaps as upstream issues (lib-commons/lib-auth are MANDATORY — third rail; fixes go upstream, never forked locally). No local code change.
**Scope:** upstream issue tracker for `lib-auth`; reference the verified findings.
**Dependencies:** none.
**Done when:** three issues are filed with reproduction context and the in-repo evidence refs: (1) in-process JWT signature verification option (JWKS — `CASDOOR_JWK_ADDRESS` is dead config today; `ParseUnverified` at `middleware.go:276`); (2) fail-closed default posture option (today `Authorize` fail-opens at `middleware.go:222-224` when disabled); (3) authz response caching / circuit-breaker / retry to remove the synchronous-remote-authz availability amplifier on the hot path. Each issue links the canonical-weakness detail from the audit. The plan records the filed issue IDs back into this section during execution.

- [x] **Done — filed 2026-06-06:** [lib-auth#106](https://github.com/LerianStudio/lib-auth/issues/106) (JWKS opt-in), [lib-auth#107](https://github.com/LerianStudio/lib-auth/issues/107) (fail-closed `AUTH_REQUIRED`), [lib-auth#108](https://github.com/LerianStudio/lib-auth/issues/108) (authz cache/breaker/retry). **Audit correction found during drafting:** `CASDOOR_JWK_ADDRESS` does NOT exist in lib-auth v2.8.0 (no config struct at all — only `ENV_NAME`/`OTEL_LIBRARY_NAME` env reads); the JWKS gap is total absence, not dead config. Issue 1 reframed accordingly. Also verified: the 30s authz bound is a client-wide `http.Client.Timeout` (`middleware.go:57`), not a per-request deadline; `gobreaker`/`backoff` are indirect deps unused in the authz path. midaz commit SHAs intentionally omitted from the public issues (unpushed branch).

---

## Self-Review

| Check | Result |
|-------|--------|
| **Spec coverage** | Critical tracer fail-open → 1.1. Single-tenant ledger fail-open → 1.2. Tracer XFF audit-IP spoof → 2.1. Ledger fees header trust → 2.2. Dead reporter-worker resolver → 2.3. Reporter-manager composition drift → 3.1. Ledger dead router → 3.2. Namespace divergence → 3.3. lib-auth canonical weaknesses (no JWT sig verify, fail-open default, no cache/breaker) → 3.4. Accepted-risk/refuted findings (APIKeyOnlyValidation, metadata-index, CRM path-shape) → Preamble. Logger/Sync/version-stamp bootstrap gaps in the spec's `bootstrap` block are NOT auth findings and are out of this plan's stated goal (auth stabilization) — intentionally excluded; flag to Fred if he wants them folded in. |
| **Vagueness scan** | Phase 1 tasks name exact files, lines, branches, and error messages. No "appropriate"/"TBD"/unnamed edge cases. Mixed-case normalization, auth-off no-op, and the verify-before-delete steps are each named. |
| **Contract consistency** | `resolveDeploymentMode` (existing) reused by 1.1; `ValidateAuthPresence` uses the sibling `(ctx, cfg, logger)` validator signature (`config.go:633`, `665`); `validateProductionConfig` posture (reporter-manager) mirrored by 1.2 keyed on `EnvName=="production"`; 2.2 reuses the org claim seeded by `MarkTrustedAuthAssertion` (`protected_routes.go:40-69`) and the already-shipped UUID parse at `fees_middlewares.go:43-58`. No contract referenced-but-undefined. |
| **Phase boundaries** | Phase 1 ends with two tested boot gates + green lint (working software). Phase 2 ends with hardened request paths + passing suites. Phase 3 ends with refactors/docs/issues, each independently shippable. |
| **Verification plausibility** | Phase-1 verification commands target real package paths (`./components/tracer/internal/bootstrap/`, `./components/ledger/internal/bootstrap/`) with focused `-run` filters and `make lint` per component. |

## Execution Handoff

Plan complete and saved to `docs/plans/2026-06-06-auth-stabilization.md`. Two execution options:

1. **Rolling-Wave Execution (this session)** — ring:executing-plans: implement Phase 1, checkpoint, elaborate Phase 2 into tasks against the real codebase, implement, repeat.
2. **Subagent-Orchestrated (ring:dev-cycle)** — lean backend cycle (Gate 0/8/9) with parallel specialist dispatch.
