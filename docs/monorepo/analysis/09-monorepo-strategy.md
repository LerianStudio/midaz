# 09 — Monorepo Strategy: Module Topology, History Import, and the Pivotal Decision

**Scope:** The single pivotal decision that gates the entire consolidation — *how* the four repos
(`tracer`, `reporter`, `plugin-fees`, `midaz`) become one tree, and what module topology they live
under. This dossier evaluates Option A (single module), Option B (`go.work` multi-module), Option C
(hybrid), and the git-history import method, with evidence pulled from the actual repos on `develop`.

Target end-state, in Fred's words: **"liso e final — sem shims, workarounds, abstrações de
compatibilidade."** Every recommendation below is scored against that intent, not against ease of entry.

---

## 0. The five moves (recap, with what each actually means structurally)

| # | Move | Nature | Service outcome |
|---|------|--------|-----------------|
| 1 | `tracer` → midaz component | **Co-locate** | Keeps own binary/service (port 4020) |
| 2 | `reporter` → midaz component(s) | **Co-locate** | Keeps manager (4005) + worker services |
| 3 | `plugin-fees` → into `components/ledger` | **Service collapse** | Fees endpoint folded into ledger; fees binary dies |
| 4 | `components/crm` → into `components/ledger` | **Service collapse** | CRM binary dies, folds into ledger |
| 5 | Harmonize into one monorepo | The decision this doc resolves | — |

Moves 1–2 are *physical relocation*. Moves 3–4 are *logical absorption* (the embedded code stops
being a separate process). The module-topology decision (A/B/C) is largely orthogonal to 3–4 — a
service collapse can happen under any module layout — but it is **decisive** for 1–2, because that is
where the dependency skew bites.

---

## 1. Ground truth: current state of each repo

### 1.1 Module identities and Go versions

| Repo | `module` path | `go` | toolchain | git commits | `.go` files |
|------|---------------|------|-----------|-------------|-------------|
| tracer | `module tracer` ⚠️ **non-qualified, unimportable** | 1.26.3 | — | 1039 | 603 |
| reporter | `github.com/LerianStudio/reporter` | 1.26 | go1.26.2 | 1605 | 839 |
| plugin-fees | `github.com/LerianStudio/plugins-fees/v3` | 1.26.3 | — | 1050 | 221 |
| midaz | `github.com/LerianStudio/midaz/v3` | 1.26.3 | — | 6942 | 902 (crm: 70) |

- **tracer's module path is `tracer`** — a bare, non-domain-qualified name. Every internal import is
  `"tracer/internal/..."`, `"tracer/pkg"` (38 distinct internal import prefixes). This is *broken for
  external import* and must be rewritten to `github.com/LerianStudio/midaz/v3/components/tracer/...`
  regardless of which option is chosen. This is unavoidable rename work, not optional.
- reporter pins `go 1.26` + `toolchain go1.26.2`; everyone else is `1.26.3`. Trivially harmonized
  (bump to 1.26.3, drop the toolchain line) — but it is a line item.

### 1.2 midaz is ALREADY a single-module monorepo

Decisive structural fact: **midaz has exactly one `go.mod` at the root.** `components/ledger` and
`components/crm` have **no own `go.mod`** — they are packages under the root module, each with its own
`cmd/app/main.go`, `Dockerfile`, `docker-compose.yml`, `Makefile`, and `.env`. There is **no
`go.work`** in midaz today.

So the target is not "introduce a monorepo." midaz *is* the monorepo. The question is whether the
three incoming repos join the **single root module** or get fenced behind a workspace.

CI proves the single-module model (`.github/workflows/build.yml`):
- `filter_paths: components/crm, components/ledger` with `shared_paths: go.mod, go.sum, pkg/, Makefile`.
- One root `go.mod`/`go.sum` is the shared build input; components are path-filtered build *targets*,
  not independent modules.
- Per-component Docker images (`midaz-ledger`, `midaz-crm`) built from the **root** context — each
  Dockerfile does `COPY go.mod go.sum` from root then `go build components/<x>/cmd/app/main.go`.
- Helm dispatch maps `midaz-crm`→`crm`, `midaz-ledger`→`ledger`. GitOps updates per-component image tags.

**Implication:** Option A is the *native shape midaz already runs in*. Options B/C introduce a module
topology midaz has never had, against a CI/build/Helm pipeline built around one go.mod.

### 1.3 Per-component build/deploy artifacts already exist in midaz

