# 08 — Build / Docker / CI / Release Harmonization

Scope: unify the BUILD, DOCKER, CI, and RELEASE machinery of four repos into one
monorepo rooted at `github.com/LerianStudio/midaz/v3`. Targets the operational
plumbing only — Go module surgery, lib-commons skew, and service-collapse logic
are covered by the sibling dossiers. End state target (Fred): *liso e final — no
shims, no compatibility abstractions.*

Repos in play:

| Repo | Module | Components / binaries | Build pattern today |
|---|---|---|---|
| midaz | `github.com/LerianStudio/midaz/v3` (Go 1.26.3) | `ledger` (3002), `crm` (4003), `infra` | Multi-component, path-filtered shared workflow |
| tracer | `module tracer` (Go 1.26.3) | single binary `tracer` (4020) | Single-image shared workflow |
| reporter | `github.com/LerianStudio/reporter` (Go 1.26) | `manager` (4005), `worker` (health 4006), `infra` | Multi-component, path-filtered shared workflow |
| plugin-fees | `github.com/LerianStudio/plugins-fees/v3` (Go 1.26.3) | single binary `plugin-fees` (4002) | Single-image shared workflow |

The single most important structural fact: **midaz and reporter already use the
exact same CI primitive** — the `LerianStudio/github-actions-shared-workflows`
`build.yml` driven by `filter_paths` + `path_level: '2'` + `app_name_prefix`.
That workflow auto-discovers every component directory under the filtered paths,
diffs them against the pushed tag, and fans out one Docker build+push per changed
component. **Consolidation is not inventing a new CI model — it is extending an
existing one** by adding more entries to `filter_paths` and one Dockerfile per
new component. That reframes the entire effort from "design CI" to "merge config".

---

## 1. The destination model (decide this first)

Moves 1 and 2 (tracer, reporter) are **co-location**: they keep their own
binaries and become `components/tracer`, `components/reporter-manager`,
`components/reporter-worker`. Moves 3 and 4 (plugin-fees, crm) are **service
collapse**: their code folds INTO `components/ledger` and their standalone
binaries/images **disappear**. This split is the spine of every decision below.

Final component topology under `components/`:

```
components/
  infra/                 # the ONE shared infra compose (superset of all 4)
  ledger/                # ledger + embedded fees + embedded crm  -> image: midaz-ledger (3002)
  tracer/                # co-located                              -> image: midaz-tracer (4020)
  reporter-manager/      # co-located                              -> image: midaz-reporter-manager (4005)
  reporter-worker/       # co-located (Chromium image!)            -> image: midaz-reporter-worker (4006)
```

Net image count goes from 6 (ledger, crm, tracer, reporter-manager,
reporter-worker, plugin-fees) to **4** (ledger, tracer, reporter-manager,
reporter-worker). The crm and plugin-fees images are deleted, not renamed —
their `build.yml` `filter_paths` entries, GitOps key mappings, Helm value
mappings, and gitops repo keys must be **removed**, which has downstream blast
radius in the GitOps repos (`midaz-firmino-gitops`) and Helm charts.

> Decision dependency: the GitOps/Helm side lives in repos this dossier did not
> read. Deleting `midaz-crm` and `plugin-fees` image tags from
> `helm_values_key_mappings` / `yaml_key_mappings` will break ArgoCD sync unless
> the chart is updated in lockstep. Flag for the deploy-model dossier.

---

## 2. Makefiles + mk/ fragments

### 2.1 Current shape

**Root Makefile is the orchestrator** in midaz and reporter; tracer and
plugin-fees have a *single-component* root Makefile (no delegation, they ARE the
component). The pattern in midaz:

- Component dirs declared as vars: `INFRA_DIR`, `LEDGER_DIR`, `CRM_DIR`.
- `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` — note `ledger` is handled *explicitly
  and separately* throughout (it predates the loop pattern), `crm` rides the
  loop. This is inconsistent and will get worse as components are added.
- Root targets (`build`, `lint`, `format`, `up`, `down`, `set-env`, etc.) loop
  over `$(COMPONENTS)` and/or hardcode `$(LEDGER_DIR)`, then delegate via
  `cd $$dir && $(MAKE) <target>`.
- `make ledger COMMAND=<cmd>` / `make infra COMMAND=<cmd>` /
  `make all-components COMMAND=<cmd>` is the component-delegation escape hatch.
- Shared logic lives in `mk/`: midaz root has only `coverage-unit.mk` +
  `tests.mk` (testcontainers-heavy, 14.5K).

