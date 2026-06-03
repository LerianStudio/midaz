# Monorepo Consolidation — Move 1: tracer → components/tracer

**Target end-state (Fred):** "liso e final — sem shims, workarounds, abstrações de compatibilidade."
**Move type:** Co-located COMPONENT. tracer keeps its own service/binary; it does NOT collapse into ledger.
**Source:** `/Users/fredamaral/repos/lerianstudio/tracer` (own git repo, branch `develop`).
**Destination:** `github.com/LerianStudio/midaz/v3/components/tracer`.

---

## 0. TL;DR for the planner

tracer is a self-contained Fiber HTTP service (transaction-validation: rules + limits + audit events + CEL evaluation) with PostgreSQL, multi-tenancy, workers, and a full test pyramid. Bringing it in as a co-located component is **structurally easy** (midaz is a single Go module; components share one root `go.mod`, so the import alignment is target-shaped) but **mechanically large** because of two stacked dependency migrations:

1. **Module rename** `tracer/...` → `github.com/LerianStudio/midaz/v3/components/tracer/...` across **465 Go files / 749 import sites / 38 distinct internal packages**.
2. **Dependency unification** — two separate jumps, both already proven by midaz:
   - **lib-commons v4 → v5** (non-observability surface: `commons`, `tenant-manager/*`, `postgres`, `net/http`, `runtime`, `server`) — ~92 import sites.
   - **observability re-platforming**: tracer's `lib-commons/v4/commons/log`, `commons/opentelemetry`, `commons/opentelemetry/metrics`, `commons/zap` → **`lib-observability`** `log`, `tracing`, `metrics`, `zap` (a *different module*) — ~123 import sites. midaz already did exactly this in PR `refactor/observability-migration` (commit `766b555d2`, 281 files, +1123/-1041). The Logger/Telemetry APIs are shape-compatible, so this is overwhelmingly an import-path rewrite, not a call-site rewrite.

The single hard constraint (`github.com/LerianStudio/lib-commons` mandatory) is **satisfied and reinforced** by this move — tracer is currently a major version behind and the migration drags it onto the org-standard v5 + lib-observability stack.

**Rough effort:** 4–7 focused days for a single engineer who has the midaz observability-migration commit as a template. The work is broad but low-novelty; the risk is in the multi-tenant tenant-manager v4→v5 API drift and the godog e2e suite, not in the rename.

---

## 1. Module rename blast radius

tracer's `go.mod` declares `module tracer` (non-qualified — unimportable from outside; this is why it was never a real dependency of anything). Folding it into midaz dissolves this `go.mod` entirely; every internal import must be rewritten to the component path.

**Scale:** 749 import statements of `tracer/...` across 465 `.go` files, touching **38 distinct internal packages**.

Distinct imported packages (count = import sites), each rewrites `tracer/X` → `github.com/LerianStudio/midaz/v3/components/tracer/X`:

| Sites | Package |
|------:|---------|
| 291 | `tracer/pkg/model` |
| 196 | `tracer/pkg/constant` |
| 192 | `tracer/internal/testutil` |
| 118 | `tracer/pkg/logging` |
| 79 | `tracer/pkg/clock` |
| 71 | `tracer/internal/adapters/postgres/db` |
| 38 | `tracer/internal/adapters/postgres/db/mocks` |
| 37 | `tracer/pkg/net/http` |
| 34 | `tracer/pkg/contextutil` |
| 34 | `tracer/internal/services/cache` |
| 29 | `tracer/internal/services/command` |
| 26 | `tracer/internal/services/query` |
| 16 | `tracer/pkg/resilience` |
| 15 | `tracer/pkg` |
| 14 | `tracer/internal/adapters/cel` |
| 13 | `tracer/internal/services/workers` |
| 11 | `tracer/internal/testutil_integration` |
| 10 | `tracer/internal/services/workers/mocks` |
| 10 | `tracer/internal/adapters/http/in/middleware` |
| 10 | `tracer/api` |
| 8 each | `tracer/tests/end2end/support`, `tracer/internal/services/query/mocks`, `tracer/internal/services`, `tracer/internal/adapters/postgres`, `tracer/internal/adapters/http/in/mocks`, `tracer/internal/adapters/http/in` |
| 6 | `tracer/internal/services/command/mocks` |
| 5 each | `tracer/internal/services/mocks`, `tracer/internal/services/cache/mocks`, `tracer/internal/bootstrap` |
| 4 | `tracer/internal/observability` |
| 3 each | `tracer/pkg/validation`, `tracer/pkg/sanitize`, `tracer/pkg/migration`, `tracer/internal/testhelper`, `tracer/internal/services/metrics` |
| 2 | `tracer/internal/testutil_dbsuite` |
| 1 | `tracer/tests/end2end/steps` |

