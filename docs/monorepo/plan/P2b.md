# Phase 2b — Reporter In-Place Dependency Migration

**Phase ID:** P2b
**Move:** #2 (reporter → `components/reporter-{manager,worker}`) — DEPENDENCY MIGRATION ONLY, performed IN reporter's own repo BEFORE any code move.
**Repo under change:** `/Users/fredamaral/repos/lerianstudio/reporter` (module `github.com/LerianStudio/reporter`, branch `develop`).
**Validated against:** reporter's OWN CI (`build.yml`, `go-combined-analysis.yml`, `pr-validation.yml`, `pr-security-scan.yml`, `release.yml`) and `make` targets.

## Objective

Bring reporter's dependency + observability posture to EXACTLY midaz's target stack while it still lives in its own repo, so the later co-location PR (Phase 2c, out of scope here) is a pure mechanical rename+fold with zero dependency churn. Scope, per locked decisions:

- Migrate observability: **241 import sites** (159 `commons/log` + 71 `commons/opentelemetry` + 11 `commons/zap`) → `lib-observability/{log,tracing,zap,middleware,metrics}` + root `lib-observability`. Same migration midaz landed in `766b555d2`.
- Migrate the lib-commons-ROOT observability surface that the v5.2.0 GA split actually DELETES — primarily `pkg/ctxutil/context.go` (the production context-propagation funnel feeding 239+ calls) and the root `NewTrackingFromContext` (195 test-side calls + 1 production call) — onto `lib-observability` root. This is the half of the split that breaks the compile, not just the `commons/*` subpackages.
- Bump `lib-auth/v2` v2.7.0 → v2.8.0 (its `NewAuthClient` logger param now wants `lib-observability/log.Logger`).
- Bump `lib-commons/v5` v5.1.3 → the **P1-frozen GA pin** (P1-T06 records `v5.2.0` primary, `v5.2.1` fallback).
- Drop the reporter `toolchain go1.26.2` directive; inherit `go 1.26.3`.
- Collapse dual mongo-driver (v1.17.9 + v2.5.0) onto midaz's choice (**v1.17.9**); v2 is used in exactly ONE test file.
- Keep manager (:4005 REST) and worker (:4006 Chromium renderer) behavior intact: RabbitMQ / Mongo / Redis / SeaweedFS unchanged.

## Ground-truth facts verified for this phase (real, not assumed — re-verified against the reporter tree and the v1.0.1/v5.2.0 module caches at revision time)

- reporter go.mod: `module github.com/LerianStudio/reporter`, `go 1.26`, `toolchain go1.26.2`, `lib-commons/v5 v5.1.3`, `lib-auth/v2 v2.7.0`, **no** `lib-observability` declared, mongo-driver **v1.17.9 AND v2.5.0**.
- Observability import sites confirmed by grep (ast-before snapshots excluded): `commons/log` 159 files, `commons/opentelemetry` 71, `commons/zap` 11. `lib-observability` usage today = **0**. Raw uber `go.uber.org/zap` = **2** sites (negligible — both test-only, in `template_test.go` / `deadline_test.go`; the dossier-09 "raw zap" claim was WRONG; reporter logs via `commons/log`).
- **`NewTrackingFromContext` = 195 call sites, ALL in exactly 10 `*_test.go` files** (re-verified by `grep -rl`): `components/worker/internal/services/{generate-report,generate-report-data,generate-report-render}_test.go`; `pkg/{circuit-breaker,datasource-config,health-checker,health-checker_metrics,health-checker_validation,recovery}_test.go`; `pkg/datasource/direct_provider_test.go`. Every one imports `libCommons "github.com/LerianStudio/lib-commons/v5/commons"` (root). **NONE of the 10 also imports a lifecycle symbol** (`RunApp`/`Launcher`/`SetConfigFromEnvVars`/`ValidateBusinessError`/`IsNilOrEmpty` — `grep -c` = 0 in every file). There is NO "mixed file / two-import split" — that framing in earlier drafts described files that do not exist. These 10 are pure single-alias swaps.
- **`pkg/ctxutil/context.go` is the production context-propagation funnel and the genuine compile-breaker.** It is a 77-line reporter-local wrapper exposing `NewLoggerFromContext`, `NewTracerFromContext`, `ContextWithLogger`, `ContextWithTracer`, `HeaderIDFromContext`. It imports `libCommons "lib-commons/v5/commons"` + `lib-commons/v5/commons/log`, and depends on `libCommons.CustomContextKey` + `libCommons.CustomContextKeyValue` (fields `.Logger`/`.Tracer`/`.HeaderID`) + `commons/log.{Logger,NopLogger}`. **All of these were DELETED in lib-commons v5.2.0 GA** (per P1.md ground-truth: GA removed `commons/context.go` holding `NewTrackingFromContext`/`NewLoggerFromContext`/`ContextWith*` and the `commons/log` package surface midaz no longer imports). So after the lib-commons GA bump (T09), `ctxutil/context.go` will FAIL TO COMPILE on undefined `libCommons.CustomContextKey`/`CustomContextKeyValue` and the moved `commons/log` types.
- **ctxutil call blast-radius = 62 production (non-test) files**; broken down by symbol: `HeaderIDFromContext` 125, `NewTracerFromContext` 85, `NewLoggerFromContext` 29 production call sites, plus `ContextWithLogger` 4 / `ContextWithTracer` 1. If `ctxutil` reads/writes a DIFFERENT context key than whatever the test-side `NewTrackingFromContext` (now lib-observability) writes, logger/tracer/HeaderID propagation breaks SILENTLY — this is the real "silent tracing-context break" risk, and it lives on the PRODUCTION path, not on the test-only Tracking sites.
- **One production-code `libCommons.NewLoggerFromContext` call exists OUTSIDE ctxutil**: `components/manager/internal/adapters/http/in/routes.go:150` (the `libCommons` there is `lib-commons/v5/commons` root). It must repoint to lib-observability root alongside the ctxutil migration.
- **lib-observability v1.0.1 provides the replacement surface (verified in the module cache):** root exports `NewTrackingFromContext`, `NewLoggerFromContext`, `ContextWithLogger`, `ContextWithTracer`, `ContextWithHeaderID`, `ContextWithMetricFactory`; the renamed key/struct `ContextKey` (a `var` of unexported `contextKey` type) and `ContextValue` (a struct whose `HeaderID string` / `Tracer trace.Tracer` / `Logger log.Logger` fields SURVIVE). `lib-observability/log` exports `Logger` and `NopLogger`. NOTE: the root does NOT export `NewTracerFromContext` or `HeaderIDFromContext` as functions — reporter's ctxutil wrapper keeps those two accessor functions but re-sources them onto `libObservability.ContextKey`/`*libObservability.ContextValue` (a direct key/struct re-source, NOT a shim — it is the exact pattern lib-observability uses internally; the 239 ctxutil callers keep their identical signatures).
- `HandleSpanError` / `HandleSpanBusinessErrorEvent` = **448 sites**, from `commons/opentelemetry` → `lib-observability/tracing`.
- Telemetry middleware: `routes.go:54 commonsHttp.NewTelemetryMiddleware(tl)` then `routes.go:56 f.Use(tlMid.WithTelemetry(tl))`, sourced from `lib-commons/v5/commons/net/http`. midaz moved this to `lib-observability/middleware.NewTelemetryMiddleware`. The `NewTelemetryMiddleware(telemetry)` + `tlMid.WithTelemetry(tl)` chain is CONFIRMED to exist intact on the new `lib-observability/middleware` type (midaz `unified-server.go:56-57`, ledger `routes.go:42-44`, crm `routes.go:36-40`). This is a path relocation + arg recheck, NOT a rewrite, and there is NO "method does not exist" fallback branch.
- mongo-driver/v2 used in EXACTLY ONE file: `pkg/itestkit/infra/mongodb/mongodb_test.go`. So the dual-driver collapse is small and confined to a test (R14 risk drops to near-zero).
- lib-auth/v2 middleware imported at 5 sites: `manager/.../bootstrap/config.go:26`, `manager/.../http/in/routes.go:18`, `worker/.../bootstrap/config_fetcher.go:23`, `pkg/auth/static_token_provider.go:12`, `..._test.go:14`. **lib-auth v2.8.0 signature: `NewAuthClient(address string, enabled bool, logger *log.Logger)` where `log` = `lib-observability/log`.** Reporter passes a NON-nil `&logger` at 2 PRODUCTION sites — `manager/.../bootstrap/config.go:591` and `worker/.../bootstrap/config_fetcher.go:102` — where `logger` is currently a `lib-commons/v5/commons/log.Logger`. Unlike midaz (which passes nil), reporter MUST convert those two `logger` vars to `lib-observability/log.Logger` or the auth call will not type-check. This is the auth-site closure point.
- Proxy verification: `lib-commons/v5` GA tags present = v5.2.0, v5.2.1, v5.3.x, v5.4.1. **P1-T06 pins the canonical downstream target at `v5.2.0` (first GA, minimal surface) with `v5.2.1` as documented fallback.** reporter follows that pin exactly. NOTE: `proxy.golang.org/.../@v/list` does NOT enumerate `v5.2.0` (it lists `v5.2.0-beta.13` then jumps to `v5.2.1`), but `@v/v5.2.0.info` returns HTTP 200 (v5.2.0 GA is real) — so the authoritative existence check is `go mod download …@<pin>`, NOT a grep of `/list`. `lib-observability` v1.0.1 present. `lib-auth/v2` v2.8.0 present.
- midaz canonical aliases (the target shape): `libLog "lib-observability/log"`, `libOpentelemetry "lib-observability/tracing"`, `libObservability "lib-observability"` (root), `libObsMiddleware "lib-observability/middleware"`. **Alias caveat:** midaz is internally INCONSISTENT — 142 non-test files alias the lib-observability root as `libCommons` while 73 still alias `lib-commons/v5/commons` as `libCommons`, so the token `libCommons` points at TWO modules across midaz. **Reporter must NOT inherit that collision** (see the alias rule below).
- midaz's own go.mod is STILL on `lib-commons/v5 v5.2.0-beta.12` at this snapshot — Phase 1 bumps it to the GA pin (P1-T06). reporter's `lib-commons` target pin is therefore the SAME GA tag Phase 1 lands on (cross-phase dependency: P2b-T01 depends_on **P1-T06**).
- reporter Dockerfiles build `FROM golang:1.26-alpine` (manager + worker). Worker is fat alpine + Chromium. Build-base bump to 1.26.3 is in-repo and belongs here; full COPY-path/build-context rewrite for nesting is Phase 2c (out of scope).