**mk/ inventory across repos** (this is where the real divergence hides):

| Fragment | midaz root | tracer | reporter | fees |
|---|---|---|---|---|
| `tests.mk` | yes | yes (18.7K) | yes | yes |
| `coverage-unit.mk` | yes | — | — | — |
| `docker.mk` | — | **yes** | — | — |
| `database.mk` | — | **yes** | — | — |
| `docs.mk` | — | **yes** | — | — |
| `quality.mk` | — | **yes** | — | — |
| `security.mk` | — | **yes** | — | — |

Tracer factored its component Makefile into clean `mk/*.mk` fragments
(`docker.mk` has tidy `up/down/start/stop/logs/rebuild-up/run/ps/build-docker`
targets keyed off `DOCKER_CMD` + `SERVICE_NAME`). Midaz's per-component
Makefiles (`ledger/Makefile`, `crm/Makefile`) inline these targets instead.

### 2.2 Unification plan

1. **Adopt one component-Makefile template** keyed off two vars
   (`SERVICE_NAME`, `MIDAZ_ROOT`) and `include`-ing shared `mk/*.mk`. Tracer's
   `mk/docker.mk` is the best existing template — promote it to `mk/docker.mk`
   at the monorepo root and have every component `include $(MIDAZ_ROOT)/mk/docker.mk`.
   This collapses 4 hand-rolled docker target blocks into one.
2. **Normalize the root Makefile component list.** Replace the
   ledger-is-special-cased pattern with a single, complete list:
   `COMPONENTS := ledger tracer reporter-manager reporter-worker` (infra handled
   separately as it is pure infra, no Go build). Every loop target
   (`build`, `lint`, `format`, `up`, `down`, `set-env`, `dev-setup`) iterates
   this list — no more "loop plus explicit `$(LEDGER_DIR)`" duplication.
3. **Fold tracer/reporter/fees Make targets in:**
   - tracer → becomes `components/tracer/Makefile` from the template. Its
     standalone root Makefile (12.2K), `make.sh`, and most `mk/*.mk` get pulled
     up to monorepo root mk/ (database.mk, docs.mk, quality.mk, security.mk are
     genuinely useful and midaz lacks them — **net upgrade for midaz**, midaz
     currently inlines sec-gosec/sec-govulncheck in the root Makefile; replace
     with `mk/security.mk`).
   - reporter manager/worker → `components/reporter-manager/Makefile` and
     `components/reporter-worker/Makefile` from the template.
   - **fees has no Make targets to fold** — it collapses into ledger. Its
     standalone Makefile, mk/tests.mk, make.sh are **deleted**. Any fees-only
     test target worth keeping migrates into `components/ledger`'s test surface.
   - **crm Makefile is deleted** (collapse into ledger). Note: `crm/Makefile`
     has a `generate-keys` target that the root `set-env` calls
     (`$(MAKE) -C $(CRM_DIR) generate-keys`). That crypto-key generation must
     migrate into ledger's setup path or the root `set-env`, or `make set-env`
     breaks. **This is a concrete migration task, not a delete.**
4. **`make ledger COMMAND=` stays; add `make tracer COMMAND=`,
   `make reporter-manager COMMAND=`, `make reporter-worker COMMAND=`.** The
   generic `all-components` loop already exists — extend its component list.
5. **Pin tool versions in ONE place.** Root Makefile pins
   `GOLANGCI_LINT_VERSION := v2.4.0` with a comment "keep in sync with
   go-combined-analysis.yml". But the repos disagree: midaz pins golangci
   `v2.4.0` / go `1.26.3`; fees pins `v2.11.3` / go `1.26.2`; tracer go `1.26.2`.
   **Pick one golangci-lint version and one go version monorepo-wide** and make
   the CI workflow read the same constant. fees uses a much newer linter
   (v2.11.3) with a 6.2K `.golangci.yml` (vs midaz 3.0K) — expect new lint
   findings in fees/crm/tracer code when they all run under the single chosen
   config. **This is real work, not cosmetic** — see §6.

### 2.3 `.golangci.yml` reconciliation