**Mechanics:** This is a deterministic, scriptable rewrite (`gofmt -r`, `goimports`, or a `sed`/`gofumpt` pass over the import path prefix), not a hand edit. The prefix `tracer/` → `github.com/LerianStudio/midaz/v3/components/tracer/` is unambiguous because tracer's bare module name never collides with a real external path. The single bare `"tracer/pkg"` import (15 sites) and the `cmd/app/main.go` `"tracer/internal/bootstrap"` + `"tracer/pkg"` also rewrite.

**Watch item:** swagger annotations and any string references to `tracer/...` in non-Go files (`.golangci.yml` module-prefix rules, `mk/*.mk` coverage path filters, `.ignorecoverunit`). Grep for the literal `tracer/` outside Go after the Go rewrite.

---

## 2. Dependency unification — the real cost

tracer's `go.mod` (`module tracer`, **go 1.26.3** — exact match with midaz) currently sits on:

- `lib-commons/v4 v4.6.3` (direct) — MAJOR behind midaz `v5.2.0-beta.12`.
- `lib-commons/v5 v5.3.0` (indirect — tracer is mid-migration already: 5 sites import `v5/commons/log` and `v5/commons/zap`).
- `lib-observability v1.0.0` (indirect — NOT yet imported in tracer Go code; midaz is on `v1.0.1` and imports it directly everywhere).
- `lib-auth/v2 v2.8.0` (direct) — **exact match with midaz**, 11 sites all `lib-auth/v2/auth/middleware`. No work.

### 2a. lib-commons v4 → v5 (non-observability)

v4 import sites tracer must move to v5 (path bump + API drift):

| v4 sites | Package | v5 parity |
|---------:|---------|-----------|
| 54 | `v4/commons` | midaz uses `v5/commons` heavily (173 sites). `SetConfigFromEnvVars`, `InitLocalEnvConfig` confirmed present and used identically in midaz v5 — direct path swap. |
| 16 | `v4/commons/tenant-manager/core` | midaz uses `v5/.../core` (66 sites). `GetTenantIDContext`, `GetPGContext` confirmed identical. |
| 7 | `v4/commons/postgres` | midaz uses `v5/commons/postgres` (33 sites). |
| 5 | `v4/commons/tenant-manager/postgres` | midaz `v5` (13 sites). `tmpostgres.Manager` is the type tracer's routes hold. |
| 4 | `v4/commons/tenant-manager/client` | midaz `v5` (19 sites). |
| 4 | `v4/commons/runtime` | present in v5. |
| 3 | `v4/commons/net/http` | midaz `v5/commons/net/http` (73 sites). `libHTTP.FiberErrorHandler`, `NewTelemetryMiddleware`, `WithHTTPLogging` — **see caveat 2c**. |
| 2 | `v4/commons/zap` | → see observability (zap moved to lib-observability). |
| 2 | `v4/commons/tenant-manager/event` | midaz `v5` (4 sites). |
| 1 each | `v4/.../redis`, `v4/.../middleware`, `v4/commons/server` | all present in v5. |

**Assessment:** The tenant-manager surface tracer uses (`core`, `postgres`, `client`, `event`, `middleware`, `redis`) is the same surface midaz exercises on v5 — midaz proves these APIs exist post-migration. Expect *some* signature drift (constructor option funcs, config struct fields) but no missing capability. Budget API-drift fixes, not redesign.

### 2b. Observability re-platforming (the bulk)

This is the larger half and it is NOT a v4→v5 bump — it is a move to a **separate module** `lib-observability`. tracer's mapping mirrors midaz's completed migration:

| tracer (v4 lib-commons) | → midaz target (lib-observability) | tracer sites |
|---|---|---:|
| `v4/commons/log` (aliased `libLog`) | `lib-observability/log` (`libLog`) | 63 |
| `v4/commons/opentelemetry` (`libOtel`) | `lib-observability/tracing` (midaz alias `libOpentelemetry`) | 48 |
| `v4/commons/opentelemetry/metrics` (`libMetrics`) | `lib-observability/metrics` | 10 |
| `v4/commons/zap` | `lib-observability/zap` (midaz `libZap`) | 2 |

