# Dossier 01 — midaz `components/ledger`: the integration host for fees + crm

Scope: this dossier maps `components/ledger` as the **host** that must absorb (a) `plugin-fees` logic
collapsed into the transaction-create flow, and (b) `components/crm` collapsed into the same binary.
It is written for a planner who has never opened the repo. Co-location of `tracer`/`reporter` is out of
scope here (separate dossiers) except where they touch the same composition root, build, or CI.

Module: `github.com/LerianStudio/midaz/v3`. Go `1.26.3`. License Elastic 2.0. lib-commons `v5`
(observability split into `lib-observability`). Single git repo on `develop`.

---

## 1. Runtime / deploy model (as-is)

`components/ledger` is **already a "unified" service**. A prior consolidation merged what used to be
separate `onboarding` and `transaction` services into one binary, one port, one Fiber app. This is the
single most important fact for the whole monorepo plan: **the host already proved the "service collapse"
pattern internally.** CRM and fees are the next two collapses into the same machine.

- One binary: `components/ledger/cmd/app/main.go` → `bootstrap.InitServersWithOptions` → `service.Run()`.
- One HTTP port: `SERVER_ADDRESS` (default `:3002`). All routes (onboarding + transaction + ledger
  metadata-index) are registered on a single `fiber.App` via `UnifiedServer`.
- Lifecycle is lib-commons `Launcher` + `RunApp(name, app)` (`internal/bootstrap/service.go`). The
  `Service` struct holds every long-lived runnable; `Run()` conditionally registers each as a Launcher app:
  - Unified HTTP Server (always)
  - RabbitMQ consumer (single-tenant `MultiQueueConsumer` **or** multi-tenant `MultiTenantConsumer`)
  - Redis queue consumer (write-behind balance pipeline)
  - Balance Sync Worker + Legacy Balance Sync Drainer
  - Tenant Event Listener (Redis Pub/Sub, MT only)
  - Circuit Breaker Health Checker
  - Streaming Producer close hook (lib-streaming drain on SIGTERM)
- Graceful drain is real: `UnifiedServer` registers Fiber `OnListen`/`OnShutdown` hooks coupling
  `/readyz` to a 12s drain delay (`internal/bootstrap/unified-server.go`).

Infra dependencies the host already owns: **2 PostgreSQL logical DBs** (onboarding, transaction),
**2 MongoDB DBs** (onboarding metadata, transaction metadata), **1 Redis** (shared connection, two
consumer repos), **1 RabbitMQ**, optional **Kafka** (lib-streaming).

Deploy artifact: `components/ledger/Dockerfile` builds from **repo root context** (`context: ../../` in
`docker-compose.yml`), compiles `components/ledger/cmd/app/main.go`, and **copies the migration SQL trees
into the image** (`/components/ledger/migrations/{onboarding,transaction}`). distroless static final image.

---

## 2. Composition root / DI wiring — `internal/bootstrap/config.go` (~1130 lines)

This is the spine. `InitServersWithOptions(opts *Options)` does everything in one (deliberately
`//nolint:gocognit,gocyclo`) function. Order matters; reproduce it when adding fees/crm:

1. `Config{}` populated by `libCommons.SetConfigFromEnvVars` then `applyConfigDefaults` (struct `default`
   tags are NOT honored by the loader — defaults are applied programmatically).
2. Guard: `MULTI_TENANT_ENABLED=true` requires `PLUGIN_AUTH_ENABLED=true` (hard fail).
3. Logger (`libZap`), startup UUID, telemetry (`libOpentelemetry.NewTelemetry` + `ApplyGlobals()`).
4. Tenant client + tenant cache + tenant loader (MT only).
5. Infrastructure init, each with a reverse-order cleanup closure:
   - `initOnboardingPostgres` → 7 repos (org, ledger, segment, portfolio, account, asset, accounttype)
   - `initTransactionPostgres` → 6 repos (transaction, operation, assetrate, balance, operationroute, transactionroute)
   - `initOnboardingMongo` / `initTransactionMongo` → 1 metadata repo each
   - `initRedisConnection` → shared `*libRedis.Client`, then `onbRedis`/`txRedis` consumer repos
   - `initRabbitMQ` → producer + consumer, then `rmq.pgManager`/`rmq.mongoManager` injected for per-message tenant resolution
   - `BuildStreamingEmitter` → `libStreaming.Emitter` (NoopEmitter when disabled)