## Alias rule (NO-SHIMS / no-misleading-alias — binding for the whole phase)

- Lifecycle stays `libCommons "github.com/LerianStudio/lib-commons/v5/commons"`.
- Observability root is `libObservability "github.com/LerianStudio/lib-observability"`.
- **It is FORBIDDEN to alias `github.com/LerianStudio/lib-observability` as `libCommons` (or any lifecycle-suggesting name) anywhere in reporter.** Reporter is a clean slate; do not replicate midaz's `libCommons→lib-observability` debt. (Reporter is the convention midaz should later converge toward.)
- Subpackage aliases mirror midaz: `libLog "lib-observability/log"`, `libOpentelemetry "lib-observability/tracing"`, `libZap "lib-observability/zap"`, `libObsMiddleware "lib-observability/middleware"`.
- Acceptance gate (enforced in T12): `grep -rn 'libCommons "github.com/LerianStudio/lib-observability"' --include='*.go' | grep -v ast-before` returns ZERO.

## Sequencing note

Per PD-6, the lib-commons GA target must EXIST before reporter is rewritten to it. The verification task (P2b-T01) is gated on the Phase 1 GA-bump exit gate (**P1-T06**). Observability migration is split into the mechanical `commons/*` subpackage sweep (log/tracing/zap), the harder middleware + Tracking-root moves, AND the production-critical `ctxutil` root-symbol relocation, so a regression is bisectable to a single step. The whole phase lands as a sequence of small PRs against reporter's own CI; the **observability migration and the co-location move MUST NOT share a commit** (that boundary is Phase 2c's first commit, not here — per PD-6, obs + co-location never share a commit).

## Intra-phase task DAG

```
P1-T06 (cross-phase: midaz GA pin recorded) ─> P2b-T01 ─> P2b-T02 ─> P2b-T03 ─> P2b-T04 ─┐
                                                                                          ├─> P2b-T05 ─> P2b-T06 ─> {P2b-T07, P2b-T08}
                                                                                          │                                  │
                                                                       (T05 also gates T08b) ────────────────────────────────┤
P2b-T08b (ctxutil root-symbol relocation) depends on P2b-T05 ─────────────────────────────────────────────────────────────────┘
P2b-T09 (lib-commons GA bump; full tree compiles) fans in T04,T05,T06,T07,T08,T08b
P2b-T09 ─> {P2b-T10, P2b-T11, P2b-T14}
P2b-T12 (full CI) fans in T09,T10,T11
P2b-T12 ─> P2b-T13 (e2e PDF smoke)
P2b-T13 + P2b-T14 ── BOTH are the phase EXIT GATE (Phase 2c.start depends_on P2b-T13 AND P2b-T14)
```