**De-risking finding — APIs are shape-compatible:**
- **Logger:** tracer calls `.With(libLog.String(...)).Log(ctx, libLog.LevelWarn, "msg")` and `logger.Log(ctx, libLog.LevelError, "msg", libLog.Err(err))`. midaz uses the identical surface (`libLog.LevelError`, `libLog.String`, `libLog.Err`, `.Log(ctx, level, msg, fields...)`). The `libLog.Logger` interface did not change shape across the move.
- **Telemetry:** tracer `internal/bootstrap/config.go:1381` calls `libOtel.NewTelemetry(libOtel.TelemetryConfig{...})`; midaz `config.go:316` calls `libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{...})` — same constructor, same config struct name. `*Telemetry` is the value tracer threads through `RoutesDeps.Telemetry` and `NewTelemetryMiddleware`.
- **Middleware:** tracer's routes use `libHTTP.NewTelemetryMiddleware(tl)`, `tlMid.WithTelemetry`, `tlMid.EndTracingSpans` from `v4/commons/net/http`. midaz moved telemetry middleware to `lib-observability/middleware` (`libObsMiddleware.NewTelemetryMiddleware` — see `unified-server.go:56`). **This is the one genuine relocation, not just a path swap** — tracer's `libHTTP.NewTelemetryMiddleware` must split: HTTP error/body helpers stay in `v5/commons/net/http`, telemetry middleware moves to `lib-observability/middleware`.

tracer's two thin wrappers localize the blast radius:
- `pkg/logging/trace.go` — wraps `v4/commons/log`. Single file to repoint.
- `internal/observability/{recorder.go,prometheus_factory.go}` — wraps `v4/commons/log` + `v4/commons/opentelemetry/metrics`. Two files. These are the right seam to migrate first; most of the 123 sites flow through `pkg/logging` and the recorder.

### 2c. lib-commons version skew is a non-issue post-merge

tracer indirect `v5.3.0` vs midaz direct `v5.2.0-beta.12`. Once tracer's code lives under one root `go.mod`, Go resolves a **single** lib-commons version for the whole module. midaz will need to bump its `lib-commons/v5` to a stable `>= 5.3.0` (or whatever tracer's v5 surface requires) — this is a one-line `go.mod` change plus a full-repo build. There is no dual-version coexistence and no shim. Same for `lib-observability` (`v1.0.0` → midaz `v1.0.1`; take the higher).

### 2d. Deps tracer brings that midaz lacks

These get added to the root `go.mod` (additive, low risk):

- **`google/cel-go` + `cel.dev/expr` + `antlr4-go/antlr/v4` + `yuin/gopher-lua`** — the CEL expression engine (`internal/adapters/cel/`, 16 files). **Unique to tracer**; midaz has zero CEL. This is tracer's domain core (rule/limit expression evaluation) and pulls the largest new dependency subtree. Verify license compatibility (cel-go is Apache-2.0; fine under EL2.0 component).
- **`cucumber/godog` + `cucumber/gherkin` + `cucumber/messages`** — BDD e2e harness (`tests/end2end/`). midaz has none. Adds a test-only dep tree and a new test execution mode the midaz Makefile/CI doesn't currently run.
- **`alicebob/miniredis/v2`, `DATA-DOG/go-sqlmock`** — test-only, not in midaz.
- Already in midaz (no add): `sony/gobreaker`, `bxcodec/dbresolver`, `shopspring/decimal`, `golang-migrate/migrate`, `swaggo/fiber-swagger`, `gofiber/*`, `pgx/v5`, `squirrel`.

---

## 3. Structure & runtime model

### Layout (clean-architecture, mirrors midaz/ledger almost exactly)

```
cmd/app/main.go                         # entry: pkg.InitLocalEnvConfig → bootstrap.InitServers → signal-aware run()
internal/
  bootstrap/                            # composition root — config.go is 73KB; +multitenant variants, selfprobe, TLS, drain
  adapters/
    cel/                                # CEL engine (UNIQUE) — compile/evaluate/environment/examples
    http/in/                            # Fiber routes.go + handlers + middleware (ClientIP, FaultInjection, AuthGuard)
    postgres/                           # repos: rule, limit, audit_event, transaction_validation, usage_counter, rule_sync
    postgres/db/                        # db plumbing + mocks
  services/
    command/  query/                    # write/read use cases (CQRS, same split as midaz)
    cache/  metrics/  workers/          # caches, prometheus recorder, per-tenant background workers
    mocks/                              # go.uber.org/mock generated
  observability/                        # prometheus_factory + recorder (lib-commons-wrapping)
  testutil/ testutil_integration/ testutil_dbsuite/ testhelper/
pkg/
  model/                                # domain models (rule, limit, scope, audit_event, check_limits, decision_maker, time_of_day)
  constant/                             # errors.go (21KB), limits, modules (ModuleName="tracer"), pagination
  clock/ contextutil/ hash/ logfields/ logging/ migration/ net/http/ resilience/ sanitize/ shell/ validation/
api/                                    # swagger/openapi artifacts
migrations/                             # 34 .sql files (numbered 000001..000017 up/down) + seeds/
tests/integration/ (45 files)  tests/end2end/ (godog features+steps+support)
mk/ (database, docker, docs, quality, security, tests)  Makefile  Dockerfile(.dev)  docker-compose.yml
```