6. **Two god-structs are assembled**: `command.UseCase` and `query.UseCase` (both in
   `internal/services/{command,query}`). Each is a flat bag of repo-interface fields (see
   `internal/services/command/command.go`). All ~13 repos + Mongo + Redis + RabbitMQ + Streaming are fields.
7. `rmq.wireConsumer(commandUseCase)` — the consumer needs the command UC to process async transactions.
8. **Handlers** are constructed as `&httpin.XHandler{Command: commandUseCase, Query: queryUseCase}` —
   12 handlers (7 onboarding + 5 transaction + 1 metadata-index). Handlers are thin; they hold pointers to
   the two UCs.
9. `buildUnifiedRouteSetup` builds the multi-tenant middleware + per-module `ProtectedRouteOptions`.
10. Three **RouteRegistrar** closures (`onboardingRouteRegistrar`, `transactionRouteRegistrar`,
    `ledgerRouteRegistrar`) are passed variadically to `NewUnifiedServer`.
11. `buildReadyzHandler` aggregates dependency checkers across all infra.
12. Workers constructed (Redis consumer, balance sync worker, legacy drainer), `Service` returned.

`Options` (injected by callers / tests): `Logger`, `CircuitBreakerStateListener`, `TenantClient`,
`TenantCache`, plus resolved MT fields. This is the test seam — `InitServersWithOptions(&Options{...})`.

### Why this matters for the collapse
The DI model is **constructor-injection into a single fat UseCase + variadic RouteRegistrars**. There is
no DI container, no service locator. Adding a domain means: add repos to infra init, add fields to the
UseCase struct(s) (or a new UseCase), add handlers, add one more RouteRegistrar to the `NewUnifiedServer`
call. The `UnifiedServer` signature is already `routeRegistrars ...RouteRegistrar` — **CRM and fees routes
plug in with zero changes to UnifiedServer.** That is the clean seam Fred wants.

---

## 3. The transaction-create flow end-to-end + the EXACT fee hook seam

### Route surface (`internal/adapters/http/in/routes.go`, `RegisterTransactionRoutesToApp`)
All five creation modes are distinct POST routes that **funnel into one private method**:

- `POST …/transactions/json`   → `CreateTransactionJSON`   → `createTransaction(c, input, input.InitialStatus())`
- `POST …/transactions/dsl`    → `CreateTransactionDSL`    (deprecated, parses gold DSL → same input) → `createTransaction`
- `POST …/transactions/inflow` → `CreateTransactionInflow` → `createTransaction`
- `POST …/transactions/outflow`→ `CreateTransactionOutflow`→ `createTransaction`
- `POST …/transactions/annotation` → `CreateTransactionAnnotation` → `createTransaction(c, input, constant.NOTED)`
- `POST …/transactions/{id}/commit|cancel|revert` → state handlers (`transaction_state_handlers.go`);
  revert routes through `createRevertTransaction` → `executeCreateTransaction(..., isRevert=true)`.

(Handler bodies live in `transaction.go`; the orchestration lives in `transaction_create.go`.)

### The single orchestration funnel
`createTransaction` / `createRevertTransaction` both call
**`executeCreateTransaction(c, transactionInput, transactionStatus, isRevert)`** in
`internal/adapters/http/in/transaction_create.go` (the `//nolint:gocyclo` method, ~lines 969–1273).
Linearized:

1. read path params, generate `transactionID` (UUIDv7), validate transaction date.
2. reject non-positive `Send.Value`.
3. `ApplyDefaultBalanceKeys` + `MutateConcatAliases` to build `fromTo` list.
4. **Idempotency**: hash body, `CreateOrCheckTransactionIdempotency` (Redis); early-return on replay.
5. **`ValidateSendSourceAndDistribute(ctx, transactionInput, status)`** → `validate` (sources, destinations, aliases, amounts).
6. `GetParsedLedgerSettings` (accounting route validation toggle).
7. `SendTransactionToRedisQueue` (backup/recovery seed — runs with nil balances).
8. `GetBalances` for all aliases; `rejectInternalScopeBalances`.
9. `buildBalanceOperations` → `enrichOverdraftOperations` (appends companion `#overdraft` legs + `companionFromTos`).
10. `ValidateAccountingRules` → `routeCache`.
11. **`ProcessBalanceOperations`** — the Lua/atomic balance mutation (Redis). After this, balances are mutated.
12. `BuildOperations` (double-entry op records) → `WriteTransaction` (DB, sync or async via RabbitMQ).
13. background goroutines: idempotency value set, audit-log queue.