---

## Tasks

### P2b-T01 — Verify lib-commons GA target + lock reporter's pin to P1-T06
**Description:** Confirm against the Go module proxy that the P1-frozen GA tag of `lib-commons/v5` is resolvable, and adopt EXACTLY the pin P1-T06 recorded (do not independently re-derive or pre-judge it). P1-T06 records `v5.2.0` as the primary canonical downstream pin with `v5.2.1` as the documented fallback; reporter's later co-location merges into midaz's go.mod, so the two MUST match. Read the pin from P1's "Frozen target pin" record, then verify it downloads. Also confirm `lib-observability v1.0.1` and `lib-auth/v2 v2.8.0` resolve on the proxy (verified present at plan time). Do NOT pin a higher v5.3/v5.4 line — divergence re-creates skew.
**Files:** none (verification only); records the chosen pin into this plan + Phase 1 handoff.
**Depends on:** P1-T06 (Phase 1 exit gate — the midaz GA bump is recorded as the canonical downstream pin; cross-phase, DAG-4).
**Acceptance criteria:**
- The `lib-commons/v5` pin string is read from P1-T06's recorded "Frozen target pin" and equals midaz's post-Phase-1 pin (primary `v5.2.0`, fallback `v5.2.1` if v5.2.0 is yanked).
- `lib-observability v1.0.1` and `lib-auth/v2 v2.8.0` confirmed downloadable.
- No beta tag is selected for any of the three.
**Tests:** `go mod download github.com/LerianStudio/lib-commons/v5@<pin>` succeeds in a scratch dir (authoritative existence check — hits `@v/<pin>.info`); same for `lib-observability@v1.0.1` and `lib-auth/v2@v2.8.0`. Do NOT gate on `curl …/@v/list | grep` — the `/list` endpoint omits `v5.2.0` even though the tag is real, so a `/list` grep would spuriously fail.
**Effort:** S / 1-2h.
**Risk refs:** R3, R4.

### P2b-T02 — Establish reporter migration baseline (green CI snapshot + full moved-symbol inventory)
**Description:** On a fresh `feature/dep-migration` branch off reporter `develop`, capture the pre-migration baseline so every later step is diffable: run reporter's full CI-equivalent locally (`make lint`, `make test-unit`, `make sec`) and record pass/fail + current coverage totals (reporter enforces a coverage gate via `scripts/coverage.sh` + `.ignorecoverunit`). Generate the authoritative import-site inventory AND the moved/renamed-root-symbol inventory into a working file so subsequent tasks have an exact checklist. The inventory MUST enumerate the FULL set of lib-commons-root symbols that v5.2.0 GA removed/renamed — not just `NewTrackingFromContext` — because a partial enumeration is how the compile-breaker (`ctxutil`) was missed in earlier drafts.

The required inventory lists:
- `commons/log` (159), `commons/opentelemetry` (71, incl. 448 `HandleSpanError`/`HandleSpanBusinessErrorEvent`), `commons/zap` (11), raw `go.uber.org/zap` (2, test-only), mongo-driver-v2 (1), lib-auth wiring (5).
- `NewTrackingFromContext` (195) — confirm all 10 host files are `*_test.go` and carry NO lifecycle-symbol overlap.
- **The lib-commons-ROOT symbol set deleted in GA:** every reference to `libCommons.CustomContextKey`, `libCommons.CustomContextKeyValue`, `libCommons.NewLoggerFromContext`, `libCommons.NewTracerFromContext`, `libCommons.ContextWithLogger`, `libCommons.ContextWithTracer`, `libCommons.ContextWithHeaderID`, `libCommons.ContextWithMetricFactory`. Confirmed at plan time: `CustomContextKey`/`CustomContextKeyValue` appear ONLY in `pkg/ctxutil/context.go` (+ its `context_test.go`); the production `NewLoggerFromContext` appears at `routes.go:150` outside ctxutil; ctxutil itself is consumed by 62 production files (125 `HeaderIDFromContext` + 85 `NewTracerFromContext` + 29 `NewLoggerFromContext` + 4 `ContextWithLogger` + 1 `ContextWithTracer` production call sites).
This is the bisect anchor.
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/` (new branch, no source change); working inventory recorded in PR description.
**Depends on:** P2b-T01.
**Acceptance criteria:**
- Branch created off `develop`; baseline `make lint && make test-unit && make sec` result recorded (green expected; any pre-existing red is documented and frozen as the comparison baseline).
- Site inventory recorded matching the verified counts above (deviations investigated, not silently accepted).
- The moved-root-symbol inventory explicitly enumerates `CustomContextKey`/`CustomContextKeyValue` (→ `pkg/ctxutil/context.go`), the `routes.go:150` production `NewLoggerFromContext`, and the 62 ctxutil-caller blast radius — these are the T08b checklist.
- ast-before snapshot dirs (`docs/codereview/ast-before-*`) confirmed git-ignored + untracked (do not migrate them).
**Tests:** `make test-unit` (reporter) green; `git check-ignore docs/codereview/ast-before-3807876316` returns the path; the inventory file lists ≥1 `CustomContextKey` reference and the `routes.go:150` site.
**Effort:** S / 2-3h.
**Risk refs:** R11.

### P2b-T03 — Add lib-observability v1.0.1 to reporter go.mod; bump lib-auth v2.7→v2.8
**Description:** Edit reporter `go.mod`: add `require github.com/LerianStudio/lib-observability v1.0.1`; bump `lib-auth/v2` v2.7.0 → v2.8.0. Do NOT bump lib-commons yet (that is P2b-T09, after the observability surface is migrated, to keep the failing-compile surface attributable). Run `go mod download` only — the tree will NOT compile yet (expected: code still imports `commons/log`/`commons/opentelemetry`/`commons/zap` which still exist at v5.1.3, and the lib-auth `&logger` args are now a HARD type mismatch — `NewAuthClient` wants `*lib-observability/log.Logger` but reporter still passes `*commons/log.Logger`; that mismatch only clears once T04 migrates the logger vars). This task only seeds the module graph.
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/go.mod`, `/Users/fredamaral/repos/lerianstudio/reporter/go.sum`.
**Depends on:** P2b-T02.
**Acceptance criteria:**
- `lib-observability v1.0.1` and `lib-auth/v2 v2.8.0` present in go.mod; `go.sum` updated.
- `go mod download` succeeds.
- lib-commons still v5.1.3 (unchanged at this step).
**Tests:** `go mod download all` succeeds; `go list -m github.com/LerianStudio/lib-observability` prints `v1.0.1`.
**Effort:** S / 1h.
**Risk refs:** R4.