Five different lint configs (midaz 3.0K, tracer 3.3K, reporter 3.0K, fees 6.2K).
One `.golangci.yml` at monorepo root governs everything. The fees config is the
strictest/newest; adopting it monorepo-wide will surface findings across ledger,
crm-now-in-ledger, tracer, reporter code. Adopting midaz's looser config keeps CI
green but loses fees' hardening. **Recommend: adopt fees' config as the floor,
budget a lint-cleanup pass per folded component** (parallelizable, low risk,
mechanical). Per-component overrides via `//nolint` or path-scoped config blocks
only where a real divergence exists (e.g. tracer's generated code).

---

## 3. Dockerfiles + docker-compose

### 3.1 Dockerfile inventory

| Service | Base (builder) | Final image | Notable |
|---|---|---|---|
| ledger | `golang:1.26.3-alpine` | `distroless/static-debian12` | build context `../../`, copies migrations |
| crm | `golang:1.26.3-alpine` | `distroless/static-debian12` | build context `../../`; **deleted on collapse** |
| tracer (prod) | `golang:1.26.3-alpine3.22` | `distroless/static-debian12:nonroot` | `GOMEMLIMIT=1800MiB`, copies migrations, context `.` |
| tracer (dev) | `golang:1.26.3-alpine` | `alpine:3.23` | shell+wget healthcheck on `/readyz` |
| reporter-manager | `golang:1.26-alpine` | **`alpine:3.23`** | OCI labels, addgroup/adduser, wget `/health` |
| reporter-worker | `golang:1.26-alpine` | **`alpine:3.23` + Chromium** | chromedp PDF gen, fonts, ~heavy image |
| plugin-fees | `golang:1.26-alpine` | `distroless/static-debian12` | **BuildKit secret `github_token`**, embedded healthcheck binary; **deleted on collapse** |

### 3.2 Build-context divergence — the load-bearing fix

- midaz components build with **context `../../` (repo root)** and
  `dockerfile: ./components/<x>/Dockerfile`. The Dockerfile then does
  `COPY go.mod go.sum ./` + `COPY . .` and builds
  `components/<x>/cmd/app/main.go`. This is the monorepo-correct pattern.
- tracer and fees build with **context `.`** (they are their own repo root).
- reporter components build with **context `../../`** (already monorepo-correct).

On move-in, tracer's and fees' Dockerfiles must switch to repo-root context and
component-relative `COPY`/build paths. For tracer this is a path rewrite
(`./cmd/app` → `components/tracer/cmd/app`, migrations copy path). For fees the
Dockerfile is **deleted** — its build target becomes part of the ledger binary.

### 3.3 The `github_token` BuildKit secret divergence (sharp edge)

- **midaz and reporter Dockerfiles do NOT use a GitHub token.** Their Lerian
  module deps (lib-auth, lib-commons, lib-observability, lib-streaming, etc.)
  resolve through the public GOPROXY. Confirmed: no `github_token` /
  `go_private_modules` anywhere in midaz `.github/`.
- **tracer and plugin-fees DO** — fees Dockerfile mounts
  `--mount=type=secret,id=github_token` and writes `~/.netrc`; both set
  `go_private_modules: "github.com/LerianStudio/*"` in go-pr-analysis. fees'
  compose wires a `.secrets/github_token.txt` file secret.
- After consolidation, **fees' private-module need vanishes** (it imported
  midaz/v3, now it IS midaz). tracer's deps are the same public Lerian libs
  midaz already resolves without a token. **Conclusion: the monorepo needs NO
  github_token machinery.** Delete the BuildKit secret, the `.secrets/`
  directory, the `go_private_modules` workflow inputs, and the `~/.netrc` dance.
  This is squarely on the "no shims, liso e final" target — it is legacy auth
  plumbing that the merge makes obsolete. (Verify no remaining transitive
  private module before deleting; midaz builds today prove the common libs are
  public, so risk is low.)

### 3.4 Reporter-worker is the odd image out

Worker's final image is `alpine:3.23` **with Chromium + a font stack** for
chromedp PDF rendering (~hundreds of MB vs distroless ~tens of MB). It cannot be
distroless. This is fine — it stays a fat alpine image. But the unified Docker
strategy can no longer assume "all components are distroless static". The
Dockerfile-per-component reality is: 3 distroless-ish (ledger, tracer-prod,
manager could go distroless) + 1 mandatory-fat (worker). Keep per-component
Dockerfiles; do not try to force a single shared base image.

> Side note: midaz/tracer use `distroless`, reporter uses `alpine` with a
> hand-rolled non-root user. Harmonize the *non-Chromium* images to distroless
> nonroot for consistency and Docker Hub health score (tracer already cites this
> motivation). Worker stays alpine. Low priority, do it during the move.

### 3.5 Unified docker-compose topology

