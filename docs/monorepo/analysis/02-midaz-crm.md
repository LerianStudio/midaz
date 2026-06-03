# Dossier 02 — Embedding `components/crm` into `components/ledger` (service collapse)

> Target end state (Fred): "liso e final — sem shims, workarounds, abstrações de compatibilidade."
> This is move #4 of the consolidation. Unlike the external repos (tracer, reporter, plugin-fees),
> CRM is **already inside the midaz module** and was migrated in recently. The work here is a
> **runtime/service collapse**, not a module migration.

---

## 0. TL;DR for the planner

- **CRM is NOT a separate Go module.** It lives at `github.com/LerianStudio/midaz/v3/components/crm`, has **no own `go.mod`/`go.sum`**, and is built from the midaz root module. lib-commons is already `v5`, lib-observability already `v1.0.0`. **There is zero dependency skew to resolve.** (Contrast with tracer/reporter/plugin-fees.)
- **CRM is MongoDB-only.** No PostgreSQL, no RabbitMQ, no Redis queue, no schedulers. The only "migration" it owns is best-effort Mongo index creation in single-tenant mode (and it does not even do that — see §5).
- **Domain is fully decoupled from ledger.** Holder/Alias never reference `mmodel.Account/Ledger/Organization/Transaction`. No shared DB, no HTTP calls to ledger, no shared auth state beyond the same `lib-auth` client pattern.
- **A clean mounting seam already exists.** Ledger's `UnifiedServer` accepts `routeRegistrars ...RouteRegistrar` where `RouteRegistrar = func(router fiber.Router)`. CRM routes slot in as a 4th registrar next to onboarding/transaction/ledger.
- **Collapse difficulty: LOW-to-MEDIUM.** The code moves cleanly. The friction is concentrated in three places: (1) the per-app Fiber middleware stack (CRM re-applies recover/telemetry/CORS that UnifiedServer already applies, and a **global error-code-rewriting shim** that would corrupt ledger errors), (2) a **third tenant scope** — CRM's tenant-manager module name is `crm-api`, distinct from ledger's `onboarding`/`transaction`, so it needs its own `tmmongo.Manager` and its own per-request DB injection, and (3) config namespacing — CRM's flat `MONGO_*` env collides with nothing in ledger only because ledger uses `Onb*`/`Txn*` prefixes; CRM's Mongo needs a `CRM_*`-prefixed home.

---

## 1. Module boundary & dependency surface

### 1.1 Module identity
- **No `go.mod` in `components/crm`.** Compiled as part of `github.com/LerianStudio/midaz/v3` (root `go.mod`, Go 1.26.3).
- Binary built directly from root context: `go build ... -o /app components/crm/cmd/app/main.go` (see `components/crm/Dockerfile`).
- **Consequence:** no module merge, no `go.sum` reconciliation, no replace directives, no import-path rewrite. This is the single biggest reason CRM is the easiest of the five moves.

### 1.2 What CRM imports from midaz `pkg/` (already shared)
| Import | Sites | Notes |
|---|---|---|
| `pkg/mmodel` | 39 | `Holder`, `Alias`, `RelatedParty`, `CreateHolderInput`, `UpdateHolderInput`, `CreateAliasInput`, `UpdateAliasInput`, plus value types `Address(es)`, `BankingDetails`, `Contact`, `Date`, `LegalPerson`, `NaturalPerson`, `RegulatoryFields`, `Representative`. Domain models for holder/alias **already live in `pkg/mmodel/holder.go` and `pkg/mmodel/alias.go`** — shared package, no move needed. |
| `pkg/net/http` | 20 | Reuses `WithBody`, `ParseUUIDPathParameters`, `WithRecover`, `WithRecoverLogger`, `QueryHeader`. Same helpers ledger uses. |
| `pkg/constant` | 20 | Error sentinels + CRM-specific error codes (see §4). |
| `pkg/utils` | 4 | e.g. `utils.TenantConnectionErrorsTotal` metric name. |
| `pkg/mongo` | 3 | `ExtractMongoPortAndParameters`, `BuildURI` helpers. |

All on lib-commons v5 / lib-observability v1.0.0 — identical to ledger. No skew.