### P2b-T04 — Migrate `commons/log` → `lib-observability/log` (159 sites) + convert the 2 lib-auth `&logger` vars
**Description:** Rewrite all 159 import sites of `github.com/LerianStudio/lib-commons/v5/commons/log` to `github.com/LerianStudio/lib-observability/log`. The symbol surface is byte-identical to midaz's usage (`log.Logger`, `log.LevelInfo/Warn/Error/Debug`, `log.String`, `log.Err`, `log.Any`, `log.Int`, `logger.Log(ctx, level, msg, fields...)` — verified in `manager/.../bootstrap/init_helpers.go`). Preserve each file's existing alias form (most are bare `log`, some `libLog`). Use a scripted gofmt-aware rewrite (`gofmt -r` / goimports re-alias) per the `ring-dev-team:migrate-observability` pattern, then human-review the diff. Do NOT touch tracing/zap/middleware/Tracking in this task.

**Auth-site logger-var conversion (in-scope here, a distinct attributable step):** the manager + worker bootstrap `logger` vars feeding `middleware.NewAuthClient(..., &logger)` at `manager/.../bootstrap/config.go:591` and `worker/.../bootstrap/config_fetcher.go:102` MUST become `lib-observability/log.Logger` as part of this sweep. lib-auth v2.8.0's `NewAuthClient` takes `*lib-observability/log.Logger`; converting these two vars here (alongside the file's own `commons/log`→`lib-observability/log` swap) is what makes the auth call type-check once T09 lands. Keeping it explicit makes a failing auth-site compile bisectable to a known cause, not mistaken for v5.1→v5.2 API drift.
**Files:** 159 `.go` files across `components/manager/`, `components/worker/`, `pkg/` (the `commons/log` import set from P2b-T02 inventory) — explicitly INCLUDING `components/manager/internal/bootstrap/config.go` and `components/worker/internal/bootstrap/config_fetcher.go` for the auth-site `logger` var.
**Depends on:** P2b-T03.
**Acceptance criteria:**
- Zero remaining `lib-commons/v5/commons/log` imports (`grep -rl` returns 0, excluding ast-before).
- Logger call signatures unchanged (no behavioral edit, pure import-path + alias swap).
- The `logger` var passed to `NewAuthClient` at `config.go:591` / `config_fetcher.go:102` is typed `lib-observability/log.Logger`.
- Package compiles for all files whose ONLY observability import was `commons/log` (files also importing opentelemetry/zap, or `ctxutil`, may still fail until T05/T06/T08b — that is expected and scoped).
**Tests:** `grep -rl 'lib-commons/v5/commons/log' --include='*.go' | grep -v ast-before | wc -l` == 0; `go build ./...` error set contains only opentelemetry/zap/tracking/ctxutil-related failures, no `commons/log` failures.
**Effort:** M / 0.5-1 day.
**Risk refs:** R4.

### P2b-T05 — Migrate `commons/opentelemetry` → `lib-observability/tracing` (71 sites, incl. 448 span-helper calls)
**Description:** Rewrite all 71 import sites of `lib-commons/v5/commons/opentelemetry` to `lib-observability/tracing`, preserving each file's alias (`libOpentelemetry` or `libOtel`). The high-frequency symbols (`HandleSpanError` 448 total, `HandleSpanBusinessErrorEvent`, `Telemetry`, `TelemetryConfig`, `NewTelemetry`, `SetSpanAttributesFromStruct`, span start/end helpers) survive intact under `lib-observability/tracing` — confirmed by midaz's identical usage (alias `libOpentelemetry`). EXCLUDE the telemetry-middleware wiring in `routes.go` — that is the genuine code move handled in P2b-T07 (do not naively path-swap it here; it comes from `net/http`, not `opentelemetry`). Scripted rewrite + human review of the span-error sites.
**Files:** 71 `.go` files (the `commons/opentelemetry` import set), including `manager/.../bootstrap/{server.go,init_helpers.go}`, `manager/.../adapters/{redis/consumer.redis.go,rabbitmq/producer.rabbitmq.go,http/in/*.go}`, worker equivalents.
**Depends on:** P2b-T04.
**Acceptance criteria:**
- Zero remaining `lib-commons/v5/commons/opentelemetry` imports.
- All `HandleSpanError` / `HandleSpanBusinessErrorEvent` / `NewTelemetry` call sites resolve against `lib-observability/tracing` with unchanged arguments.
- No behavioral change to span lifecycle (defer span.End() patterns preserved; child-span ctx handling untouched).
**Tests:** `grep -rl 'lib-commons/v5/commons/opentelemetry' --include='*.go' | grep -v ast-before | wc -l` == 0; `go vet ./...` shows no undefined `libOpentelemetry.*` symbols.
**Effort:** M / 1-1.5 days.
**Risk refs:** R4, R13.

### P2b-T06 — Migrate `commons/zap` → `lib-observability/zap` (11 sites) + fold the 2 raw uber-zap sites
**Description:** Rewrite the 11 `lib-commons/v5/commons/zap` import sites to `lib-observability/zap`, preserving aliases (mix of bare `zap` and `libZap`). Audit the 2 raw `go.uber.org/zap` import sites (both test-only — `template_test.go` / `deadline_test.go`) — if they are constructing loggers that should flow through lib-observability, align them with midaz's pattern; if they are legitimately low-level (e.g. a zapcore option), leave them and document why **as a code comment** (not merely a PR note — the rationale must survive the Phase 2c co-location move, which a PR note would not). Confirm no behavioral logging-format change.
**Files:** 11 `.go` files (the `commons/zap` import set) + the 2 `go.uber.org/zap` sites.
**Depends on:** P2b-T05.
**Acceptance criteria:**
- Zero remaining `lib-commons/v5/commons/zap` imports.
- The 2 raw-zap sites are either migrated or explicitly justified in a one-line CODE comment at the import.
- Logging output format unchanged (spot-check a manager startup log line vs baseline).
**Tests:** `grep -rl 'lib-commons/v5/commons/zap' --include='*.go' | grep -v ast-before | wc -l` == 0; manager boots and emits the same structured log shape as baseline (smoke).
**Effort:** S / 2-4h.
**Risk refs:** R4.