Today's topology is **one compose file per component**, each joining a shared
`infra-network` declared `external: true`, plus a private per-component network.
The infra compose (`components/infra/docker-compose.yml`) *owns* `infra-network`
and the shared backing services. Reporter's compose files **already declare
`infra-network` external with the comment "Shared network from broader Lerian
platform (created by midaz infra)"** — reporter was written anticipating this
merge. That is a gift; it means the network model is already designed for it.

**Backing-service consolidation is the hard part of compose unification:**

| Service | midaz infra | reporter infra | fees | tracer |
|---|---|---|---|---|
| PostgreSQL | primary+replica `postgres:17` (logical repl) | — | — | `postgres:16-alpine` |
| MongoDB | `mongo:8` (replSet rs0) + init | `mongo:8` | `mongo:8` | — |
| Valkey/Redis | `valkey:8` | `valkey:8.0-alpine` | — | — |
| RabbitMQ | `rabbitmq:4.1.3-mgmt-alpine` | `rabbitmq:4.0-mgmt-alpine` | — | — |
| otel-lgtm | `grafana/otel-lgtm` | — | — | — |
| SeaweedFS (S3) | — | **`seaweedfs:4.05`** | — | — |
| KEDA | — | **`keda:2.16.0`** | — | — |

Plan:
1. **midaz `components/infra` becomes the single source of truth** for Postgres,
   Mongo, Valkey, RabbitMQ, otel-lgtm — it already runs the most production-like
   versions (primary+replica Postgres, Mongo replica set).
2. **Reporter brings two genuinely new infra services** the monorepo lacks:
   **SeaweedFS** (S3-compatible object store for generated reports) and **KEDA**
   (autoscaler for the worker queue). These must be **added to the unified infra
   compose** (or kept in a reporter-specific infra overlay). They are not
   duplicates — they are net-new dependencies.
3. **Tracer's standalone `tracer-postgres` (postgres:16-alpine) is dropped.**
   tracer must use the shared `postgres:17` primary. Verify schema/migration
   compatibility (16→17 is fine for migrations; tracer's `migrations/` dir runs
   against the shared DB under its own database/schema). This is a
   databases-dossier concern but it lands in the compose: delete
   `tracer-postgres` service, point tracer at `midaz-postgres-primary`.
4. **fees' `plugin-fees-mongodb` is dropped** — on collapse, fees' MongoDB
   metadata usage rides ledger's existing Mongo. (Databases dossier owns whether
   fees gets its own database within the shared Mongo or merges collections.)
5. **Version skew to reconcile:** RabbitMQ 4.1.3 (midaz) vs 4.0 (reporter);
   Valkey 8 vs 8.0-alpine. Pick the midaz versions (newer). One-line edits.
6. **Network model:** keep `infra-network` as the single external bridge owned
   by infra. Per-component private networks (`ledger-network`, `tracer-network`,
   `reporter-manager-network`, etc.) can stay or be dropped — they add little in
   a single-host dev compose. **Recommend dropping the per-component private
   networks** in the unified dev topology; everything on `infra-network` is
   simpler and matches "liso". Keep them only if a component genuinely needs
   isolation (none observed).
7. **Port map — verified, no collisions:** ledger 3002, fees 4002 (vanishes),
   crm 4003 (vanishes), reporter-manager 4005, reporter-worker health 4006,
   tracer 4020. After collapse: 3002 (ledger), 4005 (manager), 4006 (worker),
   4020 (tracer). Clean.

---

## 4. GitHub Actions / CI consolidation

### 4.1 Current per-repo workflow set (near-identical, all delegate to shared workflows)

| Workflow | midaz | tracer | reporter | fees | Trigger |
|---|---|---|---|---|---|
| build.yml | ✓ | ✓ | ✓ | ✓ | `push: tags '**'` |
| pr-validation.yml | ✓ | ✓ | ✓ | ✓ | PR to develop/rc/main |
| go-combined-analysis.yml | ✓ | ✓ | ✓ | ✓ | PR (paths-ignore docs) |
| pr-security-scan.yml | ✓ | ✓ | ✓ | ✓ | PR (paths-ignore docs) |
| release.yml | ✓ | ✓ | ✓ | ✓ | `push: branches develop/rc/main` |
| gptchangelog.yml | ✓ | ✓ | — | ✓ | after Release (main) |
| release-notification.yml | ✓ | — | — | — | post-release |
| dependabot-auto-merge.yml | — | ✓ | — | — | dependabot PRs |

Every job is a thin `uses: LerianStudio/github-actions-shared-workflows/...@vX`
caller. **The CI logic lives in the shared repo, not these files.** Consolidation
is therefore mostly about (a) merging the `with:` inputs and (b) deleting
duplicates.

