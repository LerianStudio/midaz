# Phase 2c — tracer in-place dependency migration (THE LONG POLE)

**Scope discipline (PD-6):** Every task in this phase runs **inside tracer's OWN repo**
(`/Users/fredamaral/repos/lerianstudio/tracer`, branch `develop`), validated against **tracer's own
CI**, **BEFORE** any module rename or co-location into `components/tracer`. The module-path rewrite and
the move are a SEPARATE later phase (P5 — tracer move). Nothing here writes under `midaz/components/tracer`.

**Objective:** Do tracer's two stacked dependency migrations as TWO bisectable jumps, each its own
commit/PR so a regression bisects to "major bump broke it" vs "split broke it". Observability split +
co-location MUST NOT share a commit. Audit-hash-chain migrations/logic stay untouched (R19).

- **Jump 1 — lib-commons v4 → v5.1.3 (non-observability surface, ~92 sites) + lib-auth pinned v2.7.0:**
  `commons`, `tenant-manager/{core,postgres,client,event,middleware,redis}` (~29 sites,
  highest-uncertainty — dedicated API diff first), `postgres`, `net/http` (HTTP helpers), `runtime`,
  `server`. Observability packages (`log`, `opentelemetry`, `zap`) stay co-located under `commons` at
  v5.1.3 — they do NOT move yet. The telemetry middleware also stays in `v5.1.3/commons/net/http`
  (verified: `withTelemetry.go` present at v5.1.3). DELETE the live v4/v5 dual-import shim in `config.go`
  (`libLogV5`/`libZapV5` aliases + `buildAuthClientLogger`) **and in the 3 auth test fixtures**; it
  becomes redundant once everything is one major and the auth client is fed the migrated `commons/log`
  logger natively.
- **Jump 2 — v5.1.3 → v5.2.1 + observability re-platform + lib-auth bump v2.7.0 → v2.8.0 (~123 sites →
  lib-observability):** `commons/{log,opentelemetry,opentelemetry/metrics,zap}` →
  `lib-observability/{log,tracing,metrics,zap}`, `NewTrackingFromContext` (105 sites) → root
  `lib-observability`, the telemetry-middleware relocation (v5.1.3 `commons/net/http` bundled
  HTTP+telemetry → telemetry moves to `lib-observability/middleware`; HTTP helpers stay in
  `v5/commons/net/http`), and the lib-auth bump to v2.8.0 — all atomic, because lib-auth v2.8.0's
  `NewAuthClient` takes `lib-observability/log.Logger`, the same type the codebase converges on at the
  split.

---

## CRITICAL CORRECTION — why the lib-auth version is pinned per jump (resolves the #1 blocking defect)

The original plan assumed Jump 1 could land on lib-commons/v5 **v5.1.3** while lib-auth stayed at
**v2.8.0**. That is **impossible under Go MVS** and was the single load-bearing false premise of the
entire two-jump design. **Verified this pass** (`/tmp/mvscheck3`, real import of
`lib-auth/v2/auth/middleware` + `lib-commons/v5/commons`):

- lib-auth `v2.8.0` transitively requires `lib-commons/v5 v5.2.0-beta.12`. Since
  `semver.Compare("v5.2.0-beta.12","v5.1.3") == 1`, MVS floats any `v5.1.3` direct pin **UP to
  v5.2.0-beta.12** the moment `lib-auth/v2/auth/middleware` is actually imported — past the observability
  split. So at Jump 1, with v2.8.0 in the graph, `commons/{log,opentelemetry,zap}` **do not exist** in
  the resolved module and the codebase cannot compile against them. The "pin DOWN to v5.1.3" step (old
  T04) and "delete the shim against v5.1.3" step (old T08) were both unachievable.

- lib-auth `v2.7.0` exists, its `NewAuthClient(address, enabled, logger *log.Logger)` takes
  **`lib-commons/v5/commons/log.Logger`** (verified: cache `auth/middleware/middleware.go:16,149`), and
  it only requires `lib-commons/v5 v5.0.2` (< v5.1.3). **Verified** (`/tmp/mvscheck2`): with lib-auth
  pinned to v2.7.0 and a v5.1.3 direct require, MVS holds at **v5.1.3**.

**Resolution adopted as a HARD rule (no shim, no adapter):**

- **Jump 1 pins lib-auth to v2.7.0.** MVS resolves lib-commons/v5 to v5.1.3 (still co-located). The
  migrated codebase logger is a single `v5.1.3 commons/log.Logger`, which `NewAuthClient` accepts
  **natively**. The dual-import shim is deleted with ZERO transient adapter.
- **Jump 2 bumps lib-auth to v2.8.0 atomically with the obs split.** v2.8.0's `NewAuthClient` wants
  `lib-observability/log.Logger`; the auth client and the rest of the codebase converge on
  `lib-observability/log` in the SAME commit. This dissolves the false v5.1.3 boundary and makes the
  shim deletion coherent without any "confirm at execution time" escape hatch.

This is two clean commits and eliminates the micro-shim entirely (old Open Item #2 is now closed, not
deferred).

---

**Verified ground truth (this planning pass — re-verified against the live tracer checkout):**

- tracer `go.mod`: `module tracer`, `go 1.26.3`, `lib-commons/v4 v4.6.3` (direct), `lib-commons/v5 v5.3.0`
  (direct/indirect-pinned, line 51) + `lib-observability v1.0.0` (indirect, line 52), `lib-auth/v2 v2.8.0`
  (direct). **`go build ./...` on develop HEAD FAILS** — `config.go:24-25` import
  `v5/commons/{log,zap}`, which the MVS-resolved `v5.3.0` no longer ships (split removed them). **There
  is NO green baseline on HEAD.** T00 must repair this before capturing metrics (see T00).
- MVS resolution (verified): tracer's go.mod directly pins `lib-commons/v5 v5.3.0`; lib-auth v2.8.0
  requires `v5.2.0-beta.12`; resolved = `v5.3.0`. The shim importing `v5/commons/{log,zap}` is therefore
  dead against the resolved module — develop is mid-migration, not green.
- Shim is real and lives in **4 files**: `config.go:24-25` (`libLogV5`/`libZapV5`), `config.go:1417`
  (`buildAuthClientLogger`), plus three auth test fixtures that each build `libLogV5.NewNop()` and feed
  `NewAuthClient`: `internal/adapters/http/in/routes_test.go:18,86`,
  `internal/adapters/http/in/routes_multitenant_test.go:16,102`,
  `internal/adapters/http/in/middleware/auth_guard_test.go:15,40`. **All four must be migrated in the
  same commit** or `go build ./...` / `make test-unit` cannot go green.
