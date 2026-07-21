# TRACER HTTP PLANE MAP — Phase 2 (swaggo→Huma) recon

Worktree `/Users/fredamaral/repos/lerianstudio/midaz-huma` · branch `feat/monorepo-consolidation` · module `github.com/LerianStudio/midaz/v4`. All refs non-test source, verified. Produced by tracer-recon agent, 2026-07-01.

**Three findings that reshape the brief's assumptions — read first:**
1. The tracer error path is ALREADY RFC 9457 problem+json (`WithError`→`withProblem`, code→status money-path preserved — a consequence of Phase 1's shared-dispatcher swap). `problem.Install()` REPLACES a local problem model, it does not introduce one. Risk = envelope equivalence, not greenfield. The one legacy-flat-shape leak is `pkgHTTP.Unauthorized` (`response.go:16`).
2. `huma/v2 v2.38.0` + `lib-commons/v5 v5.8.0` (both `openapi` and `problem` pkgs) are ALREADY in the tree (`go.mod`). No dependency-add task.
3. The 4 reservation transition handlers collapse to 2 shared helpers (`terminate`/`terminateByTransaction`) — 2 bodies + 4 thin `huma.Register` shells, not 4 rewrites.

---

## 1. BOOTSTRAP / WIRING

- Fiber app built in ONE func: `NewRoutes(deps RoutesDeps) (*fiber.App, error)` — `routes.go:165`.
- `fiber.New` seam: `routes.go:186-189`, `ErrorHandler: pkgHTTP.CanonicalFiberErrorHandler`.
- Middleware order (`routes.go:194-239`): telemetry → recover → cors (`:214` AllowHeaders has `Authorization,X-API-Key`) → **otelfiber `:220`** → clientIP → httpLogging → faultInjection; closing `tlMid.EndTracingSpans` `:412`.
- Public routes BEFORE `/v1` group (`routes.go:247-255`): `/health`, `/readyz`, `/metrics` (promhttp adaptor), `/version`, **`/swagger/* → fiberSwagger.WrapHandler` `:255`** (current swagger mount).
- Protected group: `api := f.Group("/v1")` `routes.go:259`.
- **Huma mount seam**: right after `fiber.New` (`routes.go:189`), before first `huma.Register`. lib-commons `openapi.New(app *fiber.App, group fiber.Router, cfg Config) huma.API` (`commons/net/http/openapi:75`) wraps `humafiber.NewV2WithGroup` (`:110`). Spec via `openapi.ServeSpec(app, api, logger, prefix, title)` (`:150`).
- **`problem.Install()`** (lib-commons `commons/net/http/problem:50`): once at init, before any Register. Home: bootstrap near `in.SetSelfProbeGate(IsSelfProbeOK)` (`config.go:2038`) or top of `NewRoutes`.
- `NewRoutes` called from ONE place: `bootstrap/config.go:1178` → feeds `NewHTTPServer(cfg, httpApp, seamTLS, ...)` `config.go:1207`. Served via `libCommonsServer.NewServerManager(...).WithHTTPServer` (`http_server.go:77`, mesh) or manual TLS listener (`http_server.go:84-91`, mtls).

## 2. THE 31 HANDLERS
31 = 8 rule + 9 limit + 2 tx-validation + 1 validation + 5 reservation + 3 audit + 3 public. All `func(c *fiber.Ctx) error`. Path params always `c.Params("id"|"transaction_id")`+`uuid.Parse`; bodies always `c.BodyParser`; list queries always `c.QueryParser`. **No handler reads a request header directly** (X-API-Key/Authorization/X-Tenant-Id consumed by middleware).

**rule_handler.go** (receiver `*Handler`): CreateRule `:70` POST /v1/rules, body `CreateRuleInput`+`Validate()`→`http.Created` model.Rule · UpdateRule `:126` PATCH /v1/rules/{id}, `c.Params("id")`+body `UpdateRuleInput`+`Validate()`/`IsEmpty()`→OK · GetRule `:193` GET /v1/rules/{id}→OK · ListRules `:252` GET /v1/rules, `QueryParser(ListRulesInput)`+`Validate()`/`SetDefaults()`→`ListRulesResponse` · ActivateRule `:316` · DeactivateRule `:363` · DraftRule `:410` (POST .../activate|deactivate|draft, id only) · DeleteRule `:457` DELETE→**`http.NoContent` 204 no-body `:483`**.

**limit_handler.go** (receiver `*LimitHandler`): CreateLimit `:71` · GetLimit `:125` · ListLimits `:185` (`QueryParser ListLimitsInput`) · **UpdateLimit `:251` — raw `json.Unmarshal(c.Body())` immutable-field probe `:272-285` before body parse** · ActivateLimit `:338` · DeactivateLimit `:385` · DraftLimit `:432` · DeleteLimit `:479`→**204 `:506`** · GetLimitUsage `:525` GET /v1/limits/{id}/usage→model.UsageSnapshot.

**transaction_validation_handler.go** (`*TransactionValidationHandler`): GetTransactionValidation `:67` GET /v1/validations/{id}→model.TransactionValidation · ListTransactionValidations `:127` GET /v1/validations, `QueryParser(ListTransactionValidationsInput)`, **heavy imperative `input.Validate()` `:256-291`** (cursor-consistency, date-range, 5 UUID filters, enums), `SetDefaults()` `:153`→`ListTransactionValidationsResponse` (array of **`ValidationSummary` flattened DTO**, not full model).

**validation_handler.go** (`*ValidationHandler`): Validate `:80` POST /v1/validations — **100KB payload guard→413 `:90`**, body `model.ValidationRequest`, `clock.Now()`+`NormalizeAndValidate(now)` `:117-118`→**DUAL 201(new)/200(idempotent dup) `:149-154`**. `NewValidationHandler` returns error (nil service/clock) `:46` — why `NewRoutes` returns error at `routes.go:365`.

**reservation_handler.go** (`*ReservationHandler`): Reserve `:81` POST /v1/reservations, body `ReserveRequest`+`NormalizeAndReserveValidate` `:115`→Created `ReserveResponse` · Confirm `:167`/Release `:187` POST .../{id}/confirm|release → shared `terminate(...)` `:279`→OK `ReservationActionResponse` · ConfirmByTransaction `:206`/ReleaseByTransaction `:225` POST .../transaction/{transaction_id}/... → shared `terminateByTransaction(...)` `:240`→OK `TransactionActionResponse`. `NewReservationHandler` returns error on nil `:49`.

**audit_event_handler.go** (`*AuditEventHandler`): ListAuditEvents `:79` GET /v1/audit-events (`QueryParser ListAuditEventsInput`, ~18 filters, `Validate`/`SetDefaults`) · GetAuditEvent `:155`→model.AuditEvent · VerifyHashChain `:203` GET /v1/audit-events/{id}/verify→**`model.HashChainVerificationResult`** (full envelope IsValid+TotalChecked, not bare bool).

**Public** (handlers.go/readyz.go): LivenessHandler() factory `handlers.go:63` GET /health — **plain-text on success (`libHTTP.Ping`), JSON on 503; `@Produce plain`**; gated by `defaultSelfProbeGate` · Version() `handlers.go:94` GET /version→`api.VersionResponse` · ReadyzHandler() factory `readyz.go:93` GET /readyz→`api.ReadyzResponse` (2 concurrent probes via SafeGo `:122-155`, 200/503).

## 3. AUTH (auth_guard.go / apikey.go / jwt_claims.go)
- `guard.With(resource, method, forceAPIKeyAuth)` `:108`: force=true→`apiKeyAuth` directly; else `Protect`.
- `Protect` `:85`: plugin on → **`extractPrincipalFromBearer(c)` FIRST `:90`**, then `authClient.Authorize(AppName, resource, method)(c)` (lib-auth/v2); plugin off → API-key fallback.
- `extractPrincipalFromBearer` `:151`: Bearer via `bearerToken` (`jwt_claims.go:22`), `ParseUnverified`, **rejects 401 `UNAUTHORIZED_MISSING_SUB` if `sub` absent `:163-172`** (hard pre-lib-auth gate), else stamps `Principal{user, sub, name}` `:179`.
- API-key `APIKeyAuth` (`apikey.go:85`): `X-API-Key` header, constant-time compare `:97`, stamps `Principal{api_key, label}` `:101`; disabled→passthrough no principal.
- **`forceAPIKeyAuth` special-case: ONLY `POST /v1/validations`** (`routes.go:370`, `cfg.APIKeyOnlyValidation`). Every other route passes `false`.
- Per-route granularity: `(resource, method)` drives `Authorize`. Resources = `rules|limits|validations|reservations|audit-events`, verbs `post|get|patch|delete`. Mirrors ledger `protectedMidaz`. Reservations = tracer's OWN authz resource, not a ledger plugin namespace (`routes.go:376`).
- **Huma must preserve**: Bearer-first-then-APIkey, the `sub`-required 401 ordering+code, the `(resource,method)` tuple, the validation-POST API-key-only divergence; auth stays MIDDLEWARE (not per-handler); per-op `Security` decls affect spec only.

## 4. SWAGGER SURFACE
- Package securityDefinitions `cmd/app/main.go:50-53`: `@securityDefinitions.apikey ApiKeyAuth`/`@in header`/`@name X-API-Key`. **Bearer MISSING** — confirmed: `api/{docs.go,swagger.yaml}` show only `ApiKeyAuth` (sole block `docs.go:3793`, 28 per-op refs, zero Bearer).
- Per-op `@Security ApiKeyAuth` count = **28** (rule 8, limit 9, reservation 5, audit 3, tx-val 2, validation 1) = exactly the protected /v1 surface. Every handler carries `@Summary/@Description/@ID/@Tags/@Accept/@Produce/@Param/@Success/@Failure/@Router` → maps to `huma.Operation{}` + In/Out struct tags. Samples: CreateRule `rule_handler.go:53-69`; Validate dual-@Success `validation_handler.go:61-79`; ListTransactionValidations ~13 `@Param query` `transaction_validation_handler.go:100-126`.
- Generated artifacts: `api/docs.go` (swaggo 2.0 template), `api/swagger.yaml`, `api/types.go` (response models: `ErrorResponse{code,title,message}:9`, `VersionResponse:16`, `ReadyzResponse:27`, `ReadyzCheck:50`). Huma emits OAS 3.1 → replaced by `openapi.ServeSpec`.

## 5. QUIRKS / SEAMS
- **ReservationService has TWO transports.** REST (5 handlers, `reservation_handler.go`, mounted `routes.go:391-401`) IS part of the 31. gRPC (`grpc/in/reservation_server.go`: Reserve/ConfirmById/ReleaseById/ConfirmByTransaction/ReleaseByTransaction) is SEPARATE, bootstrapped via `NewGRPCServer` (`bootstrap/grpc_server.go:46`), opt-in on `TRACER_GRPC_PORT`. **Phase 2 does NOT touch gRPC.**
- health/readyz/version/metrics/swagger public, pre-`/v1`, unauthenticated by design (K8s probes; `routes.go:242-245`).
- **NO CloudEvents/Kafka/lib-streaming/outbox producer in the HTTP plane** (grep across `internal` non-test/mock = empty). Audit events → Postgres hash-chained, not broker.
- Non-standard shapes: plain-text /health; dual 200/201 Validate; 204 no-body deletes; list DTO wrappers (own cursor DTOs `pkg/net/http/cursor.go`, not ledger mmodel envelope).
- Error envelope already RFC 9457 (`CanonicalFiberErrorHandler` `fiber_error_handler.go:28`; `WithError`→`withProblem` `errors.go:29`).

## 6. MIGRATION LANDMINES
1. **Bearer/ApiKeyAuth spec-vs-runtime gap** — add BearerAuth scheme + dual per-op security (the brief's goal, confirmed).
2. **`forceAPIKeyAuth` on POST /v1/validations** (`routes.go:370`) — the one divergent-auth route.
3. **`sub`-required 401 pre-gate** (`auth_guard.go:163-172`) — preserve ordering + `UNAUTHORIZED_MISSING_SUB` code or audit attribution breaks.
4. **422 risk from imperative validation** — highest: `ListTransactionValidations.Validate()` (`:256-291`, cursor/date/UUID logic struct tags can't express) and `Validate.NormalizeAndValidate` (clock-based, uses `MOCK_TIME`). **Keep these as imperative service-layer validation, do NOT push into struct tags.**
5. **UpdateLimit raw-body immutable probe** (`limit_handler.go:272-285`) — "field present in JSON" not natively Huma-expressible; needs pre-decode hook or nullable-pointer. Money path, don't drop.
6. **Dual-status Validate** (`:149-154`) — Huma one-op-one-default-status; needs explicit per-response status.
7. **204 no-body deletes** (DeleteRule/DeleteLimit) — empty output struct + `DefaultStatus: 204`.
8. **Plain-text /health** — likely stays a raw fiber route outside the huma group (already pre-`/v1`).
9. **Shared reservation helpers** — retype twice, register 4 thin ops.
10. **Tenant middleware/ctx propagation** — JWT-claim tenant MW (`routes.go:268-333`) + reservation-only `reservationTenantMiddleware` (trusts `X-Tenant-Id` on mTLS seam, `reservation_tenant_middleware.go:34`) stash `*sql.DB`/tenant-id into `c.UserContext()` via `tmcore`; handlers read it implicitly through service→repo. **Verify humafiber threads `c.UserContext()` (not a fresh ctx) into the huma handler ctx — the one silent-break risk to test.**
11. **`pkgHTTP.Unauthorized` legacy flat shape** (`response.go:16`, used by `apikey.go:98` + `auth_guard.go:164`) — the ONLY non-problem-envelope 401 path; reconcile if standardizing on `problem.MapError`.

**API facts for tasks**: lib-commons sigs — `openapi.New(app *fiber.App, group fiber.Router, cfg Config) huma.API`, `openapi.ServeSpec(app, api, logger, prefix, title)`, `problem.Install()`, `problem.MapError(...)`. Consolidated ledger routes (per-op-security reference): `components/ledger/internal/adapters/http/in/routes.go` (single component, not split onboarding/transaction).