### 4.2 Shared-workflow version skew (must pin one)

| Workflow ref | midaz | tracer | reporter | fees |
|---|---|---|---|---|
| build.yml | v1.27.5 | v1.30.0 | v1.30.0 | v1.27.4 |
| go-pr-analysis | v1.27.5 | v1.32.0 | v1.30.0 | v1.28.7 |
| pr-security-scan | v1.27.5 | v1.32.0 | v1.28.9 | v1.28.7 |
| pr-validation | v1.27.5 | v1.32.0 | v1.30.0 | v1.28.7 |
| release.yml | v1.27.5 | v1.30.0 | v1.30.0 | v1.28.7 |
| gitops-update | v1.27.5 | **v1.32.0** | v1.30.0 | v1.27.4 |

Six versions in flight. **Pin one shared-workflow version monorepo-wide**
(recommend the newest validated, tracer's `v1.32.0`, or whatever is current at
merge time). One find/replace across the consolidated `.github/workflows/`.
Mind that newer shared-workflow majors may change input names — read the shared
repo's changelog v1.27→v1.32 before pinning.

### 4.3 The consolidated `build.yml` — extend filter_paths, do NOT rewrite

The midaz/reporter `build.yml` already does per-component path-filtered builds.
The consolidated version is **the union of the filter_paths and the mapping
configs**:

```yaml
jobs:
  build:
    uses: LerianStudio/github-actions-shared-workflows/.github/workflows/build.yml@v1.32.0
    with:
      runner_type: "blacksmith-4vcpu-ubuntu-2404"
      filter_paths: |-
        components/ledger
        components/tracer
        components/reporter-manager
        components/reporter-worker
      shared_paths: |
        go.mod
        go.sum
        pkg/
        Makefile
      path_level: '2'
      app_name_prefix: "midaz"
      enable_dockerhub: true
      enable_ghcr: true
      dockerhub_org: lerianstudio
      enable_gitops_artifacts: true
      enable_helm_dispatch: true
      helm_chart: "midaz"
      helm_detect_env_changes: true
      helm_values_key_mappings: >-
        {"midaz-ledger":"ledger","midaz-tracer":"tracer",
         "midaz-reporter-manager":"manager","midaz-reporter-worker":"worker"}
    secrets: inherit
```

Notes:
- `components/crm` and the fees image **drop out of filter_paths** (collapsed).
- `app_name_prefix: "midaz"` means images become `midaz-tracer`,
  `midaz-reporter-manager`, etc. — **this renames reporter/tracer/fees images**.
  Downstream Helm/GitOps must follow. Alternative: keep historical prefixes per
  component, but the shared workflow takes ONE `app_name_prefix`, so a single
  prefix is the path of least resistance. **Recommend `midaz-` prefix
  everywhere; accept the image rename as part of the consolidation.**
- The shared workflow auto-detects components from filter_paths at `path_level: 2`
  (i.e. the second path segment under repo root). `components/reporter-manager`
  and `components/reporter-worker` are distinct level-2 dirs → two images. Good.
- `enable_dockerhub`: midaz+fees publish to DockerHub+ghcr; **reporter is
  ghcr-only**, tracer is both. Harmonize to **both registries** for all
  components (most permissive existing policy) unless there is a licensing reason
  reporter stays ghcr-only — flag to confirm.
- Migration S3 upload jobs (`upload-onboarding-migrations`,
  `upload-transaction-migrations`) stay as-is for ledger. **tracer also ships
  migrations** — if tracer's migrations need S3 upload (check the deploy model),
  add a third S3-upload job. Reporter/worker have no SQL migrations.

### 4.4 Consolidated `go-combined-analysis.yml` and `pr-security-scan.yml`

Same merge: union the `filter_paths`, drop crm/fees-standalone entries, pick ONE
`go_version` and `golangci_lint_version`. Crucially, **drop
`go_private_modules: "github.com/LerianStudio/*"`** (tracer/reporter/fees set it;
midaz proves the common libs are public — see §3.3). go-pr-analysis already
supports the multi-component path-filter, so per-component coverage thresholds
(85%) apply per directory automatically.

### 4.5 Path-filtered vs matrix — recommendation

