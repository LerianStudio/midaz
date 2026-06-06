# Bootstrap Parity Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Bring the four Go deploy units (ledger, tracer, reporter-manager, reporter-worker) to bootstrap parity — correct logger configuration, log flush on exit, explicit telemetry shutdown, build-stamped version info, and harmonized config/MT conventions — with `components/ledger` as the canonical reference, fixed first where it is weaker.

**Architecture:** Each service is a co-located component in a single-root Go module (`github.com/LerianStudio/midaz/v4`, no `go.work`). They share the lib-commons `Launcher` lifecycle and lib-observability `zap`/`tracing` packages. The work harmonizes *bootstrap* code only — composition roots (`internal/bootstrap/`), `cmd/app/main.go`, config structs, and `Service.Run()` shutdown paths. No business logic, no HTTP routes, no domain models change. The parity model is "ledger is canonical, but correct ledger first" — the explicit telemetry-shutdown runnable lands in ledger before the other services copy a corrected pattern. lib-commons usage is mandatory (third rail): no forks, no replacements, no working around the Launcher.

**Tech Stack:** Go 1.26 (toolchain go1.26.4), `github.com/LerianStudio/lib-commons/v5` (Launcher, `SetConfigFromEnvVars`), `github.com/LerianStudio/lib-observability` (`zap`, `log`, `tracing`), `runtime/debug.ReadBuildInfo`, `-ldflags -X` stamping via the existing Makefiles.

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | All four services flush zap on exit, reporter-manager honors `ENV_NAME`/`LOG_LEVEL`, and ledger telemetry-shutdown ownership is explicit and documented | 1.1, 1.2, 1.3 | ✅ Complete (commits `f2ddc8c5e`, `de8e77636`, + docs commit; 1.1 resolved as ownership documentation — see Execution Notes) |
| 2 | Every REST service reports VCS build info on `/version`; the worker reports it in `/readyz` body — stamped via `debug.ReadBuildInfo` + ldflags, ledger first | 2.1, 2.2 | ✅ Complete (commits `8e9c6efa5`, `523e23fa2`, `28cbcb19c`, `4f3fbc6d7`) |
| 3 | Config/MT conventions harmonized (tracer MT-suffix naming, worker struct-tag unification) and shared cancellable shutdown context decided via a lib-commons upstream issue + interim in-repo pattern | 3.1, 3.2, 3.3 | Epic-level |

---

## Phase 1 — Logger correctness, log flush, explicit telemetry shutdown

Closes the high- and medium-severity bootstrap gaps. At the end of Phase 1: a production reporter-manager uses the production zap encoder and honors the operator's `LOG_LEVEL`; all four services flush buffered zap records on SIGTERM so the last lines of a shutdown are never lost; and ledger drains its telemetry via an explicit, auditable Launcher runnable instead of relying solely on the implicit `NewServerManager` path. Order inside the phase: **1.1 (ledger ShutdownTelemetry) first** — it establishes the corrected canonical pattern the parity work points at — then 1.2 (reporter-manager logger), then 1.3 (Sync-on-exit across the three services).

### Epic 1.1: Ledger explicit telemetry-shutdown runnable

**Goal:** Ledger registers an explicit `RunApp("Shutdown Telemetry", ...)` Launcher runnable that calls `telemetry.ShutdownTelemetry()` on SIGINT/SIGTERM, mirroring reporter-manager (`init_helpers.go:104`) and reporter-worker (`service.go:152-156`), instead of relying solely on the implicit drain inside `NewServerManager`.
**Scope:** `components/ledger/internal/bootstrap/` (`service.go`, `unified-server.go`).
**Dependencies:** none.
**Done when:** `Service.Run()` includes a telemetry-shutdown runnable in `launcherOpts`; on shutdown the telemetry provider is explicitly flushed exactly once with no double-shutdown against `NewServerManager`; a unit test asserts the runnable invokes `ShutdownTelemetry` on signal.

#### Task 1.1.1: Add a telemetry-shutdown runnable to the ledger Launcher

- [x] Done — **resolved as ownership documentation, runnable rejected** (owner decision 2026-06-06; see Execution Notes)

**Context:** Ledger's drain today is implicit. `Service.Run()` (`components/ledger/internal/bootstrap/service.go:49-103`) assembles `launcherOpts` from `libCommons.RunApp(...)` entries and calls `libCommons.NewLauncher(launcherOpts...).Run()`. The only telemetry teardown is inside `UnifiedServer.Run()` → `libCommonsServer.NewServerManager(nil, s.telemetry, s.logger)` (`unified-server.go:146`), which owns the HTTP drain and is the implicit path the evaluation flags as "ABSENT explicit ShutdownTelemetry". The telemetry handle lives on `UnifiedServer.telemetry` (`unified-server.go:35,133`), type `*libOpentelemetry.Telemetry` (`unified-server.go:17`). reporter-manager calls `telemetry.ShutdownTelemetry()` in its cleanup (`reporter-manager/internal/bootstrap/init_helpers.go:102-105`); reporter-worker calls it last in `Service.Run()` (`reporter-worker/internal/bootstrap/service.go:151-156`). This file already contains the adapter pattern to copy: `streamingProducerRunnable` (`unified-server.go`/`service.go:105-136`) and `eventListenerRunnable` (`service.go:138-149`) both wrap a teardown hook into a `libCommons.App` that blocks on `signal.NotifyContext(..., os.Interrupt, syscall.SIGTERM)` then runs the hook.

