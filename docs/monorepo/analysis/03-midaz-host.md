# 03 — Midaz Host Shell (Monorepo Consolidation Target)

**Scope:** `/Users/fredamaral/repos/lerianstudio/midaz` analyzed at migration altitude as the **host monorepo shell** that will absorb `tracer`, `reporter`, `plugin-fees`, and collapse `crm` into `ledger`.

**Bottom line:** Midaz is already a working monorepo — single Go module, multiple co-located components, one `infra` component providing shared backing services, a root Makefile that delegates to per-component Makefiles, and CI that fans out over an **explicit, hardcoded component list**. The structural slot for a NEW co-located component (tracer, reporter) is well-defined and low-friction. The two HARD parts are (1) the single-module dependency unification — every incoming repo must drop onto midaz's `lib-commons v5.2.0-beta.12` / `lib-observability v1.0.1` / Go 1.26.3 baseline with **no `replace` directives and no per-component go.mod** — and (2) the service collapses (fees→ledger, crm→ledger) which are code-integration problems the host shell does not solve for you.

---

## 1. Module & Repo Topology

- **Single Go module:** `github.com/LerianStudio/midaz/v3`, `go 1.26.3`. One root `go.mod` (160 require lines, ~8KB) and one `go.sum` (~50KB). **No nested go.mod anywhere, no `replace` directives, no workspace (`go.work`).** Confirmed by grep — clean single-module monorepo.
- **Components are packages, not modules.** `components/ledger` (602 `.go` files) and `components/crm` (70 `.go` files) are just package trees under the one module. `components/infra` has **no Go code at all** — it is pure ops (docker-compose + provisioning scripts).
- **Implication for incoming repos:** tracer (`module tracer`, non-qualified — currently un-importable), reporter (`github.com/LerianStudio/reporter`), and plugin-fees (`github.com/LerianStudio/plugins-fees/v3`) all **lose their module identity** on entry. Their package import paths must be rewritten to `github.com/LerianStudio/midaz/v3/components/<name>/...`. Their `go.mod`/`go.sum` are deleted; their dependencies are merged into the root `go.mod` and reconciled to midaz's pinned versions. This is the single largest mechanical change and the place version skew bites.

### Pinned shared-library baseline (the bar every incoming repo must clear)

| Library | Midaz (host) | tracer | reporter | plugin-fees |
|---|---|---|---|---|
| `lib-commons` | **v5.2.0-beta.12** | v4.6.3 (MAJOR behind) | v5.1.3 | v5.1.0 |
| `lib-observability` | **v1.0.1** (split out of lib-commons) | n/a (still inside lib-commons v4) | varies | varies |
| `lib-streaming` | **v1.4.0** | — | — | — |
| `lib-auth` | **v2.8.0** | — | — | — |
| Go toolchain | **1.26.3** | — | 1.26 | — |

The `lib-observability v1.0.1` line matters: midaz has already **migrated observability out of lib-commons** (see `main.go` importing `lib-observability/log`, `lib-observability/zap`). tracer on lib-commons **v4** predates even the v5 line — its observability code still lives inside lib-commons. tracer is therefore not a version bump, it is a **library-architecture migration** (v4 monolith → v5 + lib-observability split). Budget tracer as the hardest single dependency reconciliation.

---

## 2. The `pkg/` Shared Layer

`pkg/` is the cross-component shared code. It is **heavily depended upon** — `pkg/mmodel` is imported by 372 files under `components/`, `pkg/constant` by 259. This is the spine of the monorepo.

| Package | Files | Role |
|---|---|---|
| `pkg/mmodel` | 40 | Domain models (Organization, Account, Transaction, …). The canonical types. CLAUDE.md mandate: **no `/internal/domain`**, models live here. |
| `pkg/constant` | 14 | Error sentinels (`errors.go`, codes 0001–0161) + entity constants (`entity.go`). **All error sentinels must be unique and defined here** — hard rule. |
| `pkg/streaming` | 65 | lib-streaming wiring + `pkg/streaming/events/` wire contracts (one file per event, JSONShape-locked). Alias mandate: `pkgStreaming`. |
| `pkg/gold` | 12 | Transaction DSL parser (ANTLR4). |
| `pkg/mtransaction` | 16 | Transaction model + validation helpers. |
| `pkg/net` | 11 | HTTP middleware, route helpers, pagination glue. |
| `pkg/utils` | 15 | Shared utilities. |
| `pkg/mbootstrap` | 3 | Shared bootstrap helpers. |
| `pkg/mongo`, `pkg/pagination`, `pkg/repository`, `pkg/shell` | small/empty | Misc shared infra. `pkg/shell` has 0 `.go` files (scripts only). |