### 1.3 lib-commons / lib-observability usage
- Migrated to lib-observability v1.0.0 in commit `04a421a65`.
- One cosmetic oddity worth flagging: several CRM files alias `libCommons "github.com/LerianStudio/lib-observability"` (NOT `lib-commons`) — e.g. `holder.mongodb.go`, `alias.mongodb.go`, `holder.go`, most `services/*.go`. It works (they call `libCommons.NewTrackingFromContext`), but the alias name `libCommons` pointing at the **observability** module is misleading. **In a "liso e final" state, rename these to `libObs` / drop the misleading alias.** Not a blocker, but it is exactly the kind of cruft the target state forbids.

---

## 2. Runtime / deploy model

### 2.1 As it runs today (standalone service)
- **Entrypoint:** `components/crm/cmd/app/main.go` → `bootstrap.InitServersWithOptions(&Options{Logger})` → `service.Run()`.
- **Port:** `4003` (`SERVER_PORT=4003`, `SERVER_ADDRESS=:4003`; Dockerfile `EXPOSE 4003`).
- **Lifecycle:** `libCommons.NewLauncher(...)` runs two apps:
  1. `RunApp("HTTP Service", app.Server)` — the Fiber HTTP server via `libCommonsServer.NewServerManager().WithHTTPServer().StartWithGracefulShutdown()`.
  2. `RunApp("Tenant Event Listener", eventListenerRunnable)` — **only when MT enabled and `MULTI_TENANT_REDIS_HOST` set** — a Redis Pub/Sub listener for tenant lifecycle events (suspend/delete/disassociate → evict Mongo pools + invalidate config cache).
- **Graceful drain:** `Server.Run` registers Fiber `OnListen` (mark readyz ready) and `OnShutdown` (StartDrain → sleep `DefaultDrainDelay` → return) hooks. **This is the exact same drain pattern ledger's `UnifiedServer` already implements** — so on collapse, CRM's hooks are redundant and must be dropped (ledger owns them).

### 2.2 As it must run after collapse (inside ledger's process)
- **No separate binary, no port 4003.** CRM routes mount on ledger's single port (`:3002`).
- **No CRM `main.go`, no CRM `Server`, no CRM `Service.Run`, no CRM `eventListenerRunnable`.** All lifecycle is owned by ledger's `Service.Run` + `UnifiedServer`.
- **The tenant event listener is the one runtime concern that does NOT trivially fold away.** Ledger already runs its own `tmevent.TenantEventListener` (field `Service.EventListener`). CRM's listener evicts **Mongo** pools for the `crm-api` module. Two options (see Decisions):
  - (a) Add CRM's mongo manager to ledger's existing dispatcher's `WithOnTenantRemoved` so one listener evicts all modules' pools, or
  - (b) keep a second listener. Option (a) is the "liso" answer; (b) is a workaround.

### 2.3 Docker / compose
- `components/crm/Dockerfile`: standalone distroless static binary, builds from root context. **Deleted on collapse** (no separate image).
- `components/crm/docker-compose.yml`: defines a `midaz-crm` container on `infra-network` + `crm-network`, env from `.env`, port `${SERVER_PORT}`. **Deleted on collapse**; CRM's Mongo dependency must be folded into ledger's compose (ledger already runs its own Mongo(s); decide whether CRM shares one — see §5/Decisions).
- **No Helm/k8s manifests found in-repo** (deployment manifests live elsewhere). The planner must account for: removing the CRM Deployment/Service, removing port 4003 from any ingress, and re-pointing API consumers from the CRM host to the ledger host. The API path prefix (`/v1/holders`, `/v1/aliases`) does not collide with ledger paths, so routing-by-path on a shared gateway is feasible.

---

## 3. Routes & HTTP integration

### 3.1 CRM routes (`components/crm/internal/adapters/http/in/routes.go`)
Application name for authz: `const ApplicationName = "plugin-crm"`.

```
POST   /v1/holders
GET    /v1/holders/:id
PATCH  /v1/holders/:id
DELETE /v1/holders/:id
GET    /v1/holders
GET    /v1/aliases
POST   /v1/holders/:holder_id/aliases
GET    /v1/holders/:holder_id/aliases/:alias_id
PATCH  /v1/holders/:holder_id/aliases/:alias_id
DELETE /v1/holders/:holder_id/aliases/:alias_id
DELETE /v1/holders/:holder_id/aliases/:alias_id/related-parties/:related_party_id
```
- Each route: `auth.Authorize("plugin-crm", <resource>, <verb>)` + `ParseUUIDPathParameters(...)` + optional `WithBody(...)`.
- **No path collision with ledger.** Ledger owns `/v1/settings/...`, `/v1/organizations/...`, transaction paths, etc. `/v1/holders` and `/v1/aliases` are free.
- **Org scoping is via HTTP header `X-Organization-Id`, not a path param.** Handlers do `organizationID := c.Get("X-Organization-Id")` and thread it into the repo (see §5). This differs from ledger's hierarchical `/v1/organizations/:org_id/ledgers/:ledger_id/...` path scoping. After collapse the header convention can stay (no collision), but it is a stylistic divergence the planner should note.

