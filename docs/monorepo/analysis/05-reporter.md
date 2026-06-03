# Monorepo Consolidation Dossier — Move #2: `reporter` → `midaz/components/reporter`

**Target repo:** `/Users/fredamaral/repos/lerianstudio/reporter` (branch `develop`, remote `github.com/LerianStudio/reporter`)
**Move type:** Co-located component(s). Reporter keeps its own service/binary boundary. NO service collapse with ledger.
**End state:** liso e final — no shims, no compat layers, no dual module. One go.mod, one CI, one observability stack.

---

## TL;DR for the planner

Reporter is **already structured like midaz** (`components/{manager,worker,infra}`, `pkg/`, per-component `cmd/app/main.go`, lib-commons launcher, Fiber, MongoDB-first). The co-location is mechanically clean for *layout* and *module rename*. But it is NOT "liso" out of the box because of **one large divergence**:

> **Reporter is still on lib-commons v5 observability (`lib-commons/v5/commons/{log,opentelemetry,zap}`, 241 import sites). midaz has fully migrated OFF that to `lib-observability` v1.0.1 (270 files, zero files still on `lib-commons/.../log`).**

A "sem shims" entry therefore requires **rewriting reporter's logging + tracing layer to lib-observability** before or during the move. This is the dominant cost — far larger than the module-path rename. Everything else (rename, pkg-collision namespacing, go.mod merge, docker/CI fold) is grind, not risk.

There is **zero Go-level coupling between reporter and midaz** — reporter does not import `LerianStudio/midaz` anywhere. It reaches ledger data over HTTP through a generic "fetcher" datasource abstraction. So the rename has no cross-repo import to reconcile; it is purely internal to reporter.

---

## 1. Runtime model: manager vs worker

Reporter is a **two-service async report pipeline**.

### Manager (`components/manager`)
- **Role:** REST API. Create/list/download reports, CRUD templates, CRUD deadlines, validate template blocks, manage datasource connections, accept notifications.
- **Framework:** Fiber v2. Port **4005** (HTTP). Swagger at `@host localhost:4005`, title "Reporter", version 1.2.0.
- **Entry:** `components/manager/cmd/app/main.go` → `bootstrap.InitServers()` → `Service.Run()` (lib-commons `NewLauncher` + `RunApp("HTTP Service", ...)`).
- **Routes:** `components/manager/internal/adapters/http/in/routes.go` (+ `report.go`, `template.go`, `deadline.go`, `notification.go`, `data-source.go`, `template-builder.go`, `metrics.go`, `readyz_handler.go`, `cors.go`, `middlewares.go`, `swagger.go`).
- **Adapters:** `redis` (consumer/cache), `rabbitmq` (producer — enqueues report jobs), `http/in`.
- **Bootstrap:** `config.go` (40.5 KB — heavy), `init_helpers.go`, `init_tenant.go`, `rabbitmq-monitor.go`, `server.go`, `service.go`. Has `/readyz` drain-state + `/health` self-probe (Ring readyz contract already implemented).
- **Dockerfile:** `components/manager/Dockerfile` — alpine, no Chromium (REST only). Binary `/manager`, port 4005, healthcheck `wget /health`.

### Worker (`components/worker`)
- **Role:** Async consumer. Pulls report-generation jobs off RabbitMQ, runs the data pipeline (fetch from datasources → decrypt/HMAC → render via pongo2 templates → PDF via chromedp/headless Chromium → store in SeaweedFS), reconciles, notifies completion.
- **No public REST API.** Only a health HTTP server on port **4006** (`health-server.go`, checks RabbitMQ connectivity).
- **Entry:** `components/worker/cmd/app/main.go` → `bootstrap.InitWorker()` → `Service.Run()` (lib-commons `NewLauncher` + `RunApp("RabbitMQ Consumer", ...)`).
- **Services:** `generate-report*.go` (data/render/storage/idempotency split), `extraction-request.go`, `process-notification.go`, `data-pipeline.go`, `data-decrypt.go`, `data-hmac.go`, `reconciler*.go`.
- **Adapters:** `redis`, `rabbitmq` (consumer + retry manager + notification consumer + DLQ retry guard).
- **Bootstrap:** even heavier than manager — `config.go` + 8 split config files (`config_fetcher.go`, `config_runtime.go` 21.7 KB, `config_event_listener.go`, `config_multitenant.go`, `config_rabbitmq.go`, `config_mongo.go`, `config_storage.go`, `config_logger.go`, `config_validation.go`), plus `consumer.go`, `cleanup_manager.go`, `retry_guard.go`. Graceful shutdown closes (in order): reconciler ctx, health checker, health server, PDF worker pool, tenant event listener, MT resources, RabbitMQ, MongoDB, telemetry.
- **Dockerfile:** `components/worker/Dockerfile` — alpine + **Chromium + nss + fonts (noto/dejavu/liberation/freefont)**. Sets `CHROME_BIN`, `CHROMEDP_USE_SYSTEM_CHROME=true`. Binary `/worker`, port 4006. This is the heaviest image in the future monorepo.