### THE FEE SEAM (load-bearing conclusion)
Fees must rewrite `transactionInput.Send.Source.From` and `transactionInput.Send.Distribute.To` (adding
fee legs and increasing `Send.Value`) **BEFORE step 5's validation consumes them and BEFORE step 7 seeds
the backup queue.** That is exactly what plugin-fees does today externally:
`pkg/fee/calculate-fee.go::CalculateFee` mutates `f.Transaction.Send.Value` and sets
`f.Transaction.Send.Source.From = updatedAmountsFromFee(resp.From)` /
`...Distribute.To = updatedAmountsFromFee(resp.To)` (`pkg/fee/distribute.go`).

**Canonical insertion point: between step 3 (alias normalization) and step 5
(`ValidateSendSourceAndDistribute`), i.e. immediately after `ApplyDefaultBalanceKeys` (~line 1007) and
before idempotency hashing (~line 1020).** Rationale, in priority order:

- It is **pre-validate, pre-persist, pre-balance-mutation** — the entire downstream machinery
  (overdraft enrichment, accounting-rule validation, double-entry op build, async/sync write) then
  operates on the fee-inclusive transaction with **zero further changes**. Fees become "just more FromTo legs."
- It must be **before idempotency hashing** if you want the persisted/replayed transaction to include
  the fee legs (almost certainly yes). If fees were applied after the hash, replays would skip fees.
- It is **before** the backup Redis seed, so crash-recovery reconstructs the fee-inclusive transaction.

Operates on `mtransaction.Transaction` (alias `mtransaction` = `pkg/mtransaction`; fees today imports the
**published** `pkg/transaction` path at `midaz/v3 v3.5.2` — see §7 conflict). The fee engine needs the
same `Send`/`FromTo`/`Amount` types the host already uses, so this is type-compatible **once the package
path is reconciled** (`pkg/transaction` vs `pkg/mtransaction`).

A clean embed adds the helper as a private method on `TransactionHandler`, e.g.
`handler.applyFees(ctx, &transactionInput, params)` returning the mutated input or a business error, mirroring
the existing `enrichOverdraftOperations` pattern (which is itself an in-flow enrichment that splices extra
legs — fees should look identical architecturally). The overdraft enrichment at lines 1115–1138 is the
best in-repo precedent to copy.

### Hidden cost: fees is NOT a pure function
`plugin-fees` is a full service, not a library:
- It has **its own MongoDB** with collections `billing_package` and `pack`
  (`internal/mongodb/{billing_package,pack}`), plus billing-package CRUD endpoints
  (`/v1/billing-packages`, `/v1/billing/calculate`, `/v1/fees`, `/v1/estimates`).
- Fee calculation needs to **resolve accounts/segments** for exemption logic via
  `internal/adapters/midaz/account_resolver.go` and `transaction_counter.go`, which today call midaz over
  **HTTP** (`pkg/net/http.MidazClient.ListAccounts/GetAccountDetailsByAlias`). After collapse these become
  **in-process calls to the host's `query.UseCase`** (GetAllAccounts / GetAccountByAlias). That is a net
  simplification — kill the HTTP client, the auth token plumbing, and the network hop — but it is real work,
  not a copy-paste.

So "embed fees" = (1) inline `CalculateFee` at the seam, (2) bring the billing-package Mongo repos + CRUD
routes into the host as a new module, (3) replace the midaz-HTTP account resolver with a direct query-UC
adapter. Item (2) is the part Fred's "service collapse" framing might underweight: fee *packages* are
persisted state with their own API surface, not just a calculation.

---

## 4. Transaction modes, pending lifecycle, async

- Modes (JSON / DSL / inflow / outflow / annotation) all converge on `executeCreateTransaction`, so the fee
  hook covers all of them with one insertion. DSL (`pkg/gold`) parses to the same `CreateTransactionInput`.
- `InitialStatus()` on the input decides status; annotation forces `NOTED`.
- Pending → commit/cancel/revert handled in `transaction_state_handlers.go`; revert builds a reverse
  transaction through the same funnel with `action=ActionRevert`. **Fee implication:** decide whether revert
  re-applies/refunds fees. Today external fees don't see reverts; embedding makes this a live design question.