### Runtime / deploy

- **Single binary** built from `cmd/app` → distroless static image, `EXPOSE 4020` (default `SERVER_PORT`), non-root.
- **HTTP service** (Fiber v2). Endpoints (all under `/v1`, AuthGuard per-route): `rules` (CRUD + activate/deactivate/draft), `limits` (CRUD + activate/deactivate/draft + usage), `validations` (POST validate, GET list/get), `audit-events` (GET list/get/verify). Public: `/health`, `/readyz`, `/metrics`, `/version`, `/swagger/*`.
- **Background workers** (`internal/services/workers`): rule-sync poller + cleanup worker, lazily spawned per-tenant on first request via `WorkerSupervisor.EnsureWorkers` (tenant-cap → 503 + Retry-After). This is real concurrent runtime state, not just request/response — the component must register worker lifecycle with midaz's app launcher.
- **Lifecycle:** signal-aware root ctx, `/readyz` drain (`READYZ_DRAIN_GRACE_SECONDS`), self-probe (`bootstrap/selfprobe.go`), SaaS TLS enforcement keyed on `DEPLOYMENT_MODE=saas`. These map onto Lerian/Ring standards midaz already follows.

### Databases & migrations

- **PostgreSQL only** (no Mongo, unlike ledger). `postgres:16-alpine` in compose.
- **34 migration files** (`golang-migrate`, numbered `000001`–`000017` up+down). Includes Postgres functions (audit hash chain, truncate prevention), enum evolution, decimal conversions, dedup/unique constraints, indexes. **Self-contained schema** — no FK or shared-table coupling to ledger. Audit-event hash chaining (`000001`, `000002`, `000017`) is integrity-sensitive: do not reorder/renumber on import.
- In midaz the migration convention is per-component dirs (`components/ledger/migrations/{onboarding,transaction}`). tracer keeps `components/tracer/migrations/` and registers with `make migrate-create COMPONENT=tracer` analog. The Dockerfile already `COPY`s `migrations` next to the binary — preserve that path or repoint to `/app/migrations`.

### Config / env (59 variables)

Families: server (`SERVER_PORT/ADDRESS`), DB (`DB_*`, `DB_SSL_MODE`, `MIGRATIONS_PATH`), auth (`API_KEY*`, `PLUGIN_AUTH_*`), CORS, telemetry (`ENABLE_TELEMETRY`, `OTEL_*`), multi-tenant (`MULTI_TENANT_*` — Redis pub/sub, circuit breaker, pool caps, TTLs, `MULTI_TENANT_SERVICE_API_KEY`, `MULTI_TENANT_URL`), workers (`RULE_SYNC_*`, `CLEANUP_*`, `TENANT_CAP_RETRY_AFTER_SECONDS`), readiness (`READYZ_*`), swagger (`SWAGGER_*`), `DEPLOYMENT_MODE`, `APPLICATION_NAME`, `CEL_COST_LIMIT`.

