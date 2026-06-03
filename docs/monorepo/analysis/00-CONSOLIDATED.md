# 00 — CONSOLIDATED Monorepo Consolidation Analysis

**Synthesis of dossiers 01–09.** Target end-state (Fred): *"liso e final — sem shims, workarounds,
abstrações de compatibilidade."* This document is the single source of truth that the eventual task
plan expands. It reconciles the nine analyzer dossiers, flags where they disagree, extracts the
pivotal decisions, and lays out phases, risks, and the decisions that must be made before planning.

Source dossiers:
- `01-midaz-ledger.md` — the integration HOST (fees + crm collapse target)
- `02-midaz-crm.md` — crm → ledger service collapse (move #4)
- `03-midaz-host.md` — host monorepo shell (Makefile/CI/Docker topology)
- `04-tracer.md` — tracer → component co-location (move #1)
- `05-reporter.md` — reporter → component(s) co-location (move #2)
- `06-plugin-fees.md` — plugin-fees → ledger service collapse (move #3)
- `07-deps-harmonization.md` — cross-repo dependency unification
- `08-build-ci-docker.md` — build / CI / Docker / release harmonization
- `09-monorepo-strategy.md` — module topology + git-history decision (move #5)

---

## 1. Executive Summary

Midaz is **already a single-module monorepo**: one root `go.mod` (`github.com/LerianStudio/midaz/v3`,
Go 1.26.3), no `go.work`, no nested modules, no `replace` directives. `components/ledger` and
`components/crm` are package trees under it, `components/infra` is pure ops. The consolidation is
therefore **not "introduce a monorepo" — it is "absorb four more bodies of code into the one that
already exists,"** and the host has already proven the hardest pattern internally: `components/ledger`
is itself a prior collapse of `onboarding` + `transaction` into one binary, one Fiber port (`:3002`),
one Launcher lifecycle, mounted via a variadic `RouteRegistrar` seam that takes new domains with **zero
changes to the unified server.**

The five moves are NOT equivalent in kind:

| # | Move | Type | End-state of its binary |
|---|------|------|--------------------------|
| 1 | tracer → `components/tracer` | **Co-locate** | survives (own service, :4020) |
| 2 | reporter → `components/reporter-{manager,worker}` | **Co-locate** | both survive (:4005 / :4006) |
| 3 | plugin-fees → `components/ledger` | **Service collapse** | dies; folds into tx-create path |
| 4 | crm → `components/ledger` | **Service collapse** | dies; folds into ledger bootstrap |
| 5 | harmonize (module/CI/release) | The framing decision | — |

Net deploy units after: **4** (ledger+fees+crm, tracer, reporter-manager, reporter-worker), down from 6.

**The single chokepoint that gates everything is `lib-commons`.** As of `lib-commons/v5 v5.2.0`, the
packages `commons/{log,opentelemetry,zap}` were removed and re-homed in a separate module
`lib-observability` (verified against the module cache: v5.1.3 has them, v5.2.0 does not). Midaz is
already on the far side (`v5.2.0-beta.12` + `lib-observability v1.0.1`). The three incoming repos are
NOT:
- **tracer** is on `lib-commons v4.6.3` — two boundaries back, AND carries a live v4/v5 dual-import
  shim in `config.go` (the exact compatibility scaffolding the end-state forbids). Its migration is a
  **library-architecture re-platforming, not a version bump** — the long pole of the whole project.
- **reporter** (v5.1.3) and **plugin-fees** (v5.1.0) are on the *near side* of the split — they still
  import `commons/{log,opentelemetry,zap}`. **This is the most-missed finding: reporter and fees ALSO
  need the observability migration, not just tracer.** (241 sites reporter, 86 sites fees.)

Because all v5 repos share the same major, **there is no `/vN`-path escape and no `replace`-bridge
allowed under "no shims": unification, not coexistence.** Every incoming repo must migrate to
`lib-observability` before/atomic-with landing.

The recommended strategy is **Option A (single root go.mod)** — the shape midaz already runs — with
the tracer v4→v5 migration as an explicit prerequisite gate, and **git-history fresh import** (origin
repos kept as read-only archives). Service collapses (fees, crm) MUST share ledger's go.mod regardless,
so the only real topology question is tracer/reporter, and a workspace there would re-introduce the
skew the consolidation exists to eliminate.

**Effort shape:** the work is dominated by three line items, in descending pain: (1) tracer v4→v5 +
lib-observability + module rename (~the dependency long pole, week-class), (2) fees double-entry
correctness embed (~3–4 weeks, dominated by validation not code — the only move touching the third
rail), (3) reporter observability rewrite (241 sites, ~observability ≈ everything-else-reporter
combined). crm is the cheap one (~3–5 days, already in-module). CI/Docker/release harmonization
(~7–8 days) is real but **strictly gated last** — nothing unified builds until the code is one module.

---

## 2. Per-Move Analysis

### 2.1 The HOST: `components/ledger` (dossier 01)

The spine everything plugs into. Key facts:

- **Composition root** is a single `//nolint:gocognit,gocyclo` `InitServersWithOptions` (~540 lines):
  `Config` via `libCommons.SetConfigFromEnvVars` + programmatic defaults; sequential infra init each
  with a reverse-order cleanup closure; **two flat god-structs** (`command.UseCase`, `query.UseCase`,
  each a bag of ~13 repo interfaces); 12 thin handlers built as `{Command, Query}` pointers; routes
  attached via `NewUnifiedServer(addr, logger, telemetry, readyz, routeRegistrars ...RouteRegistrar)`.
- **The clean seam:** the variadic `RouteRegistrar` list. CRM and fees mount as additional registrars
  with ZERO changes to `UnifiedServer` or the Launcher lifecycle in `service.go`.
- **The fee seam (load-bearing):** all 5 tx modes (json/dsl/inflow/outflow/annotation) + revert funnel
  into ONE method, `executeCreateTransaction` (`transaction_create.go` ~L969). Insert fee application
  **between `ApplyDefaultBalanceKeys` (~L1007) and idempotency hashing (~L1020)** — pre-validate,
  pre-backup-seed, pre-balance-mutation. Mirror the existing `enrichOverdraftOperations` enrichment
  pattern (the in-repo precedent). Downstream machinery then treats fee legs as ordinary `FromTo`.
- **Migrations** are golang-migrate SQL baked into the Docker image, run at startup in single-tenant
  mode via `libPostgres.NewMigrator` with a CWD-relative path; MT delegates to tenant-manager.
  **CRM and fees are Mongo-only → NO new Postgres migrations, Dockerfile unchanged.**
- **Multi-tenancy:** one `tmmiddleware.NewTenantMiddleware` keyed by `constant.Module*` — only
  `ModuleOnboarding`/`ModuleTransaction` exist today. New `ModuleCRM`/`ModuleFees` constants + `WithMB`
  registrations + tenant-manager per-tenant provisioning are **required or MT requests to those domains
  fail at DB resolution.**
- **Prerequisite refactor:** extract `initCRM()` / `initFees()` helpers from the god-function (mirroring
  `initTransactionPostgres`) BEFORE bolting on two domains, or the composition root becomes unreviewable.

### 2.2 crm → ledger (dossier 02) — the cheap collapse

Easiest of the five: **already inside the midaz module** (no go.mod, no skew, lib-commons v5,
lib-observability already migrated). Mongo-only, no Postgres/RabbitMQ/Redis-queue, no static
migrations (per-org dynamic collections `holders_<orgID>`/`aliases_<orgID>` keyed off
`X-Organization-Id`). Domain fully decoupled from ledger (Holder/Alias never reference
Account/Ledger/Transaction). Models (`pkg/mmodel/holder.go`, `alias.go`) and 31 `CRM-*` error
sentinels already shared.

The real work concentrates in three places:
1. **Multi-tenant Mongo wiring.** CRM uses tenant **module** name `crm-api` (distinct from
   onboarding/transaction) but the SAME tenant **service** identity (`APPLICATION_NAME=ledger`). So MT
   already resolves both under service `ledger`, different modules — the cleanest possible MT story.
   Needs: a 3rd `tmmongo.Manager(WithModule("crm-api"))` reusing ledger's tmClient/tenantCache/
   tenantLoader, a CRM-scoped tenant middleware, and **folding CRM pool eviction into ledger's existing
   `TenantEventListener` `WithOnTenantRemoved`** (not a second listener).
2. **Middleware de-duplication.** CRM's `NewRouter` re-applies recover/telemetry/CORS/logging/health/
   readyz that `UnifiedServer` already does once. **DANGER:** CRM registers a global `ErrorCodeTransformer`
   that rewrites the `code` field of EVERY 4xx/5xx response via `CRMErrorMapping` — mounted globally on
   ledger's shared app it would corrupt ledger's own codes (e.g. `0046` → `CRM-0014`). Must be deleted
   or route-group-scoped (product decision, see PD-2).
3. **Config namespacing + crypto carry-over.** CRM's flat `MONGO_*`/`LCRYPTO_*` env must become
   `CRM_`-prefixed in ledger's Config. The `libCrypto.Crypto` cipher (`LCRYPTO_HASH/ENCRYPT_SECRET_KEY`)
   that encrypts holder PII **must be carried with the same key values or existing holder/alias docs
   become undecryptable** (data-integrity risk, not just config).

Teardown: delete CRM main/Server/Service/Dockerfile/compose/Makefile/swagger dup; preserve the
`crm-api` tenant-manager identity (do NOT rename to `ledger`); fix misleading `libCommons`-aliasing-
lib-observability imports.

### 2.3 plugin-fees → ledger (dossier 06) — the hardest move (double-entry third rail)

fees today is a **stateless compute service**: `POST /v1/fees` takes a full transaction, runs the
`pkg/fee` engine (proportional split, deductible vs non-deductible, exemptions by alias/segment,
Ceil/Floor rounding on the max account + `applyFeeCorrection` to keep `sum(legs)==fee total`), and
returns the SAME payload mutated in place. **It persists nothing to the ledger and never POSTs a
transaction.** CRITICAL: midaz has ZERO references to fees — the fees-then-ledger sequencing is done by
an EXTERNAL client/gateway, not by code. fees' only outbound calls to ledger are READS for
account/segment resolution, via m2m auth.

The integration seam is a gift: **both fees and ledger call the same function,
`mtransaction.ValidateSendSourceAndDistribute`** — in ledger at `transaction_create.go:1045` and
`transaction_state_handlers.go:433`. Embed point: invoke the in-process fee engine to rewrite the legs,
then **RE-RUN validation on the mutated payload.**

Three categories of work:
- **Mechanical (small):** repoint the 18 `midaz/v3/pkg/transaction` imports to in-tree `pkg/mtransaction`
  (the path was renamed and no longer exists at HEAD — commit `1b4913dab`); delete main/bootstrap/docker/CI.
- **Infra teardown (the big simplification):** delete `internal/m2m/*` (AWS Secrets Manager per-tenant
  creds) + the `MidazService` HTTP client + account cache; the 4 outbound reads (GetAccount,
  GetAccountDetails, CountByRoute, ListAccounts) become direct `query.UseCase` calls. Absorb the
  billing-package Mongo collections + 11 index migrations + CRUD endpoints. **Fees is a STATEFUL service,
  not a pure function — the "collapse" framing underweights the persisted billing-package state and its
  API surface.**
- **Correctness (the third rail, ~1.5–2.5 weeks):** the fee engine's asset-precision rounding
  (`pkg/fee/asset_precision.go::getAssetPrecision`) MUST be unified with ledger's scale/Lua machinery,
  or fee legs round one way and balances another → off-by-one cents → **unbalanced transaction = third-rail
  violation**. The free re-validation that exists today (caller resubmits) disappears on embed, so
  `ValidateSendSourceAndDistribute` MUST be re-run after fee mutation. Plus: `FromTo.Route` is deprecated
  for `RouteID` and the engine writes synthetic Route strings (silent route-validation breakage if not
  re-pointed); overdraft-enrichment ordering (fees before enrichment); idempotency hash is over the raw
  payload pre-mutation (stable in the common case — document the package-config-churn assumption).

### 2.4 tracer → component (dossier 04) — the dependency long pole

Self-contained Fiber v2 service (rule/limit/audit-event validation with CEL, PostgreSQL-only, per-tenant
lazily-spawned workers, godog BDD e2e). Go 1.26.3 exact match. Co-location is structurally target-shaped
(single module dissolves `module tracer`) but mechanically large, dominated by two stacked migrations
midaz has ALREADY proven (commit `766b555d2`, the observability migration):

1. **Module rename:** scripted prefix rewrite `tracer/...` → `…/components/tracer/...` across 465 files /
   749 sites / 38 packages. Deterministic, not hand work. (Also rewrite non-Go string refs: `.golangci.yml`,
   `mk/*.mk` path filters, `.ignorecoverunit`, swagger annotations.)
2. **Dependency unification, two jumps:**
   - **lib-commons v4→v5** non-observability surface (~92 sites: commons, tenant-manager/{core,postgres,
     client,event,middleware,redis}, postgres, net/http, runtime, server). All confirmed present in
     midaz's v5 usage. Expect constructor-option/config-struct drift, not missing capability.
   - **observability re-platforming** (~123 sites): `lib-commons/v4/commons/{log,opentelemetry,
     opentelemetry/metrics,zap}` → `lib-observability/{log,tracing,metrics,zap}`. Logger/Telemetry APIs
     are **shape-compatible** (`.Log(ctx,level,msg,fields...)`, `libLog.String/Err`,
     `NewTelemetry(TelemetryConfig{})`), so this is overwhelmingly an import-path sweep.

The ONE genuine non-mechanical code move: **telemetry middleware relocation.** tracer's
`libHTTP.NewTelemetryMiddleware` comes from `v4/commons/net/http` (HTTP helpers + telemetry bundled);
midaz split it — HTTP helpers stay in `v5/commons/net/http`, telemetry middleware moved to
`lib-observability/middleware`. A naive path-swap will NOT compile.

Sharp edges: tenant-manager v4→v5 API drift (highest-uncertainty surface, the likely real breakage);
godog BDD e2e is a test mode midaz CI doesn't run (new CI plumbing + cucumber dep tree); audit-event
hash chaining (migrations 000001/000002/000017 + `VerifyHashChain`) is SOX/GLBA-sensitive — pure
relocation only, no renumber/no hash-logic touch; CEL engine (cel-go + antlr + gopher-lua) is a large
unique dep subtree. Only `pkg/shell` is true dead-duplication; tracer's `pkg/{constant,model,net/http}`
are domain-specific and co-exist cleanly under `components/tracer/pkg`.

### 2.5 reporter → component(s) (dossier 05) — the observability-rewrite move

Already shaped like midaz (`components/{manager,worker,infra}`, lib-commons launcher, Fiber,
MongoDB-first). Two services that MUST stay separate deploy units: **manager** (REST API, :4005,
produces report jobs to RabbitMQ) and **worker** (headless renderer, consumes the queue, pongo2 →
chromedp/Chromium → PDF → SeaweedFS; **mandatory fat alpine+Chromium image, cannot be distroless**).
They communicate via RabbitMQ + shared Mongo/Redis/SeaweedFS, not HTTP/DB-handoff.

CRITICAL: **zero Go-level coupling to midaz** — reporter reaches ledger data over HTTP via a generic
"fetcher" datasource abstraction (connects to `midaz_onboarding`/`midaz_transaction` and arbitrary
customer DBs configured at runtime). So the 518-file module rename is purely internal, no cross-repo
import to reconcile.

The DOMINANT cost is the **observability rewrite: 241 sites** (`commons/log` 159, `commons/opentelemetry`
71, `commons/zap` 11) → `lib-observability/{log,tracing,zap,middleware,metrics}`. APIs differ (not a
prefix swap). Roughly observability ≈ everything-else-reporter combined. Do it IN reporter's own repo
FIRST so the move PR stays a mechanical rename+fold.

Placement: reporter's `pkg/` lands at `components/reporter/pkg/` (NOT midaz/pkg) to avoid three
collisions (`constant`, `net`, `shell`). No SQL migrations (Mongo schemaless). Net-new infra: **SeaweedFS**
(S3 object store) and **KEDA** (worker autoscaler). Dual mongo-driver major (v1.17.9 + v2.5.0) to
collapse onto midaz's choice. The 35MB `docs/codereview/ast-before-*` snapshots are git-ignored +
untracked — confirmed safe to exclude (enforce git-based move).

> **Cross-dossier discrepancy to resolve (flagged, not papered over):** dossiers 05 and 07 say reporter
> still imports `lib-commons/v5/commons/{log,opentelemetry,zap}` (241 sites, on the near side of the
> split). Dossier 09 says reporter "never adopted lib-observability — it logs via raw zap directly."
> These are different claims about reporter's current logging stack. **Both lead to the same conclusion
> — reporter needs an observability migration to match midaz's lib-observability posture — but the exact
> source state must be verified against reporter's go.mod/imports before scoping** (it changes whether
> it's a `commons/log → lib-observability/log` rewrite or a `zap → lib-observability` rewrite). Treat the
> 241-site figure (05/07, the more specific evidence) as the working number; confirm at plan time.

---

## 3. Cross-Cutting Dependency Matrix

The Lerian shared-library bar every incoming repo must clear (from 03/07/09):

| Library | midaz (host/target) | tracer | reporter | plugin-fees | Unified target |
|---|---|---|---|---|---|
| **lib-commons** | **v5.2.0-beta.12** | v4.6.3 (MAJOR behind) | v5.1.3 | v5.1.0 | **v5.2.x GA** (move midaz off beta first) |
| **lib-observability** | **v1.0.1** (used) | v1.0.0 (indirect) | absent / disputed | v1.1.0-beta.5 (declared, unused) | **v1.0.1** |
| lib-auth/v2 | v2.8.0 | v2.8.0 (exact match) | v2.7.0 | v2.7.0 | **v2.8.0** |
| lib-streaming | v1.4.0 | — | — | — | v1.4.0 (midaz-only) |
| lib-license-go/v2 | — | — | — | v2.3.4 | **v2.3.4** (enters ledger via fees embed — conscious) |
| Go toolchain | 1.26.3 | 1.26.3 | 1.26 / tc 1.26.2 | 1.26.3 | **1.26.3** (drop reporter toolchain line) |

**The only hard cross-major conflict is lib-commons v4 (tracer) vs v5 (everyone).** It cannot be
MVS-resolved because the highest version (v5.2+) *removed packages the lower versions import*.

Everything third-party is same-major minor/patch skew that **MVS resolves upward without code changes**:
otel 1.43/1.44→1.44.0, redis 9.18/9.19/9.20→9.20.0, testcontainers 0.41/0.42→0.42.0, fasthttp
1.69/1.71→1.71.0, validator 10.30.x→10.30.3, grpc 1.80/1.81→1.81.1, rabbitmq 1.10/1.11→1.11.0; fiber
2.52.13, pgx 5.9.2, mongo-driver 1.17.9, migrate 4.19.1, uuid 1.6.0, decimal 1.4.0 already aligned.

**Observability migration scope by repo (the most-missed cross-cutting fact):**

| Repo | `commons/log` | `commons/opentelemetry` | `commons/zap` | Total | Major bump? |
|---|---|---|---|---|---|
| tracer (v4) | 63 | 48 (+10 metrics) | 2 | ~123 | **Yes** (v4→v5 too) |
| reporter (v5.1) | 159 | 71 | 11 | 241 | No (split only) |
| plugin-fees (v5.1) | 43 | 36 | 7 | 86 | No (split only) |

`NewTrackingFromContext` is the single highest-volume rename (301 sites in tracer alone) — moves to the
ROOT `lib-observability` package.

**Net-new dependency surface entering the unified module:** reporter (heaviest) — chromedp+cdproto, full
aws-sdk-go-v2 (S3+Secrets Manager), go-mssqldb, go-ora/v2 (Oracle), go-sql-driver/mysql, pongo2/v6,
resty/v2, cloud.google.com/go/*; tracer — cel-go+cel.dev/expr+antlr+gopher-lua, godog, miniredis,
sqlmock; fees — lib-license-go/v2. Go's per-binary dead-code elimination keeps the ledger artifact clean
(it won't link chromedp); the cost is go.sum size, mod-download time, and a unified vuln-scan/`make sec`
surface across the whole monorepo.

**Auth/RBAC namespace divergence (does NOT collapse for free):** ledger authorizes under `midaz`/`routing`,
CRM under `plugin-crm`, fees under `plugin-fees` (resources fees/estimates/packages/billing-*).
Tenant-manager RBAC policies key on these strings. Route merge ≠ authz merge — preserve namespaces or
do a deliberate coordinated policy migration. Silent rename breaks authorization.

---

## 4. Build / CI / Docker / Release Plan (dossiers 03, 08)

**Reframe:** midaz and reporter already use the same CI primitive — the
`LerianStudio/github-actions-shared-workflows` `build.yml` driven by `filter_paths` + `path_level:2` +
`app_name_prefix`, which auto-discovers component dirs and fans out one image per changed component.
**Consolidation is merging config, not designing CI.** Do NOT hand-roll a `strategy.matrix`.

Final component topology / images (6 → 4):
```
components/infra/                # single shared infra compose (superset)
components/ledger/               # + embedded fees + crm   → midaz-ledger (3002)
components/tracer/               # co-located               → midaz-tracer (4020)
components/reporter-manager/     # co-located               → midaz-reporter-manager (4005)
components/reporter-worker/      # co-located (Chromium)    → midaz-reporter-worker (4006)
```
`midaz-crm` and `plugin-fees` images are **deleted, not renamed** — their `filter_paths`,
`helm_values_key_mappings`, and `midaz-firmino-gitops` `yaml_key_mappings` entries must be removed, with
ArgoCD/Helm updated in lockstep (cross-team blast radius). Ports verified non-colliding.

Workstreams (all GATED behind module/lib-commons unification — CI is the LAST step):
- **Makefile/mk:** promote tracer's clean `mk/{docker,database,docs,quality,security}.mk` to root
  (net upgrade — midaz inlines these today); normalize the root component list (kill the
  `ledger`-special-cased-outside-`$(COMPONENTS)` footgun); migrate crm's `generate-keys` target into
  ledger's `set-env` or it breaks.
- **Dockerfiles:** tracer/fees build context `.` → repo-root `../../`; fees Dockerfile deleted; **drop
  the `github_token` BuildKit secret + `.secrets/` + `go_private_modules`** (tracer/fees use it; midaz/
  reporter prove the common Lerian libs resolve publicly; fees' only private import was midaz/v3, which
  vanishes). Verify no transitive private module before deleting. Worker stays fat alpine; harmonize
  non-Chromium images to distroless-nonroot.
- **docker-compose:** midaz `components/infra` is the single source of truth (most production-like:
  postgres:17 primary+replica, mongo:8 rs0, valkey:8, rabbitmq:4.1.3, otel-lgtm). ADD SeaweedFS + KEDA
  (reporter-only, net-new). DROP tracer-postgres (point tracer at shared postgres:17 — verify 16→17
  migration compat) and fees-mongo. Adopt reporter's `wait-for-infra` health gate (midaz lacks it).
- **CI workflows:** union `filter_paths`, drop crm/fees entries, pin ONE shared-workflow version (six in
  flight v1.27.4–v1.32.0), ONE go (1.26.2 vs 1.26.3) and ONE golangci (v2.4.0 vs v2.11.3). Adopt fees'
  strict `.golangci.yml` as the floor + a per-component lint-cleanup pass. **85% coverage hard-fail gate**
  applies to all incoming code — likely real test-backfill work, easily underestimated. Add tracer/reporter
  S3 migration-upload jobs if needed (tracer has migrations; reporter/worker don't). Harmonize registry
  policy to DockerHub+ghcr (reporter is ghcr-only).
- **Release/versioning:** single repo-wide semantic-release (what all four already do). One global
  version, fan-out build skips unchanged components. Keep midaz CHANGELOG canonical (770KB), archive the
  other three. Add `@saithodev` backmerge + a `.releaserc.hotfix.yml`. PR-title scopes: add tracer/
  reporter/fees, remove crm. External Helm chart `midaz` + `midaz-firmino-gitops` + APIDog e2e must
  extend in lockstep — a co-located component without them builds but never deploys.

---

## 5. Monorepo Strategy Recommendation (dossier 09)

**Primary: OPTION A — single root go.mod**, with tracer's lib-commons v4→v5 migration as an explicit
prerequisite gate, and **git-history fresh import** (origin repos kept as read-only archives).

Why A:
1. It is the shape midaz already runs (one root go.mod, path-filtered component builds, per-component
   Dockerfiles). B and C introduce a topology midaz has never had.
2. The feared "dep conflict hard-blocks" risk is mostly absent — the ONLY true cross-major conflict is
   lib-commons v4/v5, a Lerian-owned third-rail lib that must be unified anyway; everything third-party
   MVS-resolves upward.
3. It is the only option that satisfies "sem shims." Moves 3 & 4 (fees, crm) are service collapses whose
   code runs INSIDE the ledger binary — they MUST share ledger's go.mod regardless. B (`go.work`) preserves
   the skew behind module walls and needs `replace` directives for `pkg/` — textbook compatibility
   workarounds. **B is co-location wearing a monorepo costume; reject it as an end-state.**
4. Cost is real but bounded and one-time; tracer's sweep is the long pole, the rest is mechanical.

**Fallback: OPTION C (hybrid)** — single module for ledger+crm+fees, fenced modules for tracer/reporter —
**if and only if** tracer's v4→v5 is scoped larger than the consolidation window can absorb. Frame it as
"A, deferred for tracer/reporter," with a committed date to collapse the fence, or it quietly becomes a
permanent B.

**Git history:** fresh import matches Fred's stated value ("liso e final," "the git log carries the past")
and avoids flooding midaz's 6942-commit log with ~3,700 incoming commits and SHA collisions. Selectively
`git filter-repo` ONLY tracer if its audit-hash-chain blame proves operationally load-bearing.

> **Cross-dossier discrepancy (flagged):** dossier 03 recommends `git subtree merge`; dossier 09
> recommends fresh import. 09's reasoning is tighter against Fred's explicit "clean tree over history"
> stance and his global instruction that the git log carries the past. **Recommend fresh import** (see
> PD-3). This is the one place the two dossiers give opposite advice; it is resolved in favor of 09.

---

## 6. Consolidated Risk Register

Severity: 🔴 critical (correctness/compile-blocking/data-loss) · 🟠 high · 🟡 medium · ⚪ low

| # | Risk | Sev | Source | Mitigation |
|---|------|-----|--------|------------|
| R1 | Fees asset-precision/rounding disagrees with ledger scale/Lua → **unbalanced transaction** (double-entry third rail) | 🔴 | 06 | Unify precision into ledger's scale handling; integration tests prove `sum(legs)==fee total` AND txn balances |
| R2 | Removing the free re-validation (caller resubmit) without re-running `ValidateSendSourceAndDistribute` on the mutated payload lets unbalanced legs through | 🔴 | 06 | Re-run validator post-mutation at BOTH call sites; never reuse pre-fee `Responses` |
| R3 | lib-commons v4 (tracer) cannot coexist with v5 in one go.mod; no `replace`-bridge allowed under no-shims | 🔴 | 03/07/09 | Migrate tracer v4→v5 fully before/atomic-with the move; gate Option A on it |
| R4 | reporter + fees ALSO need observability migration (not just tracer) — they're on the near side of the v5.1→v5.2 split; 241 + 86 sites fail to compile in the unified module | 🔴 | 07 | Migrate observability IN-REPO first for both; it is a *precondition* of the fees embed |
| R5 | `pkg/transaction`→`pkg/mtransaction` rename is compile-blocking for all 18 fees import sites; `FromTo.Route` deprecated for `RouteID` is a silent route-validation breakage | 🔴 | 01/06/09 | Repoint imports; re-point fee engine's synthetic Route writes to `RouteID`; diff v3.5.2→HEAD field-by-field |
| R6 | CRM `ErrorCodeTransformer` mounted globally rewrites ledger's own error codes | 🟠 | 02 | Delete the shim (preferred) or route-group-scope it; never global on the unified app |
| R7 | CRM PII crypto keys (`LCRYPTO_*`) not carried with same values → existing holder/alias docs undecryptable | 🟠 | 02 | Carry exact key values into ledger config; key-management, not just config |
| R8 | Missing `ModuleCRM`/`ModuleFees` tenant-middleware + provisioning silently breaks MT requests at DB resolution | 🟠 | 01/02 | Add constants + `WithMB` registration + per-tenant provisioning; fold CRM eviction into ledger's one listener |
| R9 | Auth/RBAC namespace divergence (midaz/routing/plugin-crm/plugin-fees) — route merge ≠ authz merge | 🟠 | 01/06 | Preserve per-domain namespaces initially; unify later as a deliberate policy migration |
| R10 | tenant-manager v4→v5 API drift in tracer (29 sites, MT wiring) — highest-uncertainty surface, likely real breakage | 🟠 | 04/07 | Dedicated API-diff against v5.2 tenant-manager before estimating |
| R11 | 85% coverage hard-fail gate turns CI red on under-tested incoming code | 🟠 | 03/08 | Budget test backfill per component; measure source-repo thresholds early |
| R12 | Deleting crm/fees image entries from helm/gitops breaks ArgoCD sync unless charts update in lockstep | 🟠 | 08 | Cross-team coordination; update Helm `midaz` + `midaz-firmino-gitops` in the same change |
| R13 | tracer telemetry middleware relocation (bundled in v4 net/http → split to lib-observability/middleware) won't compile on naive path-swap | 🟡 | 04/07 | Treat as a real code move, not a sweep; repoint to `lib-observability/middleware` |
| R14 | reporter dual mongo-driver (v1+v2) collapse may surface BSON/codec issues only at runtime | 🟡 | 05/07 | Integration-test against real Mongo; collapse to midaz's chosen major |
| R15 | godog BDD e2e (tracer) is a test mode midaz CI doesn't run — new plumbing + cucumber deps | 🟡 | 04 | Stand up godog in midaz CI as discrete work; budget it |
| R16 | Migrations run at startup with CWD-relative path baked into image — merge changes to working dir/build context can break single-tenant startup migration | 🟡 | 01 | Preserve migration path/context; verify ledger startup migration post-merge |
| R17 | Env-file merge (fees+crm into ledger) silent missing var → runtime failure that passes CI build | 🟡 | 08 | Careful 3-way diff of all .env.example surfaces; namespace collisions (SERVER_PORT/MONGO_*/DB_*) |
| R18 | Fee-on-revert/cancel policy unspecified → incorrect double-entry on reversals | 🟡 | 01/06 | Specify refund-vs-recharge-vs-ignore BEFORE implementation (product decision, PD-5) |
| R19 | Audit-event hash chaining (tracer) is SOX/GLBA-sensitive — accidental migration renumber/hash touch = compliance regression | 🟡 | 04 | Pure relocation only; do not renumber migrations or alter hash logic |
| R20 | reporter-worker Chromium image is large; unified CI/registry doesn't shrink it — scan-time/image-size impact | ⚪ | 05/08 | Accept; keep worker fat-alpine, distroless others |
| R21 | Reporter fetcher connects to external customer DBs (incl. midaz_onboarding/transaction) at runtime — co-location must not change network reachability/credential handling | ⚪ | 05 | Preserve fetcher connection config and network paths |
| R22 | CRM `X-Organization-Id` header scoping vs ledger's path-based org hierarchy persists post-collapse — API-shape inconsistency | ⚪ | 01/02 | Document in unified API surface; rework only if desired |
| R23 | plugin-fees license relicense to Elastic License 2.0 (check-license-header.sh gate) | ⚪ | 03/09 | Relicense source headers on entry |
| R24 | Single global CHANGELOG + repo-wide semantic-release — a misclassified commit type bumps the whole monorepo | ⚪ | 03 | Enforce conventional-commit scopes in pr-validation |
| R25 | STRUCTURE.md already stale (shows onboarding/transaction as components) | ⚪ | 03 | Rewrite as part of harmonization; documentation debt |

---

## 7. Pivotal Decisions (the few that change the whole plan's direction)

See the structured output `pivotal_decisions` for the canonical list (PD-1..PD-7). In brief:
- **PD-1 Module topology:** single root go.mod (Option A) vs go.work (B) vs hybrid (C). → **A**, with
  tracer v4→v5 as a gate; C only as a funded-deferral fallback.
- **PD-2 CRM error-code wire contract:** delete the `ErrorCodeTransformer` shim vs route-group-scope it.
  → **Delete** (it is exactly the compat shim the end-state forbids) — pending confirmation no external
  consumer parses `CRM-00xx`.
- **PD-3 Git-history strategy:** fresh import vs subtree vs filter-repo. → **Fresh import** (resolves the
  03/09 disagreement in favor of 09).
- **PD-4 lib-commons line:** move midaz off `v5.2.0-beta.12` to GA first vs consolidate-on-beta. →
  **GA first** (establishes a stable target all incoming code is rewritten to).
- **PD-5 Fee-on-revert semantics:** refund vs re-charge vs ignore. → **Refund** (least-surprising) — but
  this is a PRODUCT call for Fred, and it gates the fees embed's correctness tests.
- **PD-6 Dependency-migration sequencing:** migrate each repo's deps in-place FIRST, then move. → **Yes,
  in-place first** (preserves bisectability; observability+co-location in one commit is un-debuggable).
- **PD-7 Fee/billing package persistence:** new collections in ledger's Mongo vs migrate to Postgres. →
  **Mongo collections** (config documents, not ledger entries; 11 indexes port cleanly).

---

## 8. Full List of Decisions That Must Be Made Before Planning

Pivotal (PD-1..PD-7 above) plus the secondary calls each dossier surfaced:

**Topology & history:** PD-1 (module topology), PD-3 (git history), PD-6 (migration sequencing).

**Dependency:** PD-4 (lib-commons GA vs beta); tracer v4→v5 single-jump vs two-step (recommend two-step
for bisectability); drop fees' stale `lib-observability v1.1.0-beta.5` to v1.0.1; confirm lib-auth v2.8.0
logger accepts `lib-observability/log.Logger` (tracer shim's reason for existing); mongo-driver dual-major
— consolidate now or defer (recommend defer, out of scope).

**Fees embed:** PD-5 (revert/cancel fee semantics — PRODUCT), PD-7 (package persistence); where fee calc
lives (recommend port `pkg/fee` + private handler helper at the seam, replace HTTP resolver with direct
query.UseCase); extend god-UseCases vs separate `fees.UseCase`/`crm.UseCase` (recommend separate UCs,
fees holds a query.UseCase ref); idempotency key over raw vs fee-mutated payload (recommend raw, document
assumption); auth resource ACL reconciliation (plugin-fees → ledger model).

**CRM embed:** PD-2 (error-code shim — PRODUCT: do external clients parse CRM-00xx?); CRM Mongo topology
(dedicated CRM_-prefixed connection vs share ledger's instance — recommend dedicated); single tenant
event listener vs two (recommend fold into ledger's one).

**Auth/RBAC:** namespace strategy for the merged binary (recommend preserve per-domain namespaces
initially, unify later).

**Reporter:** pkg/ placement (recommend `components/reporter/pkg/`); observability migrate before vs
during move (recommend before, in-repo); release pipeline/versioning after fold (recommend full midaz
scheme — ops/product call); SeaweedFS/RabbitMQ infra reuse vs separate (recommend reuse midaz RabbitMQ,
add SeaweedFS as reporter-specific); resolve the 05-vs-09 reporter-observability-state discrepancy by
verifying reporter's actual logging stack.

**tracer:** keep bespoke `guard.With()` vs conform to `http.ProtectedRouteChain()` (recommend keep for
liso entry, polish later); ride midaz monorepo release / delete tracer semantic-release (recommend yes);
keep tracer pkg co-located vs hoist (recommend co-located, only consolidate dead `pkg/shell`); tracer's
role vs infra's existing otel-lgtm (replace/complement/independent — ARCHITECTURE call for Fred, gates
tracer's compose/networking and whether otel-lgtm stays).

**Reporter packaging:** one `components/reporter` with two `cmd/` entrypoints vs two top-level components
(recommend verify the shared build.yml emits two images for one path at `path_level:2`; if not, the
no-shims mandate forces the two-component layout).

**Build/CI/release:** PD-relevant — single repo-wide version (recommend, what all four already do); single
`app_name_prefix` `midaz-*` (recommend, accept image rename + update helm/gitops); delete github_token
machinery (recommend, verify no transitive private module); single `.golangci.yml` floor (recommend fees'
strict config + cleanup pass); registry policy (recommend DockerHub+ghcr both).

**Out-of-repo coordination (hard deploy dependencies):** Helm chart `midaz`, `midaz-firmino-gitops`,
APIDog e2e must extend in lockstep — confirm ownership and sequencing before any image-rename/delete lands.