### 3.2 The mounting seam (good news)
Ledger's `UnifiedServer` (`components/ledger/internal/bootstrap/unified-server.go`):
```go
type RouteRegistrar func(router fiber.Router)
func NewUnifiedServer(serverAddress string, logger, telemetry, readyzHandler, routeRegistrars ...RouteRegistrar) *UnifiedServer
```
Called with `onboardingRouteRegistrar, transactionRouteRegistrar, ledgerRouteRegistrar`. **CRM becomes a 4th `crmRouteRegistrar`** that registers the 11 routes onto the shared app. Ledger's own `RegisterMetadataRoutesToApp(f fiber.Router, ...)` is the template to mirror — CRM should expose `RegisterCRMRoutesToApp(f fiber.Router, auth, holderHandler, aliasHandler)`.

### 3.3 The middleware conflict (the real work)
CRM's `NewRouter` builds a **full standalone Fiber app** with its own middleware stack:
```go
f.Use(ErrorCodeTransformer())              // <-- COMPAT SHIM, app-global
f.Use(http.WithRecover(...))               // duplicate of UnifiedServer
f.Use(tlMid.WithTelemetry(tl))             // duplicate of UnifiedServer
f.Use(cors.New())                          // duplicate of UnifiedServer
f.Use(libObsMiddleware.WithHTTPLogging(...))// duplicate of UnifiedServer
f.Get("/health" ...), /version, /swagger, /readyz   // duplicate of UnifiedServer
if tenantMw != nil { f.Use(tenantMw) }     // tenant DB injection
... routes ...
f.Use(tlMid.EndTracingSpans)               // duplicate of UnifiedServer
```
On collapse:
- **Drop** the duplicated recover/telemetry/CORS/logging/health/version/swagger/readyz/EndTracingSpans — `UnifiedServer` already applies them once for the whole app.
- **The `ErrorCodeTransformer()` is the dangerous one.** It is registered as an **app-global `f.Use`** middleware that rewrites the `"code"` field of **every 4xx/5xx JSON response** via `CRMErrorMapping` (`error_mapping.go`, `error_transformer.go`). If mounted globally on ledger's shared app, it would rewrite **ledger's** error codes too (e.g. a ledger `0046` internal-server would become `CRM-0014`). **This must NOT be a global middleware on the unified app.** Options:
  - (a) **Delete it entirely** (truest "liso e final" — accept that holder/alias clients now see canonical midaz error codes, breaking the CRM-specific `CRM-00xx` contract). The shim's own comment admits it exists only "for backward compatibility ... after the migration from a standalone repository." That is precisely a compatibility abstraction Fred wants gone.
  - (b) Scope it to a Fiber **route group** mounted under the CRM routes only (e.g. a sub-router) so it never sees ledger responses. Still a shim, but contained.
  - This is a **product decision** (do external CRM API consumers depend on `CRM-00xx` codes?), not a pure engineering one — flag to Fred.
- **The tenant middleware** (`tenantMw`) cannot be global either: ledger's tenant middleware resolves Postgres+Mongo for `onboarding`/`transaction`; CRM needs the `crm-api` Mongo. Either CRM routes get a group-scoped tenant middleware bound to the CRM mongo manager, or ledger's middleware is extended to also inject the CRM mongo DB. Group-scoping is cleaner.

---