**Implementation vision:** Introduce a `telemetryShutdownRunnable` struct in `service.go` alongside the existing runnable adapters, holding `telemetry *libOpentelemetry.Telemetry` and `logger libLog.Logger`. Its `Run(_ *libCommons.Launcher) error` mirrors `streamingProducerRunnable.Run` exactly: nil-guard the struct and the telemetry handle (return nil if either is nil), `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`, `defer stop()`, `<-ctx.Done()`, then call `telemetry.ShutdownTelemetry()`. `ShutdownTelemetry` returns no error (confirmed against reporter-manager/worker call sites which ignore a return), so just call it and log an Info `"Telemetry shut down"` line; do not invent error handling that the API does not provide. Register it in `Service.Run()` as the LAST `launcherOpts` entry (append after the streaming-producer block at `service.go:97-100`) so it drains after the HTTP server and workers — `RunApp("Shutdown Telemetry", &telemetryShutdownRunnable{telemetry: s.UnifiedServer.telemetry, logger: s.Logger})`. **Double-shutdown edge case:** `NewServerManager(nil, s.telemetry, s.logger)` (`unified-server.go:146`) may also flush telemetry. Before wiring, read `NewServerManager`/`StartWithGracefulShutdown` in lib-commons to confirm whether it calls `ShutdownTelemetry`. If it does, pass `nil` as the telemetry argument to `NewServerManager` (the signature already accepts it — current call passes `s.telemetry`) so the explicit runnable becomes the single owner of telemetry teardown; if it does not, leave the `NewServerManager` call unchanged. Either way, telemetry must be flushed exactly once. `ShutdownTelemetry` is idempotent in lib-observability, but rely on ownership, not idempotency. Access the handle via `s.UnifiedServer.telemetry`; if it is not reachable from the `Service` struct, add a `telemetry` field to `Service` populated at construction rather than exporting `UnifiedServer.telemetry`.

**Files:**
- Modify: `components/ledger/internal/bootstrap/service.go` (new runnable struct + append to `launcherOpts` near `service.go:97-100`)
- Modify: `components/ledger/internal/bootstrap/unified-server.go:146` (only if lib-commons `NewServerManager` already flushes telemetry — pass `nil` to avoid double shutdown)
- Test: `components/ledger/internal/bootstrap/service_test.go` (create if absent)

**Verification:** `go test ./components/ledger/internal/bootstrap/... -run TestTelemetryShutdownRunnable -v` — runnable returns nil on a cancelled context and calls a stubbed/fake telemetry exactly once. Then `go build ./components/ledger/...`.

**Done when:** a `RunApp("Shutdown Telemetry", ...)` entry is present in `Service.Run()`, the explicit runnable is the single owner of `ShutdownTelemetry`, no double shutdown occurs against `NewServerManager`, and the unit test passes.

---

### Epic 1.2: reporter-manager logger honors ENV_NAME and LOG_LEVEL

**Goal:** reporter-manager resolves the zap `Environment` from `cfg.EnvName` and wires `cfg.LogLevel`, so production deploys use the production encoder and the operator's `LOG_LEVEL` is respected — matching reporter-worker (`config_logger.go:33-58`) and ledger (`cmd/app/main.go:52-56`).
**Scope:** `components/reporter-manager/internal/bootstrap/init_helpers.go`.
**Dependencies:** none.
**Done when:** `initConfigAndLogger` builds the zap config from `resolveZapEnvironment(cfg.EnvName)` and `cfg.LogLevel`; the hardcoded `EnvironmentLocal` / missing-`Level` defect is gone; a unit test maps representative `ENV_NAME` values to the expected `zap.Environment` and asserts `Level` is passed through.

#### Task 1.2.1: Wire EnvName and LogLevel into the reporter-manager logger

- [x] Done (`f2ddc8c5e`)