### P2b-T07 — Relocate telemetry middleware to `lib-observability/middleware` (the genuine code move)
**Description:** Rewrite the telemetry-middleware wiring. Today: `routes.go:54 commonsHttp.NewTelemetryMiddleware(tl)` + `routes.go:56 f.Use(tlMid.WithTelemetry(tl))`, where `commonsHttp` = `lib-commons/v5/commons/net/http` (HTTP helpers + telemetry middleware bundled). midaz split this: HTTP helpers stay in `commons/net/http`; telemetry middleware moved to `lib-observability/middleware`. The CONFIRMED target shape (verified in midaz `unified-server.go:56-57`, ledger `routes.go:42-44`, crm `routes.go:36-40`) is:

```go
tlMid := libObsMiddleware.NewTelemetryMiddleware(tl)
f.Use(tlMid.WithTelemetry(tl))
```

The `WithTelemetry` chaining method EXISTS on the new `lib-observability/middleware` type — there is NO "if it does not exist" branch, and **no wrapper/bridge is permitted**. Re-point ONLY the telemetry-middleware construction to `lib-observability/middleware` (alias `libObsMiddleware`); keep any genuine `commons/net/http` HTTP helpers on lib-commons. This is NOT a path swap — a naive `opentelemetry`-style path swap won't compile because the source package changed from `net/http` to `lib-observability/middleware`.
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/components/manager/internal/adapters/http/in/routes.go` (+ any worker health-server telemetry wiring if present).
**Depends on:** P2b-T05.
**Acceptance criteria:**
- Telemetry middleware constructed via `lib-observability/middleware.NewTelemetryMiddleware`, aliased `libObsMiddleware` (NOT `libCommons`).
- Fiber middleware chain (`f.Use(tlMid.WithTelemetry(tl))`) compiles and preserves the same request-telemetry behavior (spans created per request as before).
- Any remaining `commons/net/http` usage is genuine HTTP-helper use, not telemetry.
- No wrapper/bridge type introduced.
**Tests:** `go build ./components/manager/...` green for routes.go; integration smoke: a manager request produces a server span with the same attributes as baseline (compare against otel exporter / log).
**Effort:** M / 0.5 day.
**Risk refs:** R13, R4.

### P2b-T08 — Repoint the 195 `NewTrackingFromContext` test sites + the routes.go:150 production site to lib-observability root
**Description:** Repoint every `NewTrackingFromContext` call from the `lib-commons/v5/commons` root to the `lib-observability` root. GROUND TRUTH (re-verified, correcting earlier drafts): all 195 `NewTrackingFromContext` calls live in exactly 10 `*_test.go` files, and NONE of those 10 files also imports a lifecycle symbol — there is NO "mixed file / two-import split", that framing described files that do not exist. Each of the 10 is a clean single-alias swap: change `libCommons "github.com/LerianStudio/lib-commons/v5/commons"` → `libObservability "github.com/LerianStudio/lib-observability"` and repoint `libObservability.NewTrackingFromContext`.

Separately repoint the ONE production-code `libCommons.NewLoggerFromContext(ctx)` call at `components/manager/internal/adapters/http/in/routes.go:150` to the lib-observability root (`libObservability.NewLoggerFromContext`). (The deeper production funnel — `pkg/ctxutil/context.go` and its 62 callers — is owned by P2b-T08b, NOT this task.)

The "silent tracing-context propagation break" risk does NOT live here (these are clean single-alias test swaps + one production accessor call); it lives in T08b. Lifecycle production files (`main.go`, `server.go`, `init_helpers.go`, `config_logger.go` — which use `libCommons.Launcher`/`SetConfigFromEnvVars`) keep their `lib-commons` import UNTOUCHED and contain NO `NewTrackingFromContext`, so they are not part of this task.
**Files:** the 10 test files — `components/worker/internal/services/{generate-report,generate-report-data,generate-report-render}_test.go`; `pkg/{circuit-breaker,datasource-config,health-checker,health-checker_metrics,health-checker_validation,recovery}_test.go`; `pkg/datasource/direct_provider_test.go` — plus `components/manager/internal/adapters/http/in/routes.go` (the single production `NewLoggerFromContext` site).
**Depends on:** P2b-T05, P2b-T06.
**Acceptance criteria:**
- Zero `NewTrackingFromContext` calls resolve against `lib-commons`; all 195 resolve against `lib-observability` root (aliased `libObservability`, never `libCommons`).
- `routes.go:150`'s `NewLoggerFromContext` resolves against `lib-observability` root.
- No `lib-observability` import is aliased `libCommons` in any touched file.
**Tests:** `grep -rn 'libCommons.NewTrackingFromContext\|libCommons.NewLoggerFromContext' --include='*.go' | grep -v ast-before | wc -l` == 0; `go build ./...` resolves all `NewTrackingFromContext`/`NewLoggerFromContext` against lib-observability (residual failures only from ctxutil until T08b / from lib-commons surface until T09).
**Effort:** S / 2-4h.
**Risk refs:** R4, R13.

### P2b-T08b — Relocate the lib-commons-ROOT observability funnel `pkg/ctxutil/context.go` (and its 62 production callers) onto lib-observability root
**Description:** THE production-critical compile-breaker and the single biggest gap in earlier drafts. `pkg/ctxutil/context.go` is reporter's local context-propagation wrapper (`NewLoggerFromContext`, `NewTracerFromContext`, `ContextWithLogger`, `ContextWithTracer`, `HeaderIDFromContext`) consumed by **62 production files** (125 `HeaderIDFromContext` + 85 `NewTracerFromContext` + 29 `NewLoggerFromContext` + 4 `ContextWithLogger` + 1 `ContextWithTracer` production calls). It depends on `libCommons.CustomContextKey` + `libCommons.CustomContextKeyValue` (fields `.Logger`/`.Tracer`/`.HeaderID`) + `commons/log.{Logger,NopLogger}` — ALL DELETED in lib-commons v5.2.0 GA. So at T09 (lib-commons GA bump) this file FAILS TO COMPILE on undefined symbols. T08 does NOT touch it; without this task the tree never compiles and the entire downstream chain (T09→T12→T13) cannot run.

**Rewrite plan (re-source the key/struct directly — NOT a shim):** lib-observability v1.0.1 root exports the RENAMED equivalents: `ContextKey` (var) and `ContextValue` (struct, with `HeaderID string` / `Tracer trace.Tracer` / `Logger log.Logger` fields surviving). Rewrite `pkg/ctxutil/context.go` to:
- import `libObservability "github.com/LerianStudio/lib-observability"` and `"github.com/LerianStudio/lib-observability/log"` (drop both `lib-commons/v5/commons` and `lib-commons/v5/commons/log`);
- replace `libCommons.CustomContextKey` → `libObservability.ContextKey`;
- replace `*libCommons.CustomContextKeyValue` / `&libCommons.CustomContextKeyValue{}` → `*libObservability.ContextValue` / `&libObservability.ContextValue{}` (the `.Logger`/`.Tracer`/`.HeaderID` field reads/writes stay byte-identical);
- replace `commons/log.Logger` / `&commons/log.NopLogger{}` → `lib-observability/log.Logger` / `&lib-observability/log.NopLogger{}`.

The reporter-local accessor functions (`NewTracerFromContext`, `HeaderIDFromContext`, etc.) KEEP their exact signatures — the 62 callers do not change. Note the root does NOT export `NewTracerFromContext`/`HeaderIDFromContext` as functions, so the ctxutil wrapper legitimately stays as reporter's accessor layer reading the new `ContextKey`/`ContextValue`; this is the same pattern lib-observability uses internally, NOT a compatibility shim. Update `pkg/ctxutil/context_test.go` to the new key/struct.

**Critical correctness check (the real "silent tracing-context break"):** the key that ctxutil READS must be the SAME key that lib-observability's `NewTrackingFromContext` / `ContextWith*` WRITE. Since both now use `libObservability.ContextKey`, that holds — but it MUST be proven by a propagation regression test: set logger+tracer+HeaderID into a context via the production write path, read them back through ctxutil's accessors, and assert identity. Run this test BEFORE and AFTER the rewrite on the production path (not the test-only Tracking sites).
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/pkg/ctxutil/context.go`, `pkg/ctxutil/context_test.go`. Verification blast-radius: the 62 production callers (must compile + preserve propagation) — chiefly under `components/{manager,worker}/internal/adapters/{redis,rabbitmq}`, `pkg/{datasource,fetcher,mongodb,postgres,storage,seaweedfs}`.
**Depends on:** P2b-T05.
**Acceptance criteria:**
- `pkg/ctxutil/context.go` imports `lib-observability` root + `lib-observability/log`; ZERO `lib-commons/v5/commons` or `commons/log` imports remain in the file.
- ZERO references to `libCommons.CustomContextKey` / `libCommons.CustomContextKeyValue` / `commons/log.NopLogger` anywhere in reporter after this task.
- The 5 ctxutil accessor functions keep their signatures; the 62 callers compile unchanged.
- A propagation regression test asserts logger+tracer+HeaderID round-trip through `libObservability.ContextKey`/`ContextValue` (production path), green before/after.
- `lib-observability` is imported as `libObservability`, never `libCommons`.
**Tests:** `grep -rn 'CustomContextKey\|CustomContextKeyValue' --include='*.go' . | grep -v ast-before | wc -l` == 0; `go build ./pkg/ctxutil/... ./pkg/datasource/... ./pkg/fetcher/... ./pkg/mongodb/... ./pkg/storage/... ./pkg/seaweedfs/... ./components/...` resolves all ctxutil callers (residual failures only from the not-yet-bumped lib-commons surface until T09); the ctxutil propagation round-trip unit test passes.
**Effort:** M / 0.5 day.
**Risk refs:** R4, R13.