## 4. Errors & constants (already co-located, mostly clean)
- CRM-specific error sentinels **already live in midaz `pkg/constant/errors.go`** (31 `CRM-*` entries, `CRM-0001`..`CRM-00xx`, e.g. `ErrHolderNotFound=CRM-0006`, `ErrAliasNotFound=CRM-0008`, `ErrHolderHasAliases=CRM-0017`, `ErrAccountAlreadyAssociated=CRM-0013`). No duplicate-sentinel problem; they coexist with ledger codes.
- The `CRMErrorMapping` (generic→CRM code translation) is the only thing tied to the runtime shim discussed in §3.3. If the shim is deleted, `error_mapping.go` + `error_transformer.go` + `error_transformer_test.go` are deleted with it; the `CRM-00xx` sentinels that map 1:1 to generic codes (`CRM-0001`,`-0002`,`-0003`,`-0005`,`-0007`,`-0009`,`-0011`,`-0012`,`-0014`,`-0015`,`-0016`,`-0004`) become dead and should be pruned. The **domain** CRM codes (HolderNotFound, AliasNotFound, etc.) stay.
- No `EntityHolder`/`EntityAlias` constants found in `pkg/constant/entity.go` — CRM does not appear to use the `constant.Entity*` pattern for entity-name resolution (worth a check during implementation; if it uses `reflect.TypeOf`, that violates project rules and should be fixed during the move).

---

## 5. Persistence & multi-tenancy (the heart of the collapse)

### 5.1 Storage shape
- **MongoDB only.** Repos: `internal/adapters/mongodb/holder/*` and `internal/adapters/mongodb/alias/*`. No SQL, no squirrel, no Postgres adapter anywhere in CRM.
- **Per-organization dynamic collections, NOT fixed collections.** Repos write to `db.Collection(strings.ToLower("holders_" + organizationID))` and `..."aliases_" + organizationID`. The collection name is derived at request time from the `X-Organization-Id` header. This is structurally different from ledger's fixed metadata collections (`organization`, `ledger`, `account`, ...). Implication: there are **no static migrations to merge**; collections materialize on first write. CRM's only index management is `ensure*Indexes`-style — and in CRM's `buildReadyzHandler`/bootstrap path I found **no equivalent of ledger's `ensureOnboardingMongoIndexes`** being called; CRM appears to rely on Mongo auto-creating collections with no explicit index pass. (Verify during implementation — if there is index creation it is per-org and lazy.)
- **Field-level crypto:** CRM constructs a `libCrypto.Crypto{HashSecretKey, EncryptSecretKey}` and passes it into both repos (PII encryption on holder/alias fields). This is CRM-specific config (`LCRYPTO_HASH_SECRET_KEY`, `LCRYPTO_ENCRYPT_SECRET_KEY`) that ledger does not currently use. **Must be carried into ledger config** (new prefixed env, e.g. `CRM_LCRYPTO_*`, or reuse if ledger ever adds crypto).

### 5.2 Connection ownership — single-tenant
- CRM today owns its own `libMongo.Client` built from flat env `MONGO_URI`/`MONGO_HOST`/`MONGO_NAME=crm`/`MONGO_USER`/`MONGO_PASSWORD`/`MONGO_PORT=5703`/`MONGO_MAX_POOL_SIZE`/`MONGO_TLS_CA_CERT` (`initMongoConnection` in `config.go`).
- **Env collision:** ledger uses `Onb*`/`Txn*`-prefixed Mongo env (`OnbPrefixedMongoURI`, etc.). CRM's **flat** `MONGO_*` keys would collide with nothing ledger reads today — but the "liso e final" answer is to give CRM a **`CRM_*` env prefix** (e.g. `CRM_MONGO_HOST`, `CRM_MONGO_NAME`, ...) and add a `CrmPrefixed*` field block to ledger's `Config`, mirroring how onboarding/transaction Mongo are separated. CRM's own Mongo client becomes a third Mongo connection owned by ledger's bootstrap (or shares ledger's onboarding/transaction Mongo if the DB is the same instance — a deploy decision).
- **DB pool ownership:** after collapse, the CRM Mongo client is created in ledger's `InitServersWithOptions` and registered for graceful close like ledger's other connections.

### 5.3 Connection ownership — multi-tenant (the hard part)
- CRM resolves per-tenant Mongo via tenant-manager with **its own module name**: `const moduleName = "crm-api"` (`config.tenant.go`). Note commit `3f38a8c8c` just fixed this from `crm` → `crm-api` to align with provisioning. Ledger's modules are `constant.ModuleOnboarding` and `constant.ModuleTransaction`.
- CRM builds its own `tmmongo.NewManager(tmClient, tenantServiceName, WithModule("crm-api"), ...)`, its own `tenantcache`, `tenantLoader`, `tmmiddleware.NewTenantMiddleware(WithMB(mongoManager), ...)`, and its own `TenantEventListener` (Redis Pub/Sub) with `WithOnTenantRemoved` closing the `crm-api` Mongo pool + invalidating config cache.
- **Tenant *service* name is shared:** CRM's `.env` sets `APPLICATION_NAME=ledger` (the tenant-manager *service* identity) while the *module* key is `crm-api`. Ledger's `ApplicationName = "ledger"` too. So in MT, both resolve tenants under the **same service `ledger`** but **different modules** — meaning the tenant manager already knows about a `crm-api` module under the `ledger` service. **This is the cleanest possible MT story for a collapse:** the tenant client/cache can be shared; only a third `tmmongo.Manager` (module `crm-api`) and a CRM-scoped tenant middleware need to exist inside ledger.
- **What must change in MT:**
  1. Ledger's bootstrap creates a `tmmongo.Manager` for module `crm-api` (reusing ledger's existing `tmClient` + `tenantCache` + `tenantLoader`).
  2. CRM routes get a **group-scoped tenant middleware** bound to that manager (so it injects the `crm-api` Mongo DB into context for CRM handlers only — ledger handlers keep their onboarding/transaction managers).
  3. CRM's standalone `TenantEventListener` is **folded into ledger's existing dispatcher**: add the `crm-api` Mongo manager to ledger's `WithOnTenantRemoved` eviction so a single listener evicts onboarding + transaction + crm-api pools on tenant suspend/delete. Running a second listener is the workaround; folding is the "liso" answer.
  4. The per-request DB lookup in the repos already uses `tmcore.GetMBContext(ctx)` with a static fallback (`getDatabase` in `holder.mongodb.go`/alias) — **no repo change needed** as long as the CRM-scoped middleware populates the right Mongo DB into context.