The shared `build.yml`'s `filter_paths` mechanism IS a path-filtered matrix
(component dirs are the matrix axis; changed-detection is the filter). **Keep it.
Do not hand-roll a `strategy.matrix`.** It already gives: per-component
changed-detection (only build what the tag touched), per-component images,
`shared_paths` to force-rebuild everything when `go.mod`/`pkg/`/`Makefile`
change. A hand-rolled matrix would re-implement this worse. The only reason to
go custom is if the shared workflow cannot express the worker's Chromium build —
it can, because the per-component Dockerfile owns that complexity.

### 4.6 Workflows to delete / merge

- **Delete 3 of each duplicated workflow** (keep midaz's, merge inputs).
- `release-notification.yml` (midaz-only) — keep, it is repo-wide.
- `dependabot-auto-merge.yml` (tracer-only) — keep, applies repo-wide. Reconcile
  dependabot config: only tracer has auto-merge; decide if the monorepo wants it.
- **Dependabot config itself**: each repo has (or lacks) `.github/dependabot.yml`.
  In a single-go.mod monorepo there is ONE go module to watch → one dependabot
  ecosystem entry + one docker ecosystem entry per Dockerfile dir. Consolidate to
  a single `.github/dependabot.yml`.

---

## 5. Release / versioning strategy

### 5.1 Current state — four independent semantic-release flows

All four `.releaserc.yml` are **near-identical**:
- Same `conventionalcommits` preset, same `releaseRules` (feat/perf/build/refactor
  → minor, fix/chore/ci/test/docs → patch, breaking → major).
- Same branch model: `main` (stable), `develop` (prerelease `beta`),
  `release-candidate` (prerelease `rc`).
- Divergences:
  - **midaz**: `@semantic-release/git` commits `CHANGELOG.md`; no backmerge.
  - **tracer**: NO `@semantic-release/git` (changelog handled out-of-band);
    has `.releaserc.hotfix.yml` for hotfix flow.
  - **reporter / fees**: `@saithodev/semantic-release-backmerge` (main→develop);
    `.releaserc.hotfix.yml` present.
- `release.yml` triggers on `push: branches [develop, release-candidate, main]`
  with `paths-ignore` for docs/env/txt/.github. It produces **one repo-wide
  semantic version tag**, which then triggers `build.yml` (on `push: tags '**'`),
  which fans out per-component images.

**The release ALREADY produces a single repo-wide tag, and the build ALREADY
fans out to per-component images.** This is exactly the monorepo model. Midaz did
not need per-component tags even with 2 components.

### 5.2 The three options

1. **Single repo-wide version (one tag, fan-out build)** — what all four repos
   already do. Tag `v3.6.0` → build.yml builds every *changed* component as
   `midaz-<component>:3.6.0`. Conventional-commit scopes (`feat(ledger):`,
   `fix(tracer):`) drive the bump but the version is monorepo-global.
2. **Per-component tags** (`ledger-v3.6.0`, `tracer-v1.2.0`) — requires a
   release tool that segments by path (e.g. release-please manifest mode, or
   multiple semantic-release configs with `tagFormat` per component). Heavier;
   semantic-release does not do this cleanly without one config per package and
   path-scoped commit analysis.
3. **Path-based release-please** — manifest-driven, per-component versions,
   per-component changelogs, single PR. Powerful but a **different tool** than
   the org-standard semantic-release shared workflow; would diverge from every
   other Lerian repo and the shared CI.

### 5.3 Recommendation: **single repo-wide version (option 1)**

Rationale:
- It is what all four repos already do via the shared workflow — **zero new
  tooling, stays aligned with org CI**. "Liso e final" favors not inventing a
  release model.
- The build fan-out already skips unchanged components (`filter_paths` +
  changed-detection), so a global version does NOT mean "rebuild everything every
  release" — only touched components get new images, all stamped with the same
  semver. That is the desirable property of a monorepo: one coherent version of
  the whole platform.
- Per-component versioning (option 2/3) buys independent release cadence, which
  the monorepo consolidation is explicitly trying to *eliminate* (the whole point
  of collapsing fees+crm into ledger is one deployable). Independent versions
  reintroduce the coordination cost the merge removes.
- Cost accepted: tracer/reporter version history resets to midaz's `v3.x` line.
  Their old tags stay in git history but the going-forward version is midaz's.
  Document the discontinuity in CHANGELOG.

Concrete `.releaserc.yml` for the monorepo: start from **midaz's** (it commits
CHANGELOG via `@semantic-release/git`), add the `@saithodev/semantic-release-backmerge`
main→develop plugin (reporter/fees have it, midaz lacks it — backmerge is good
hygiene and the recent git log shows midaz already does manual
`chore(changelog): backmerge` commits, so automate it). Keep the
`.releaserc.hotfix.yml` (tracer/reporter/fees have one; midaz does not — adopt it
for hotfix releases off `main`).