- Async is `RABBITMQ_TRANSACTION_ASYNC=true`: `WriteTransaction` publishes to RabbitMQ; the consumer
  (`internal/adapters/rabbitmq/consumer.rabbitmq.go`) reconstructs and persists. Bulk mode
  (`BULK_RECORDER_ENABLED`) batches. **Fees run synchronously in the HTTP handler regardless** (they mutate
  the input before the write), so async mode does not complicate the fee hook — fees are applied once,
  pre-queue.

---

## 5. Persistence + migrations

- PostgreSQL: squirrel-built SQL, pgx/v5. Two managers (`tmpostgres.Manager`) keyed per module in MT mode.
- MongoDB: `go.mongodb.org/mongo-driver`, per-module managers. Used for metadata (flat key/value).
- Redis: balance atomic operations via embedded Lua scripts
  (`internal/adapters/redis/transaction/scripts/*.lua`, `//go:embed`).
- **Migrations**: numbered golang-migrate-style `.sql` pairs under
  `components/ledger/migrations/{onboarding,transaction}` (transaction is at `000033`). Run **on startup in
  single-tenant mode** via `libPostgres.NewMigrator(MigrationConfig{ MigrationsPath: "components/ledger/migrations/transaction" })`
  in `config.postgres.transaction.go` (`defaultTransactionPostgresMigrator`, exposed as a package var for
  test override). The path is **relative to the process CWD** and the SQL is **copied into the Docker image**.
  In MT mode, migrations are handled by tenant-manager provisioning, not at startup.
- **CRM and fees are MongoDB-only** (no Postgres migrations to merge). CRM persists holders + aliases;
  fees persists billing packages + packs. So the migration surface does NOT grow from these two collapses —
  only new Mongo collections + (for MT) new tenant-manager module registrations.

---

## 6. Config / env + multi-tenancy

- `Config` struct (`config.go` lines ~48–238) is large: **122 env vars** in `.env.example`. Fields are
  prefixed per concern: `DB_ONBOARDING_*` / `DB_TRANSACTION_*`, `MONGO_ONBOARDING_*` / `MONGO_TRANSACTION_*`,
  `REDIS_*`, `RABBITMQ_*`, `STREAMING_*`, `MULTI_TENANT_*`, auth (`PLUGIN_AUTH_*`, `CASDOOR_JWK_ADDRESS`).
- MT wiring (`buildUnifiedRouteSetup`, lines 1007–1073): a single `tmmiddleware.NewTenantMiddleware` is
  built with `WithPG(mgr, constant.ModuleOnboarding)`, `WithPG(mgr, constant.ModuleTransaction)`, and the
  matching `WithMB(...)` for Mongo, plus tenant cache/loader. Per-route-group `ProtectedRouteOptions` attach
  `[authAssertion, tenantMiddleware.WithTenantDB]` as PostAuthMiddlewares. The tenant ID comes from JWT via
  auth middleware; tenant DB resolution is keyed by module name.
- Module constants live in `pkg/constant/module.go` — currently only `ModuleOnboarding="onboarding"` and
  `ModuleTransaction="transaction"`. **CRM/fees collapse must add `ModuleCRM`/`ModuleFees` constants** and
  register their Mongo managers in the unified tenant middleware.
- Tenant-manager **module name** is a known footgun: CRM uses `moduleName = "crm-api"`
  (`crm/internal/bootstrap/config.tenant.go:29`) — a recent hotfix aligned this with tenant-manager
  provisioning (`crm → crm-api`). When CRM folds into ledger, its tenant config/provisioning identity must
  be preserved or migrated, not silently renamed to `ledger`.

---

## 7. CRM as-is (the easier collapse) — `components/crm`

- Already inside the module: `github.com/LerianStudio/midaz/v3/components/crm`. Same Go version, same
  lib-commons v5, same lib-observability. **No module-skew problem** (unlike fees/tracer/reporter).
- Bootstrap (`crm/internal/bootstrap/config.go`) is a structural twin of ledger's, but **MongoDB-only**:
  no Postgres, no RabbitMQ, no Redis (except MT tenant Pub/Sub). It builds a `libCrypto.Crypto` cipher
  (`LCRYPTO_HASH_SECRET_KEY` / `LCRYPTO_ENCRYPT_SECRET_KEY`) to encrypt PII on holders — **this is unique to
  CRM and must be carried over.** 36 env vars.