- Stale refactor-history comments to DELETE (CLAUDE.md no-narration rule): `config.go:940` ("bootstrap
  is still on v4. buildAuthClientLogger constructs a v5 zap ...") and `config.go:1406` ("constructs a
  lib-commons/v5 logger for lib-auth v2.7.0 ..."). These go when `buildAuthClientLogger` goes (T08).
- lib-auth v2.7.0 `NewAuthClient` takes `lib-commons/v5/commons/log.Logger` (cache:
  `auth/middleware/middleware.go:16,149`), requires `lib-commons/v5 v5.0.2`. lib-auth v2.8.0
  `NewAuthClient` takes `lib-observability/log.Logger` (cache: `auth/middleware/middleware.go:17,150`),
  requires `lib-commons/v5 v5.2.0-beta.12` + `lib-observability v1.0.0`.
- v4 import-site counts (excluding `docs/codereview/ast-before-*` snapshots): `commons/log` 63,
  `commons/opentelemetry` 48, `commons/opentelemetry/metrics` 10, `commons/zap` 2 (=123 observability);
  `commons` root 54, `postgres` 8, `runtime` 4, `server` 1, `net/http` 3; tenant-manager 29
  (core 16, postgres 5, client 4, event 2, middleware 1, redis 1). Total v4 sites 222. (These v4 counts
  are correct — raw == ast-before-excluded, because the snapshots use no v4 import paths.)
- **`NewTrackingFromContext`: 105 call sites across 46 files** (verified `grep -rn ... | grep -v
  ast-before | wc -l` == 105 sites / 46 files; raw == excluded in the live checkout). The earlier
  301/132 figure was an ~3x overcount (it counted gitignored `ast-before-*` snapshot copies). Likewise
  `ContextWithTracer` = **6** (not 8), `ContextWithMetricFactory` = **1** (not 3). Canonical counting
  command for this phase: `grep -rn '<symbol>' --include='*.go' . | grep -v ast-before` (or any
  gitignore-respecting tool — pick ONE per executor so counts are reproducible).
- Telemetry MW split confirmed: at **v5.1.3**, `commons/net/http` STILL bundles telemetry
  (`withTelemetry.go` present) — so Jump 1 compiles the telemetry symbols off v5.1.3's net/http with no
  relocation. At **v5.2.1**, `commons/net/http` has NO telemetry symbols (verified absent) — relocation
  is mandatory at Jump 2. Targets in lib-observability v1.0.1 (verified present):
  `middleware/telemetry.go:81 NewTelemetryMiddleware(*tracing.Telemetry) *TelemetryMiddleware`,
  `:86 (tm *TelemetryMiddleware) WithTelemetry(...) fiber.Handler`,
  `:175 (tm *TelemetryMiddleware) EndTracingSpans(*fiber.Ctx) error`,
  `middleware/logging.go:183 WithHTTPLogging(...) fiber.Handler`, `:64 WithCustomLogger(obslog.Logger)`.
  Root `lib-observability` (verified): `:134 NewTrackingFromContext`, `:94 ContextWithTracer`,
  `:108 ContextWithMetricFactory`. `FiberErrorHandler` stays in `v5.2.1 commons/net/http` (verified:
  `net/http/...:109`). `lib-observability/log.NewNop()` exists (verified `log/...:9`).
- Proxy versions (verified): lib-commons `v5.1.3` exists; `v5.2.0`, `v5.2.1`, `v5.3.x`, `v5.4.1` GA exist.
  **PD-4 GA target = `v5.2.1`** (latest v5.2.x GA). lib-observability `v1.0.1` is latest stable (keep it;
  `v1.1.0` only beta). lib-auth `v2.7.0` and `v2.8.0` both exist as GA.
- midaz currently pins `lib-commons/v5 v5.2.0-beta.12` + `lib-observability v1.0.1` + `lib-auth/v2 v2.8.0`.
  midaz's own bump to v5.2.1 GA is Phase 1 (PD-4). **midaz is NOT yet on v5.2.1 GA** — the alignment
  checkpoint (P2c-T23) records the exact triple so the later single-go.mod merge (P5/co-location) can
  diff against it.
- godog e2e at `tests/end2end/{features,steps,support}` + `e2e_test.go`. 34 migrations; hash-chain in
  `000001/000002/000017` + `audit_event_repository.go` `VerifyHashChain`. `docs/codereview/` =
  `ast-before-*` snapshots (exclude from all counts/moves; PD-3 says exclude on the eventual move too).
- mk layout: `mk/{database,docker,docs,quality,security,tests}.mk`. **Coverage note (corrects R11
  framing):** the local `mk/tests.mk` coverage target (≈line 137-150) PRINTS coverage and continues; it
  only hard-fails on test failure, NOT on a coverage threshold. The 85% hard-fail gate lives in
  shared-workflows CI (`go-combined-analysis.yml`), not in local `make`. Additionally `.ignorecoverunit`
  EXCLUDES `/bootstrap/` and `/cmd/app/` from unit coverage — exactly where the tenant-manager rewrite
  (T06) and auth wiring (T08) live. So the coverage gate gives little protection for the riskiest
  changes; the real safety net there is the MT chaos integration tests (`16_*`/`17_*`) + godog. The
  orchestrator must NOT over-trust R11 for the bootstrap surface.

**Out of scope for this phase (later phases):** module rename `tracer/` → `components/tracer/`, go.mod
deletion, pkg/shell consolidation, CI fold into midaz, Dockerfile build-context change, compose network
repoint, midaz's own go.mod bumps. This phase ends with tracer green on its OWN CI on v5.2.1 +
lib-observability v1.0.1 + lib-auth v2.8.0, ready to move (P2c-T22 is the explicit ready-to-move gate
that P5-T00 depends on, per DAG-3).

---

## Dependency DAG

```
P2c-T00 (verify GA + REPAIR baseline to green + capture CI/coverage)
   ├─> P2c-T01 (tenant-manager v4→v5 API diff)        [highest-uncertainty, do first]
   ├─> P2c-T02 (lib-auth jump-pinning plan: v2.7.0 @ Jump1, v2.8.0 @ Jump2)
   ├─> P2c-T03 (exclude ast-before snapshots from analysis scope)
   └─> P2c-T23 (record exact v5.2.1+v1.0.1+v2.8.0 triple for merge-phase alignment)

JUMP 1 (v4 → v5.1.3, non-observability + lib-auth v2.7.0 + shim delete):
   P2c-T04  (go.mod: add v5.1.3 direct, PIN lib-auth v2.7.0, keep v4 transitional) depends T00,T02
   P2c-T05  (rewrite commons root 54 sites v4→v5.1)        depends T04
   P2c-T06  (rewrite tenant-manager 29 sites v4→v5.1)      depends T04,T01
   P2c-T07  (rewrite postgres/runtime/server/net-http non-tel) depends T04
   P2c-T08  (DELETE shim libLogV5/libZapV5 + buildAuthClientLogger across 4 files; wire auth on commons/log) depends T05,T02
   P2c-T09  (drop lib-commons/v4 from go.mod; tidy; assert v5 resolves==v5.1.3; build) depends T05,T06,T07,T08
   P2c-T10  (Jump-1 validation: lint+unit+integration+godog on tracer CI) depends T09

JUMP 2 (v5.1.3 → v5.2.1 + observability split + lib-auth v2.8.0):
   P2c-T11  (go.mod: bump v5.2.1, add lib-observability v1.0.1 direct, BUMP lib-auth v2.8.0) depends T10
   P2c-T12  (repoint seam files: pkg/logging/trace.go + internal/observability/*) depends T11
   P2c-T13  (sweep commons/log → lib-observability/log, 63 sites + auth-client logger) depends T12
   P2c-T14  (sweep commons/opentelemetry → lib-observability/tracing, 48) depends T12
   P2c-T15  (sweep commons/opentelemetry/metrics → lib-observability/metrics, 10) depends T12
   P2c-T16  (sweep commons/zap → lib-observability/zap, 2)             depends T12
   P2c-T17  (NewTrackingFromContext 105 sites → root lib-observability) depends T13
   P2c-T18  (telemetry-middleware relocation — the one real code move)  depends T13,T14
   P2c-T19  (drop residual indirect v5.3.0 / obs v1.0.0; tidy; assert no pseudo-versions; build) depends T13..T18
   P2c-T20  (Jump-2 validation: lint+unit+integration+godog+sec)        depends T19
   P2c-T21  (audit-hash-chain non-regression proof, R19)                depends T20
   P2c-T24  (decide godog CI ownership for the eventual midaz fold)     depends T20
   P2c-T22  (open PRs against tracer CI, two commits, confirm green; READY-TO-MOVE GATE) depends T21,T24
```

Downstream phases bind to gate tasks (DAG-3 / DAG-5): **P5-T00 (tracer move) depends_on P2c-T22**
(tracer ready-to-move). This phase's own external edge is **P2c-T00 / P2c-T04 do NOT depend on Phase 1**
(tracer pins its own libs independently; the midaz v5.2.1 bump is P1-T06 and is reconciled at the
co-location merge, tracked by P2c-T23 — not a blocking edge here).

---

## Tasks

### P2c-T00 — Verify GA on proxy; REPAIR tracer to a green baseline; capture CI/coverage
**Description:** develop HEAD does **not** build (`config.go:24-25` import `v5/commons/{log,zap}` that the
MVS-resolved `v5.3.0` no longer ships). There is no green state to anchor a bisection to. This task is
verify-AND-repair, not snapshot-only. Steps: (1) Confirm GA targets on the public proxy
(`go list -m -versions github.com/LerianStudio/lib-commons/v5` and `... lib-observability`,
`... lib-auth/v2`). **Recorded this pass:** lib-commons GA `v5.2.0`/`v5.2.1`/`v5.3.x`/`v5.4.1`; pick
`v5.2.1` (PD-4). lib-observability latest stable `v1.0.1` (keep). lib-auth `v2.7.0` and `v2.8.0` both GA.
(2) Establish a **compiling green anchor commit**: the cleanest restoration is to pin lib-auth back to
`v2.7.0` (which requires `lib-commons/v5 v5.0.2` ≤ v5.1.3 and wants `commons/log`), so the v5
`commons/{log,zap}` shim imports resolve again and `go build ./...` is green on the v4-era baseline.
Record the exact anchor commit SHA. (3) On that green anchor run the full suite once and record: coverage
% per package (respecting `.ignorecoverunit` excludes — NOTE `/bootstrap/` and `/cmd/app/` are excluded,
so coverage is NOT the safety net for T06/T08), lint pass/fail, integration+godog pass/fail,
`go build ./...` clean. **Acceptance is gated on a GREEN build** — if HEAD cannot be repaired to green by
the lib-auth pin-back, STOP and escalate before any jump starts.
**Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`, `go.sum`, `mk/tests.mk` (read),
`.ignorecoverunit` (read), `internal/bootstrap/config.go` (read).
**depends_on:** []
**Acceptance:** Documented pin list (`lib-commons/v5 v5.2.1` target, `lib-observability v1.0.1`,
`lib-auth/v2 v2.8.0` final / `v2.7.0` Jump-1); a named green-anchor commit SHA where `go build ./...`,
`make test-unit`, `make test-integration`, godog ALL pass; baseline coverage/lint/test status captured in
the PR description template; explicit note that `/bootstrap/` + `/cmd/app/` are coverage-excluded.
**Tests:** `cd /Users/fredamaral/repos/lerianstudio/tracer && go build ./... && make test-unit && make
test-integration` (must be GREEN on the anchor commit; if HEAD is red, the lib-auth v2.7.0 pin-back IS
the first migration step and the green anchor is that repaired commit).
**Effort:** M, 0.5 day (HEAD-red repair adds time over a pure snapshot). **risk_refs:** R3, R11.

### P2c-T01 — tenant-manager v4→v5 API diff (highest-uncertainty surface)
**Description:** Before touching code, produce a symbol-level diff of the 6 tenant-manager subpackages
tracer uses (`core` 16, `postgres` 5, `client` 4, `event` 2, `middleware` 1, `redis` 1 = 29 sites)
between `lib-commons/v4@v4.6.3` and `lib-commons/v5@v5.1.3`, then forward to `v5.2.1` (verified: no
subpackage reshuffle — `core/postgres/client/event/middleware/redis` all present in v5.2.1; v5.2.1 also
adds `cache/consumer/log/mongo` which tracer does not use). For each symbol tracer references (constructor
option funcs, config struct fields, `Manager` types, `GetTenantIDContext`, `GetPGContext`), record v4
signature vs v5 signature and the exact code change needed. Cross-check against midaz's live v5 usage —
**note midaz is on `v5.2.0-beta.12`, NOT v5.2.1 GA** (Open Item 1); confirm the beta.12 tenant-manager
API == v5.2.1 GA before treating midaz as the "production-exercised" reference, else validate the diff
against the v5.2.1 cache directly. Output: a per-site change table consumed by P2c-T06.
**Files (read):** tracer `internal/bootstrap/config_multitenant_wiring.go`, `config_multitenant.go`,
`config.go`, `tenant_listener_app.go`, `internal/adapters/postgres/db/adapter.go`,
`internal/services/cache/rule_cache.go`, `internal/services/workers/{supervisor,pool_resolver,rule_sync_worker,usage_cleanup_worker}.go`;
midaz `components/ledger/internal/bootstrap/*multitenant*`; lib-commons v5.2.1 module cache
`commons/tenant-manager/*`.
**depends_on:** [P2c-T00]
**Acceptance:** A per-symbol v4→v5 diff table covering all 29 sites; every v5 symbol confirmed present
in `v5.2.1` module cache; a one-line note recording whether midaz beta.12 == v5.2.1 GA for the symbols
referenced; no "missing capability" surprises flagged (signature drift only) OR each genuine gap escalated
to open_items.
**Tests:** N/A (analysis) — validated downstream by P2c-T06/T10 compile + MT integration tests.
**Effort:** M, 1 day. **risk_refs:** R10, R3.

### P2c-T02 — lib-auth jump-pinning plan (v2.7.0 @ Jump 1, v2.8.0 @ Jump 2) — closes the micro-shim
**Description:** Codify the lib-auth version-per-jump decision that eliminates the micro-shim entirely
(see CRITICAL CORRECTION). **Verified this pass:** lib-auth `v2.7.0` `NewAuthClient(address, enabled,
logger *log.Logger)` takes `lib-commons/v5/commons/log.Logger` (cache `auth/middleware/middleware.go:16,
149`) and requires `lib-commons/v5 v5.0.2` (≤ v5.1.3) — so with a v5.1.3 direct pin MVS holds at v5.1.3
(verified `/tmp/mvscheck2`); lib-auth `v2.8.0` `NewAuthClient` takes `lib-observability/log.Logger` (cache
`:17,150`) and floats v5 to `v5.2.0-beta.12` (verified `/tmp/mvscheck3`). The plan:
(a) **Jump 1 pins lib-auth v2.7.0** — the migrated `commons/log` logger feeds `NewAuthClient` natively;
delete `buildAuthClientLogger` + `libLogV5`/`libZapV5` with NO adapter.
(b) **Jump 2 bumps lib-auth v2.8.0 atomically with the obs split (T11 + T13)** — auth client moves to
`lib-observability/log.Logger` in the same commit as the rest of the codebase.
Document that this is two clean commits with zero transient scaffolding, and that the stale comments at
`config.go:940` (`bootstrap is still on v4 ...`) and `:1406` (`... for lib-auth v2.7.0`) are DELETED at
T08 per the CLAUDE.md no-refactor-narration rule.
**Files (read):** tracer `internal/bootstrap/config.go` (lines 16-25, 940, 947, 1406-1442); lib-auth
cache `v2.7.0/auth/middleware/middleware.go`, `v2.8.0/auth/middleware/middleware.go`.
**depends_on:** [P2c-T00]
**Acceptance:** Written plan stating (a) Jump-1 lib-auth = v2.7.0, auth logger sourced from migrated
`v5.1.3 commons/log`, no adapter; (b) Jump-2 lib-auth = v2.8.0 bumped atomically with the obs split, auth
logger = `lib-observability/log.Logger`; (c) the two MVS facts cited file:line; (d) the stale comments to
delete at T08. NO shim, adapter, or fence introduced at either jump.
**Tests:** N/A (analysis) — proven by P2c-T08 (Jump-1 compiles, no second alias) and P2c-T13/T19 (Jump-2
auth client on lib-observability/log).
**Effort:** S, 2–3h. **risk_refs:** R3.

### P2c-T03 — Fence off `docs/codereview/ast-before-*` snapshots from migration scope
**Description:** tracer carries `docs/codereview/ast-before-*` AST snapshots that contain stale copies of
`routes.go`/imports and would inflate grep counts (they are the source of the old 301/132 phantom count).
They are not compiled source. Confirm git-tracked status; ensure all migration sweeps (T05–T18) and all
counting commands EXCLUDE this path (`grep -v ast-before` or `-path './docs/codereview' -prune`). This
prevents the sweeps from rewriting dead snapshot files and corrupting the diff, and keeps counts
reproducible. (PD-3 already excludes these on the eventual move; here we only need them out of the in-repo
migration's blast radius.) Pin the canonical counting command for the phase:
`grep -rn '<symbol>' --include='*.go' . | grep -v ast-before` (or a gitignore-respecting tool — ONE per
executor).
**Files (read):** tracer `docs/codereview/` (verify tracked status via `git status`/`git ls-files`).
**depends_on:** [P2c-T00]
**Acceptance:** Every sweep + count command in this phase documented with the `ast-before` exclusion;
`git diff --stat` after each sweep shows ZERO files under `docs/codereview/` changed; one canonical
counting command recorded.
**Tests:** `git -C /Users/fredamaral/repos/lerianstudio/tracer diff --stat -- docs/codereview/` returns
empty after each sweep task.
**Effort:** XS, <1h. **risk_refs:** R3.

### P2c-T23 — Record the exact v5.2.1 + v1.0.1 + v2.8.0 triple for the merge-phase alignment check
**Description:** midaz is currently on `lib-commons/v5 v5.2.0-beta.12` + `lib-observability v1.0.1` +
`lib-auth/v2 v2.8.0`; midaz's bump to v5.2.1 GA is Phase 1 (P1-T06, PD-4). The eventual single-root
go.mod merge (P5 / co-location) must resolve tracer's pins and midaz's pins to the SAME minors under MVS.
Open Item 1 flagged the risk but assigned no owner. This task records, in the phase's PR description, the
EXACT post-Jump-2 dependency triple tracer lands on (`lib-commons/v5 v5.2.1`, `lib-observability v1.0.1`,
`lib-auth/v2 v2.8.0`) plus the resolved `go list -m all` lines for those three, so the merge phase can
diff against a concrete record rather than re-derive it. Pure bookkeeping — no code change.
**Files:** none (records into the phase PR description / handoff doc).
**depends_on:** [P2c-T00]
**Acceptance:** Recorded triple + verbatim `go list -m github.com/LerianStudio/lib-commons/v5
github.com/LerianStudio/lib-observability github.com/LerianStudio/lib-auth/v2` output captured for the
green-anchor and updated again after T19; a one-line flag if midaz P1-T06 picks a GA other than v5.2.1 so
the merge phase re-pins.
**Tests:** N/A (record).
**Effort:** XS, <1h. **risk_refs:** R3.

---

### JUMP 1 — lib-commons v4 → v5.1.3 (non-observability + lib-auth v2.7.0 + shim delete)

### P2c-T04 — go.mod: promote lib-commons/v5 to v5.1.3 direct, PIN lib-auth v2.7.0, keep v4 transitional
**Description:** (1) PIN `github.com/LerianStudio/lib-auth/v2 v2.7.0` (DOWN from v2.8.0) — this is what
lets MVS hold lib-commons/v5 at v5.1.3; v2.8.0 would float it to v5.2.0-beta.12 past the split (verified).
(2) Add `github.com/LerianStudio/lib-commons/v5 v5.1.3` as a DIRECT require (currently pinned at v5.3.0;
pin DOWN to v5.1.3 so observability packages are still co-located under `commons` for Jump 1). (3) Keep
`lib-commons/v4 v4.6.3` present until all v4 sites are rewritten (T05–T08). This is the only step where
both majors legally coexist — different import paths, NOT a shim. Run `go mod tidy`, then ASSERT the
resolved v5 is exactly v5.1.3 and there are no pseudo-versions/branch refs on lib-commons/lib-observability/
lib-auth (`go list -m all` clean of pseudo-versions — DAG-5 bare-edge pinning).
**Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`,
`/Users/fredamaral/repos/lerianstudio/tracer/go.sum`.
**depends_on:** [P2c-T00, P2c-T02]
**Acceptance:** `go.mod` shows `lib-commons/v5 v5.1.3` direct + `lib-commons/v4 v4.6.3` direct +
`lib-auth/v2 v2.7.0`; `go list -m github.com/LerianStudio/lib-commons/v5` returns exactly `v5.1.3`;
`go list -m all` shows no pseudo-versions/branch refs for lib-commons/lib-observability/lib-auth;
`go mod download` succeeds. (Build is NOT yet green — code still on v4 paths; that lands at T09.)
**Tests:** `cd /Users/fredamaral/repos/lerianstudio/tracer && go list -m github.com/LerianStudio/lib-commons/v5
&& go list -m all | grep -E 'lib-(commons|observability|auth)' | grep -E '\-0\.[0-9]{14}-|/[a-f0-9]{12}$' ; test $? -ne 0`.
**Effort:** S, 1h. **risk_refs:** R3.

### P2c-T05 — Rewrite `v4/commons` root → `v5/commons` (54 sites)
**Description:** Scripted path rewrite `github.com/LerianStudio/lib-commons/v4/commons` →
`.../v5/commons` for the root `commons` package (54 sites). Symbols confirmed parity in midaz v5:
`SetConfigFromEnvVars`, `InitLocalEnvConfig`, `ValidateBusinessError`, `App`, `RunApp`, `Launcher`,
`NewLauncher`, `LauncherOption`, `WithLogger`, `ValidateServerAddress`, `IsNilOrEmpty`, `Contains`,
`GetMapNumKinds`, `Response`. `NewTrackingFromContext` lived in `v4/commons` but **stays under
`v5/commons` at v5.1.3** (it only moves to lib-observability at Jump 2) — do NOT touch it here. Exclude
`docs/codereview/ast-before-*` (T03).
**Files:** all tracer `.go` files importing `v4/commons` (root) incl.
`internal/bootstrap/config.go`, `cmd/app/main.go`, `pkg/*`, `internal/services/*`, `internal/adapters/*`.
**depends_on:** [P2c-T04]
**Acceptance:** Zero `v4/commons"` (root) imports remain (grep, ast-before excluded); `go build ./...`
compiles for the rewritten packages (full build blocked until T08/T09).
**Tests:** `grep -rn 'lib-commons/v4/commons"' --include='*.go' . | grep -v ast-before` → empty.
**Effort:** M, 0.5 day. **risk_refs:** R3.

### P2c-T06 — Rewrite tenant-manager v4 → v5.1.3 (29 sites) applying the API diff
**Description:** Rewrite the 6 tenant-manager subpackages (`core` 16, `postgres` 5, `client` 4,
`event` 2, `middleware` 1, `redis` 1) from `v4/commons/tenant-manager/*` → `v5/commons/tenant-manager/*`
AND apply the signature/option-func/config-struct changes from the T01 diff table. This is the
highest-uncertainty surface (R10) — expect constructor-option drift on the per-tenant Postgres
`Manager`, Redis pub/sub listener, circuit breaker, tenant cache. Concentrated in
`config_multitenant_wiring.go`, `config_multitenant.go`, `tenant_listener_app.go`,
`adapters/postgres/db/adapter.go`, `services/workers/{supervisor,pool_resolver,rule_sync_worker}.go`,
`services/cache/rule_cache.go`. NOTE these live under `/bootstrap/` which is coverage-excluded — the real
verification is the MT chaos integration tests at T10, not unit coverage. Exclude ast-before.
**Files:** tracer `internal/bootstrap/{config_multitenant_wiring,config_multitenant,config,tenant_listener_app}.go`,
`internal/adapters/postgres/db/adapter.go`, `internal/adapters/http/in/routes.go`,
`internal/services/cache/rule_cache.go`,
`internal/services/workers/{supervisor,pool_resolver,rule_sync_worker,usage_cleanup_worker}.go`.
**depends_on:** [P2c-T04, P2c-T01]
**Acceptance:** Zero `v4/.../tenant-manager` imports remain; rewritten packages compile; MT wiring
type-checks (`tmpostgres.Manager` and friends resolve to v5 types).
**Tests:** `go build ./internal/bootstrap/... ./internal/services/workers/... ./internal/adapters/postgres/...`;
MT integration: `tests/integration/16_multitenant_chaos_tm_outage_test.go`,
`17_multitenant_chaos_redis_outage_test.go` (run at T10).
**Effort:** L, 1.5–2 days (drift debugging). **risk_refs:** R10, R3.

### P2c-T07 — Rewrite postgres/runtime/server + net/http NON-telemetry helpers v4 → v5.1.3
**Description:** Path-rewrite `v4/commons/postgres` (8) → `v5/commons/postgres`, `v4/commons/runtime`
(4) → `v5/commons/runtime`, `v4/commons/server` (1) → `v5/commons/server`. For `v4/commons/net/http`
(3 sites: `handlers.go:8`, `routes.go:14`, `readyz.go:13`), rewrite to `v5/commons/net/http`. At v5.1.3
this package STILL bundles BOTH the HTTP helpers (`FiberErrorHandler`, `WithHTTPLogging`,
`WithCustomLogger`) AND telemetry middleware (`NewTelemetryMiddleware`, `WithTelemetry`,
`EndTracingSpans`) — verified `withTelemetry.go` present at v5.1.3. So at this jump leave ALL of them on
the single `v5.1.3 commons/net/http` alias; the telemetry relocation only becomes mandatory at Jump 2
(v5.2.1 drops telemetry from net/http — T18). Exclude ast-before.
**Files:** tracer `internal/adapters/http/in/{handlers,routes,readyz}.go`,
`internal/adapters/postgres/db/adapter.go`, files importing `v4/commons/{runtime,server}`.
**depends_on:** [P2c-T04]
**Acceptance:** Zero `v4/commons/{postgres,runtime,server,net/http}` imports remain; HTTP layer compiles
on v5.1.3 net/http (HTTP helpers AND telemetry both resolving from it).
**Tests:** `go build ./internal/adapters/http/... ./internal/adapters/postgres/...`.
**Effort:** S, 0.5 day. **risk_refs:** R3.

### P2c-T08 — DELETE the v4/v5 dual-import shim across ALL 4 files; wire auth on commons/log natively
**Description:** Remove the compatibility scaffolding the end-state forbids, in EVERY file that carries
it (verified 4 files). In `internal/bootstrap/config.go`: delete `:24-25` (`libLogV5`/`libZapV5`), the
stale comment block at `:940` ("bootstrap is still on v4 ..."), the stale doc comment at `:1406`
("... for lib-auth v2.7.0 ..."), and `buildAuthClientLogger` (`:1406-1442`); repoint the auth-client
construction at `:947` to feed the single migrated `v5.1.3 commons/log` logger (`libLog`) directly into
`NewAuthClient` — lib-auth is pinned to **v2.7.0** at Jump 1 (T04), whose `NewAuthClient` takes exactly
that `commons/log.Logger`, so this compiles natively with NO adapter. In the three test fixtures
(`internal/adapters/http/in/routes_test.go:18,86`, `routes_multitenant_test.go:16,102`,
`internal/adapters/http/in/middleware/auth_guard_test.go:15,40`): drop the `libLogV5` import and replace
`authLoggerV5 := libLogV5.NewNop()` with the `v5.1.3 commons/log` Nop (`libLog.NewNop()`), passed into
`NewAuthClient` the same way production does. The hard requirement: ZERO dual-major lib-commons imports
and ZERO `libLogV5`/`libZapV5`/`buildAuthClientLogger` references remain ANYWHERE in tracer source or
tests; NO second alias, NO local adapter, NO fence introduced.
**Files:** tracer `internal/bootstrap/config.go`, `internal/adapters/http/in/routes_test.go`,
`internal/adapters/http/in/routes_multitenant_test.go`,
`internal/adapters/http/in/middleware/auth_guard_test.go`.
**depends_on:** [P2c-T05, P2c-T02]
**Acceptance:** Repo-wide grep is empty —
`grep -rn 'libLogV5\|libZapV5\|buildAuthClientLogger' --include='*.go' . | grep -v ast-before` → empty;
no file imports two majors of `lib-commons`; the stale `:940`/`:1406` comments are gone;
`config.go` and all three test files compile.
**Tests:** `go build ./internal/bootstrap/... ./internal/adapters/http/...`;
`grep -rn 'libLogV5\|libZapV5\|buildAuthClientLogger' --include='*.go' . | grep -v ast-before` → empty;
`grep -rn 'lib-commons/v4' --include='*.go' . | grep -v ast-before` trending to zero.
**Effort:** S, 0.5 day. **risk_refs:** R3.

### P2c-T09 — Drop lib-commons/v4 from go.mod; tidy; assert v5==v5.1.3; full build
**Description:** With all v4 sites rewritten (T05–T08) and lib-auth pinned to v2.7.0 (T04), remove
`lib-commons/v4` from `go.mod`, run `go mod tidy`, and ASSERT the resolved `lib-commons/v5` is exactly
`v5.1.3` (it MUST be — v2.7.0 requires only v5.0.2, and nothing else in the graph floats it above the
v5.1.3 direct pin; if it floats above v5.1.3 here, a transitive dep is forcing the split and Jump 1's
boundary is wrong — STOP and investigate, do NOT proceed). Full `go build ./...` + `go vet ./...` must be
GREEN — this is the Jump-1 compile milestone.
**Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`, `go.sum`.
**depends_on:** [P2c-T05, P2c-T06, P2c-T07, P2c-T08]
**Acceptance:** `go.mod` has NO `lib-commons/v4`; `go list -m github.com/LerianStudio/lib-commons/v5` ==
`v5.1.3`; `go build ./...` + `go vet ./...` green;
`grep -rn 'lib-commons/v4' --include='*.go' . | grep -v ast-before` → empty.
**Tests:** `cd /Users/fredamaral/repos/lerianstudio/tracer && go mod tidy && go list -m
github.com/LerianStudio/lib-commons/v5 && go build ./... && go vet ./...`.
**Effort:** S, 2–3h. **risk_refs:** R3.

### P2c-T10 — Jump-1 validation on tracer's OWN CI (lint + unit + integration + godog)
**Description:** Prove the major bump + lib-auth v2.7.0 pin is correct, isolated from the observability
split. Run the full tracer suite: `make lint`, `make test-unit`, `make test-integration`
(testcontainers Postgres), and the godog e2e (`mk/tests.mk`). MT chaos tests (`tests/integration/16_*`,
`17_*`) specifically validate the T06 tenant-manager rewrite — and are the PRIMARY safety net for it,
since `/bootstrap/` is coverage-excluded. Coverage compared against the T00 baseline (R11) is a guideline
for non-bootstrap packages, not a hard local gate (the 85% hard-fail lives in shared-workflows CI, not
`make`). This is a commit/PR boundary — Jump 1 lands GREEN before Jump 2 starts.
**Files:** none (validation).
**depends_on:** [P2c-T09]
**Acceptance:** All four suites green on tracer CI; MT chaos tests pass; coverage on non-excluded packages
≥ T00 baseline (or any drop explained); Jump-1 commit ready to merge.
**Tests:** `make lint && make test-unit && make test-integration` + godog target from `mk/tests.mk`.
**Effort:** M, 0.5–1 day (incl. flake/drift fixes). **risk_refs:** R10, R11, R15.

---

### JUMP 2 — lib-commons v5.1.3 → v5.2.1 + observability re-platform + lib-auth v2.8.0

### P2c-T11 — go.mod: bump lib-commons/v5 → v5.2.1, add lib-observability v1.0.1 direct, BUMP lib-auth → v2.8.0
**Description:** Bump `lib-commons/v5` to `v5.2.1` (latest v5.2.x GA, PD-4), add
`github.com/LerianStudio/lib-observability v1.0.1` as a DIRECT require, AND bump
`github.com/LerianStudio/lib-auth/v2` to `v2.8.0`. The lib-auth bump is part of THIS commit (not a
separate one) because v2.8.0's `NewAuthClient` takes `lib-observability/log.Logger` — the auth client
must converge on lib-observability/log in the same commit as the rest of the codebase (T13). At this
point `commons/{log,opentelemetry,zap}` vanish from lib-commons → the codebase will NOT compile until the
sweeps (T12–T18) land. Expect a red build immediately after this require change; that is intended (it
enumerates every site needing the split). Assert no pseudo-versions/branch refs on the three libs
(DAG-5 bare-edge pinning).
**Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`, `go.sum`.
**depends_on:** [P2c-T10]
**Acceptance:** `go.mod` shows `lib-commons/v5 v5.2.1` + `lib-observability v1.0.1` direct +
`lib-auth/v2 v2.8.0`; `go list -m all` shows no pseudo-versions/branch refs for those three;
`go mod download` succeeds; `go build ./...` fails ONLY on `commons/{log,opentelemetry,zap}` import
errors (the migration worklist) — NOT on lib-auth (the auth-logger break is fixed in the same Jump-2
sweep at T13).
**Tests:** `go build ./... 2>&1 | grep -c 'commons/log\|commons/opentelemetry\|commons/zap'` enumerates
the remaining sites; `go list -m all | grep -E 'lib-(commons|observability|auth)' | grep -E
'\-0\.[0-9]{14}-|/[a-f0-9]{12}$' ; test $? -ne 0`.
**Effort:** S, 1h. **risk_refs:** R3, R4.

### P2c-T12 — Repoint the observability seam files first (localized blast radius)
**Description:** tracer localizes observability behind two seams — repoint them BEFORE the wide sweep so
most call sites flow through migrated wrappers. (a) `pkg/logging/trace.go` wraps `commons/log` → repoint
to `lib-observability/log`. (b) `internal/observability/recorder.go` + `prometheus_factory.go` wrap
`commons/log` + `commons/opentelemetry/metrics` → repoint to `lib-observability/log` +
`lib-observability/metrics`. APIs are shape-compatible (`.Log(ctx,level,msg,fields...)`,
`libLog.String/Err`, metric factory surface) per midaz's completed migration.
**Files:** tracer `pkg/logging/trace.go`, `pkg/logging/trace_test.go`,
`internal/observability/{recorder,prometheus_factory,recorder_test}.go`.
**depends_on:** [P2c-T11]
**Acceptance:** Seam files import `lib-observability/{log,metrics}`, not `commons/*`; the three seam
packages compile in isolation (`go build ./pkg/logging/... ./internal/observability/...`).
**Tests:** `go build ./pkg/logging/... ./internal/observability/...`; `go test ./pkg/logging/...`.
**Effort:** S, 0.5 day. **risk_refs:** R4.

### P2c-T13 — Sweep `commons/log` → `lib-observability/log` (63 sites) + repoint the auth-client logger
**Description:** Path rewrite `github.com/LerianStudio/lib-commons/v5/commons/log` →
`github.com/LerianStudio/lib-observability/log`, keeping the `libLog` alias. Symbol surface is identical
(`libLog.String/Int/Any/Bool/Err/Logger/Level{Info,Warn,Error,Debug}/Field/NewNop`). Because lib-auth was
bumped to v2.8.0 in T11, the auth-client construction (`config.go:947` + the 3 test fixtures) now feeds
`NewAuthClient` a `lib-observability/log.Logger` — which is exactly what this sweep produces, so the auth
seam closes inside this same sweep with no special-casing. Exclude ast-before.
**Files:** all 63 tracer `.go` sites importing `v5/commons/log` (services, adapters, bootstrap, pkg) incl.
the auth-client site `internal/bootstrap/config.go` and the 3 test fixtures from T08.
**depends_on:** [P2c-T12]
**Acceptance:** Zero `v5/commons/log` imports remain (grep, ast-before excluded); the auth client and its
test fixtures compile against `lib-observability/log` + lib-auth v2.8.0.
**Tests:** `grep -rn 'lib-commons/v5/commons/log' --include='*.go' . | grep -v ast-before` → empty;
`go build ./internal/bootstrap/... ./internal/adapters/http/...`.
**Effort:** M, 0.5 day. **risk_refs:** R4.

### P2c-T14 — Sweep `commons/opentelemetry` → `lib-observability/tracing` (48 sites)
**Description:** Path rewrite `v5/commons/opentelemetry` → `lib-observability/tracing` (midaz aliases it
`libOpentelemetry`; tracer aliases it `libOtel` — keep tracer's alias, repoint the path). Symbols
confirmed surviving: `HandleSpanError`, `HandleSpanBusinessErrorEvent`, `SetSpanAttributesFromStruct`,
`SetSpanAttributesFromValue`, `Telemetry`, `TelemetryConfig`, `NewTelemetry`,
`InitializeTelemetryWithError`. The `NewTelemetry(TelemetryConfig{...})` call at `config.go:1381`
repoints with identical struct shape. Exclude ast-before. (Telemetry MIDDLEWARE is NOT here — that is
T18.)
**Files:** 48 tracer `.go` sites incl. `internal/bootstrap/config.go`, services, adapters.
**depends_on:** [P2c-T12]
**Acceptance:** Zero `v5/commons/opentelemetry"` imports remain; `NewTelemetry` call compiles against
`lib-observability/tracing`.
**Tests:** `grep -rn 'lib-commons/v5/commons/opentelemetry"' --include='*.go' . | grep -v ast-before` → empty.
**Effort:** M, 0.5 day. **risk_refs:** R4, R13.

### P2c-T15 — Sweep `commons/opentelemetry/metrics` → `lib-observability/metrics` (10 sites)
**Description:** Path rewrite `v5/commons/opentelemetry/metrics` → `lib-observability/metrics`. Mostly
flows through the T12 recorder/prometheus_factory seam; this catches the remaining direct importers.
Exclude ast-before.
**Files:** 10 tracer `.go` sites (`internal/services/metrics/*`, `internal/observability/*`, callers).
**depends_on:** [P2c-T12]
**Acceptance:** Zero `v5/commons/opentelemetry/metrics` imports remain.
**Tests:** `grep -rn 'commons/opentelemetry/metrics' --include='*.go' . | grep -v ast-before` → empty.
**Effort:** S, 2–3h. **risk_refs:** R4.

### P2c-T16 — Sweep `commons/zap` → `lib-observability/zap` (2 sites)
**Description:** Path rewrite the 2 remaining `v5/commons/zap` sites → `lib-observability/zap`. (The
shim's `libZapV5` from this package was already deleted at T08; these are the genuine zap users.)
Exclude ast-before.
**Files:** the 2 tracer `.go` sites importing `v5/commons/zap`.
**depends_on:** [P2c-T12]
**Acceptance:** Zero `v5/commons/zap` imports remain.
**Tests:** `grep -rn 'lib-commons/v5/commons/zap' --include='*.go' . | grep -v ast-before` → empty.
**Effort:** XS, <1h. **risk_refs:** R4.

### P2c-T17 — Repoint `NewTrackingFromContext` → root `lib-observability` (105 sites / 46 files)
**Description:** The highest-volume rename — **105 call sites across 46 files** (verified with ast-before
excluded; the prior 301/132 figure counted gitignored `ast-before-*` snapshot copies and is wrong). In
v4/v5.1 `commons`, `NewTrackingFromContext` lives in the root `commons` package; in the split it moves to
the ROOT `lib-observability` package (`libObservability.NewTrackingFromContext` — confirmed root
`lib-observability` v1.0.1 `:134`, and midaz `create_account.go:34`). Rewrite all 105 call sites: add/
repoint the import to `github.com/LerianStudio/lib-observability` (root) and qualify the call. Also
repoint `ContextWithTracer` (**6 sites**, root `:94`) and `ContextWithMetricFactory` (**1 site**, root
`:108`). Exclude ast-before. Mechanical — script it, then spot-verify. NOTE: the canonical count was taken
with ast-before excluded; an executor whose grep tooling does not respect gitignore MUST use
`grep -v ast-before` or they will chase ~196 phantom snapshot hits and risk corrupting dead files.
**Files:** the 46 tracer `.go` files calling `NewTrackingFromContext` (services/command, services/query,
adapters, workers, bootstrap).
**depends_on:** [P2c-T13]
**Acceptance:** Zero `commons.NewTrackingFromContext`/unqualified references remain; all 105 sites resolve
to root `lib-observability`; the 6 `ContextWithTracer` and 1 `ContextWithMetricFactory` sites repointed;
`go build ./...` progresses past these sites.
**Tests:** `grep -rn 'NewTrackingFromContext' --include='*.go' . | grep -v ast-before | grep -vc
'lib-observability'` → 0 (allowing the alias); same pattern for `ContextWithTracer`/`ContextWithMetricFactory`.
**Effort:** M, 0.5 day (105 mechanical sites — re-baselined down from the phantom-inflated "L, 1 day").
**risk_refs:** R4.

### P2c-T18 — Telemetry-middleware relocation (the one genuine code move, not a sweep)
**Description:** At v5.1.3 `commons/net/http` bundled HTTP helpers AND telemetry middleware; v5.2.1
removes telemetry from net/http (verified absent). The split moves telemetry middleware to
`lib-observability/middleware`. Repoint tracer `routes.go`: `NewTelemetryMiddleware` (`:169`),
`tlMid.WithTelemetry` (`:175`), `tlMid.EndTracingSpans` (`:338`) → `lib-observability/middleware`.
NOTE these are METHODS on `*TelemetryMiddleware` (verified v1.0.1 `middleware/telemetry.go:81`
`NewTelemetryMiddleware(*tracing.Telemetry) *TelemetryMiddleware`, `:86 (tm).WithTelemetry`,
`:175 (tm).EndTracingSpans`) — the existing `tlMid.WithTelemetry`/`tlMid.EndTracingSpans` notation is
already correct; do NOT attempt a package-level path swap or the symbol will appear "missing".
`WithHTTPLogging`/`WithCustomLogger` (`:205`) → also `lib-observability/middleware` (verified
`middleware/logging.go:183`/`:64`). `FiberErrorHandler` (`:162`) STAYS in `v5/commons/net/http` (verified
present at v5.2.1 `net/http/...:109`). Split the single `libHTTP` alias into two: `libHTTP`
(v5 commons/net/http for FiberErrorHandler ONLY) + `libObsMiddleware` (lib-observability/middleware for
telemetry+logging). The split-alias acceptance must NOT require `FiberErrorHandler` from the obs
middleware package — it is not there. A naive path-swap will NOT compile — this is a real edit.
**Files:** tracer `internal/adapters/http/in/routes.go`, `handlers.go`, `readyz.go`.
**depends_on:** [P2c-T13, P2c-T14]
**Acceptance:** `routes.go` imports both `v5/commons/net/http` (FiberErrorHandler) and
`lib-observability/middleware` (NewTelemetryMiddleware/WithTelemetry/EndTracingSpans/WithHTTPLogging);
HTTP layer compiles; telemetry middleware chain order preserved (WithTelemetry → ... → EndTracingSpans).
**Tests:** `go build ./internal/adapters/http/...`; HTTP integration tests exercising the telemetry span
chain (run at T20).
**Effort:** M, 0.5–1 day. **risk_refs:** R13, R4.

### P2c-T19 — Drop residual indirect v5.3.0 / obs v1.0.0; tidy; assert no pseudo-versions; full build
**Description:** After all sweeps, run `go mod tidy`. Confirm the stale indirect `lib-commons/v5 v5.3.0`
collapses to the `v5.2.1` direct pin and indirect `lib-observability v1.0.0` rises to the `v1.0.1`
direct. Full `go build ./...` + `go vet ./...`. No `commons/{log,opentelemetry,zap}` imports anywhere.
Re-assert no pseudo-versions/branch refs on lib-commons/lib-observability/lib-auth (DAG-5). Update the
P2c-T23 record with the final resolved triple.
**Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`, `go.sum`.
**depends_on:** [P2c-T13, P2c-T14, P2c-T15, P2c-T16, P2c-T17, P2c-T18]
**Acceptance:** `go.mod`: `lib-commons/v5 v5.2.1`, `lib-observability v1.0.1`, `lib-auth/v2 v2.8.0`, no
v4, no stale v5.3.0 direct; `go list -m all` clean of pseudo-versions for the three libs; `go build ./...`
+ `go vet ./...` green;
`grep -rn 'commons/log\|commons/opentelemetry\|commons/zap' --include='*.go' . | grep -v ast-before` → empty.
**Tests:** `cd /Users/fredamaral/repos/lerianstudio/tracer && go mod tidy && go build ./... && go vet ./...`.
**Effort:** S, 2–3h. **risk_refs:** R4, R3.

### P2c-T20 — Jump-2 validation on tracer's OWN CI (lint + unit + integration + godog + sec)
**Description:** Full suite green on the post-split stack: `make lint`, `make test-unit`,
`make test-integration` (testcontainers Postgres), godog e2e, and `make sec` (gosec/trivy via
`mk/security.mk`, `.trivyignore`). Coverage compared against the T00 baseline (R11) on non-excluded
packages; the auth/bootstrap surface is validated by integration/MT/godog, not coverage. godog is the
test mode midaz CI does not run today — confirm it passes here in tracer's own CI before any CI fold
(R15); T24 owns the decision of how godog folds into midaz CI later.
**Files:** none (validation).
**depends_on:** [P2c-T19]
**Acceptance:** lint + unit + integration + godog + sec all green; coverage ≥ baseline on non-excluded
packages (or drops explained); telemetry span chain verified via integration tests.
**Tests:** `make lint && make test-unit && make test-integration && make sec` + godog target.
**Effort:** M, 0.5–1 day. **risk_refs:** R4, R11, R13, R15.

### P2c-T21 — Audit-hash-chain non-regression proof (R19, SOX/GLBA)
**Description:** The audit-event hash chain is integrity/compliance-sensitive. This phase touches ONLY
import paths and logger/telemetry wiring — migrations `000001/000002/000017` and the `VerifyHashChain`
logic in `audit_event_repository.go` must be byte-unchanged. Prove it: `git diff` shows ZERO changes to
`migrations/*.sql` and to the hash-chain functions; run the audit integration tests that exercise the
chain end-to-end on the migrated build.
**Files:** tracer `migrations/000001_*.sql`, `000002_*.sql`, `000017_*.sql`,
`internal/adapters/postgres/audit_event_repository.go`, `tests/integration/07_audit_events_test.go`,
`13_audit_event_dedup_test.go`, `17_audit_actor_test.go`, `09_bootstrap_migrations_test.go`,
`10_upgrade_path_test.go`.
**depends_on:** [P2c-T20]
**Acceptance:** `git diff -- migrations/` empty; no diff to `VerifyHashChain`/hash functions; audit
integration + e2e audit steps green on the migrated build.
**Tests:** `git -C /Users/fredamaral/repos/lerianstudio/tracer diff --stat -- migrations/` empty; run
`tests/integration/07_audit_events_test.go`, `13_*`, `17_*` + `tests/end2end/steps/audit_steps.go`.
**Effort:** S, 2–3h. **risk_refs:** R19.

### P2c-T24 — Decide godog CI ownership for the eventual midaz fold (resolve FG3)
**Description:** godog e2e runs in tracer's own CI today but midaz CI does not run a godog/cucumber test
mode. The decision of HOW godog folds into midaz CI later — reuse a shared-workflows step vs a bespoke
midaz workflow — has been repeatedly deferred with no owner. This task DECIDES it (it does not implement
the fold, which is a later CI-fold phase): document the cucumber dependency tree godog pulls, whether the
existing midaz shared-workflows (`go-combined-analysis.yml` et al.) can host a godog step or whether a
new workflow is required, and record the recommended approach + owner. This removes the dangling FG3
deferral so the CI-fold phase has a concrete decision to execute against.
**Files:** none (decision recorded in the phase handoff / open-items); reads tracer `mk/tests.mk`,
`tests/end2end/*`, `.github/workflows/*` and midaz `.github/workflows/*`.
**depends_on:** [P2c-T20]
**Acceptance:** A written decision: shared-vs-bespoke godog workflow for midaz, the cucumber dep tree it
introduces, and the named owner of the eventual fold. No code change in this phase.
**Tests:** N/A (decision).
**Effort:** S, 2–3h. **risk_refs:** R15.

### P2c-T22 — Open the two-commit PR(s) against tracer CI; confirm green; READY-TO-MOVE GATE
**Description:** Land the work as TWO commits/PRs (Jump 1 then Jump 2) so bisectability is preserved
(PD-6: observability split + co-location must not share a commit — here we also keep the major bump and
the split as separate commits). Jump 1 = v4→v5.1.3 + lib-auth v2.7.0 + shim delete; Jump 2 = v5.1.3→v5.2.1
+ obs split + lib-auth v2.8.0. Confirm tracer's shared-workflows CI (`build.yml`,
`go-combined-analysis.yml`, `pr-security-scan.yml`, `pr-validation.yml`) goes green on `develop`. **This
is the explicit READY-TO-MOVE GATE that downstream phases reference (DAG-3): P5-T00 (tracer move)
depends_on P2c-T22.** Tag the commit so the eventual filter-repo/move phase (PD-3) can reference it.
**Files:** none (CI/PR).
**depends_on:** [P2c-T21, P2c-T24]
**Acceptance:** Two distinct commits (Jump 1: v4→v5.1.3 + lib-auth v2.7.0 ; Jump 2: v5.1.3→v5.2.1 + obs +
lib-auth v2.8.0) merged to tracer `develop`; tracer CI green on both; tracer on `lib-commons/v5 v5.2.1` +
`lib-observability v1.0.1` + `lib-auth/v2 v2.8.0` with ZERO `lib-commons/v4` and ZERO
`commons/{log,opentelemetry,zap}` references; no shim/adapter/replace/fence anywhere in tracer; P2c-T23
triple recorded; tag applied. This task is the gate node other phases bind to.
**Tests:** tracer GitHub Actions CI run green on the merge commit(s).
**Effort:** M, 0.5 day. **risk_refs:** R3, R4, R10, R13, R15, R19.

---

## Exit criteria (phase complete when ALL hold)

1. tracer `go.mod`: `lib-commons/v5 v5.2.1`, `lib-observability v1.0.1` (direct), `lib-auth/v2 v2.8.0`;
   NO `lib-commons/v4`, NO stale indirect v5.3.0 direct, NO `lib-observability v1.1.0` beta; no
   pseudo-versions/branch refs on those three.
2. `grep -rn 'lib-commons/v4' --include='*.go' . | grep -v ast-before` → empty.
3. `grep -rn 'commons/log\|commons/opentelemetry\|commons/zap' --include='*.go' . | grep -v ast-before` → empty.
4. The dual-import shim (`libLogV5`/`libZapV5`/`buildAuthClientLogger`) is DELETED from ALL 4 files
   (config.go + the 3 auth test fixtures); repo-wide grep for those symbols (ast-before excluded) is
   empty; the stale `config.go:940`/`:1406` narration comments are gone; no shim, adapter, replace, or
   fence anywhere in tracer.
5. Two bisectable commits: Jump 1 (v4→v5.1.3 + lib-auth v2.7.0, green CI) then Jump 2 (v5.1.3→v5.2.1 +
   obs split + lib-auth v2.8.0, green CI).
6. `make lint && make test-unit && make test-integration && make sec` + godog e2e all green on tracer's
   own CI; coverage ≥ T00 baseline on non-excluded packages (bootstrap/cmd are coverage-excluded — their
   safety net is MT chaos integration + godog).
7. Audit-hash-chain migrations + `VerifyHashChain` byte-unchanged (R19); audit tests green.
8. P2c-T22 (ready-to-move gate) merged and green; the v5.2.1+v1.0.1+v2.8.0 triple is recorded (T23) and
   the godog-CI-fold decision is made (T24); tracer is dependency-identical to midaz's shared-lib posture
   and ready for the module rename + co-location phase (still NOT done here).

## Risks addressed

- **R3** (v4 cannot coexist with v5; no replace/shim): killed by the full v4→v5 migration + shim deletion
  across all 4 files (T04–T09) — single major, no bridge. The lib-auth v2.7.0→v2.8.0 jump-pinning
  (T02/T04/T11) is what makes the no-adapter shim deletion actually compilable under MVS.
- **R4** (observability split — 123 sites fail to compile in unified module): the entire Jump 2 (T11–T20).
- **R10** (tenant-manager v4→v5 API drift — highest uncertainty): dedicated API diff (T01) before any
  rewrite, applied + chaos-tested (T06, T10); coverage does NOT cover this surface, MT chaos tests do.
- **R11** (coverage gate): baseline captured on a GREEN anchor (T00). CORRECTION: the 85% hard-fail is a
  shared-workflows CI gate, not local `make`; `.ignorecoverunit` excludes `/bootstrap/` + `/cmd/app/`, so
  R11 does NOT protect the tenant-manager/auth surface — MT chaos integration + godog do (T10, T20).
- **R13** (telemetry middleware relocation won't compile on naive path-swap): treated as a real code move
  (T18), not a sweep; methods-on-`*TelemetryMiddleware` + FiberErrorHandler-stays-in-net/http both pinned.
- **R15** (godog is a test mode midaz CI doesn't run): exercised here in tracer's OWN CI at both gates
  (T10, T20); the fold-into-midaz decision is owned by T24.
- **R19** (audit-hash-chain SOX/GLBA): explicit non-regression proof (T21), pure relocation only.

## Open items / flags for the orchestrator

1. **v5.2.x GA target = v5.2.1** (not the bare v5.2.0). midaz is still on `v5.2.0-beta.12`; midaz's own
   bump to v5.2.1 GA is Phase 1 (P1-T06, PD-4). This phase pins tracer to the SAME v5.2.1 and records the
   exact resolved triple (T23) so the eventual single go.mod resolves cleanly. If P1-T06 picks a different
   GA (e.g. v5.3.x), re-pin tracer to match in a follow-up; T23 flags the divergence.
2. **lib-auth jump-pinning is NOT optional (CLOSED, was the micro-shim risk).** Jump 1 MUST run on
   lib-auth v2.7.0 (it holds lib-commons/v5 at v5.1.3 and accepts a `commons/log.Logger` natively) and
   Jump 2 MUST bump to v2.8.0 atomically with the obs split (v2.8.0 accepts `lib-observability/log.Logger`).
   This is the verified resolution to the original "two-jump bisectability could force a micro-shim" risk:
   there is no adapter, no fence, and no "confirm at execution time" escape hatch. Verified via
   `/tmp/mvscheck2` (v2.7.0 → v5.1.3) and `/tmp/mvscheck3` (v2.8.0 → v5.2.0-beta.12, past the split).
3. **godog in midaz CI:** this phase proves godog green in tracer's own CI and DECIDES the fold approach
   (T24), but standing godog up in midaz CI is a separate later-phase task (CI fold). T24 hands that phase
   a concrete shared-vs-bespoke decision + owner so it is not lost again.
4. **No relicense needed for tracer:** main.go already carries EL2.0 headers — R23 (relicense) is a
   fees-only concern, not tracer.
5. **ast-before snapshots:** excluded from every sweep AND every count here (T03); they were the source of
   the original 301/132 phantom count (real live count is 105/46). PD-3 excludes them on the eventual
   move. Confirm git-tracked status at execution so the exclusion strategy (prune vs gitignore-already) is
   correct.
6. **Coverage is NOT the safety net for the riskiest code.** `.ignorecoverunit` excludes `/bootstrap/` and
   `/cmd/app/` — exactly where T06 (tenant-manager) and T08 (auth wiring) live — and local `make` coverage
   is print-only (hard-fail is in shared-workflows CI). The orchestrator must treat MT chaos integration
   (`16_*`/`17_*`) and godog as the primary verification for those tasks, not R11.