### 5.4 Changelog strategy

- midaz CHANGELOG.md is **770KB** (huge history); tracer 3.8K, reporter 4.8K,
  fees 130KB. **Do not concatenate.** Keep midaz's CHANGELOG.md as the canonical
  monorepo changelog going forward. Archive the other three under
  `docs/monorepo/legacy-changelogs/` for provenance, do not merge into the live
  one.
- **gptchangelog**: midaz/tracer/fees run it post-release; midaz's runs in
  *single-app mode* (comment: "No filter_paths = single app mode (non-monorepo)").
  For a single repo-wide version, **single-app mode is correct** — one changelog
  for the whole release. Keep midaz's gptchangelog.yml, delete the others.
  (If per-component changelogs were ever wanted, gptchangelog supports filter_paths;
  not needed under option 1.)
- PR-title scopes (`pr-validation.yml` `pr_title_scopes`): currently
  `crm, ledger, api, pkg, infra, migrations, scripts, deps, workflows`. **Add
  `tracer, reporter, fees`** (fees as a scope under ledger is reasonable since
  fees code lives in ledger now), **remove `crm`** (or keep as a sub-scope of
  ledger if crm code is identifiable). This is the human-facing taxonomy of the
  monorepo — get it right so commit scopes map to components.

---

## 6. Local dev (set-env / up / down)

### 6.1 Current orchestration