`components/ledger/`: `Dockerfile` (712B), `docker-compose.yml` (478B), `Makefile` (10.8K), `.env`.
`components/crm/`: `Dockerfile` (509B), `docker-compose.yml` (460B), `Makefile` (15.8K), `.env`.

The root `Makefile` orchestrates: `COMPONENTS := infra crm`, `make up` brings up ledger + crm + infra
compose stacks, `make ledger COMMAND=<x>` delegates into the component Makefile. tracer and reporter
each already ship the same artifact set (own Dockerfile, compose, Makefile, mk/ includes) — so they
**slot into the existing per-component pattern cleanly**. This is the part that is genuinely easy.

---

## 2. The dependency-skew reality (the thing that decides A vs B)

### 2.1 lib-commons — the load-bearing skew

| Repo | lib-commons (direct) | Note |
|------|----------------------|------|
| tracer | **v4.6.3** | also pulls v5.3.0 **indirect** — already straddling v4/v5 |
| reporter | v5.1.3 | |
| plugin-fees | v5.1.0 | |
| midaz | **v5.2.0-beta.12** | the target floor |

tracer is the only repo on lib-commons **v4** (a full major behind). CLAUDE.md makes lib-commons usage
a **hard constraint / third rail**. Under a single module (Option A), there is exactly **one**
lib-commons version in the build graph — so **tracer MUST migrate v4 → v5** before or during the merge.
That migration is the single largest hard-cost item in the whole consolidation. Notably, tracer's
go.mod **already lists v5.3.0 as indirect**, meaning some transitive dep already drags v5 in; tracer is
effectively running a split-brain lib-commons today. Option A forces the cleanup that is arguably
overdue anyway.

### 2.2 lib-observability — three different versions, one already a beta

| Repo | lib-observability |
|------|-------------------|
| tracer | v1.0.0 (indirect) |
| reporter | **(none)** — uses raw `go.uber.org/zap v1.27.1` + OTel directly |
| plugin-fees | **v1.1.0-beta.5** |
| midaz | v1.0.1 |