**Context:** `initConfigAndLogger` (`components/reporter-manager/internal/bootstrap/init_helpers.go:60-79`) hardcodes the logger: `zap.New(zap.Config{Environment: zap.EnvironmentLocal, OTelLibraryName: "reporter"})` (`init_helpers.go:70-73`). It never reads `cfg.EnvName` (`reporter-manager/internal/bootstrap/config.go:34`) or `cfg.LogLevel` (`config.go:36`), so a production reporter-manager emits the dev/local console encoder and silently ignores `LOG_LEVEL`. reporter-worker already solves this exactly: `loadConfigAndLogger` (`reporter-worker/internal/bootstrap/config_logger.go:33-43`) passes `Environment: resolveZapEnvironment(cfg.EnvName)`, `Level: cfg.LogLevel`, `OTelLibraryName: cfg.OtelLibraryName`, with the `resolveZapEnvironment` switch at `config_logger.go:45-58` mapping `production/prod`, `staging`, `uat`, `development/dev`, default→`EnvironmentLocal`. The two packages are siblings under `pkg/reporter` but the helper lives in the worker's `bootstrap` package, not a shared location.

**Implementation vision:** In `init_helpers.go`, change the `zap.Config` at `init_helpers.go:70-73` to `Environment: resolveZapEnvironment(cfg.EnvName)`, `Level: cfg.LogLevel`, `OTelLibraryName: cfg.OtelLibraryName`. Add a `resolveZapEnvironment(envName string) zap.Environment` function to the reporter-manager `bootstrap` package, copied verbatim from `reporter-worker/internal/bootstrap/config_logger.go:45-58` (same case set, same `strings.ToLower(strings.TrimSpace(...))` normalization, same default `EnvironmentLocal`). Do NOT extract a shared helper across the two components in this task — that is a YAGNI-violating refactor outside scope; duplicate the 14-line switch. Verify `cfg.OtelLibraryName` exists on the reporter-manager Config (`config.go:42` shows `OtelLibraryName`); if it is empty by default in the manager's env, keep the existing literal `"reporter"` as the `OTelLibraryName` value instead of `cfg.OtelLibraryName` to preserve current telemetry library naming — confirm the env default before choosing. Add the `strings` import if not already present. **Edge cases:** (1) empty/unset `ENV_NAME` → falls through to default `EnvironmentLocal`, preserving today's behavior for local dev — intentional. (2) empty `LOG_LEVEL` → passed through to `zap.New`, which applies its own default; do not pre-default it in reporter-manager (reporter-worker does not, so matching it keeps the encoder behavior identical). (3) mixed-case `ENV_NAME=Production` → handled by the lowercasing in the switch.

**Files:**
- Modify: `components/reporter-manager/internal/bootstrap/init_helpers.go:70-73` (zap config) and add `resolveZapEnvironment` to the same file
- Test: `components/reporter-manager/internal/bootstrap/init_helpers_test.go` (create if absent)

**Verification:** `go test ./components/reporter-manager/internal/bootstrap/... -run TestResolveZapEnvironment -v` — `production`→`EnvironmentProduction`, `staging`→`EnvironmentStaging`, `uat`→`EnvironmentUAT`, `dev`→`EnvironmentDevelopment`, `""`→`EnvironmentLocal`, `"Production"`→`EnvironmentProduction`. Then `go build ./components/reporter-manager/...`.

**Done when:** the hardcoded `EnvironmentLocal` and the absent `Level` are gone; logger config derives from `cfg.EnvName` + `cfg.LogLevel`; the env-mapping test passes; build is green.

---

### Epic 1.3: zap Sync-on-exit for tracer, reporter-manager, reporter-worker

**Goal:** All three non-ledger services flush the zap logger on shutdown so buffered records survive SIGTERM, mirroring ledger's deferred flush (`cmd/app/main.go:68,75`). After this epic, every deploy unit calls `logger.Sync(ctx)` exactly once at the end of its shutdown path.
**Note for implementers:** `Sync` flushes only because the concrete runtime logger is `*zap.Logger` from `libZap.New` (lib-observability `zap/zap.go:126` does `l.must().Sync()`); the `log.GoLogger.Sync` fallback (`log/go_logger.go:110`) is a no-op returning nil. If you grep the interface first and find the no-op, the epic still holds — the services construct via `libZap.New`.
**Scope:** `components/tracer/internal/bootstrap/` and/or `cmd/app/main.go`; `components/reporter-manager/internal/bootstrap/service.go`; `components/reporter-worker/internal/bootstrap/service.go`.
**Dependencies:** none (independent of 1.1/1.2; can run in parallel).
**Done when:** each of the three services flushes its zap logger as the final shutdown action with the error swallowed (`_ = logger.Sync(ctx)`, consistent with ledger `main.go:75`); a test or manual SIGTERM run confirms `Sync` runs once on the shutdown path without panicking; builds are green.

#### Task 1.3.1: Flush zap on exit in reporter-manager and reporter-worker Service.Run

- [x] Done (`de8e77636`)