### P2b-T09 — Bump lib-commons v5.1.3 → GA pin; full tree compiles
**Description:** With every `commons/{log,opentelemetry,zap}` import removed (T04-T06), telemetry middleware relocated (T07), Tracking root repointed (T08), and the `ctxutil` root funnel relocated (T08b), bump `lib-commons/v5` v5.1.3 → the GA pin from P2b-T01. The packages/symbols removed in v5.2.0 GA (`commons/{log,opentelemetry,zap}`, `commons/context.go` holding `NewTrackingFromContext`/`CustomContextKey*`/`ContextWith*`, the bundled telemetry middleware) are no longer imported, so this is now a clean version bump. Run `go get github.com/LerianStudio/lib-commons/v5@<pin>` then `go mod tidy`. Resolve any non-observability v5.1→v5.2 API drift (constructor-option/config-struct shifts — expected minor, none on the removed-package surface). The lib-auth v2.8.0 `&logger` args already became `lib-observability/log.Logger` in T04, so the 5 lib-auth wiring sites now type-check; verify the constructor constructs without type error.
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/go.mod`, `go.sum`; any files needing v5.1→v5.2 API-drift fixes (lib-auth wiring: `manager/.../bootstrap/config.go`, `manager/.../http/in/routes.go`, `worker/.../bootstrap/config_fetcher.go`, `pkg/auth/static_token_provider.go`).
**Depends on:** P2b-T04, P2b-T05, P2b-T06, P2b-T07, P2b-T08, P2b-T08b.
**Acceptance criteria:**
- `lib-commons/v5` pinned to the GA target (== P1-T06 pin); `go mod tidy` clean.
- ENTIRE tree compiles: `go build ./...` green.
- The single closure point for lib-auth is the LOGGER TYPE IDENTITY: the `*log.Logger` passed to `NewAuthClient` at all sites is `*lib-observability/log.Logger` (not `*lib-commons/v5/commons/log.Logger`); auth middleware constructs without type error.
- No `lib-observability v1.1.0-beta.*` or any beta of lib-commons/lib-observability/lib-auth leaks into go.mod.
**Tests:** `go build ./...` green; `go list -m github.com/LerianStudio/lib-commons/v5` == GA pin; `make lint` (reporter) passes.
**Effort:** M / 0.5-1 day.
**Risk refs:** R3, R4.

### P2b-T10 — Collapse dual mongo-driver onto v1.17.9 (rewrite the single v2 test file)
**Description:** Reporter pulls both `go.mongodb.org/mongo-driver v1.17.9` and `go.mongodb.org/mongo-driver/v2 v2.5.0`. midaz uses ONLY v1.17.9. The v2 driver is imported in EXACTLY ONE file — `pkg/itestkit/infra/mongodb/mongodb_test.go` (a testkit integration helper). Rewrite that file's v2 BSON/client API to the v1.17.9 API (the v1 surface is what all production-side files already use), then `go mod tidy` to drop `go.mongodb.org/mongo-driver/v2` from go.mod entirely. This neutralizes R14: the dual-driver concern was BSON/codec drift at runtime, but v2 never touches a runtime path — only this test helper.
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/pkg/itestkit/infra/mongodb/mongodb_test.go`, `go.mod`, `go.sum`.
**Depends on:** P2b-T09.
**Acceptance criteria:**
- `go.mongodb.org/mongo-driver/v2` absent from go.mod after `go mod tidy`.
- The testkit mongodb helper compiles + runs against v1.17.9 API.
- No production file references mongo-driver/v2 (was already true; assert it stays true).
**Tests:** `grep -rl 'mongo-driver/v2' --include='*.go' | grep -v ast-before | wc -l` == 0; `go list -m go.mongodb.org/mongo-driver/v2` returns "not a known dependency"; the testkit's integration test (testcontainers mongo) passes.
**Effort:** S-M / 3-5h.
**Risk refs:** R14.