### 5.4 Background work / schedulers / queues
- **None beyond the tenant event listener.** No cron, no worker pool, no RabbitMQ consumer, no Redis queue consumer. CRM is request/response + (optional) tenant Pub/Sub listener only. This is far simpler than reporter (manager+worker) or ledger (RabbitMQ/Redis/balance-sync workers).

---

## 6. Config / env consolidation

CRM env that must be absorbed into ledger's `Config` (with a `CRM_`/`Crm*Prefixed` namespace to stay "liso"):
- Mongo: `MONGO_URI/HOST/NAME/USER/PASSWORD/PORT/PARAMETERS/MAX_POOL_SIZE/TLS_CA_CERT`.
- Crypto (CRM-only today): `LCRYPTO_HASH_SECRET_KEY`, `LCRYPTO_ENCRYPT_SECRET_KEY`.
- Auth: `PLUGIN_AUTH_ADDRESS`, `PLUGIN_AUTH_ENABLED` — **already shared semantics with ledger** (ledger uses `PLUGIN_AUTH_HOST`/`PLUGIN_AUTH_ENABLED` + `CASDOOR_JWK_ADDRESS`; reconcile the auth-address key name).
- Multi-tenant: `MULTI_TENANT_*` — **structurally identical to ledger's MT block** (same keys: URL, TIMEOUT, CB threshold/timeout, MAX_TENANT_POOLS, IDLE_TIMEOUT_SEC, SERVICE_API_KEY, REDIS_*, CACHE_TTL_SEC, CONNECTIONS_CHECK_INTERVAL_SEC). These **merge into ledger's single MT block** — no new keys except the module wiring is code-side.
- Observability: `OTEL_*`, `ENABLE_TELEMETRY`, `LOG_LEVEL`, `ENV_NAME`, `DEPLOYMENT_MODE`, `VERSION` — all shared with ledger; CRM's `OTEL_RESOURCE_SERVICE_NAME=crm` and `OTEL_LIBRARY_NAME=.../components/crm` get dropped (one unified service identity = ledger).
- Server: `SERVER_PORT=4003` / `SERVER_ADDRESS` — **dropped**; ledger's `:3002` is the single port.

The `.env.example` already carries an inline note that CRM was migrated from a standalone repo and that `APPLICATION_NAME=ledger` — confirming the intent to converge on the ledger service identity.

---