**Context:** Ledger flushes via `defer`-style explicit calls in main: `_ = logger.Sync(context.Background())` at `cmd/app/main.go:68` (error path) and `:75` (normal exit). The reporter services own their logger inside bootstrap, not main — reporter-manager's `main` calls `bootstrap.InitServers()` then `svc.Run()` with no logger handle in scope; reporter-worker's `main` (`cmd/app/main.go:16-29`) calls `bootstrap.InitWorker()` then `svc.Run()`, also no logger handle. Both `Service` structs embed `log.Logger`: reporter-manager `Service` embeds it (`reporter-manager/internal/bootstrap/service.go:20-22`) and reporter-worker `Service` embeds it (`reporter-worker/internal/bootstrap/service.go:23-25`). reporter-manager `Service.Run()` ends at `service.go:74` (`app.Info("Graceful shutdown complete")`); reporter-worker `Service.Run()` ends at `service.go:158`, AFTER it flushes telemetry at `service.go:151-156`. The correct flush slot is the very last line of each `Service.Run()`, after telemetry shutdown — Sync must be last so it captures the shutdown log lines themselves.

**Implementation vision:** In each `Service.Run()`, add `_ = app.Logger.Sync(context.Background())` as the final statement, after the existing `"Graceful shutdown complete"` Info line. Match ledger's posture exactly: swallow the error (Sync on a console/stderr sink legitimately returns a benign `sync /dev/stderr: invalid argument` on some platforms — ledger ignores it at `main.go:75`, so do the same; do NOT log the Sync error, that would require a logger that is being flushed). For reporter-worker, place the Sync call after `app.Info("Graceful shutdown complete")` at `service.go:158` — it comes after telemetry flush, which is correct ordering. For reporter-manager, place it after `app.Info("Graceful shutdown complete")` at `service.go:74` and after the existing `app.cleanup()` call (`service.go:70-72`) — note reporter-manager's telemetry shutdown lives inside `cleanup` (`init_helpers.go:102-105`), so Sync-after-cleanup keeps Sync last. The embedded `log.Logger` is reachable as `app.Logger` (or `app.Log...` — use the embedded field directly; `app.Logger.Sync` is the explicit form). Confirm the embedded field is named `Logger` (it is embedded as `log.Logger`, so `app.Logger` resolves). **Edge case:** if `app.Logger` could be nil at this point it cannot — both services construct the logger before the Service and would have failed boot otherwise; no nil guard needed, matching ledger which does not guard.

**Files:**
- Modify: `components/reporter-manager/internal/bootstrap/service.go:74` (append Sync after the final Info)
- Modify: `components/reporter-worker/internal/bootstrap/service.go:158` (append Sync after the final Info)

**Verification:** `go build ./components/reporter-manager/... ./components/reporter-worker/...`. Manual: run each service locally, send SIGTERM, confirm the `"Graceful shutdown complete"` line and any preceding shutdown lines appear in output (no truncation) and the process exits 0 without panic.

**Done when:** both `Service.Run()` methods call `Sync` exactly once as their final action, after telemetry teardown; builds are green; SIGTERM produces complete, untruncated shutdown logs.

#### Task 1.3.2: Flush zap on exit in tracer

- [x] Done (`de8e77636`)

**Context:** The tracer has zero `Sync` calls anywhere in `components/tracer` (confirmed: only `libZap.New` at `config.go:1460`, no `.Sync(`). The logger is created inside `initCoreInfra` (`tracer/internal/bootstrap/config.go:1453-1469`, `var logger libLog.Logger = zapLogger` at `config.go:1469`) and threaded into the Service via `InitServers(ctx)`. tracer's `main.run()` (`cmd/app/main.go:48-65`) calls `bootstrap.InitServers(ctx)` then `service.Run()` and has NO logger handle in scope — unlike ledger, tracer's main cannot flush. The Service returned by `InitServers` must expose the logger or perform the Sync inside its own `Run()`. Tracer's `Service`/`Run()` lives in `tracer/internal/bootstrap/service.go` (per the component CLAUDE.md, `service.go` holds the `Service` struct with `Run()`/`Shutdown()`).

**Implementation vision:** Read `tracer/internal/bootstrap/service.go` first to locate the `Service` struct and the end of its `Run()` (and/or `Shutdown()`) shutdown path — the tracer uses a richer drain than the reporter services. Add `_ = <logger>.Sync(context.Background())` as the final action of the shutdown path, after telemetry/server teardown, error swallowed (same posture as ledger `main.go:75` and Task 1.3.1). If the tracer `Service` already holds the logger as a field, call Sync on that field; if it does not, add a `logger libLog.Logger` field to the `Service` struct populated from the `logger` created in `initCoreInfra` (`config.go:1469`) and threaded through `InitServers`. Do NOT relocate the Sync into `main.run()` by exporting the logger from `InitServers` — keeping the flush inside the Service's own shutdown path matches the reporter services' shape (Task 1.3.1) and keeps tracer's signal-aware bootstrap ctx (`cmd/app/main.go:54`) untouched. **Edge case ordering:** tracer flushes telemetry as part of its shutdown; Sync must come after it so telemetry-shutdown log lines are captured — place Sync last, mirroring reporter-worker ordering.