- **midaz root `up`**: `cd infra && make up` → `cd ledger && make up` →
  `cd crm && make up`. No explicit infra-health gate between infra and backends
  (relies on each component compose's own `depends_on`/healthchecks, but infra is
  a *separate compose project* so cross-project depends_on does not work — known
  gap, see reporter's comment about it).
- **reporter root `up`**: `cd infra && make up` → **`make wait-for-infra`** →
  loop backends. Reporter solved the cross-compose health-ordering problem with
  an explicit `wait-for-infra` target. **Midaz lacks this and should adopt it.**
- **set-env**: copies `.env.example`→`.env` per component; midaz additionally
  runs `make -C crm generate-keys` (crypto keys for CRM). On collapse this
  key-gen must move (see §2.2.3).

### 6.2 Unification plan

1. **Single root `make up`** sequences: `infra up` → `wait-for-infra` (adopt
   reporter's target) → loop `[ledger, tracer, reporter-manager, reporter-worker]`
   `make up`. Worker depends on RabbitMQ+SeaweedFS being healthy → `wait-for-infra`
   must cover the new SeaweedFS/KEDA services too.
2. **Single `make down`** reverses: backends down → infra down. Already the
   pattern in both midaz and reporter; just extend the component list.
3. **`make set-env`** copies `.env.example` for the unified component set. The
   crm `generate-keys` step migrates into ledger's env setup (crm collapses into
   ledger, so ledger's `.env` now needs crm's crypto keys). fees' `.env.example`
   variables fold into ledger's `.env.example`. **This is a real env-file merge,
   not just a copy-loop edit** — fees and crm env vars must land in ledger's
   `.env.example` without key collisions (ledger `.env.example` is already 12.9K).
4. **`.env` consolidation is the sharp dev-ergonomics edge.** Today each
   component has its own `.env`. After collapse, ledger's `.env` absorbs fees +
   crm config. tracer and reporter keep their own component `.env` (co-located,
   not collapsed). Infra `.env` is shared. Watch for **port/credential variable
   name collisions** when merging fees+crm env into ledger (e.g. both define
   `SERVER_PORT`, `MONGO_*`, `DB_*` — ledger's win, fees/crm-specific knobs get
   prefixed or namespaced). This belongs to the config dossier but it surfaces in
   `make set-env`.
5. **`.dockerignore` / `.gitignore` merge**: each repo has its own. Union them at
   root; reconcile `.secrets/` (fees) — delete since github_token goes away.
6. **`.githooks/`**: all four have `.githooks/`. Consolidate to one set at root;
   `make setup-git-hooks` already points at `.githooks`. Reconcile any
   component-specific hook logic (gitleaks scope, gofumpt paths).

---

## 7. Effort, sequencing, and the hard parts

### 7.1 Hard parts (ranked by pain)

1. **lib-commons skew gates everything CI-related.** Tracer is on lib-commons
   **v4.6.3 (a full major behind)** and its module is `module tracer`
   (non-qualified, unimportable). Until tracer compiles against the monorepo's
   lib-commons v5.x under a real module path, no unified `go build`, no unified
   `golangci-lint`, no unified Docker build works. **The build/CI consolidation
   cannot complete before the module/lib-commons migration (other dossiers)
   lands.** CI work here is downstream of that.
2. **Env-file + key-generation merge for the collapses** (fees+crm into ledger).
   The crm `generate-keys` make target and the fees/crm `.env` surfaces must fold
   into ledger cleanly. Mechanical but error-prone (silent missing env var → 
   runtime failure).
3. **Single `.golangci.yml` + one go/lint version** will surface new lint and
   build findings across the folded code (fees has the strictest config; tracer
   on old Go patterns). Budget a cleanup pass per component.
4. **GitOps/Helm downstream blast radius** (repos not read here): renaming images
   to `midaz-*` prefix and deleting crm/fees image mappings will break ArgoCD
   sync unless charts update in lockstep. Cross-team coordination.
5. **Infra superset assembly**: adding SeaweedFS + KEDA, dropping
   tracer-postgres/fees-mongo, reconciling RabbitMQ/Valkey versions, extending
   `wait-for-infra`. Fiddly compose work, low intellectual difficulty.

### 7.2 Effort sizing

- **Makefile + mk/ consolidation**: ~1.5 days. Mostly template promotion +
  loop-list normalization. Low risk.
- **Dockerfile move + context rewrites + delete crm/fees Dockerfiles + drop
  github_token**: ~1 day. Low risk once module paths resolve.
- **docker-compose unification (infra superset + network model + port map)**:
  ~1.5 days. Adding SeaweedFS/KEDA + env merges is the time sink.
- **CI workflow consolidation (merge inputs, pin versions, drop dupes,
  reconcile registries/private-modules)**: ~1 day editing + ~1 day validating a
  real tag-driven build fan-out end to end.
- **Release/versioning (one .releaserc, hotfix config, changelog archive,
  pr-scope taxonomy)**: ~0.5 day.
- **Local dev (set-env merge, wait-for-infra, env collisions)**: ~1 day,
  dominated by the fees+crm→ledger env merge.

**Total build/CI/docker/release surface: ~7-8 engineer-days**, but **gated** —
it cannot finish until the Go module unification and lib-commons v5 alignment
(other dossiers) are done. Practically: CI consolidation is the *last* step of
the monorepo merge, not the first. Do it after the code compiles as one module.

### 7.3 Suggested sequencing within this surface

1. (Blocked on module/lib-commons dossiers) — single go.mod compiles all code.
2. One `.golangci.yml` + one go/lint version pinned; cleanup pass.
3. Makefile/mk template consolidation + component-list normalization.
4. Dockerfiles moved/rewritten/deleted; github_token machinery removed.
5. Infra compose superset + per-component composes on `infra-network`;
   `wait-for-infra`; env merges; `make up/down/set-env` extended.
6. CI workflows merged (build/go-analysis/security/pr-validation/release),
   shared-workflow version pinned, dupes deleted.
7. One `.releaserc.yml` (+hotfix), changelog archive, gptchangelog single-app.
8. Validate a real tag → fan-out build → 4 images pushed → GitOps/Helm updated.

---

## 8. Concrete conflicts (block clean merge until resolved)

- **`go_private_modules` / `github_token`**: tracer+fees require it, midaz+reporter
  do not. Monorepo should NOT need it — but verify no transitive private module
  before deleting the BuildKit secret + `.secrets/`.
- **Registry policy**: reporter is ghcr-only; others DockerHub+ghcr. One policy
  required (recommend both).
- **Shared-workflow version**: six versions across repos. Pin one.
- **go/golangci versions**: go 1.26.2 vs 1.26.3; golangci v2.4.0 vs v2.11.3. Pin one.
- **`app_name_prefix`**: shared workflow takes ONE → images rename to `midaz-*`.
  Breaks downstream Helm/GitOps mappings unless updated together.
- **CHANGELOG.md size**: 770KB midaz vs 130KB fees — do not concatenate; archive.
- **crm `generate-keys`**: root `set-env` calls it; must migrate on collapse or
  `make set-env` breaks.
- **Image base divergence**: distroless (midaz/tracer) vs alpine (reporter);
  worker MUST stay fat alpine (Chromium). No single base image possible.
- **Backing-service version skew**: RabbitMQ 4.1.3/4.0, Valkey 8/8.0-alpine,
  Postgres 17/16. Pick newest (midaz).