## 7. Build / Docker / CI
- **Dockerfile + docker-compose.yml: deleted** (no standalone image/container).
- **CRM `Makefile` (~15.8K, full standalone target set: build/run/up/down/test/lint/format/sec/generate-docs/validate-api-docs/...): collapses** into ledger's Makefile / root `make ledger COMMAND=...` delegation. The CRM-specific `validate-api-docs`/`validate-api-implementations` JS scripts (`scripts/`) and the `.swaggo`/`api/` swagger artifacts either fold into ledger's swagger generation or are dropped if the unified ledger swagger absorbs holder/alias docs.
- **Swagger:** CRM ships its own `api/openapi.yaml`/`swagger.json`/`swagger.yaml` and `api/docs.go` with a `@title CRM API @host localhost:4003`. On collapse, holder/alias endpoints either (a) merge into ledger's unified swagger (single `/swagger`), or (b) the separate CRM swagger is dropped. The unified ledger swagger already advertises "all Onboarding + Transaction + Metadata endpoints in a single service" — adding CRM is consistent with that framing.
- **CI:** verify the root `.github/workflows` build/test already cover `components/crm` (they should, since it is in the root module). The standalone CRM release/build pipeline (if any external one references the CRM image/tag) must be retired. (Workflows not deep-read here — flag for the build-owner.)

---

## 8. Test surface
- CRM has substantial unit + integration tests co-located: `internal/services/*_test.go` (per use-case), `internal/adapters/mongodb/{holder,alias}/*_test.go` including `*_integration_test.go` and `*_tenant_test.go` (testcontainers-style), `internal/adapters/http/in/*_test.go`, and `internal/bootstrap/*_test.go` (readyz, tls_detection, config, backward_compat).
- **`backward_compat_test.go` + `error_transformer_test.go`** test the compat shim. If the shim is deleted (§3.3 option a), these tests are deleted too.
- Bootstrap tests (`readyz_test.go`, `config_test.go`, `tls_detection_test.go`) target CRM's standalone bootstrap; on collapse most of this logic moves to ledger's bootstrap and the tests either migrate or are subsumed by ledger's bootstrap tests.

---

## 9. Coupling to ledger — summary
| Coupling type | Present? | Detail |
|---|---|---|
| Shared Go module | YES (already) | Same `midaz/v3` module — the enabling fact. |
| Shared `pkg/` | YES | `mmodel` (holder/alias models), `net/http`, `constant`, `utils`, `mongo`. |
| Shared domain entities | NO | Holder/Alias never reference Account/Ledger/Organization/Transaction. |
| Shared database | NO | CRM = own Mongo (`crm` DB / per-org collections); ledger = Postgres + onboarding/transaction Mongo. |
| HTTP calls to ledger | NO | None found. |
| Shared auth | PARTIAL | Same `lib-auth` `middleware.NewAuthClient` pattern; different authz app name (`plugin-crm`). |
| Shared tenant infra | PARTIAL | Same tenant *service* (`ledger`) in MT, but distinct *module* (`crm-api`) and a separate `tmmongo.Manager` + listener. |
| Shared queues/workers | NO | CRM has no queues/workers (only optional tenant Pub/Sub listener). |

**Verdict:** clean module boundary, deeply decoupled domain, single shared concern that needs real engineering = the multi-tenant Mongo wiring (third module manager + listener fold) and the middleware-stack de-duplication (especially neutralizing/removing the global error-code shim).

---

## 10. Collapse difficulty estimate: **LOW-to-MEDIUM** (~3-5 focused days)
Ordered by effort:
1. **Tenant Mongo wiring (highest):** add `crm-api` `tmmongo.Manager` to ledger bootstrap, CRM-scoped tenant middleware, fold CRM eviction into ledger's existing `TenantEventListener`. Carry `LCRYPTO_*` crypto config. ~1.5-2 days incl. MT integration tests.
2. **Route mounting + middleware de-dup:** `crmRouteRegistrar` + `RegisterCRMRoutesToApp`; strip duplicated middleware; **decide the fate of `ErrorCodeTransformer`** (product call). ~1 day.
3. **Config consolidation:** `CrmPrefixed*` Mongo block + crypto in ledger `Config`; merge MT block; reconcile auth-address key. ~0.5 day.
4. **Single-tenant Mongo client ownership** in ledger bootstrap + graceful close registration + readyz checker fold. ~0.5 day.
5. **Teardown:** delete CRM `main.go`/`Server`/`Service`/`bootstrap` runtime, Dockerfile, compose, Makefile, swagger duplication, dead `CRM-00xx` 1:1 codes, misleading `libCommons`-aliasing-observability imports. Migrate/subsume tests. ~0.5-1 day.

The estimate assumes the product decision on the error-code shim is "delete" (cleanest). If the `CRM-00xx` wire contract must be preserved, add ~0.5 day to scope a route-group-local transformer (and accept it as a retained shim — a direct tension with "sem shims").