**Files:**
- Modify: `components/tracer/internal/bootstrap/service.go` (add Sync at the end of the shutdown path; add a `logger` field to `Service` if one is not already present, populated from `config.go:1469` via `InitServers`)

**Verification:** `go build ./components/tracer/...`. Manual: run tracer locally, send SIGTERM, confirm shutdown log lines are complete and the process exits 0 without panic. If a bootstrap-level test exists, `go test ./components/tracer/internal/bootstrap/... -run TestService -v`.

**Done when:** tracer calls `logger.Sync` exactly once as the final shutdown action; build is green; SIGTERM produces complete shutdown logs with no truncation.

---

## Phase 2 — VCS build-info version stamping

**Milestone:** Every REST service (`ledger`, `tracer`, `reporter-manager`) reports VCS build information (commit, dirty flag, build time) on its `/version` endpoint, and the worker (`reporter-worker`) reports it in its `/readyz` body — sourced from `runtime/debug.ReadBuildInfo` with ldflags-stamped overrides, ledger implemented first as the canonical pattern. At phase end every deploy unit surfaces a real build identity, not just a `VERSION` env string.

### Epic 2.1: Build-info plumbing + ledger /version (canonical)

**Goal:** A shared-shape build-info accessor reads `debug.ReadBuildInfo` (vcs.revision, vcs.time, vcs.modified) with ldflags `-X` overrides for environments where VCS stamping is unavailable, and ledger's `/version` surfaces it; the build pipeline stamps the values at build time.
**Ground truth (verified 2026-06-06):** all three REST services delegate `/version` to lib-commons `libHTTP.Version` (`commons/net/http/handler.go:33`), which returns `{"version": GetenvOrDefault("VERSION","0.0.0"), "requestDate": time.Now().UTC()}`. Ledger registers it at `unified-server.go:73` (NOT readyz.go as originally estimated). Ledger's `/version` is NOT in swagger.json and is explicitly excluded from `TestContractSpecMatchesRoutes` (`contract_spec_routes_test.go:35-41`) — no docs regen needed for ledger. Docker build context is the repo root (`context: ../../` in every compose file), there is NO root `.dockerignore`, and every Dockerfile does `COPY . .` — so `.git` IS in the builder stage; the only missing piece for in-Docker VCS stamping is the `git` binary (golang:alpine lacks it). Per-component `.dockerignore` files (tracer's excludes `.git/`) are INERT because dockerignore only applies at the context root. Host Makefile builds run inside the work tree → VCS stamping is automatic. Therefore: ldflags `-X` is the fallback override, NOT the primary mechanism — no ARG plumbing in Dockerfiles.

#### Task 2.1.1: `pkg/buildinfo` package (TDD)

- [x] Done (`8e9c6efa5`)

**Files:** Create `pkg/buildinfo/buildinfo.go`, `pkg/buildinfo/handler.go`, `pkg/buildinfo/buildinfo_test.go`, `pkg/buildinfo/handler_test.go`.

**Implementation vision:** Package-level unexported string vars `commit`, `buildTime` (override path: `-ldflags "-X github.com/LerianStudio/midaz/v4/pkg/buildinfo.commit=<sha> -X ...buildTime=<rfc3339>"`). `Info struct { Commit string; BuildTime string; Dirty bool }`. Core is a pure, testable `compute(bi *debug.BuildInfo, ldCommit, ldBuildTime string) Info`: precedence ldflags > `bi.Settings` (`vcs.revision`, `vcs.time`, `vcs.modified`) > `"unknown"`/`"unknown"`/false. `Get() Info` wraps `compute(debug.ReadBuildInfo())` under `sync.Once`. `VersionHandler(version string) fiber.Handler` in handler.go returns `{"version": version-or-"0.0.0", "requestDate": time.Now().UTC(), "commit": i.Commit, "buildTime": i.BuildTime, "dirty": i.Dirty}` — the first two fields preserve the lib-commons wire shape exactly (retain-VERSION decision). Tests: table test on `compute` (ldflags win over vcs settings; vcs-only; nil BuildInfo → unknowns; `vcs.modified=true` → Dirty), handler test via fiber+httptest asserting the five JSON keys and the `"0.0.0"` fallback on empty version.

#### Task 2.1.2: Ledger /version serves build info

- [x] Done (`8e9c6efa5`)

**Files:** Modify `components/ledger/internal/bootstrap/unified-server.go:73`.

**Implementation vision:** Replace `app.Get("/version", libHTTP.Version)` with `app.Get("/version", buildinfo.VersionHandler(<version>))`, where `<version>` is the ledger `Config.Version` (env `VERSION`) if reachable at registration, else `os.Getenv("VERSION")` — handler defaults empty→`"0.0.0"`, preserving lib-commons behavior byte-for-byte on the existing fields. `/version` is contract-test-excluded and spec-absent → no `make generate-docs`. Verify `TestContractSpecMatchesRoutes` stays green.