### P2b-T11 — Drop the `toolchain` directive; align go directive + Docker build base to 1.26.3
**Description:** Remove the `toolchain go1.26.2` line from reporter go.mod and set the `go` directive to `1.26.3` (matching the unified module). Update the Docker builder base in both `components/manager/Dockerfile` and `components/worker/Dockerfile` from `FROM golang:1.26-alpine` to the 1.26.3 base used by midaz (verify midaz's exact builder tag and match it). This is in-repo because reporter's own CI builds these images. Do NOT rewrite COPY paths / build context for nesting — that is Phase 2c (the move). Worker stays fat alpine + Chromium (unchanged).
**Files:** `/Users/fredamaral/repos/lerianstudio/reporter/go.mod`, `components/manager/Dockerfile`, `components/worker/Dockerfile`.
**Depends on:** P2b-T09.
**Acceptance criteria:**
- No `toolchain` directive in go.mod; `go` directive == `1.26.3`.
- Both Dockerfiles build on the 1.26.3 base.
- `go build ./...` green on 1.26.3.
**Tests:** `grep -c '^toolchain' go.mod` == 0; `grep '^go ' go.mod` == `go 1.26.3`; `docker build -f components/manager/Dockerfile .` and worker equivalent succeed.
**Effort:** S / 2-3h.
**Risk refs:** R3.

### P2b-T12 — Full reporter CI validation: lint, unit, integration, security, coverage gate + alias-hygiene gate
**Description:** Run reporter's complete CI surface on the migrated branch and prove parity with the P2b-T02 baseline. Pin the exact reporter integration target name before running (reporter's Makefile exposes `set-env`/`lint`/`sec`/`format`/`imports` cleanly; confirm the integration/testcontainers target name — do not assume `make test-integration`). Run `make lint` (golangci against reporter's `.golangci.yml`), `make test-unit`, the integration/testcontainers suite (manager + worker, incl. the mongodb testkit from T10), `make sec` (gosec + govulncheck), and the coverage gate (`scripts/coverage.sh` vs `.ignorecoverunit`). Any new lint findings from the observability rewrite (e.g. `dogsled` on `NewTrackingFromContext` 4-tuple — midaz uses `//nolint:dogsled`) are resolved to match midaz's conventions, not suppressed wholesale.

**Alias-hygiene gate (NO-SHIMS enforcement, binding):** assert reporter never aliases `lib-observability` as `libCommons`. This is a hard gate — the whole point of doing reporter clean is not inheriting midaz's `libCommons→lib-observability` collision.
**Files:** none (validation); fixes land in the relevant migrated files if CI surfaces drift.
**Depends on:** P2b-T09, P2b-T10, P2b-T11.
**Acceptance criteria:**
- `make lint`, `make test-unit`, `make sec` all green (or no worse than the documented baseline).
- Integration suite green (under the pinned target name): manager `/health` + `/readyz`, worker consumes `generate-report`.
- Coverage gate met (≥ reporter's configured threshold; backfill tests if the observability rewrite dropped covered lines).
- `govulncheck` shows no NEW vulnerabilities introduced by lib-observability / lib-auth / lib-commons GA bumps.
- `grep -rn 'libCommons "github.com/LerianStudio/lib-observability"' --include='*.go' . | grep -v ast-before` returns ZERO.
**Tests:** `make lint && make test-unit && make sec` (reporter); the pinned integration target green; coverage report ≥ baseline; the alias-hygiene grep returns 0.
**Effort:** M / 0.5-1 day (more if coverage backfill needed).
**Risk refs:** R11, R4.

### P2b-T13 — End-to-end pipeline smoke: PDF render proves manager↔worker↔infra intact (PHASE EXIT GATE, with T14)
**Description:** Final behavioral proof that the dependency migration changed nothing operationally. Stand up reporter's stack (manager + worker + Mongo + Redis + RabbitMQ + SeaweedFS) via reporter's own docker-compose, submit a report request to manager :4005, and confirm the worker consumes from `generate-report`, fetches a datasource, renders via pongo2 → chromedp Chromium → PDF, stores to SeaweedFS, updates Mongo status, and the report downloads. This exercises the full observability path (spans + logs) under the new lib-observability stack — including the relocated `ctxutil` funnel from T08b — and the collapsed mongo-driver. Compare emitted spans/logs against baseline shape, with explicit attention to span PARENTAGE on the production path (the ctxutil-extracted tracer must produce the same parent-child chain as baseline).
**Files:** none (runtime validation); uses reporter's existing `components/{infra,manager,worker}/docker-compose.yml`.
**Depends on:** P2b-T12.
**Acceptance criteria:**
- A report submitted to manager renders end-to-end to a downloadable PDF.
- Worker logs/spans emit through lib-observability (no `commons/log` artifacts), same structured shape as baseline; span parent-child chain matches baseline at a sampled production request path.
- RabbitMQ / Mongo / Redis / SeaweedFS wiring unchanged; no port change (4005/4006).
**Tests:** scripted e2e: `POST /v1/reports` → poll status → `GET` download returns a valid PDF; assert worker consumed the queue (RabbitMQ management metric) and wrote to SeaweedFS; assert a sampled span has the expected parent.
**Effort:** M / 0.5 day.
**Risk refs:** R14, R20, R21.

### P2b-T14 — Confirm no private-module dependency remains (clean `go mod download`) — PHASE EXIT GATE (with T13)
**Description:** Verify reporter resolves entirely from public proxies after the bumps — a precondition for later dropping the `github_token` BuildKit secret / `.secrets/` / `go_private_modules` machinery in Phase 2c/CI harmonization. Reporter declares no `midaz/v3` import (zero Go coupling, per dossier 05) and no other private Lerian module besides the public lib-* set. Run `GOFLAGS=-mod=readonly GONOSUMDB= GOPRIVATE= go mod download all` in a clean env (no netrc/token) and confirm success. Record the result so the CI phase can delete the token machinery without guessing.
**Files:** none (verification); finding recorded for Phase 2c CI task.
**Depends on:** P2b-T09.
**Acceptance criteria:**
- `go mod download all` succeeds with `GOPRIVATE` empty and no auth token configured.
- No module in go.mod requires private auth (lib-commons/lib-observability/lib-auth all resolve publicly).
- Explicit statement recorded: "reporter has no private-module dependency; github_token machinery safe to drop in CI phase."
**Tests:** `env -u GITHUB_TOKEN GOPRIVATE= go mod download all` exits 0 in a fresh checkout/container.
**Effort:** S / 1-2h.
**Risk refs:** R3.

---

## Phase exit gate

**Phase 2c.start depends_on `P2b-T13` AND `P2b-T14`.** Both are leaves of the DAG: T13 is the terminal node of the validation spine (T13←T12←{T09,T10,T11}) and T14 is the private-module proof hanging off T09. The phase is NOT complete on T13 alone — T14 must also be green, or Phase 2c could delete the token machinery without proof it is safe. No single task is the sole gate; the pair is.

## Exit criteria (phase complete when ALL hold)

1. reporter go.mod: `go 1.26.3`, no `toolchain`, `lib-commons/v5` == GA pin (== P1-T06's recorded pin, `v5.2.0` primary), `lib-observability v1.0.1`, `lib-auth/v2 v2.8.0`, no `mongo-driver/v2`, no betas.
2. Zero imports of `lib-commons/v5/commons/{log,opentelemetry,zap}` anywhere in reporter (excluding untracked ast-before snapshots).
3. Zero references to the GA-removed lib-commons-root observability symbols: `CustomContextKey`, `CustomContextKeyValue`, root `NewTrackingFromContext`/`NewLoggerFromContext` via `lib-commons`. `pkg/ctxutil/context.go` sourced from `lib-observability` root + `lib-observability/log`; all 62 ctxutil callers compile and preserve propagation.
4. `NewTrackingFromContext` (195) sourced from `lib-observability` root; lifecycle symbols (`RunApp`, `Launcher`, `SetConfigFromEnvVars`) still from `lib-commons/v5/commons`.
5. Telemetry middleware sourced from `lib-observability/middleware` (aliased `libObsMiddleware`).
6. NO file aliases `lib-observability` as `libCommons` (alias-hygiene gate, T12).
7. `go build ./...` green; reporter CI (`make lint && make test-unit && make sec` + integration + coverage gate) green at ≥ baseline.
8. E2E PDF render passes with span parentage intact on the production path; manager :4005 / worker :4006 / RabbitMQ / Mongo / Redis / SeaweedFS behavior unchanged.
9. Confirmed: reporter has no private-module dependency (clean `go mod download` without token).
10. Observability migration and any later co-location are in SEPARATE commits/PRs (this phase ships in reporter's repo; the move is Phase 2c — obs + co-location never share a commit, PD-6).

## Risks addressed

- **R4** (reporter on near side of v5.1→v5.2 split; 241 import sites + the lib-commons-ROOT observability symbols fail to compile in unified module) — primary target of T03-T09. The genuine compile-breaker is `pkg/ctxutil/context.go` (T08b), not just the `commons/*` subpackages.
- **R3** (lib-commons coexistence / no-shim) — T01, T09, T11, T14: clean GA bump pinned to P1-T06, no replace/shim, public resolution, no misleading aliases (T12).
- **R13** (telemetry middleware bundled→split won't compile on naive swap; root-symbol relocation) — T07 treats middleware as a confirmed code move (no fallback branch); T08 repoints the Tracking root; T08b relocates the production ctxutil funnel where the silent-propagation risk genuinely lives.
- **R14** (dual mongo-driver runtime BSON drift) — T10 collapses to v1.17.9; verified v2 is test-only, so risk is near-eliminated.
- **R11** (coverage hard-fail on under-tested incoming code) — T02 baselines, T12 enforces + backfills.
- **R20/R21** (worker Chromium image; fetcher external-DB reachability) — T13 e2e proves the pipeline + datasource fetch unchanged.

## Open items / flags

- **Cross-phase pin coupling:** reporter's `lib-commons` GA target (P2b-T01) MUST equal P1-T06's recorded pin (`v5.2.0` primary, `v5.2.1` fallback). Do NOT independently pick a higher v5.3/v5.4 here — divergence re-creates skew. P1 owns the call; reporter follows.
- **No shim anywhere:** the `NewTelemetryMiddleware`/`WithTelemetry` relocation (T07), the Tracking-root repoint (T08), and the `ctxutil` root-symbol relocation (T08b) are the three places a tired engineer is tempted to add a thin wrapper. All three have clean lib-observability targets with confirmed APIs — no wrapper is permitted. The ctxutil accessor functions that survive are reporter's own legitimate accessor layer (the same pattern lib-observability uses internally), NOT a compatibility shim: they re-source the renamed `ContextKey`/`ContextValue` directly.
- **Misleading-alias debt is NOT inherited:** midaz aliases lib-observability root as `libCommons` in 142 files (collision with the 73 `lib-commons/v5/commons` `libCommons` aliases). Reporter does this RIGHT (`libObservability` for observability root, `libCommons` only for lib-commons lifecycle), enforced by the T12 alias-hygiene gate. This is the convention midaz should later converge toward (out of scope here; a midaz-side cleanup, surfaced for Phase 9's whole-tree alias sweep).
- **Raw uber-zap (2 sites):** corrected the dossier-09 claim — reporter logs via `commons/log`, not raw zap. The 2 raw-zap sites are test-only; if they are legitimate low-level zapcore usage, they stay with a one-line CODE comment (T06) so the rationale survives the Phase 2c move.
- **lib-auth v2.7→v2.8 logger type:** confirmed signature `NewAuthClient(address string, enabled bool, logger *lib-observability/log.Logger)`. Reporter passes non-nil `&logger` at `config.go:591` + `config_fetcher.go:102` (unlike midaz's nil), so the two logger vars are converted to `lib-observability/log.Logger` in T04 and the type identity is the single auth closure point verified in T09.
- **Integration target name:** pin reporter's exact integration/testcontainers Makefile target before T12 (do not assume `make test-integration`); reporter's Makefile exposes `set-env`/`lint`/`sec`/`format`/`imports` — confirm the integration target to avoid a stall at the integration gate.
- **Out-of-scope for this phase (Phase 2c — flagged for the move planner):** module-path rename (518 files), Dockerfile COPY-path/build-context rewrite for nesting under `components/reporter-{manager,worker}`, compose/infra fold (SeaweedFS + KEDA into midaz infra), CI workflow collapse + `github_token`/`.secrets/` deletion, release-pipeline identity collapse, `pkg/` placement at `components/reporter/pkg`.
- **Coverage backfill unknown:** if the observability rewrite reduces covered-line counts (e.g. error branches in telemetry/ctxutil init), T12 may surface real test-backfill work. Budget contingency.