reporter never adopted lib-observability — it logs via zap directly. midaz's whole recent direction is
the observability migration (latest merge on develop: `refactor/observability-migration`, #2124). Under
Option A, reporter's zap-based logging becomes an island inside a tree that standardized on
lib-observability — a "sem shims" purist would want reporter migrated to lib-observability too, which
is *additional* scope beyond mere relocation. plugin-fees on a `-beta.5` while midaz is on `v1.0.1`
release is a backward pin that must be reconciled.

### 2.3 lib-auth, lib-license, lib-streaming

- **lib-auth/v2:** tracer v2.8.0, midaz v2.8.0, reporter v2.7.0, fees v2.7.0. Minor skew; MVS resolves
  to v2.8.0 cleanly under one module. Low risk.
- **lib-license-go/v2:** **only plugin-fees has it (v2.3.4).** This is the proprietary-licensing
  client. midaz, tracer, reporter do not gate on a license server. Folding fees into ledger drags the
  license-go dependency and its bootstrap gate **into the ledger binary** — a behavioral change worth a
  conscious decision (does ledger now phone home to a license server? see §6).
- **lib-streaming v1.4.0:** midaz only. Incoming repos don't emit streaming events. No conflict, but
  the embedded fees/crm code will now live next to streaming infra it doesn't use.

### 2.4 Third-party shared deps — do they actually conflict?

| Dep | tracer | reporter | fees | midaz | MVS-resolvable? |
|-----|--------|----------|------|-------|------------------|
| `go.opentelemetry.io/otel` | 1.44.0 | 1.43.0 | 1.43.0 | 1.44.0 | ✅ → 1.44.0 |
| `testcontainers-go` | 0.42.0 | 0.41.0 | 0.41.0 | 0.42.0 | ✅ → 0.42.0 (test-only) |
| `gofiber/fiber/v2` | 2.52.13 | 2.52.13 | 2.52.13 | 2.52.13 | ✅ identical |
| `jackc/pgx/v5` | 5.9.2 | 5.9.2 | 5.9.2 | 5.9.2 | ✅ identical |
| `go-playground/validator/v10` | 10.30.3 | 10.30.1 | 10.30.1 | 10.30.2 | ✅ → 10.30.3 |
| `shopspring/decimal` | 1.4.0 | ×4 | — | — | ✅ identical |
| `redis/go-redis/v9` | 9.19.0 | 9.18.0 | 9.18.0 | 9.20.0 | ✅ → 9.20.0 |
| `mongo-driver` | 1.17.9 | 1.17.9 | 1.17.9 | 1.17.9 | ✅ identical |

**Key insight:** the *third-party* surface is almost entirely **patch/minor skew that Go's MVS resolves
upward without code changes.** Fiber, pgx, mongo-driver, decimal are pinned identically. otel,
testcontainers, validator, redis differ only by minor/patch and pick the highest. None of these is a
*major*-version conflict, which is the only kind that genuinely hard-blocks a single module.

So the "a dep two components disagree on hard-blocks until resolved" risk for Option A is **largely
theoretical here** — the *only* true cross-major conflict in the entire graph is **lib-commons v4
(tracer) vs v5 (everyone)**, and that is a Lerian-internal lib the team controls and must unify anyway.

### 2.5 reporter's heavy unique deps (size, not conflict)

reporter drags deps **no one else has**: `chromedp` + `cdproto` (headless Chrome for PDF rendering),
`microsoft/go-mssqldb`, `sijms/go-ora` (Oracle), `go-sql-driver/mysql`, `aws-sdk-go-v2/service/s3`,
`flosch/pongo2/v6` (templating), seaweedfs. These don't *conflict* with midaz — but under Option A they
all enter the **root go.sum**, inflating the dependency graph for every component (including ledger,
which has no business knowing about Oracle drivers or Chrome). This is the strongest pragmatic argument
*against* a single module — not correctness, but graph hygiene and build-surface bloat.

---

## 3. Integration-point specifics for the two service collapses (Moves 3 & 4)

### 3.1 plugin-fees → ledger: the stale-path landmine

- **Coupling is narrow:** fees imports midaz only via `github.com/LerianStudio/midaz/v3/pkg/transaction`
  — **18 import sites across 7 files** (`internal/services/{calculate-fee,estimate-fee-calculation,payload_builder}.go`, `pkg/net/http/body_validator.go`, `pkg/fee/{distribute,calculate-fee}.go`, `pkg/model/fees.go`).
  Symbols used: `Amount, Distribute, FromTo, Responses, Send, Share, Source, Transaction,
  ValidateSendSourceAndDistribute`.
- **THE LANDMINE:** fees pins `midaz/v3 **v3.5.2**`, whose package was `pkg/transaction`. **midaz
  develop has already renamed `pkg/transaction` → `pkg/mtransaction`** (commit `1b4913dab`: *"refactor:
  rename pkg/transaction to pkg/mtransaction"*). So fees imports a path **that no longer exists** in the
  target tree. I verified all 9 symbols fees needs **still exist in `pkg/mtransaction`** today — so the
  collapse is a mechanical rewrite of 18 sites `pkg/transaction` → `pkg/mtransaction`, *plus* whatever
  drift accumulated between v3.5.2 and develop (fees is pinned ~ many commits behind HEAD).
- **Call direction:** fees has an `internal/m2m` provider that authenticates **outbound to "ledger"**
  (`NewM2MCredentialProvider(..., "plugin-fees", "ledger", ...)`). Today the flow is service-to-service
  HTTP with M2M auth. Fees exposes `POST /v1/fees` (CalculateFee) and `POST /v1/billing/calculate`.
  Collapsing fees into ledger means the M2M hop, the separate fees HTTP server, the fees auth gate, and
  the lib-license-go gate all **dissolve into in-process calls** — which is the whole point of "service
  collapse, not co-location," and the real elimination win.
- ledger currently has **no fee/plugin call code** in `internal/services` and only `PLUGIN_AUTH_*`
  config (generic plugin auth, not fees-specific). So the integration is genuinely greenfield inside
  ledger — there is no existing fees-client shim to delete, but there is also no scaffolding to reuse.

### 3.2 crm → ledger: small surface, separate bootstrap

crm is **70 `.go` files**, its own `bootstrap`, `adapters`, `services`, `cmd/app/main.go`, Dockerfile,
compose, 15.8K Makefile, and migration scripts (`scripts/`). It already lives in the same module as
ledger, so there is **zero dependency work** — the collapse is purely about merging two bootstraps into
one process and one route tree, and deciding DB/schema co-tenancy. Smallest of the four bodies of code,
and the only move with no module-boundary friction at all.

---

## 4. Runtime / deploy model after consolidation

| Service | Source after merge | Port | DB(s) | Survives as own binary? |
|---------|--------------------|----- |-------|--------------------------|
| ledger | components/ledger (+ fees + crm folded in) | 3002 | PG (onboarding + transaction), Mongo, Redis, RabbitMQ | ✅ |
| tracer | components/tracer | 4020 | own PG (audit hash chain) + Redis | ✅ |
| reporter-manager | components/reporter/manager | 4005 | Mongo + PG/MySQL/MSSQL/Oracle datasources, S3/seaweedfs, RabbitMQ, Redis | ✅ |
| reporter-worker | components/reporter/worker | — (consumer) | RabbitMQ consumer + Redis + storage | ✅ |
| ~~crm~~ | folded into ledger | — | — | ❌ dies |
| ~~plugin-fees~~ | folded into ledger | — | — | ❌ dies |

Net: **4 deployable units** after (ledger, tracer, reporter-manager, reporter-worker), down from 6
repos' worth of services. reporter is multi-component internally (manager + worker), matching midaz's
`components/<x>` pattern — it can land as `components/reporter/{manager,worker}` or as two top-level
components. tracer has its own migrations (`migrations/*.sql`, golang-migrate) and an
audit-hash-chain Postgres schema with DB triggers (`calculate_audit_event_hash`,
`verify_audit_hash_chain`, `prevent_truncate`) — it owns a distinct database that must be provisioned
separately; CI's S3 migration-upload step (currently ledger onboarding+transaction only) will need
tracer and reporter migration paths added.

---

## 5. The three options, scored

### OPTION A — Single Go module (`github.com/LerianStudio/midaz/v3`)

Everything becomes packages under the existing root go.mod. tracer/reporter become
`components/tracer`, `components/reporter/*`. One go.sum, one dependency set.

**Feasibility:** High. midaz already runs exactly this shape; only tracer's lib-commons v4→v5 is a true
blocker, and tracer already half-pulls v5. Every third-party skew MVS-resolves upward (§2.4). Per-component
Dockerfiles/compose already exist and slot in.

**Cost (upfront, ordered by pain):**
1. **tracer lib-commons v4 → v5 migration** — the dominant cost. Full surface sweep of tracer's
   v4 usages. Non-trivial; v4→v5 is a major bump.
2. **tracer module rename** — `module tracer` → packages under `midaz/v3/components/tracer`; rewrite
   38 internal import prefixes (`tracer/...` → `github.com/LerianStudio/midaz/v3/components/tracer/...`).
   Mechanical but touches every tracer file.
3. **fees `pkg/transaction` → `pkg/mtransaction`** rewrite (18 sites) + drift reconciliation v3.5.2→HEAD.
4. reporter zap → lib-observability migration (if "sem shims" is enforced strictly; otherwise reporter
   keeps zap as an accepted island — but that *is* an inconsistency, so a purist says migrate).
5. go.sum bloat: Oracle/MSSQL/MySQL/Chrome/seaweedfs deps enter the root graph (§2.5).
6. reporter `go 1.26`/toolchain harmonized to 1.26.3.

**Blast radius:** Largest single dependency-resolution surface — but the conflicts are concentrated in
**one** lib (lib-commons) the team owns. A break in any shared dep version stops the *whole* build until
resolved (no fence). Conversely, after unification there is exactly one of everything to reason about.

**"liso/final" fit:** **Best.** This is the only option with no module boundaries, no per-module
version drift, no workspace indirection. One go.mod is maximal elimination. It is the literal definition
of "sem abstrações de compatibilidade."

### OPTION B — `go.work` multi-module workspace

Each component keeps its own go.mod; a root `go.work` ties them for local dev. Versions evolve per
module.

**Feasibility:** High to stand up, low friction at entry — you can `go work use ./components/*` and
build today without touching tracer's lib-commons. **But** midaz today is a *single* module; adopting B
means **splitting ledger/crm/pkg out into their own modules too**, or running a mixed model (root module
+ sub-modules) that go.work tolerates but that complicates the existing CI `shared_paths: pkg/` model
(pkg/ is imported by everyone; if it stays in the root module, sub-modules import it via a `replace` or
a published version — replace directives are exactly the "workaround" Fred wants gone).

**Cost:** Low upfront. High *ongoing*: version drift is now permitted and will happen; the lib-commons
v4/v5 split can persist indefinitely behind module walls (tracer stays v4). `go.work` is **not committed
in CI builds** by convention (it's a dev-loop tool) — release builds use each module's own go.mod, so B
does **not** unify the release artifact dependency sets; it only unifies the local editor experience.

**Blast radius:** Smallest at merge time. But it **defers** every hard problem rather than solving it.

**"liso/final" fit:** **Worst.** B is co-location wearing a monorepo costume. It explicitly *preserves*
the version skew (tracer stays on lib-commons v4), which is the opposite of "sem shims." `replace`
directives to wire `pkg/` across module boundaries are textbook compatibility workarounds. **B violates
the stated intent.** It is the right answer only if the real constraint were "ship co-location this
sprint, unify later" — which contradicts "liso e final."

### OPTION C — Hybrid (single module for ledger+crm+fees; separate modules for tracer+reporter)

The collapse targets (ledger, crm, fees) merge into the root module — they have to, since fees/crm stop
being separate services. tracer and reporter stay as their own modules under a `go.work` (or even nested
modules in the same tree).

**Feasibility:** High, and it isolates the two genuinely awkward dependency situations: tracer's
lib-commons v4 and reporter's Oracle/Chrome/zap baggage stay fenced off from the ledger build graph.

**Cost:** Medium. You still do fees `pkg/transaction`→`mtransaction` + crm fold (mandatory for the
collapse). You *avoid* tracer's v4→v5 and reporter's zap migration **for now**. But you carry a mixed
module model permanently, with the same `replace`/go.work caveats as B for the tracer/reporter side.

**Blast radius:** Contained. The collapse work (the part with real product meaning) happens cleanly in
one module; the relocation work (tracer/reporter) stays loosely coupled.

**"liso/final" fit:** **Partial.** The half that matters most for product correctness (ledger + fees +
crm as one collapsed service) is fully "liso." The tracer/reporter half remains co-located-not-unified —
honest about the fact that those two are independent services that don't deeply share code with ledger
anyway. It is the pragmatic compromise: clean where collapse demands it, fenced where independence is
real.

---

## 6. Git-history import method

Three ways to bring a repo's files into midaz:

| Method | History | Tree cleanliness | Effort | Notes |
|--------|---------|------------------|--------|-------|
| **git subtree merge** | ✅ preserved, interleaved into midaz log | medium | medium | 1039+1605+1050 commits flood midaz's 6942-commit log; SHAs don't survive as-is; bisect across the seam is awkward |
| **git-filter-repo path-move** | ✅ preserved, rewritten under `components/<x>/` | clean paths | medium-high | rewrites each repo's history so files appear under target path, then merge; cleanest *if* you want history |
| **fresh import (copy tree, single commit)** | ❌ lost (git log of origin repos still exists, archived) | cleanest | lowest | one "import tracer" commit; origin repos become read-only archives for history lookup |

**Recommendation: fresh import.** Fred's stated value is **clean code over history** ("liso e final"),
and his global instructions explicitly state *"the git log carries the past"* and reject history-narration
in code. The four origin repos remain as archived/read-only GitHub repos — history is never *destroyed*,
just not interleaved into midaz's mainline. A fresh import per move (one commit: "import tracer as
components/tracer", etc.) gives the cleanest possible tree, no SHA collisions, no 3,700-commit log
pollution, and a clear before/after boundary. Use git-filter-repo path-move **only** if a specific
service's blame/bisect history is operationally load-bearing (tracer's audit-hash-chain logic might
qualify — it's compliance-sensitive code where "why was this written this way" matters). Default to
fresh import; selectively filter-repo tracer if its blame is worth the extra effort.

---

## 7. Recommendation

**Primary: OPTION A (single module), with the tracer lib-commons v4→v5 migration treated as an explicit
prerequisite gate, and FRESH IMPORT for history.**

Reasoning:
1. **It is the shape midaz already is.** One root go.mod, path-filtered component builds, per-component
   Dockerfiles. A is not new architecture; B and C are.
2. **The feared "dep conflict hard-blocks" risk is mostly absent** (§2.4). The only real cross-major
   conflict is lib-commons v4/v5 — a Lerian-owned lib that is a third rail and must be unified anyway.
   Everything third-party MVS-resolves upward without code changes.
3. **It is the only option that satisfies "sem shims/liso."** B *preserves* skew behind module walls and
   needs `replace` directives for `pkg/` — that is the workaround Fred is trying to eliminate.
4. The cost is real but **bounded and one-time**: tracer's v4→v5 sweep is the long pole; everything else
   is mechanical (renames, import rewrites, the fees path fix).

**Fallback: OPTION C**, *if and only if* the tracer lib-commons v4→v5 migration is scoped larger than the
team can absorb in the consolidation window. C lets the collapse (ledger+fees+crm, the product-critical
part) land cleanly now while fencing tracer/reporter until their lib unification is funded. C is a
defensible *staging* of A — but it should be framed as "A, deferred for tracer/reporter," with a
committed date to collapse the fence, or it quietly becomes permanent B.

**Reject: OPTION B** outright as the end-state. It is co-location mislabeled as harmonization and
directly contradicts "liso e final."

### What each option blocks / unblocks downstream

- **A unblocks:** single dependency audit, one `make lint`/`make test` graph, one Dependabot surface,
  uniform lib-observability/lib-commons posture, trivial cross-component refactors (fees↔ledger↔crm
  share types with no module hop). **A blocks until done:** any merge cannot complete until tracer is on
  lib-commons v5 (hard gate).
- **B unblocks:** immediate co-location, independent release cadence per component. **B blocks:** true
  unification forever (drift is structural); needs `replace` directives; does not unify release-artifact
  dep sets; perpetuates tracer v4.
- **C unblocks:** the product-critical service collapse (fees+crm into ledger) lands clean now without
  waiting on tracer's lib migration. **C blocks:** tracer/reporter stay un-unified until the fence is
  removed; carries a mixed module model with the same `replace`/go.work caveats as B for that half.

---

## 8. Hard parts (the things that will actually hurt), ranked

1. **tracer lib-commons v4 → v5 migration** (third-rail lib, full major bump, gates Option A). Biggest
   single cost. tracer already pulls v5 indirect → split-brain that this forces clean.
2. **plugin-fees stale-path coupling:** imports `midaz/v3@v3.5.2/pkg/transaction` which **no longer
   exists** on develop (renamed to `pkg/mtransaction`, commit `1b4913dab`). 18 sites + v3.5.2→HEAD drift
   to reconcile. Symbols verified present in mtransaction, so it is mechanical — but it is silent
   breakage if anyone assumes the path still resolves.
3. **tracer module rename** `module tracer` → qualified path; 38 internal import prefixes rewritten.
   Mechanical, touches every tracer file, must be atomic.
4. **reporter dependency bloat** (Oracle/MSSQL/MySQL/Chrome/pongo2/seaweedfs) entering the root go.sum
   under A — graph hygiene cost, the strongest pragmatic case for C.
5. **reporter observability island** (raw zap, no lib-observability) vs midaz's just-completed
   observability migration. "sem shims" purism says migrate reporter too → extra scope.
6. **lib-license-go gate moving into ledger** when fees folds in — ledger inherits a license-server
   dependency it never had. Conscious decision, not an accident to sleepwalk into.
7. **CI/migration plumbing:** build.yml `filter_paths`/`shared_paths`, S3 migration upload (today
   ledger-only), Helm value mappings, GitOps tag mappings all must learn about tracer + reporter
   components and their separate databases/migrations.
8. **plugin-fees has no LICENSE file** while tracer/reporter/midaz are Elastic License 2.0 — license
   header harmonization needed on import.

---

## 9. Evidence index (paths)

- midaz single-module proof: `/Users/fredamaral/repos/lerianstudio/midaz/go.mod` (only go.mod in tree),
  no `go.work`, `components/{ledger,crm}` have no go.mod.
- CI single-module model: `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/build.yml`.
- Component Dockerfile (root-context build): `/Users/fredamaral/repos/lerianstudio/midaz/components/ledger/Dockerfile`.
- tracer broken module path: `/Users/fredamaral/repos/lerianstudio/tracer/go.mod` (`module tracer`),
  `/Users/fredamaral/repos/lerianstudio/tracer/cmd/app/main.go` (`"tracer/internal/bootstrap"`).
- lib-commons skew: tracer go.mod (v4.6.3 + v5.3.0 indirect), reporter (v5.1.3), fees (v5.1.0),
  midaz (v5.2.0-beta.12).
- fees→midaz coupling: `grep -rn 'midaz/v3/pkg/transaction'` → 18 sites, 7 files; symbols verified in
  `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mtransaction/`.
- pkg/transaction→mtransaction rename: midaz commit `1b4913dab`.
- fees outbound-to-ledger M2M: `/Users/fredamaral/repos/lerianstudio/plugin-fees/internal/m2m/provider.go`.
- reporter components: `/Users/fredamaral/repos/lerianstudio/reporter/components/{manager,worker,infra}`.
- reporter heavy deps: `/Users/fredamaral/repos/lerianstudio/reporter/go.mod` (chromedp, go-mssqldb, go-ora).
- crm fold target: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm` (70 .go files, own bootstrap).