#### Task 2.1.3: Ledger Docker builder gets git (VCS stamping in images)

- [x] Done (`8e9c6efa5`)

**Files:** Modify `components/ledger/Dockerfile` (builder stage).

**Implementation vision:** Add `RUN apk add --no-cache git` in the builder stage before the build. With `.git` already in the context (root context + `COPY . .`), `go build` stamps `vcs.*` automatically. Host builds need nothing (work tree + host git). Verification: `go build -o /tmp/ledger-vcs components/ledger/cmd/app/main.go && go version -m /tmp/ledger-vcs | grep vcs` shows revision/time/modified; optionally `docker build` the ledger image and confirm the same via `go version -m` on the extracted binary or hitting `/version`.

**Epic done when:** ledger `/version` returns commit + build time + dirty flag; values come from `debug.ReadBuildInfo` by default and ldflags when stamped; `VERSION` env string retained as the `version` field; contract test green.

### Epic 2.2: Propagate build-info to tracer, reporter-manager (/version) and reporter-worker (/readyz body)

**Goal:** The three remaining services adopt `pkg/buildinfo` — tracer and reporter-manager on `/version`, reporter-worker in its existing `/readyz` body since it has no REST API by design.
**Ground truth (verified 2026-06-06):** tracer wraps `libHTTP.Version` in a local annotated handler (`tracer/internal/adapters/http/in/handlers.go:86-88`, route `routes.go:240`) and tracer's swagger.json DOES document `/version` with `api.VersionResponse` — response-shape change requires updating that model and running `make generate-docs` in the same commit. reporter-manager registers `commonsHttp.Version` directly (`routes.go:137`); its spec does NOT document `/version`. reporter-worker's health server is net/http (`health-server.go:176-217`); its `/readyz` body comes from `pkg/reporter/readyz.NewNetHTTPHandler(checkers, drainState, cfg.Version, deploymentMode, metrics)` — shared with reporter-manager's readyz deps, so added fields appear on both (additive, acceptable).
**Dependencies:** Epic 2.1 (defines accessor + response shape).

#### Task 2.2.1: tracer + reporter-manager /version adopt VersionHandler

- [x] Done (`523e23fa2`)

**Files:** Modify `components/tracer/internal/adapters/http/in/handlers.go:86-88` (+ the `api.VersionResponse` model wherever it is defined), `components/reporter-manager/internal/adapters/http/in/routes.go:137`; regenerate `components/tracer/api/*` (+ specs/postman if the pipeline publishes tracer) via `make generate-docs`.

**Implementation vision:** tracer: keep the exported annotated `Version` handler, body becomes `return buildinfo.VersionHandler(os.Getenv("VERSION"))(c)` or equivalent pre-built handler var; extend `api.VersionResponse` with `commit`, `buildTime`, `dirty` fields so the annotation stays truthful; run `make generate-docs` in the same commit (tracer spec contains `/version`). reporter-manager: swap `commonsHttp.Version` → `buildinfo.VersionHandler(os.Getenv("VERSION"))` at `routes.go:137` (spec-absent, no regen strictly needed but the pipeline run covers it). Both preserve `version`/`requestDate` semantics.

#### Task 2.2.2: reporter-worker build info in /readyz body

- [x] Done (`28cbcb19c`)

**Files:** Modify `pkg/reporter/readyz` (the `NewNetHTTPHandler` response struct + population) and its tests.

**Implementation vision:** Add `commit`, `buildTime`, `dirty` to the readyz response body, populated from `buildinfo.Get()` at handler construction (not per-request). Keep the existing `version` field untouched. Note: reporter-manager's readyz shares this handler — fields appear there too; additive and harmless. Extend the package's existing tests to assert the new fields.

#### Task 2.2.3: builder-stage git for tracer, reporter-manager, reporter-worker Dockerfiles

- [x] Done (`4f3fbc6d7`) — tracer Dockerfiles already carried `git`; only the two reporter builders changed

**Files:** Modify `components/tracer/Dockerfile` (+ `Dockerfile.dev` — compose uses it), `components/reporter-manager/Dockerfile`, `components/reporter-worker/Dockerfile` builder stages.

**Implementation vision:** Same one-liner as 2.1.3 (`apk add --no-cache git` in builder). tracer's production Dockerfile already has an `apk add` block (line 5) — check whether `git` is in it and append if not; same check on `Dockerfile.dev`.

**Epic done when:** tracer and reporter-manager `/version` and reporter-worker `/readyz` all report the same build-info shape; tracer spec regenerated atomically; **reporter-worker gets no new `/version` endpoint** (locked decision); builds green.

---

## Phase 3 — Convention harmonization + shutdown-context investigation