### How they communicate
- **Queue (RabbitMQ), not DB-handoff and not HTTP.** Manager produces to the `generate-report` queue (`RABBITMQ_GENERATE_REPORT_QUEUE` / `_KEY`) via `RABBITMQ_EXCHANGE`; worker consumes. Worker also has a notification queue/handler (`handler.notification.fetcher_completion`) and a retry/DLQ path (`retry_manager.go`, `retry_guard.go`).
- **Shared state:** MongoDB (report/template/deadline/extraction documents) + Redis (cache, locks, idempotency, tenant pub/sub event listener) + SeaweedFS (binary template + rendered report storage).
- **Deploy units:** TWO separate containers/pods. They MUST stay separate after the move (different images, different scaling profile — manager is request-bound, worker is CPU/Chromium-bound). Co-location does not merge them.

```
 client → [manager:4005 HTTP] → Mongo (report doc) + RabbitMQ(generate-report)
                                              │
                                              ▼
                          [worker:4006] consume → fetch datasources (HTTP via fetcher)
                                              → render pongo2 → chromedp PDF
                                              → SeaweedFS → Mongo(status) → notify
```

---

## 2. Module rename blast radius

- **Current module:** `github.com/LerianStudio/reporter` (`go 1.26`, `toolchain go1.26.2`).
- **Target:** `github.com/LerianStudio/midaz/v3/components/reporter/...` (folded under midaz's single root module — see §4).
- **Files referencing the module path:** **518** `.go` files (excludes the throwaway snapshots in §7).
- **Import-line hotspots** (occurrences, not files):
  - `pkg/constant` 226, `pkg` 163, `pkg/model` 91, `pkg/net/http` 89, `pkg/ctxutil` 64, `pkg/mongodb/*` (report/template/deadline/extraction) ~150 combined, `pkg/postgres` 37, `pkg/datasource` 35, `pkg/rabbitmq` 31, `pkg/redis` 29, `pkg/seaweedfs/*` 47, `pkg/fetcher` 24, `pkg/storage` 23.
  - `components/manager/internal/services` 31, `components/worker/internal/services` 10, plus bootstrap/adapter cross-refs.
- **Difficulty:** LOW per import, HIGH only in volume. This is a single `gofmt -r` / `find … sed` pass on the import path prefix plus the module declaration. **No external consumer** imports `LerianStudio/reporter` (it's a service, not a lib), and reporter imports **no midaz code**, so there is no bidirectional rename to coordinate. The only subtlety is the new prefix depth: `…/reporter/pkg/constant` → `…/midaz/v3/components/reporter/pkg/constant` (see §4 on where `pkg/` lands).

---

## 3. Dependency alignment (lib-commons / lib-auth / Go)

| Dependency | reporter | midaz | Verdict |
|---|---|---|---|
| `lib-commons/v5` | **v5.1.3** | **v5.2.0-beta.12** | Same major (v5), same import paths. Beta-patch skew, not a rewrite. Pin to midaz's version on merge. |
| `lib-auth/v2` | **v2.7.0** | **v2.8.0** | One minor behind. Bump to v2.8.0; check auth-middleware signature drift. |
| Go | 1.26 / toolchain 1.26.2 | 1.26.3 | Aligned; bump reporter to 1.26.3. |
| **observability** | `lib-commons/v5/commons/{log,opentelemetry,zap}` | `lib-observability` v1.0.1 (`/log`, `/tracing`, `/metrics`, `/middleware`, `/zap`) | **DIVERGENT — this is the real work.** See §3.1. |

### 3.1 Observability migration (the dominant cost)
midaz CLAUDE.md and recent commit `766b555d2 (refactor/observability-migration)` confirm midaz uses `lib-observability` everywhere (270 files), and **zero** midaz files import `lib-commons/v5/commons/log`.

Reporter still uses the old surface across **241 import sites**:
- `lib-commons/v5/commons/log` — **159 sites**
- `lib-commons/v5/commons/opentelemetry` — **71 sites**
- `lib-commons/v5/commons/zap` — **11 sites**

For "sem shims / sem abstrações de compatibilidade," these MUST be rewritten to `lib-observability/{log,tracing,zap,middleware,metrics}`. This is not a prefix swap — the package APIs differ (logger construction, tracer/span helpers, Fiber middleware wiring, span-error handling like `HandleSpanError`). midaz already has the canonical patterns (see midaz CLAUDE.md "Observability" + "Logging" sections) to mirror. There is a Ring skill for exactly this: `ring-dev-team:migrate-observability`. Budget this as its own work-stream, ~the size of all the other reporter moves combined.

### 3.2 Heavy deps reporter adds to the merged go.mod
Reporter's `require` block is ~175 lines. Notable deps midaz likely does NOT already carry (verify against midaz go.sum during merge):
- `chromedp/chromedp` + `chromedp/cdproto` (headless Chrome driving — worker only).
- `flosch/pongo2/v6` (template engine).
- `aws-sdk-go-v2` (config, credentials, **s3**, **secretsmanager**) — SeaweedFS S3 API + secrets.
- `go-sql-driver/mysql` + `jackc/pgx/v5` (datasource fetcher connects to **external customer DBs**, both MySQL and Postgres).
- `Shopify/toxiproxy/v2`, `testcontainers-go/modules/{mongodb,mysql,postgres,redis}` (chaos + integration tests).
- `go-resty/resty/v2` (fetcher HTTP client).
- `go.mongodb.org/mongo-driver` **v1.17.9 AND v2.5.0** (reporter pulls both mongo-driver majors — reconcile with midaz's mongo-driver version; risk of conflict).

These are additive and uncontroversial except the **dual mongo-driver major** — confirm midaz's choice and collapse reporter to one if "liso" demands it.

---

## 4. Structure / module boundary — how it lands in midaz

**midaz is a SINGLE Go module** (one `go.mod` at root, `github.com/LerianStudio/midaz/v3`). crm proves multiple service-components live in one module (`components/crm` has its own `cmd/app/main.go`, `internal/`, `api/`, Dockerfile, docker-compose). Reporter follows the same shape, so:

### Recommended target layout (mirrors crm, keeps two binaries)
```
components/reporter/
  manager/   { cmd/app/main.go, internal/{bootstrap,services,adapters}, api/ }
  worker/    { cmd/app/main.go, internal/{bootstrap,services,adapters} }
  pkg/       ← reporter's pkg/ moves HERE, NOT to midaz/pkg (see §5 collisions)
  Dockerfile.manager / Dockerfile.worker (or keep per-subdir)
  docker-compose.yml
  templates/examples/ (2 sample .tpl files)
```

**Decision point — where does reporter's `pkg/` go?**
- **Option A (recommended): `components/reporter/pkg/`.** Keeps reporter self-contained, avoids the `pkg/constant`, `pkg/net`, `pkg/shell` collisions with midaz/pkg (see §5), and matches the "co-located component, not collapsed" intent. Import prefix becomes `…/midaz/v3/components/reporter/pkg/...`.
- Option B: hoist into `midaz/pkg/`. Rejected — three name collisions, forces a merge of unrelated `constant`/`net`/`shell` packages, and couples reporter's internals into the shared namespace. Violates "co-located, keeps own boundary."

**`go.mod` fate:** reporter's `go.mod`/`go.sum` are **deleted**. Its `require` block merges into midaz's root go.mod, then `go mod tidy`. Reporter's `module github.com/LerianStudio/reporter` declaration disappears.

---

## 5. `pkg/` collisions with `midaz/pkg`

Three directory-name collisions if reporter's pkg were hoisted into `midaz/pkg`:
- **`constant`** — both have a `constant` package, different contents. (midaz: entity/error constants; reporter: its own.)
- **`net`** — both have `net` (reporter `pkg/net/http` is heavily used, 89 import sites).
- **`shell`** — both have `shell`.

**Resolution:** keep reporter's `pkg/` under `components/reporter/pkg/` (Option A above). Collisions vanish because the import paths differ entirely. No package merge, no rename of internal packages. This is the clean path and the reason Option A wins.

Non-colliding reporter pkgs (auth, crypto, ctxutil, datasource, fetcher, itestkit, model, mongodb, multitenant, pdf, pongo, postgres, rabbitmq, readyz, redact, redis, seaweedfs, storage, template_builder, templateutils) carry over untouched aside from the path prefix.

---

## 6. Databases, migrations, config/env

- **Primary store: MongoDB.** Collections under `pkg/mongodb/{report,template,deadline,extraction}`. Env: `MONGO_*` / `MONGO_DB_*` (host/port/user/password/database/uri/pool sizing/TLS). Reporter runs its **own MongoDB**, independent of ledger's.
- **Redis:** cache, locks, idempotency, tenant pub/sub event listener. Env: `REDIS_*` (host/port/password/db/TLS, GCP IAM auth, token lifetime, sentinel master name).
- **RabbitMQ:** job queue + notifications + DLQ. Env: `RABBITMQ_*` (host/port-amqp/port-host, uri, exchange, `GENERATE_REPORT_QUEUE`/`_KEY`, `NUMBERS_OF_WORKERS`, TLS, health-check url, vm-memory-watermark).
- **SeaweedFS (S3-compatible):** binary storage for templates + rendered reports. Uses aws-sdk-go-v2 s3. `components/infra/seaweedfs`.
- **External datasource DBs (MySQL + Postgres):** the worker's fetcher connects to **customer/source databases** to extract report data — including `midaz_onboarding` and `midaz_transaction` referenced in the fetcher. These are NOT reporter-owned schemas; they are read targets configured at runtime via `/v1/management/connections`.
- **Migrations:** **none in-repo.** No `migrations/` directory, no `.sql` schema files. Mongo is schemaless; collections are created on write. → **No migration tooling to fold.** (Contrast with ledger's SQL migrations.)
- **Config loading:** `libCommons.InitLocalEnvConfig()` in both mains, then big `config.go` structs. Multi-tenant aware (`MULTI_TENANT_*`, tenant-manager mongo/rabbitmq/redis/valkey/s3 managers — same lib-commons tenant-manager surface midaz uses). `set-env` / `check-envs` Make targets exist.

---

## 7. Throwaway snapshots to EXCLUDE — CONFIRMED

`docs/codereview/ast-before-3807876316/` and `docs/codereview/ast-before-3371843854/` are **full repo snapshots** (each has its own `go.mod`, `go.sum`, `components/`, `pkg/`, `tests/` — including duplicate `manager/cmd/app/main.go` and `worker/cmd/app/main.go`). These are AST-diff baselines from a code-review tool.

**Status: SAFE TO EXCLUDE.**
- They are **git-ignored** (`git check-ignore` confirms) and **untracked** (`git ls-files docs/codereview/` returns 0 files).
- ~**35 MB** on disk, not in version control.
- They will NOT follow into the monorepo via any git-based move (clone, subtree, `git mv`). Only a raw `cp -r` of the working tree would drag them in — so the move method must be git-based or must explicitly exclude `docs/codereview/ast-before-*`.

---

## 8. Build / Docker / CI to fold

### Docker
- Per-component Dockerfiles: `components/manager/Dockerfile`, `components/worker/Dockerfile` (Chromium image). Both build from repo root (`COPY pkg/ pkg/; COPY components/<x>/ ...`) — **the COPY paths assume reporter's root layout and will break once nested under `components/reporter/`.** Rewrite COPY paths and build context.
- Per-component `docker-compose.yml` × 3 (`manager`, `worker`, `infra`). midaz has its own `components/{ledger,crm,infra}/docker-compose.yml` + root `make up`. Reporter's compose must be reconciled into midaz's compose topology (shared infra network, avoid port clashes — reporter uses 4005/4006; ledger/crm use other ports, verify no overlap).
- `components/infra/{rabbitmq,seaweedfs}` — reporter ships its own RabbitMQ + SeaweedFS infra definitions. Decide: reuse midaz's shared infra or keep reporter's SeaweedFS as a reporter-specific add-on (midaz core has no SeaweedFS dependency today).

### Makefile
Reporter has a full 23 KB Makefile mirroring midaz conventions (`set-env`, `up/down`, `test-unit`-equivalents, `lint/format/imports/tidy`, `sec-gosec/sec-govulncheck`, `build-docker`, `generate-docs`, `generate-mocks`, multi-tenant test target). Fold into midaz's component-delegation pattern (`make reporter COMMAND=<target>` style, mirroring `make ledger COMMAND=...`).

### CI (`.github/workflows`)
Reporter has 5 workflows: `build.yml` (push), `go-combined-analysis.yml` (PR), `pr-security-scan.yml` (PR), `pr-validation.yml` (PR), `release.yml` (push). Plus `.releaserc.yml` + `.releaserc.hotfix.yml` (semantic-release), `dependabot.yml`, `labeler.yml`, `CODEOWNERS`. These **collapse into midaz's existing workflows** — reporter loses its independent release pipeline and rides midaz's. Independent versioning of reporter's container images (currently tagged `Reporter Manager API` / `Reporter Worker`) must be reconciled with midaz's release/versioning scheme. **This is a real product decision, not a mechanical fold** — see decisions.

---

## 9. Hard parts (ranked)

1. **Observability rewrite (lib-commons → lib-observability), 241 sites.** Largest, highest-touch. Pure prefix-swap won't compile; APIs differ. Use `ring-dev-team:migrate-observability`. Do this as a discrete PR, ideally in-repo on reporter BEFORE the move so it's reviewable in isolation, then move clean code.
2. **Release/CI identity collapse.** Reporter has independent semantic-release + dual container images. Folding into midaz's pipeline changes how reporter is versioned and shipped. Operational + product decision.
3. **Docker build-context rewrite.** Every Dockerfile COPY path breaks under the new nesting; compose topology + infra (SeaweedFS, reporter-own RabbitMQ) must merge without port/network clashes.
4. **go.mod merge + dual mongo-driver.** Reporter pulls mongo-driver v1 AND v2; midaz must pick one. Resolve toxiproxy/testcontainers version skew during `go mod tidy`.
5. **Module rename (518 files).** High volume, low risk — scripted prefix swap. The `pkg/` placement decision (§5) determines the exact prefix.
6. **lib-auth v2.7→v2.8 + lib-commons v5.1.3→v5.2.0-beta.12 API drift.** Small, but recompile-and-fix surface in auth middleware and tenant-manager wiring.

---

## 10. What "clean entry" requires (checklist)

- [ ] Migrate reporter observability to `lib-observability` v1.0.1 (241 sites) — **do first, separately.**
- [ ] Bump reporter to lib-commons v5.2.0-beta.12, lib-auth v2.8.0, Go 1.26.3; fix API drift.
- [ ] Move tree to `components/reporter/{manager,worker,pkg,...}` (keep two binaries, mirror crm).
- [ ] Rename module path prefix across 518 files; delete reporter `go.mod`/`go.sum`.
- [ ] Merge reporter's `require` block into midaz root go.mod; `go mod tidy`; collapse dual mongo-driver.
- [ ] Rewrite Dockerfile COPY paths + build contexts; reconcile compose + infra (SeaweedFS, RabbitMQ) into midaz topology; confirm port 4005/4006 free.
- [ ] Fold Makefile into `make reporter COMMAND=...`; fold/retire 5 CI workflows + releaserc into midaz's pipeline.
- [ ] Exclude `docs/codereview/ast-before-*` (git-ignored already — ensure move method doesn't drag them).
- [ ] Verify: both binaries build, manager `/health` + `/readyz`, worker consumes `generate-report`, full pipeline renders a PDF end-to-end.