**Config-merge decision:** midaz runs components as separate services with their own `.env` per component (`components/ledger/.env`). tracer keeps its own `components/tracer/.env(.example)`. **No env-namespace collision risk** because services are separate processes — but if a single `make up` brings both up, the shared `DB_*`/`OTEL_*` names need either per-component env files (recommended, matches midaz) or a `TRACER_`-prefix scheme (NOT recommended — would require renaming 59 vars and diverging from tracer's deploy charts). Keep per-component env files; that is the "liso" choice.

---

## 4. pkg/ duplication & collisions vs midaz/pkg

tracer `pkg/` and midaz `pkg/` share **directory names** but are **distinct Go packages** under different import paths post-rename (`components/tracer/pkg/...` vs root `pkg/...`). So nothing compile-collides. The real question is *duplication the "no shims, liso" target should resolve*:

| tracer/pkg | midaz/pkg | Verdict |
|---|---|---|
| `shell/` (`ascii.sh`, `colors.sh`, `makefile_colors.mk`, `makefile_utils.mk`) | `shell/` (`ascii.sh`, `colors.sh`, `logo.txt`) | **True duplication** — build scaffolding. Consolidate to one `pkg/shell`; tracer's `.mk` helpers fold into midaz's `mk/`. |
| `constant/` (`errors.go` 21KB, `limits.go`, `modules.go` `ModuleName="tracer"`, `pagination.go`, `datetime.go`) | `constant/` (`errors.go` 68KB, entity/status codes) | **Domain-specific, NOT a collision.** tracer error sentinels are its own; keep under `components/tracer/pkg/constant`. Do NOT merge into midaz `pkg/constant` (would violate the "errors unique, defined per owner" model and pull tracer-only codes into shared space). |
| `net/http/` (cursor pagination, `response.go`, `with_body.go`, tracer error envelope) | `net/http/` (midaz HTTP helpers) | **Parallel, domain-shaped.** Some overlap in concept (response envelope, cursor pagination) but tracer's is wired to its own error constants. Keep co-located initially; a later sweep could lift shared cursor-pagination helpers to root `pkg`, but that is a SIMPLIFICATION pass, not a blocker for "liso entry." |
| `model/`, `clock/`, `contextutil/`, `hash/`, `logfields/`, `logging/`, `migration/`, `resilience/`, `sanitize/`, `validation/` | (midaz has `mmodel`, `mbootstrap`, etc. — different names) | No name collision. `model` vs `mmodel` differ. Keep tracer's under component pkg. **Note:** midaz CLAUDE.md mandates domain models in `pkg/mmodel` and forbids `/internal/domain` — tracer uses `pkg/model`, which is fine for a co-located component keeping its own pkg, but flag it if the planner wants tracer to conform to the `mmodel` convention (cosmetic, not required for a co-located service). |
| `logging/` (wraps `v4/commons/log`) | (midaz logs via `lib-observability/log` directly) | tracer's wrapper repoints to `lib-observability/log` in the observability migration; keep the wrapper or inline it. Either is "liso." |

**Net:** Only `pkg/shell` is genuine dead-duplication to eliminate. The rest is legitimate per-component code that co-exists cleanly under `components/tracer/pkg/`.

---

## 5. Convention conformance gaps (midaz CLAUDE.md / PROJECT_RULES)

What needs alignment for a clean ("liso") entry vs what is already compliant:

**Already aligned:** Go 1.26.3 exact match; Fiber v2; squirrel + `PlaceholderFormat(Dollar)`; `uuid.UUID` IDs; `lib-auth/v2/auth/middleware`; CQRS command/query split; clean-architecture inward dependency flow; structured logging with `libLog.String/Err` (no `fmt.Sprintf` in logs); EL2.0 license headers; `/readyz` canonical probe; testcontainers for repo integration tests; `go.uber.org/mock`.

**Needs decision / light work:**
1. **Route protection pattern.** tracer uses a bespoke `guard.With("resource","verb",apiKeyOnly)` AuthGuard chain. midaz mandates `http.ProtectedRouteChain()`. For a **co-located component keeping its own service**, tracer's guard can stay — it is internally consistent and not a third-rail. Conforming to `ProtectedRouteChain()` is optional polish; flag as a follow-up, not a blocker. (If the planner wants strict uniformity, this is a route-file rewrite, ~1 day.)
2. **`pkg/model` vs `pkg/mmodel` naming.** Cosmetic; co-located components may keep their own pkg. Decide whether uniformity is worth a rename pass.
3. **Telemetry middleware relocation** (§2b caveat 2c) — the one non-mechanical observability fix.
4. **`docker-compose.yml`** uses a private `tracer-network`; midaz's compose joins shared `infra-network`. Repoint network + DB service name to fit the unified stack (or keep separate compose for standalone dev).

---

## 6. Build / Docker / CI fold

- **Makefile/mk:** tracer has a thin root `Makefile` (build, set-env, hooks, pre-push/pre-merge) delegating to `mk/{database,docker,docs,quality,security,tests}.mk`. midaz's root Makefile uses a `COMPONENTS` loop + `make <component> COMMAND=<target>` delegation. Fold tracer's targets into the midaz component-delegation model: add `TRACER_DIR := ./components/tracer` to the `COMPONENTS` list, port the per-component `tests.mk` (18KB — godog e2e + integration + coverage filters) into tracer's component Makefile. **The 18KB `tests.mk` is the heaviest single CI artifact** because it orchestrates the godog BDD suite midaz doesn't currently run.
- **Dockerfile:** already distroless/static/non-root, builds `./cmd/app`, copies `migrations`. After rename, build context becomes `components/tracer` and the build path `./components/tracer/cmd/app` (or a per-component Dockerfile as ledger has). Trivial.
- **CI workflows:** tracer `.github/workflows/` overlaps midaz by name (`build.yml`, `go-combined-analysis.yml`, `gptchangelog.yml`, `pr-security-scan.yml`, `pr-validation.yml`, `release.yml`, plus `dependabot-auto-merge.yml`). These get **deleted from tracer and the build/test matrix folded into midaz's existing workflows** (add tracer to the component build/test/lint matrix). The org uses `LerianStudio/github-actions-shared-workflows`, so the fold is mostly adding tracer paths/targets to midaz's existing reusable-workflow calls. tracer's separate `release.yml` / `.releaserc.yml` (semantic-release) must be reconciled with midaz's release process — **decision needed** (see structured return): does tracer keep an independent release tag/version or ride midaz's monorepo release?
- **golangci / coverage:** port `.golangci.yml`, `.ignorecoverunit`, `.trivyignore` rules into midaz's config (merge ignore lists; repoint any `tracer/`-prefixed path rules to `components/tracer/`).

---

## 7. What "liso, no shims" specifically requires (ordered)

1. **Rename:** scripted prefix rewrite `tracer/` → `github.com/LerianStudio/midaz/v3/components/tracer/` across 465 files. Delete tracer `go.mod`/`go.sum`.
2. **Observability migration:** repoint `pkg/logging/trace.go` + `internal/observability/{recorder,prometheus_factory}.go` first (the seam), then sweep the remaining ~123 sites `lib-commons/v4/commons/{log,opentelemetry,opentelemetry/metrics,zap}` → `lib-observability/{log,tracing,metrics,zap}`. Relocate telemetry middleware to `lib-observability/middleware`. Use midaz commit `766b555d2` as the literal pattern.
3. **lib-commons v5 bump:** swap ~92 `v4/...` sites to `v5/...`; fix tenant-manager / config API drift. Bump root `go.mod` lib-commons to `>= 5.3.0` stable.
4. **Merge deps into root go.mod:** add cel-go/antlr/gopher-lua, godog, miniredis, sqlmock; take higher version on shared deps; `go mod tidy`; full-module build.
5. **Eliminate true duplication:** consolidate `pkg/shell`.
6. **Fold tooling:** Makefile component-delegation entry, `mk/` (esp. `tests.mk` godog), Dockerfile build path, migrations dir, env file, networks.
7. **Fold CI:** delete tracer workflows, add tracer to midaz matrices, reconcile release/versioning.
8. **Verify:** `make lint`, `make test-unit`, `make test-integration` (testcontainers), and the godog e2e suite all green inside midaz.

No compatibility shims are needed at any step — single go.mod resolves versions, and the observability/v5 APIs are forward-compatible. The only "translation" is the telemetry-middleware relocation, which is a real code move, not a shim.

---

## 8. Risks & sharp edges

- **godog e2e suite** (`tests/end2end/`) is a test execution mode midaz CI does not currently run. Folding it requires new CI plumbing and the cucumber dep tree. Highest-friction non-rename item.
- **Audit-event hash chaining** (migrations `000001/000002/000017`, `pkg/model/audit_event.go`, repo `VerifyHashChain`) is integrity/compliance-sensitive (SOX/GLBA comments throughout). Do not renumber migrations or alter hash logic during the move — pure relocation only.
- **Multi-tenant tenant-manager v4→v5 drift** is the most likely source of real (non-mechanical) breakage: per-tenant Postgres `Manager`, Redis pub/sub listener, circuit breaker, tenant cache. midaz exercises all of these on v5, so the capability exists, but constructor-option/config-struct signatures may have shifted between v4.6.3 and v5.x. Budget debugging here.
- **CEL engine** is a large new dependency subtree unique to the monorepo; one-time `go mod` bloat and license-surface review (Apache-2.0, compatible).
- **Release/versioning collision** (tracer semantic-release vs midaz monorepo release) — must be decided before CI fold or tracer's tagging will fight midaz's.
- **Telemetry middleware split** (`v4/commons/net/http` bundled HTTP+telemetry; lib-observability separates them) — the one place a naive path-swap will not compile.