**Milestone:** Cosmetic/consistency drift is removed (tracer MT naming follows the CLAUDE.md `MT`-suffix mandate; reporter-worker env struct tags are unified) and the cross-cutting shutdown-context limitation is resolved into a concrete decision: an upstream lib-commons issue plus an interim in-repo coordination pattern. At phase end the codebase is convention-consistent and the shutdown-context path forward is documented and (for the interim pattern) decided.

### Epic 3.1: Tracer multi-tenant naming → MT-suffix convention

**Goal:** Tracer's multi-tenant wiring uses the `MT`-suffix naming mandated by the root CLAUDE.md (`NewFooMT`, `runFooMT`, `mtEnabled`, `isMTReady`) instead of the current `multiTenant`/`mt` prefixes.
**Scope:** `components/tracer/internal/bootstrap/config_multitenant_wiring.go:31-36` and call sites within tracer bootstrap.
**Dependencies:** none.
**Done when:** tracer MT identifiers follow the `MT`-suffix convention; no prefix-style `multiTenant`/`mt` names remain in tracer bootstrap; build and existing tracer tests are green. Pure rename — no behavior change.

### Epic 3.2: reporter-worker env struct-tag unification

**Goal:** reporter-worker Config uses a single env-default struct tag style. The reconciler fields use `envDefault` (`reporter-worker/internal/bootstrap/config.go:97-99`) while the rest use `default` (`config.go:101-117`); both are honored but the mix is drift.
**Scope:** `components/reporter-worker/internal/bootstrap/config.go:97-117`.
**Dependencies:** none.
**Done when:** all Config fields use one tag style (the one matching `libCommons.SetConfigFromEnvVars`' documented/canonical tag — confirm which of `default`/`envDefault` lib-commons actually reads before changing); a test or a boot-with-defaults check confirms every defaulted field still resolves to the same value it did before. No behavior change.

### Epic 3.3: INVESTIGATION — shared cancellable shutdown context across ledger workers

**Goal:** Resolve the root cause of ledger's per-worker independent signal handling (each worker makes its own `signal.NotifyContext`; `service.go:123,151`, `circuitbreaker.go:214`) into a concrete decision. The root cause is a lib-commons `Launcher` v4 limitation: it provides no shared cancellable context for coordinated drain ordering (documented at `components/ledger/internal/bootstrap/balance_sync.worker.go:147-150`). This epic is an investigation/decision epic, not an implementation epic.
**Scope:** Analysis across `components/ledger/internal/bootstrap/` worker runnables; an upstream lib-commons issue; a written decision on an interim in-repo coordination pattern.
**Dependencies:** Phase 1 (ledger bootstrap stabilized, including the new telemetry-shutdown runnable from Epic 1.1, which shares the same signal-handling shape).
**Done when:** (1) a filed lib-commons issue requests a shared cancellable shutdown context / coordinated drain ordering on the `Launcher`, with the ledger use case and the `balance_sync.worker.go:147-150` reference; (2) a written decision selects an interim in-repo coordination pattern (e.g., a single shared `signal.NotifyContext` derived once and threaded into runnables, vs. accepting independent contexts until upstream lands) with its tradeoffs; (3) the decision explicitly honors the third rail — lib-commons is mandatory, so no fork/replacement of the Launcher is proposed, only an upstream request plus an in-repo pattern that composes with the existing `Launcher`. No production worker behavior changes in this epic unless the interim pattern is explicitly approved for implementation in a follow-up.

---

## Execution Notes — Phase 2 (2026-06-06)

- **ldflags demoted to fallback:** Docker build context is the repo root with no root `.dockerignore` and `COPY . .` — `.git` was always inside the builder; the only missing piece was the `git` binary in golang:alpine. With it installed, `go build` stamps `vcs.*` natively, so no ARG/`-X` plumbing was added to Dockerfiles or Makefiles. The `pkg/buildinfo` ldflags vars remain as the override path for environments without VCS (e.g. the shared CI workflow, which this repo cannot edit — adopting `-X` there is a possible follow-up, not a need).
- **Per-component `.dockerignore` files are inert** (dockerignore applies only at the context root) — tracer's `.git/` exclusion never did anything. Left untouched (out of scope), but worth a cleanup someday since it is actively misleading.
- **`api.VersionResponse` is midaz-owned** (`components/tracer/api/types.go:16`) — extended in place; tracer spec + postman regenerated atomically with the handler change (routes+spec contract lesson from the CRM plan).
- **Version-source nuance:** tracer and reporter-manager configs have no `VERSION` field (only `OTEL_RESOURCE_SERVICE_VERSION`); lib-commons read the `VERSION` env directly, so both adopt `buildinfo.VersionHandler(os.Getenv("VERSION"))` to preserve semantics. Ledger threads `cfg.Version` through `NewUnifiedServer`.
- **reporter-manager `/readyz` gained the fields too** — shared `pkg/reporter/readyz` handler; additive, accepted in elaboration.

## Execution Notes — Phase 1 (2026-06-06)

- **Epic 1.1 reshaped (owner decision):** ground truth refuted the plan's premise. lib-commons `ServerManager` v5.4.1 (`commons/server/shutdown.go:619`) already calls `ShutdownTelemetry()` — and `logger.Sync()` — **after** the HTTP drain, precisely so final-request spans export. The plan's yes-branch (pass `nil` to `NewServerManager`, signal-fired runnable owns shutdown) would have flushed telemetry concurrently with the drain: Launcher runnables wake on SIGTERM with no drain-ordering guarantee (the exact Epic 3.3 limitation), so the runnable was a span-fidelity regression, not an auditability win. Resolution: `ServerManager` stays the single owner; ownership documented at `components/ledger/internal/bootstrap/unified-server.go:146` with a do-not-move warning. The real fix for drain ordering is Epic 3.3's upstream lib-commons issue.
- **Plan refs that drifted before execution:** reporter-manager has NO `RunApp("Shutdown Telemetry", ...)` registration (the plan cited `init_helpers.go:104`) — its telemetry shutdown is a `cleanup` closure invoked at the end of `Service.Run()`. Tracer `initCoreInfra` is at `config.go:1509-1524`, not `1453-1469`.
- **Found, flagged, NOT fixed (out of plan scope):** reporter-manager double-shuts telemetry — `ServerManager` (post-drain) and the `cleanup()` closure both call `ShutdownTelemetry()`. Idempotent, so harmless today, but it violates "ownership, not idempotency"; candidate for Phase 3 cleanup.
- **OTelLibraryName decision (Task 1.2.1):** used `cfg.OtelLibraryName`, not the `"reporter"` literal — `.env`/`.env.example` ship a non-empty `OTEL_LIBRARY_NAME`, and every other telemetry surface in the process (initTelemetry, tracer, meters) already reads the cfg field; keeping the literal would diverge the logger from them.
- **Sync call form:** `app.Sync(...)` not `app.Logger.Sync(...)` — staticcheck QF1008 flags the explicit embedded-field selector; behavior-identical.
- **Pre-existing-failure note:** the 3 reporter-worker `config_mt_test.go` failures recorded in earlier session memory no longer reproduce on this branch — already resolved upstream. Tracer carries 2 pre-existing `gocyclo` findings in untouched functions (`config.go:1617`, `config_multitenant_wiring.go:83`).

---

## Out of Scope

- **reporter-worker `/version` endpoint** — the worker has no REST API by design; build info lives in the `/readyz` body (Epic 2.2). Adding a dedicated `/version` endpoint is explicitly out of scope.
- Auth/CRM findings from the source evaluation (fail-open gates, header-trust, namespace divergence, CRM scoping redesign) — those are separate plans; this plan covers `result.bootstrap` only.
- Extracting a shared cross-component logger/build-info package beyond what each epic needs — duplicate small helpers rather than introduce a premature shared abstraction (YAGNI).

---

## Self-Review

- **Spec coverage:** Every `result.bootstrap` finding maps to an epic. Logger reporter-manager (high) → Epic 1.2. Logger Sync tracer/reporter-manager/reporter-worker (medium ×3) → Epic 1.3 (Tasks 1.3.1, 1.3.2). MT naming tracer (low) → Epic 3.1. Worker struct-tag drift (low) → Epic 3.2. Worker version-in-readyz (low, "acceptable") → respected; no `/version` added, build-info added to `/readyz` body in Epic 2.2. Canonical weakness "no explicit ShutdownTelemetry" → Epic 1.1. Canonical weakness "no shared cancellable shutdown context" → Epic 3.3 (investigation). Canonical weakness "no VCS build-stamp" → Phase 2 (Epics 2.1, 2.2). No gap unaddressed.
- **Vagueness scan:** Phase 1 tasks name exact files, lines, the canonical pattern to copy, and each edge case (empty `ENV_NAME`, empty `LOG_LEVEL`, mixed case, Sync error swallowing, double-telemetry-shutdown ownership). No "appropriate"/"TBD" in the detailed wave.
- **Contract consistency:** The telemetry-shutdown runnable (1.1) reuses the existing `streamingProducerRunnable`/`eventListenerRunnable` adapter contract in the same file. `resolveZapEnvironment` (1.2) is the same signature/behavior as reporter-worker's existing helper. Sync posture (`_ = logger.Sync(ctx)`) is identical to ledger `main.go:75` across 1.3. Phase 2's build-info accessor is defined once in Epic 2.1 and consumed by 2.2.
- **Phase boundaries:** Phase 1 ends with all four services flushing logs + correct manager logger + explicit ledger telemetry drain (testable, buildable). Phase 2 ends with build info surfaced everywhere (testable via endpoint hit). Phase 3 ends with conventions unified + a filed issue and written decision. Each phase is independently shippable.
- **Verification plausibility:** All Phase 1 verification commands target real package paths confirmed present (`components/<svc>/internal/bootstrap/...`) and assert outcomes the code can produce.