**Migration consequences:**
- Incoming components SHOULD route their domain types into `pkg/mmodel` and their error sentinels into `pkg/constant/errors.go` if they want to participate in midaz conventions. For **service collapses** (fees, crm) this is mandatory — collapsed code lives inside `components/ledger` and must use ledger's `pkg` types, not its own copies. Duplicate error sentinels are explicitly banned.
- For **co-located components** (tracer, reporter) that keep their own binary, the question is how much of their own model/error code stays component-local vs. promotes to `pkg/`. Default: keep component-private code under `components/<name>/internal/`, promote to `pkg/` only when genuinely shared. Do not bloat `pkg/` with reporter-only or tracer-only types.
- **STRUCTURE.md is stale** — it still documents `onboarding` and `transaction` as separate components, which have already been collapsed into `ledger` (ledger's Makefile header literally reads "Unified Onboarding + Transaction"). Treat STRUCTURE.md as aspirational/outdated; the live truth is `components/{crm,infra,ledger}` + `pkg/`. STRUCTURE.md will need a rewrite as part of consolidation (it is not load-bearing for build/CI, so it is documentation debt, not a blocker).

---

## 3. Component Layout Convention (how a NEW component slots in)

Every Go-bearing component (ledger, crm) follows an **identical skeleton**:

```
components/<name>/
  cmd/app/main.go        # entrypoint — InitLocalEnvConfig, build libZap logger, bootstrap.InitServersWithOptions, service.Run()
  internal/
    adapters/http/in/    # Fiber handlers + routes.go
    adapters/postgres|mongodb|redis|rabbitmq/   # as needed
    bootstrap/           # config.go (env struct), service.go, readyz.go, tls_detection.go, server wiring
    services/command|query/   # CQRS use cases, one file per operation
  migrations/<db>/       # raw .up.sql/.down.sql pairs (ledger only; crm is mongo-only, no migrations)
  api/                   # swagger/openapi artifacts
  artifacts/  .bin/      # build output (gitignored)
  Dockerfile             # builds from REPO ROOT context, single binary
  docker-compose.yml     # one service, joins infra-network (external) + <name>-network
  Makefile               # standardized target set (see §5)
  .env / .env.example
```

**`main.go` is a near-identical template** across ledger and crm:
1. `libCommons.InitLocalEnvConfig()`
2. read `LOG_LEVEL`, `ENV_NAME`, `OTEL_RESOURCE_SERVICE_NAME` (defaults `info`/`development`/component-name)
3. `libZap.New(...)` → logger
4. `bootstrap.InitServersWithOptions(&bootstrap.Options{Logger: logger})`
5. `service.Run()`

A new co-located component (tracer, reporter) replicates this skeleton. The bootstrap `config.go` is the composition root — ledger's is **46.7KB** (huge: postgres primary/replica, mongo onboarding+transaction, rabbitmq, redis consumer, multi-tenant, circuit breaker, streaming, readyz, TLS detection, balance-sync worker). crm's is **14KB** and far simpler (mongo + tenant + readyz + TLS). **Reporter's manager+worker split** is the interesting case: it is two services from one repo. Two clean options:
- (a) one `components/reporter` package tree with TWO `cmd/` entrypoints (`cmd/manager/main.go`, `cmd/worker/main.go`) producing two binaries — minimal footprint, shares internal code.
- (b) two components `components/reporter-manager` + `components/reporter-worker` — heavier, duplicates Makefile/Dockerfile/compose, but maps 1:1 onto CI's per-component path filtering.
Recommendation: **(a)** unless CI's path-level filtering (§6) forces (b). The Dockerfile already supports building an arbitrary `main.go` target, so two entrypoints under one component is trivial to build.

---

## 4. Runtime / Deploy Model

- **One binary per component**, statically linked (`CGO_ENABLED=0`, distroless `static-debian12` base, `nonroot` user). Dockerfile multi-stage: `golang:1.26.3-alpine` builder → `COPY go.mod go.sum && go mod download && COPY . . && go build ... components/<name>/cmd/app/main.go`. **Build context is the repo root** (`context: ../../` in each compose file), so the build sees the whole module — this is what makes single-module monorepo Docker builds work.
- **Migrations are embedded + S3-published.** Ledger's Dockerfile copies `migrations/onboarding` and `migrations/transaction` into the image; build.yml separately uploads them to `s3://lerian-migration-files/ledger/{onboarding,transaction}/postgresql`. Raw versioned `NNNNNN_name.{up,down}.sql` files. **CRM has no migrations** (MongoDB-only). Reporter/tracer migrations (if any) need both the Dockerfile copy and a matching S3-upload job in build.yml.
- **Ports (host-mapped):** ledger `:3002`, crm `:4003`. Incoming components need unique ports — check tracer/reporter defaults for collisions in the `30xx`/`40xx` range.
- **Networking:** shared external `infra-network` (declared once in `components/infra/docker-compose.yml`, referenced `external: true` by every component) + a per-component bridge network (`ledger-network`, `crm-network`). Incoming components add `<name>-network` and join `infra-network`. **infra is the shared-services anchor:** PostgreSQL 17 primary+replica (logical WAL, replication slots), MongoDB 8 (replica set rs0), Valkey 8 (Redis), RabbitMQ 4.1, and `grafana/otel-lgtm` (the observability sink — Loki/Grafana/Tempo/Mimir in one container, OTLP gRPC+HTTP receivers).
- **Observability sink already exists in infra.** tracer's job is presumably to feed/own tracing — there is potential conceptual overlap with the existing `midaz-otel-lgtm` service and the `lib-observability` tracing path. Flag for the planner: **does tracer replace, complement, or sit alongside otel-lgtm?** This is a product/architecture decision the host shell does not answer.

---

## 5. Root Makefile + Per-Component Delegation

The build system is a two-layer Makefile: a 617-line **root Makefile** that orchestrates, and per-component Makefiles that do the work. Shared fragments live in `mk/` (`coverage-unit.mk`, `tests.mk`, both `include`d by root and by component Makefiles).

**Component list is declared explicitly at the top of the root Makefile:**
```makefile
INFRA_DIR  := ./components/infra
LEDGER_DIR := ./components/ledger
CRM_DIR    := ./components/crm
COMPONENTS := $(INFRA_DIR) $(CRM_DIR)        # NB: ledger handled separately, not in COMPONENTS
```
**Ledger is treated specially** — it is NOT in `$(COMPONENTS)`; nearly every fan-out target (`build`, `lint`, `up`, `down`, `set-env`, …) appends `$(LEDGER_DIR)` by hand after iterating `$(COMPONENTS)`. This is brittle: adding tracer/reporter means **touching every fan-out target**, not just one variable. Each incoming component requires: a `<NAME>_DIR` var, addition to `$(COMPONENTS)` (or special-casing like ledger), and threading through `build`/`lint`/`format`/`up`/`down`/`start`/`stop`/`restart`/`rebuild-up`/`clean-docker`/`logs`/`set-env`.

**Delegation idiom:** `make ledger COMMAND=<target>` → `cd $(LEDGER_DIR) && $(MAKE) <target>`. Generic `make infra/ledger COMMAND=...` and `make all-components COMMAND=...`. New components want their own delegation target (`make tracer COMMAND=...`, `make reporter COMMAND=...`).

**Standardized per-component target set** (must be implemented by every incoming component's Makefile for the fan-out to work): `build test lint format tidy sec build-docker up start down stop restart rebuild-up clean-docker logs logs-api ps run generate-docs dev-setup help`. crm adds extras (`setup-git-hooks check-hooks check-envs set-env generate-keys validate-api-docs`). Each component Makefile derives `MIDAZ_ROOT ?= $(shell cd ../.. && pwd)`, sets `COVERAGE_PACKAGES := ./components/<name>/...`, and `include`s `$(MIDAZ_ROOT)/mk/coverage-unit.mk`.

**Pinned tool versions** at root: `GOLANGCI_LINT_VERSION := v2.4.0` (root Makefile comment explicitly says "keep in sync with go-combined-analysis.yml" — it is). `make dev-setup` installs gitleaks, gofumpt, goimports, gosec, mockgen@v0.6.0, golangci-lint@v2.4.0.

**`up` ordering is sequential and infra-first:** `infra → ledger → crm`. `down` is reverse. Incoming components slot into this ordering. For **service collapses** (fees, crm → ledger) there is NO new compose service — fees/crm code runs inside the ledger binary, so the crm compose service eventually disappears and crm drops out of the Makefile fan-out entirely.

---

## 6. CI / GitHub Actions (the highest-friction integration surface)

All workflows are thin wrappers that call `LerianStudio/github-actions-shared-workflows/.github/workflows/*.yml@v1.27.5`. The midaz repo only supplies `with:` inputs. **The component list is hardcoded in multiple workflows and MUST be updated per new component:**

| Workflow | Trigger | Hardcoded component refs that need editing for a new component |
|---|---|---|
| `build.yml` | push tags | `filter_paths: components/crm, components/ledger`; `shared_paths: go.mod go.sum pkg/ Makefile`; `helm_values_key_mappings`; `yaml_key_mappings` (gitops); **per-component S3 migration-upload jobs** (currently two: onboarding, transaction) |
| `go-combined-analysis.yml` | PR | `filter_paths: ["components/crm", "components/ledger"]`, `path_level: 2`, `go_version: 1.26.3`, `golangci_lint_version: v2.4.0`, `coverage_threshold: 85` (fail-on) |
| `pr-security-scan.yml` | PR | `filter_paths: components/crm, components/ledger`; `shared_paths` |
| `pr-validation.yml` | PR | `pr_title_scopes: crm ledger api pkg infra migrations scripts deps workflows` — add `tracer`, `reporter`, `fees` scopes |
| `gptchangelog.yml` | after Release | **explicitly `# No filter_paths = single app mode (non-monorepo)`** — changelog is repo-global, NOT per-component |
| `release.yml` | push to develop/rc/main | no per-component config; whole-repo semantic-release |
| `release-notification.yml` | release published | product-level (`Midaz`), no per-component config |

**Key CI facts for the planner:**
- **`path_level: 2`** means CI identifies a component by the path segment at depth 2 (`components/<name>`). A new component is auto-discovered as a unit IFF it sits at `components/<name>` AND is added to `filter_paths`. The reporter manager+worker decision (§3) interacts here: option (b) two components = two `filter_paths` entries = two independent build/test units; option (a) one component with two binaries = one CI unit, and the build job must be taught to produce two images (the shared build workflow's `app_name_prefix` + helm mappings assume one image per component path — verify the shared workflow supports multi-binary, or option (a) may force a workaround, which violates the "no workarounds" end-state).
- **`shared_paths` (go.mod, go.sum, pkg/, Makefile)** are the "rebuild everything" triggers — a change there invalidates all components' path filters. Correct and already in place; nothing to add unless you introduce a new shared dir.
- **Coverage gate is hard:** `coverage_threshold: 85, fail_on_coverage_threshold: true`. Incoming code must hit 85% unit coverage or CI fails. tracer/reporter/fees test suites must be brought up to this bar — likely real work, especially if their current repos run looser thresholds. This is a non-trivial, easily-underestimated cost.
- **Helm / GitOps coupling:** build.yml dispatches to helm chart `midaz` and updates `LerianStudio/midaz-firmino-gitops` with `yaml_key_mappings` per component image tag. New components need helm chart entries and gitops key mappings — **deploy infrastructure outside this repo** must be extended in lockstep. The helm chart and gitops repo are out of scope of this analysis but are hard dependencies for actually shipping a new component.
- **E2E:** build.yml runs APIDog e2e tests after gitops update. New API-bearing components may need APIDog scenario coverage.

---

## 7. Versioning / Changelog / Release Conventions

- **Single repo-wide version, semantic-release.** `.releaserc.yml`: branches `main` (stable), `develop` (prerelease `beta`), `release-candidate` (prerelease `rc`). Conventional-commits drive bumps (`feat`/`refactor`/`perf`/`build` → minor, `fix`/`chore`/`ci`/`test`/`docs` → patch, `BREAKING CHANGE` → major). **No per-component tags** — one version for the whole monorepo. `main.go` swagger annotations carry an app-level version string (`v3.7.0`) decoupled from the semantic-release tag.
- **CHANGELOG.md is one global file** (770KB), AI-generated by `gptchangelog.yml` in **explicit single-app (non-monorepo) mode**. Incoming components' history does NOT get its own changelog stream; everything folds into the one CHANGELOG.
- **Commit-scope governance:** `pr-validation.yml` enforces conventional-commit types and offers (non-required) scopes. Add `tracer`/`reporter`/`fees` to `pr_title_scopes`.
- **Git history:** each incoming repo is its own git repo on `develop`. Consolidation must decide history-preservation strategy (subtree merge vs. flat copy). Not a code constraint, but a one-way decision — flag it.

---

## 8. Conventions Incoming Code MUST Conform To (hard constraints)

From `CLAUDE.md` / `AGENTS.md` / `docs/PROJECT_RULES.md` (1130 lines, **must not be overwritten**) / `.golangci.yml` / `revive.toml`:

- **lib-commons usage is mandatory** (Lerian-wide third rail). tracer on lib-commons v4 violates the *version* baseline, not the mandate, but the migration v4→v5+lib-observability is forced.
- `any` not `interface{}`; `uuid.UUID` not string IDs; context first param; `http.Method*` constants not literals.
- **snake_case.go filenames**, one operation per file; imports ordered stdlib → external → internal.
- **Error sentinels unique & centralized** in `pkg/constant/errors.go`; typed errors from `pkg/errors.go` via `pkg.ValidateBusinessError(...)`. No duplicate `errors.New("code")`.
- **CQRS layering:** HTTP handlers → command/query use cases → repository interfaces → adapters. Dependencies flow inward; no domain logic in handlers or repos. Models in `pkg/mmodel`, never `/internal/domain`.
- **Observability discipline:** structured logs (`libLog.String/Err`, never `fmt.Sprintf` in log calls); span lifecycle rules; `app.request.*` attr namespace for inputs; never log secrets/PII/balances/SQL args.
- **SQL via squirrel** with `PlaceholderFormat(squirrel.Dollar)`; no manual placeholder concatenation. Repo/adapter code covered by integration tests (testcontainers).
- **Cursor/page pagination, max limit 100; no offset pagination** for new endpoints.
- **lib-streaming conventions** (aliases `libStreaming`/`pkgStreaming`, `EmitImportant`, post-commit emission, wire contracts in `pkg/streaming/events/`).
- **License header:** Elastic License 2.0 header on every source file (`scripts/check-license-header.sh` enforces). Both `main.go` files carry the `// Copyright (c) 2026 Lerian Studio` header. plugin-fees is licensed differently (likely) — its source headers must be relicensed to EL2.0 on entry. **plugin-fees license reconciliation is a flag for the planner.**
- **Repo guards:** `make check-logs` (error-logging audit), `make check-tests` (coverage presence), `.golangci.yml` (v2.4.0 ruleset), gosec/trivy security. All run in CI and locally.

---

## 9. The Five Moves Against This Host — Difficulty Map

| Move | Type | Host-shell difficulty | Where the pain is |
|---|---|---|---|
| **1. tracer → component** | co-locate (own binary) | **HIGH** | Non-qualified `module tracer` → full import-path rewrite. lib-commons **v4→v5** + observability split is a library-architecture migration, not a bump. Possible conceptual overlap with infra's otel-lgtm. 85% coverage gate. |
| **2. reporter → component(s)** | co-locate (manager+worker) | **MEDIUM-HIGH** | lib-commons v5.1.3→v5.2.0-beta.12 (minor). manager+worker → one-component-two-binaries vs. two-components decision, which the shared CI build workflow may or may not support cleanly (multi-binary risk). New DB/migrations → Dockerfile copy + S3-upload job. Module rename. |
| **3. plugin-fees → embed in ledger** | service collapse | **HIGH** | Not co-location — code must integrate with the transactions endpoint inside `components/ledger`. Already imports midaz/v3 via `pkg/transaction` (18 sites) — that import surface becomes intra-component. lib-commons v5.1.0→v5.2.0-beta.12. License relicense to EL2.0. Must fold into ledger's CQRS, `pkg/mmodel`, `pkg/constant`. The 46.7KB ledger config.go grows. |
| **4. crm → embed in ledger** | service collapse | **MEDIUM-HIGH** | crm ALREADY lives in-repo as a clean co-located component (the easy half is done). Collapsing it INTO ledger means merging crm's services/adapters into ledger's bootstrap, dropping crm's binary/compose/Makefile/CI entries, and unifying mongo wiring. crm is mongo-only (no migrations) — simpler than fees. Precedent exists: onboarding+transaction already collapsed into ledger. |
| **5. harmonize monorepo** | cleanup | **MEDIUM** | Rewrite stale STRUCTURE.md, fix the brittle Makefile ledger-special-casing (move to a real component loop), update all CI `filter_paths`/scopes/helm/gitops mappings, single CHANGELOG/version reconciliation, decide git-history strategy, relicense fees, dedupe any `pkg`-overlapping types/sentinels. End-state demands NO shims — so every compatibility crutch introduced mid-migration must be removed before done. |

---

## 10. Hard Parts / Risks (concentrated)

1. **Single-module dependency unification is all-or-nothing.** One root go.mod, no replace, no go.work. The moment tracer's lib-commons v4 and midaz's v5 must coexist, there is no escape hatch except actually migrating tracer's code. The "no shims" end-state forbids a `replace` bridge even temporarily-shipped. Sequence the dependency migration FIRST, per incoming repo, before the code physically lands.
2. **lib-observability split (v4 monolith → v5 + lib-observability v1.0.1).** tracer is the worst case; any incoming repo still on lib-commons-internal observability must migrate to the `lib-observability/log|zap|tracing` packages midaz now uses.
3. **85% coverage hard gate** (`fail_on_coverage_threshold: true`). Incoming suites must meet it or CI is red. Easy to underestimate.
4. **CI component list is hardcoded in ~4 workflows + Makefile is hardcoded in ~12 targets.** Every new component is a multi-file edit across `build.yml`, `go-combined-analysis.yml`, `pr-security-scan.yml`, `pr-validation.yml`, and the root Makefile. The ledger special-casing in the Makefile is a refactor opportunity (and a footgun if missed).
5. **Helm chart + gitops repo (external) are hard deploy dependencies.** `midaz-firmino-gitops`, helm chart `midaz`, APIDog e2e — all need per-component extension outside this repo. Co-located component without these = builds but never deploys.
6. **Reporter manager+worker shape vs. shared CI build workflow.** If the shared `build.yml` assumes one image per `components/<name>` path, a one-component-two-binaries layout may not produce two images without a workaround — directly conflicts with the "no workarounds" mandate. Verify shared workflow multi-binary support early; it may force the two-component layout.
7. **Service collapses (fees, crm) are code problems, not packaging problems.** The host shell makes co-location trivial; it does nothing to merge fees into the transactions use cases or crm into ledger's bootstrap. The onboarding+transaction→ledger precedent proves it is done-able in-house, but it is real engineering, not a move.
8. **plugin-fees license divergence** → must relicense source headers to Elastic License 2.0; `scripts/check-license-header.sh` will fail otherwise.
9. **STRUCTURE.md already lies** (shows onboarding/transaction as components). Documentation is drifting from reality; consolidation must include a docs pass or it compounds.
10. **Port collisions** — ledger 3002, crm 4003; verify tracer/reporter defaults don't clash on the shared host network.

---

## Key Files (host shell)

- `go.mod` / `go.sum` — single module, the dependency reconciliation target.
- `Makefile` (root, 617 lines) — component delegation; ledger special-cased outside `$(COMPONENTS)`.
- `mk/coverage-unit.mk`, `mk/tests.mk` — shared Makefile fragments included by components.
- `components/infra/docker-compose.yml` — shared backing services + `infra-network` (external) anchor.
- `components/{ledger,crm}/docker-compose.yml` — per-component service template (root build context, infra-network + own net).
- `components/{ledger,crm}/Dockerfile` — multi-stage, distroless, builds one `cmd/app/main.go`.
- `components/{ledger,crm}/Makefile` — standardized target set every component implements.
- `components/{ledger,crm}/cmd/app/main.go` — identical entrypoint template.
- `components/ledger/internal/bootstrap/config.go` (46.7KB) — composition root, env struct, all wiring.
- `.github/workflows/{build,go-combined-analysis,pr-security-scan,pr-validation,release,gptchangelog,release-notification}.yml` — CI with hardcoded `components/{crm,ledger}` filter paths.
- `.releaserc.yml` — single repo-wide semantic-release config.
- `.github/CODEOWNERS` — `components/*` ownership patterns (auto-cover new components).
- `pkg/mmodel` (40 files, 372 importers), `pkg/constant` (14 files, 259 importers) — shared spine.
- `CLAUDE.md`, `AGENTS.md`, `docs/PROJECT_RULES.md` (do-not-overwrite), `STRUCTURE.md` (stale).