- DI: a single `services.UseCase{ HolderRepo, AliasRepo }` (note: CRM uses a unified `UseCase`, NOT the
  command/query split). Two handlers (`HolderHandler`, `AliasHandler`).
- Routes (`crm/internal/adapters/http/in/routes.go`): `NewRouter` builds a **whole standalone fiber.App**
  (own middleware stack, own `/health`, `/version`, `/swagger`, `/readyz`, own tenant middleware). Routes:
  `/v1/holders`, `/v1/holders/:id`, `/v1/holders/:holder_id/aliases…`. Auth resource namespace is
  `ApplicationName = "plugin-crm"` (vs ledger's `midaz` / `routing`).
- Lifecycle (`crm/internal/bootstrap/service.go`): Launcher + RunApp("HTTP Service") + optional tenant
  event listener. Identical pattern to ledger.

### CRM collapse path (clean, no shims)
- Convert `NewRouter` into a **`RouteRegistrar`** (drop the standalone app, middleware, health/swagger —
  the host already provides those). Pass it as a 4th registrar to `NewUnifiedServer`.
- Init the CRM Mongo connection + cipher + holder/alias repos inside the host's `InitServersWithOptions`.
  Either give the host a `crmUseCase` field or fold holder/alias repos into a CRM-specific UC.
- Register CRM's Mongo manager in the unified tenant middleware under a new `ModuleCRM` constant; preserve
  the `crm-api` tenant-manager identity.
- Auth namespace: keep `plugin-crm` resource strings (RBAC policies in tenant-manager reference them) OR
  do a coordinated policy migration. **Do not silently rename** — that breaks authorization.
- Route prefix collision check: CRM uses `/v1/holders`, `/v1/aliases` — **no collision** with ledger's
  `/v1/organizations/...` tree. Safe to mount on the same port. (Note CRM's holder/alias endpoints are
  NOT org/ledger-scoped in the path — they're tenant-scoped only. This differs from ledger's deep path
  nesting; harmless for routing but worth noting for API consistency.)

---

## 8. Build / Docker / CI surface on the host

- `components/ledger/Dockerfile`: golang:1.26.3-alpine builder, root build context, single binary, copies
  both migration trees. Adding CRM/fees Mongo means **no Dockerfile change** (Mongo has no baked migrations);
  the same binary just gains routes. Only if you keep CRM/fees as separate binaries would you need new
  Dockerfiles — but the brief says collapse, so one binary, one Dockerfile.
- `components/ledger/Makefile`: standard targets (`build`, `test`, `lint`, `up`, `down`, `run`,
  `generate-docs`, …). No migration-create target here (that's at root).
- **Root Makefile is stale relative to the codebase**: `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` — it lists
  infra and CRM but **NOT ledger** in the iterate-all list (ledger is handled by explicit `make ledger
  COMMAND=...` delegation). When collapsing CRM into ledger, the root Makefile's `COMPONENTS` and the
  per-component loops need rework so lint/test/format still cover the merged code. `migrate-create` requires
  `COMPONENT`/`NAME` args.
- Swagger: each component has its own `@title`/`api` docs; the unified ledger already merged onboarding +
  transaction Swagger. CRM/fees endpoints would need to be folded into the ledger Swagger generation
  (`make generate-docs`) or the combined OpenAPI loses them.

---

## 9. The hard parts (ranked)

1. **`pkg/transaction` vs `pkg/mtransaction` + version skew.** plugin-fees imports
   `github.com/LerianStudio/midaz/v3/pkg/transaction` at the **published tag `v3.5.2`** (go.mod), 18 sites.
   The current host tree has `pkg/mtransaction` (the transaction-create handler uses alias `mtransaction`)
   and `pkg/gold/transaction` — **there is no `pkg/transaction` directory in the working tree.** Either the
   published `v3.5.2` had `pkg/transaction` and it was since renamed to `mtransaction`, or the package moved.
   Embedding fees means **reconciling these type packages into one** and updating all 18 fees import sites to
   the in-tree path. This is the #1 mechanical risk and must be resolved before any fee code compiles in-tree.
2. **Fee state is a service, not a function.** Billing-package Mongo collections + CRUD API + the
   midaz-HTTP account resolver must all move in. Replacing the HTTP resolver with a direct `query.UseCase`
   call is the cleanest part; absorbing the billing-package CRUD as a host module is the larger part.
3. **lib-commons / lib-observability skew across repos.** Host is lib-commons v5 / Go 1.26.3; plugin-fees is
   lib-commons v5.1.0 / lib-observability v1.1.0-beta.5 / Go 1.26.3 / midaz v3.5.2. CRM is already in-tree so
   no skew. Fees code must be ported to the host's exact lib versions; any API drift in lib-observability
   tracing/log/metrics or lib-commons tenant-manager between fees' pins and the host's pins is hand-fix work.
   (tracer at lib-commons v4.x is a MAJOR-version problem, but that's the tracer dossier.)
4. **Auth namespace / RBAC policies.** Ledger uses `midaz` + `routing` resource namespaces; CRM uses
   `plugin-crm`; fees uses its own. Tenant-manager RBAC policies are keyed on these. Collapsing routes onto
   one binary does NOT collapse the authz model for free — policies must be preserved or migrated deliberately.
5. **Revert/cancel fee semantics.** Embedding fees in the create funnel forces a decision on what happens to
   fees on revert (refund? re-charge? ignore?). External fees never had to answer this.
6. **Tenant module registration.** Each new Mongo-backed domain (CRM, fees) needs a `constant.Module*` and
   a `WithMB(...)` registration in the unified tenant middleware, plus tenant-manager provisioning of those
   modules per tenant. Miss this and MT requests for the new domains fail at DB resolution.
7. **The god-function.** `InitServersWithOptions` is already `gocognit/gocyclo`-suppressed at ~540 lines.
   Bolting on two more domains without first extracting per-domain init helpers will make it unmaintainable.
   Recommend extracting `initCRM(...)` / `initFees(...)` helpers that return repos+registrars, mirroring the
   existing `initTransactionPostgres` style.

---

## 10. Integration surface (concrete touch points for the planner)

- `components/ledger/internal/bootstrap/config.go` — `InitServersWithOptions`: add CRM/fees infra init, UC
  fields, handlers, RouteRegistrars; add `ModuleCRM`/`ModuleFees` to tenant middleware.
- `components/ledger/internal/bootstrap/unified-server.go` — `NewUnifiedServer(... routeRegistrars ...)`:
  already variadic; just pass more registrars. No change needed.
- `components/ledger/internal/bootstrap/service.go` — `Service` struct + `Run()`: add runnables only if
  fees/crm need workers (CRM/fees are HTTP-only today; likely none).
- `components/ledger/internal/adapters/http/in/transaction_create.go` —
  `executeCreateTransaction`: insert `applyFees` between alias-normalize (~L1007) and idempotency (~L1020).
- `components/ledger/internal/adapters/http/in/routes.go` — add `RegisterCRMRoutesToApp` /
  `RegisterFeesRoutesToApp` (or registrar closures) following `RegisterTransactionRoutesToApp`.
- `components/ledger/internal/services/{command,query}` — either extend the UseCase god-structs or add a
  `crm.UseCase` / `fees.UseCase`. Fees calc lib goes into a `pkg/fee`-equivalent in-tree path.
- `pkg/constant/module.go` — add `ModuleCRM = "crm-api"`, `ModuleFees = "..."`.
- `pkg/transaction` / `pkg/mtransaction` — reconcile into one package; repoint fees' 18 imports.
- `components/ledger/Dockerfile` — unchanged (Mongo, no baked migrations).
- Root `Makefile` — fix `COMPONENTS` list and delegation after CRM folds into ledger.
- Swagger generation — fold CRM/fees endpoints into the unified ledger docs.

---

## 11. Effort sizing (host-side only)

- **CRM collapse**: M. Same module, no skew, Mongo-only, no route collision, clean RouteRegistrar
  conversion. Real work is cipher carry-over, tenant module registration, auth-namespace decision, Swagger
  merge, killing the standalone server/main, root-Makefile rework.
- **Fees embed**: L. Type-package reconciliation (`pkg/transaction`↔`pkg/mtransaction`) + version port +
  in-process account resolver + billing-package Mongo+CRUD absorption + the one-line-conceptually-but-
  load-bearing seam insertion + revert semantics decision. The calculation itself is small; the surrounding
  service-to-module conversion is the bulk.
- Both are gated on first extracting per-domain init helpers from `InitServersWithOptions` to keep the
  composition root sane.
