# Phase 2a — plugins-fees IN-PLACE dependency migration

**Phase ID:** P2a
**Objective:** In plugins-fees' OWN repo (`/Users/fredamaral/repos/lerianstudio/plugin-fees`, module path `github.com/LerianStudio/plugins-fees/v3`), BEFORE any code moves into midaz: (1) migrate observability off `lib-commons/v5/commons/{log,opentelemetry,zap}` + root `commons.NewTrackingFromContext`/`NewLoggerFromContext` + the `commons/net/http` telemetry-middleware family onto `lib-observability/{log,tracing,zap,middleware}` and root `lib-observability`; align `lib-commons` to v5.2.x GA, `lib-observability` to v1.0.1 (drop the stale declared-but-unused `v1.1.0-beta.5`), `lib-auth/v2` to v2.8.0; and (2) run a PRE-MOVE fees-engine correctness spike that freezes three load-bearing fee invariants while the engine still lives in its own repo with its native property/fuzz harness. Validate against plugins-fees' own CI including its 85% coverage hard gate. This is a PRECONDITION of the Phase 4 embed (R4): fees must compile against midaz target lib versions before its code enters the ledger module, and the fee-correctness facts P4 depends on must be characterized before the move commits.

> **Repo naming note (do not get tripped up):** the directory on disk is `plugin-fees` (singular), but the Go module path is `github.com/LerianStudio/plugins-fees/v3` (plural, with `/v3`). All `go.mod`/import assertions in this phase target the **lib-commons / lib-observability** paths, not the module path, so the disk-name vs module-name mismatch never affects a grep — but the executor must `cd /Users/fredamaral/repos/lerianstudio/plugin-fees` and read `module github.com/LerianStudio/plugins-fees/v3` in go.mod.

**Locked decisions in force:** PD-4 (lib-commons v5.2.x GA, keep lib-observability v1.0.1), PD-6 (in-place migration first, validated against own CI; observability migration and co-location MUST NOT share a commit — bisectability). TRACER ROLE independent of otel-lgtm. ZERO shims: no `replace`, no go.work, no dual-import aliases, no compat layer.

---

## Scope correction (verified against source — supersedes the dossier's bare 86 count)

The phase brief and dossiers 06/07 quote **86 observability sites** (43 `commons/log` + 36 `commons/opentelemetry` + 7 `commons/zap`). That is the floor — it counts only the three relocated **subpackages**. Direct inspection of plugins-fees HEAD shows the migration is materially larger because two more symbol families move out of `lib-commons`:

| Source symbol (today) | Sites | Canonical midaz target (verified at midaz HEAD) | Target alias |
|---|---|---|---|
| `lib-commons/v5/commons/log` (import) | 43 files | `lib-observability/log` | `libLog` |
| `lib-commons/v5/commons/opentelemetry` (import) | 36 files | `lib-observability/tracing` | `libOpentelemetry` |
| `lib-commons/v5/commons/zap` (import) | 7 files | `lib-observability/zap` | `libZap` |
| `*.NewTrackingFromContext` (root pkg, any alias) | **67** | root `lib-observability` | `libObservability` |
| `*.NewLoggerFromContext` (root pkg, any alias) | **3** | root `lib-observability` | `libObservability` |
| `commonsHttp.NewTelemetryMiddleware` | 1 | `lib-observability/middleware` | `libObsMiddleware` |
| `commonsHttp.WithHTTPLogging` | 1 | `lib-observability/middleware` | `libObsMiddleware` |
| `commonsHttp.WithCustomLogger` | 1 | `lib-observability/middleware` | `libObsMiddleware` |

> **NewLoggerFromContext is 3 sites, not 2 (CORRECTED).** Verified at fees HEAD:
> `internal/cache/package_cache.go:145` (`commons.NewLoggerFromContext`), `internal/cache/billing_package_cache.go:282` (`commons.NewLoggerFromContext`), and **`pkg/net/http/withRecover.go:51` (`libCommons.NewLoggerFromContext`)**. The third uses the alias `libCommons` for root `lib-commons/v5/commons`, NOT the bare `commons.` qualifier. The plan's old grep `commons\.NewLoggerFromContext` substring-matched the `ommons.` tail of `libCommons.NewLoggerFromContext` by accident and reported 2 (it counted the call but the file enumeration omitted withRecover.go). T03/T07/T09 use **alias-agnostic** patterns so this can never recur.

**Stays in lib-commons (do NOT move):** root `commons.{IsNilOrEmpty,Response,RunApp,NewLauncher,SetConfigFromEnvVars,WithLogger,ValidateServerAddress,GetMapNumKinds,SafeInt,NormalizeDate,...}` (47 files import root commons); `commons/net/http.{Respond(47),RespondStatus(4),FiberErrorHandler,DecodeCursor,JSONResponse,Version}` (alias `commonsHttp`, 11 files); `commons/tenant` (18), `commons/secretsmanager` (4), `commons/server` (1), `commons/mongo` (1), `commons/license` (1).

**The `commons/net/http` SPLIT is the R13-class trap, present in fees (not just tracer):** the same import path `lib-commons/v5/commons/net/http` provides BOTH the telemetry middleware (which moved to `lib-observability/middleware` in the split) AND the HTTP response helpers (which stayed). A naive whole-file path swap will not compile. `internal/http/in/routes.go` must import `libObsMiddleware` for the 3 telemetry-middleware symbols while keeping `commonsHttp` for `Respond`/etc.

### Canonical alias decision — corrected against midaz HEAD reality

The prior draft asserted that `libObservability` is midaz's **dominant/canonical** alias for the root `github.com/LerianStudio/lib-observability` package and that normalizing fees to it makes the Phase 4 embed "a no-op on import style." **That premise is factually wrong at midaz HEAD and is corrected here:**

- Root `lib-observability` is aliased `libCommons` in **143** files and `libObservability` in only **45** (plus 1 bare `commons`). `libObservability` is the **minority** convention, not the dominant one.
- Worse, midaz overloads the SAME alias name `libCommons` onto TWO different root packages: `lib-commons/v5/commons` (**174** files) AND `lib-observability` (**143** files). midaz only survives this overload because **no single file imports both roots** — the collision is partitioned by file.

**Decision: fees standardizes the root `lib-observability` package on `libObservability` DELIBERATELY** — not because it is the midaz majority (it is not), but because:
1. It removes the `libCommons`-means-two-different-modules ambiguity that midaz currently lives with. A fees file that imports BOTH roots (which exists — see below) CANNOT reuse midaz's single-`libCommons` habit without a same-file collision.
2. It makes the intent of every import line self-evident to a reviewer at embed time.

**Cross-phase consequence (binds to P9, SS1):** because midaz HEAD still has 143 `libCommons`-on-lib-observability files, fees normalized to `libObservability` will NOT match midaz's dominant alias at embed time. This is the correct trade (see decision above), but it means the Phase 4 embed is **not** "zero alias churn against midaz" — it is "zero alias churn within fees, deliberately diverging from midaz's overloaded-libCommons habit." The whole-tree `libCommons`-on-lib-observability → `libObservability` normalization across midaz itself is **P9's** job (the "liso e final" misleading-alias sweep, DAG-2 bound to **P8-T18** ci-harmonization is unrelated; the alias sweep is the reference-hygiene task in P9). P2a cross-references P9 so the executor does not mistake the divergence for a defect, and so the exit criterion verifies fees matches the **agreed target** (`libObservability`), not the plan's old false "midaz majority" claim.

**Co-location hazard for Phase 4 (binds to P4):** any fold-time midaz file that imports BOTH the lib-commons root AND the lib-observability root MUST keep distinct aliases (`libCommons` for `lib-commons/v5/commons`, `libObservability` for `lib-observability`). fees has files that touch the root lib-observability symbol while living near root-commons usage (`pkg/net/http/withRecover.go` imports `libCommons` for `lib-commons/v5/commons` AND will gain `libObservability` for `NewLoggerFromContext`). T07 already handles the retain-both-imports case; this note makes the P4 owner aware the distinct-alias rule is load-bearing at the embed.

**Alias normalization is part of the work, not optional cosmetics.** Current fees imports are inconsistent (`clog`, `libCommonsLog`, `libLog`, plain `log`, `libCommonsOtel`, `libOpentelemetry`, plain `zap`, `libZap`, `libCommons` on root commons). The migration normalizes ALL observability imports to: `libLog`, `libOpentelemetry`, `libZap`, `libObservability`, `libObsMiddleware`. T10 verifies this by a **path-anchored** completeness check (every observability import path resolves through exactly the canonical alias), not a hardcoded three-alias grep that would silently skip the `libCommons`-root and plain-`log`/`zap`/`opentelemetry` forms.

**Metrics:** fees does NOT import `lib-commons/v5/commons/opentelemetry/metrics` (verified: 0 sites). `internal/metrics/tenant_metrics.go` uses raw `go.opentelemetry.io/otel/metric` directly. So there is NO `lib-observability/metrics` migration forced by fees' own code — the phase objective lists it as a candidate target but the subpackage is unused. Confirm-and-skip (task P2a-T03 verifies).

**GA verification (done at plan time, re-verify at execution — versions move):**
- `lib-commons/v5`: GA tags `v5.2.0`, `v5.2.1` both exist on proxy (plus v5.3.x, v5.4.1). Latest v5.2.x GA = **v5.2.1**.
- `lib-observability`: `v1.0.1` GA exists; `v1.1.0` is beta-only (no GA) — confirms PD-4's "drop to v1.0.1".
- `lib-auth/v2`: `v2.8.0` GA exists.
- **fees HEAD baseline (verified):** `lib-commons/v5 v5.1.0`, `lib-observability v1.1.0-beta.5`, `lib-auth/v2 v2.7.0`, `midaz/v3 v3.5.2`, `lib-commons/v2 v2.9.1 // indirect`.
- **midaz HEAD baseline (verified):** `lib-commons/v5 v5.2.0-beta.12`, `lib-observability v1.0.1`, `lib-auth/v2 v2.8.0`. midaz is STILL on a **v5.2.0-beta**, NOT a v5.2.x GA — the Phase 1 GA bump (PD-4, P1-T06) has not landed in this working tree. fees' pin MUST match whatever Phase 1 (P1-T06) lands midaz on; default target **v5.2.1** with a cross-phase reconciliation note (T16).

---

## Task DAG (sequence)

```
P2a-T00 (verify GA) ─┐
                     ├─> P2a-T01 (go.mod pins) ─> P2a-T02 (go mod tidy/download baseline)
P2a-T03 (scope audit)┘                                  │
                                                        ├─> P2a-T04 (log split)  ─┐
                                                        ├─> P2a-T05 (tracing split)│
                                                        ├─> P2a-T06 (zap split)    ├─> P2a-T09 (compile gate)
                                                        ├─> P2a-T07 (NewTracking/  │
                                                        │           NewLogger root)│
                                                        └─> P2a-T08 (net/http      ┘
                                                                    middleware split)
P2a-T09 ─> P2a-T10 (alias normalize sweep) ─> P2a-T11 (go mod tidy + drop indirects)
P2a-T11 ─> P2a-T12 (make lint/sec) ─> P2a-T13 (unit + 85% coverage gate)
P2a-T13 ─> P2a-T14 (integration/property/fuzz) ─> P2a-T15 (full CI green on branch)
P2a-T15 ─> P2a-T16 (cross-phase pin reconciliation note for P1/P4)

P2a-T17 (fees-engine correctness spike) ── independent of the observability chain;
         runs in parallel from phase start (no observability dependency).
         GATES Phase 4 (P4-T01/P4-T02 depend on it) and is a P2a exit criterion.
```

Tasks T04–T08 are independent per-symbol-family rewrites and may run in parallel; they converge at T09.

> **Parallel-build caveat for T04–T08 (non-blocking, but executors must know):** the five rewrite tasks are independent in their END state (they converge at T09), but per-task incremental builds are NOT all standalone-compilable. `routes.go` (T08) passes `tl` to `NewTelemetryMiddleware`, and `tl`'s type only becomes lib-observability's `*tracing.Telemetry` after T05 lands. A parallel executor should land T05 and T08 in the same change (or land T05 first) — T08's compile only succeeds once the moved Telemetry type exists. No task claims to build standalone, so this is a sequencing caveat, not a correctness break.

P2a-T17 has no dependency on the observability rewrite chain (it touches `pkg/fee/distribute.go`, `pkg/model/package.go`, `pkg/fee/calculate-fee.go` only for read/characterization plus a new conservation test) and SHOULD run in parallel from phase start so its frozen artifact is ready when Phase 4 begins.

---

### P2a-T00 — Verify v5.2.x GA / lib-observability v1.0.1 / lib-auth v2.8.0 exist on the proxy and fix the target pins
**Description:** Run `go list -m -versions` against the public proxy for `lib-commons/v5`, `lib-observability`, `lib-auth/v2`. Confirm a v5.2.x GA tag exists; if multiple, select the highest v5.2.x GA (currently v5.2.1) UNLESS Phase 1 (P1-T06) has already pinned midaz to a specific v5.2.x GA — in that case match it exactly. Confirm `lib-observability v1.0.1` is GA and that there is no `v1.1.0` GA (only betas) so dropping the stale `v1.1.0-beta.5` to `v1.0.1` is the correct stable target. Confirm `lib-auth/v2 v2.8.0` is GA. Record the exact chosen pins as the authoritative target for T01. NOTE: midaz HEAD is currently on `lib-commons/v5 v5.2.0-beta.12` (not a GA), so P1-T06 (the canonical GA bump) gates the final reconciliation; until P1-T06 lands, default to v5.2.1.
**Files:** none (verification only); records pins consumed by P2a-T01.
**Depends on:** (none)
**Acceptance criteria:**
- A v5.2.x GA tag is confirmed present on the proxy and a single target pin is chosen (default v5.2.1, or the P1-T06 pin if P1 landed it).
- `lib-observability v1.0.1` confirmed GA; absence of a `v1.1.0` GA confirmed.
- `lib-auth/v2 v2.8.0` confirmed GA.
**Tests:** `GOPROXY=https://proxy.golang.org go list -m -versions github.com/LerianStudio/lib-commons/v5` (and the other two) returns the expected GA tags.
**Effort:** S / 1-2h
**Risk refs:** R3, R4

---

### P2a-T03 — Audit and freeze the exact observability migration surface (alias-agnostic)
**Description:** Produce the authoritative file/site inventory the rewrite tasks consume, so no site is missed and no in-scope symbol is left behind. Use **alias-agnostic, path-anchored** patterns throughout — the prior draft's alias-specific greps undercounted (see NewLoggerFromContext below). Grep plugins-fees for: (a) the 3 relocated subpackage imports `lib-commons/v5/commons/{log,opentelemetry,zap}`, capturing the FULL alias zoo per import (`libLog`/`clog`/`libCommonsLog`/plain `log`; `libOpentelemetry`/`libCommonsOtel`/plain `opentelemetry`; `libZap`/plain `zap`); (b) root-pkg `*.NewTrackingFromContext` and `*.NewLoggerFromContext` via `grep -rEn '\.NewTrackingFromContext|\.NewLoggerFromContext'` (matches ANY qualifier — `commons.`, `libCommons.`, etc.); (c) `commonsHttp.{NewTelemetryMiddleware,WithHTTPLogging,WithCustomLogger}`; (d) confirm `commons/opentelemetry/metrics` is NOT imported (skip lib-observability/metrics — verified 0 sites). Record the per-family file lists INCLUDING the alias each site uses today (the rewrite must flip the qualifier, not just the import path). Confirm the root-`commons` symbols that STAY (IsNilOrEmpty, Response, RunApp, NewLauncher, etc.) and the `commonsHttp` helpers that STAY (Respond, RespondStatus, FiberErrorHandler, DecodeCursor, JSONResponse, Version) so the split is unambiguous.
**Files:** none (audit only); output drives T04-T08 and T10.
**Depends on:** (none)
**Acceptance criteria:**
- Per-symbol-family file lists captured matching the counts in this plan's scope table (43 log, 36 tracing, 7 zap, **67 NewTrackingFromContext, 3 NewLoggerFromContext**, 3 middleware) or a documented reconciliation if HEAD has drifted.
- The 3 `NewLoggerFromContext` sites are explicitly enumerated, including `pkg/net/http/withRecover.go:51` and its `libCommons` qualifier (NOT `commons`).
- For every relocated import, the current alias is recorded so T10's normalization target is unambiguous (full alias zoo captured: `libCommons`-root, `libLog`/`clog`/`libCommonsLog`/plain `log`, `libOpentelemetry`/`libCommonsOtel`/plain `opentelemetry`, `libZap`/plain `zap`).
- Explicit list of root-`commons` and `commonsHttp` symbols that must remain on lib-commons.
- Confirmation that no `commons/opentelemetry/metrics` import exists.
**Tests:** alias-agnostic `grep -rEn` site counts reproduce the inventory (`grep -rEn '\.NewLoggerFromContext' --include='*.go' .` returns exactly 3, including withRecover.go:51); the lists are committed to the migration branch description or a scratch file (not shipped).
**Effort:** S / 1-2h
**Risk refs:** R4, R13

---

### P2a-T17 — Fees-engine correctness SPIKE (pre-move gate; resolves P4-T02 open conflict)
**Description:** Run a pre-move correctness spike IN the plugins-fees repo, while the fee engine still has its native property/fuzz harness, to freeze three load-bearing facts that Phase 4 depends on. This is the audit-mandated item (TF3/FG1/SP2): the highest-correctness-risk decisions in P4 (P4-T02 route shape, P4-T11 balancing, P4-T14 revert) currently land late with no earlier de-risking. Produce a **frozen artifact** the P4 owner consumes. Three concrete deliverables, all verified against fees HEAD source so they are executable here:

1. **Leg-sum conservation invariant of `applyFeeCorrection`.** `pkg/fee/distribute.go:350 applyFeeCorrection` applies the residual `delta` to the max account's fee entry. No existing property test asserts `sum(distributed legs) == fee total` exactly (verified: the property suite under `tests/property/*` asserts per-calc properties — non-negative, commutative, gross=events×unit, net≤gross — but contains ZERO conservation assertion). ADD a new property/unit test that proves, under `decimal.Decimal` with ZERO tolerance (`decimal.Equal`, not float epsilon), that across `applyFeeCorrection` / `distribute.go` the sum of distributed fee legs equals the fee total exactly. This is the in-fees-repo proof of the invariant PD-5's reversal-balances (sum==0) guarantee rests on at P4.
2. **Route-value shape — RESOLVE P4-T02's open conflict NOW.** Determine and DOCUMENT whether the synthetic route values returned by `pkg/model/package.go:43 GetRouteFrom()` / `:51 GetRouteTo()` are UUID-shaped. Verified at fees HEAD: the struct fields carry example values `RouteFrom *string ... example:"taxa_débito"` / `RouteTo *string ... example:"taxa_crédito"` — **human label strings, NOT UUIDs** — and `distribute.go:328` builds composite map keys like `feeKey+"->"+feeModel.GetRouteFrom()`. Therefore the P4-T02 question "are these UUID-shaped?" resolves to **NO**. Record the exact key-composition format (`feeKey->routeLabel`, and the `credit->fee_sourceN->payer->routeId` synthetic-key family) so the P4 owner inherits a DECIDED question: writing these strings to `RouteID` (which carries `validate:"omitempty,uuid"`) WILL fail uuid validation on any route-validation-enabled path; a `name→ID` resolution step (or keeping them on the passive `Route` field) is required at the seam. This converts P4.md's "Open behavioral conflict (must resolve)" into a resolved input.
3. **`CalculateFee` top-level amount mutation.** Confirm by inspection that `pkg/fee/calculate-fee.go::CalculateFee` (and the distribute path) mutates only `Transaction.Send.Value` at the top level (verified anchor: `distribute.go:72` `f.Transaction.Send.Value = f.Transaction.Send.Value.Add(result.Value)`). Document that `Send.Value` is the SOLE top-level amount change — this is what P4-T12 relies on when it reassigns the single `validate` variable so every downstream consumer reads the post-fee result, and what P4-T14 relies on for revert balancing (`tran.Amount` derives from the mutated `Send.Value`).
**Files:** `pkg/fee/distribute.go` (read + new conservation test), `pkg/model/package.go` (read), `pkg/fee/calculate-fee.go` (read), `pkg/fee/calculate-fee_test.go` or `tests/property/fee_conservation_test.go` (NEW — the leg-sum proof); `docs/monorepo/plan/artifacts/P2a-fees-engine-correctness-spike.md` (NEW — the frozen artifact P4 consumes).
**Depends on:** (none — independent of the observability rewrite chain; runs in parallel from phase start)
**Acceptance criteria:**
- A new test proves `sum(distributed fee legs) == fee total` exactly (`decimal.Equal`, zero tolerance) across `applyFeeCorrection`/`distribute.go`, and it is green on the fees migration branch.
- The artifact documents that `GetRouteFrom`/`GetRouteTo` return label strings (NOT UUIDs), records the composite key-composition format, and explicitly states the resolution for P4-T02 (the synthetic values cannot go to `RouteID`'s uuid-validated field without a `name→ID` step or staying on the passive `Route` field).
- The artifact confirms `Send.Value` is the only top-level amount mutation in `CalculateFee`/distribute, citing the verified line anchors.
- The artifact is marked as a **hard dependency for Phase 4** (P4-T01/P4-T02 MUST NOT start the embed/repoint until this spike is recorded) and as a **P2a exit criterion**.
**Tests:** `go test ./tests/property/... ./pkg/fee/...` includes and passes the new conservation test; the artifact file exists and is referenced from P4-T02.
**Effort:** M / 4-6h
**Risk refs:** R4, R5

---

### P2a-T01 — Repin go.mod: lib-commons v5.2.x GA, lib-observability v1.0.1, lib-auth v2.8.0
**Description:** Edit `go.mod`: change `github.com/LerianStudio/lib-commons/v5` from `v5.1.0` to the v5.2.x GA chosen in T00; change `github.com/LerianStudio/lib-observability` from `v1.1.0-beta.5` to `v1.0.1`; change `github.com/LerianStudio/lib-auth/v2` from `v2.7.0` to `v2.8.0`. Do NOT yet rewrite any imports — this task only moves the version pins so the next `go build` surfaces the removed-package compile errors that the rewrite tasks then fix. Leave `midaz/v3 v3.5.2` untouched (the embed/repoint to `pkg/mtransaction` is Phase 4, out of this phase). Leave the transitive `lib-commons/v2 v2.9.1 // indirect` for the tidy task (T11 asserts it drops — it is already orphaned, see T11).
**Files:** `/Users/fredamaral/repos/lerianstudio/plugin-fees/go.mod`
**Depends on:** P2a-T00
**Acceptance criteria:**
- `go.mod` shows lib-commons at the v5.2.x GA pin, lib-observability `v1.0.1`, lib-auth/v2 `v2.8.0`.
- `midaz/v3 v3.5.2` unchanged (still required this phase).
- No `replace` directive added anywhere.
**Tests:** `grep -E 'lib-commons/v5|lib-observability|lib-auth/v2' go.mod` shows the new pins; `go build ./...` fails ONLY with "package lib-commons/v5/commons/{log,opentelemetry,zap} is not in module" / undefined-symbol errors (proving the pin took and pinpointing the rewrite surface).
**Effort:** S / <1h
**Risk refs:** R3, R4

---

### P2a-T02 — Establish a clean go mod download + build-failure baseline
**Description:** With the new pins, run `go mod download` to confirm all three lib versions resolve from the proxy (validates T00's pins are fetchable, not just listed), then `go build ./...` to capture the full set of compile errors as the rewrite worklist. This baseline is the "before" state; T09 is the "after". Confirm the download does not require any private credential beyond the existing `github.com/LerianStudio/*` GOPRIVATE/github_token already in fees' CI (NOT in scope to remove here — fees still imports midaz/v3 v3.5.2 this phase).
**Files:** `/Users/fredamaral/repos/lerianstudio/plugin-fees/go.sum` (updated by download)
**Depends on:** P2a-T01
**Acceptance criteria:**
- `go mod download` succeeds for all three repinned modules.
- `go build ./...` error list captured; every error is an observability symbol/package error (no unexpected unrelated breakage from the version bump).
- No new private-module requirement introduced by the bumps.
**Tests:** `go mod download github.com/LerianStudio/lib-commons/v5 github.com/LerianStudio/lib-observability github.com/LerianStudio/lib-auth/v2` exits 0; `go build ./... 2>&1 | tee` error set reviewed.
**Effort:** S / 1h
**Risk refs:** R3, R4

---

### P2a-T04 — Rewrite `commons/log` → `lib-observability/log` (43 files)
**Description:** In every file importing `github.com/LerianStudio/lib-commons/v5/commons/log`, repoint the import to `github.com/LerianStudio/lib-observability/log` and set the alias to `libLog` (midaz canonical — `libLog`-on-lib-observability/log IS genuinely the midaz majority, 242 sites; this is a true canonical, unlike the root-package alias). Update all symbol references (`libLog.String/.Int/.Any/.Bool/.Err/.Logger/.Level{Info,Warn,Error,Debug}/.Field/.NewNop`, etc.) to the normalized alias. The `lib-observability/log` surface is shape-identical to the lib-commons logger (verified: midaz uses exactly these symbols against `libLog`). Files currently using non-canonical aliases (`clog`, `libCommonsLog`, plain `log`) must converge to `libLog`. Note `pkg/net/http/withRecover.go:12` carries `libLog "github.com/LerianStudio/lib-commons/v5/commons/log"` — repoint its path (the alias name already matches).
**Files:** the 43 `*.go` files from the T03 `commons/log` list (production + tests across `internal/`, `pkg/`, `cmd/`, `tests/`).
**Depends on:** P2a-T01, P2a-T03
**Acceptance criteria:**
- Zero remaining imports of `lib-commons/v5/commons/log` in the repo.
- All logger imports aliased `libLog`.
- Logger call sites compile against `lib-observability/log`.
**Tests:** `grep -rn 'lib-commons/v5/commons/log' --include='*.go'` returns nothing; `go build ./...` no longer reports `commons/log` errors.
**Effort:** M / 3-5h
**Risk refs:** R4

---

### P2a-T05 — Rewrite `commons/opentelemetry` → `lib-observability/tracing` (36 files + struct-literal site)
**Description:** Repoint every `github.com/LerianStudio/lib-commons/v5/commons/opentelemetry` import to `github.com/LerianStudio/lib-observability/tracing`, alias `libOpentelemetry` (midaz canonical — `libOpentelemetry`-on-lib-observability/tracing IS genuinely the midaz majority, 177 sites). Symbols in use (verified): `HandleSpanError` (148), `HandleSpanBusinessErrorEvent` (56), `SetSpanAttributesFromValue` (31), `InjectHTTPContext` (4), `Telemetry`, `TelemetryConfig`, `NewTelemetry` (1 each) — all present in `lib-observability/tracing` with identical names (midaz uses `HandleSpanError`/`HandleSpanBusinessErrorEvent` against `libOpentelemetry`). Normalize the `libCommonsOtel` alias to `libOpentelemetry`.
**Two named non-pure-rename sites that the prior draft missed or under-specified:**
1. `internal/bootstrap/config.go:24,444` — `libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{...})` (already aliased `libOpentelemetry`, only the path flips). Add a sub-step: **diff the `lib-observability/tracing.TelemetryConfig` struct definition against `lib-commons/v5/commons/opentelemetry.TelemetryConfig` at execution time.** If fields drifted (renames/additions in the v5→observability split), treat `config.go` as a MANUAL edit, not a path swap. (At plan time these structs are byte-identical between lib-commons v5.1.0 and lib-observability v1.0.1 — same 10 fields, same `NewTelemetry` signature — so this is expected to be a pure path swap; the diff sub-step is the contingency, since this split historically bit the tracer.)
2. `internal/bootstrap/server_test.go:13,61` — aliases `libCommonsOtel` and constructs a **struct literal directly**: `&libCommonsOtel.Telemetry{}`. This site needs BOTH the path repoint AND the `libCommonsOtel`→`libOpentelemetry` rename. The prior draft's enumerated set omitted it.
**Files:** the 36 `*.go` files from the T03 `commons/opentelemetry` list (notably `internal/services/*.go`, `internal/http/in/*.go`, `internal/bootstrap/config.go`, `internal/bootstrap/server_test.go`, `pkg/fee/*.go`).
**Depends on:** P2a-T01, P2a-T03
**Acceptance criteria:**
- Zero remaining imports of `lib-commons/v5/commons/opentelemetry`.
- All tracing imports aliased `libOpentelemetry` (including the formerly-`libCommonsOtel` `server_test.go` site).
- `NewTelemetry(TelemetryConfig{...})` in bootstrap and the `&Telemetry{}` struct literal in `server_test.go` compile against `lib-observability/tracing`.
- The TelemetryConfig field-parity diff is recorded; if fields drifted, `config.go` was hand-edited accordingly (no blind swap on a drifted struct).
**Tests:** `grep -rn 'lib-commons/v5/commons/opentelemetry' --include='*.go'` returns nothing; `grep -rn 'libCommonsOtel' --include='*.go'` returns nothing; `go build ./...` no longer reports opentelemetry errors.
**Effort:** M / 4-6h
**Risk refs:** R4

---

### P2a-T06 — Rewrite `commons/zap` → `lib-observability/zap` (7 files)
**Description:** Repoint every `github.com/LerianStudio/lib-commons/v5/commons/zap` import to `github.com/LerianStudio/lib-observability/zap`, alias `libZap` (midaz canonical — 22 sites). Symbols in use: `New`, `Config`, `Environment{Local,Development,Production}` — all present in `lib-observability/zap`. Normalize the plain-`zap.`-aliased usages (in `tests/property/*`, `tests/fuzzy/*`, `internal/bootstrap/config.go`, `pkg/fee/calculate-fee_test.go`, `pkg/fee/calculate-fee_fuzz_test.go`) to `libZap` and match midaz convention. NOTE (rationale correction): the prior draft justified this normalization as avoiding collision with `go.uber.org/zap` style references — that justification is **fictional**. Verified: NO fees file imports both `commons/zap` and `go.uber.org/zap` (checked all 7 zap files; `uberZap`=0 in every one). The normalization is still correct and harmless — it is done for alias consistency with midaz, not to dodge a non-existent collision.
**Files:** the 7 `*.go` files from the T03 `commons/zap` list.
**Depends on:** P2a-T01, P2a-T03
**Acceptance criteria:**
- Zero remaining imports of `lib-commons/v5/commons/zap`.
- All zap imports aliased `libZap`.
**Tests:** `grep -rn 'lib-commons/v5/commons/zap' --include='*.go'` returns nothing; `go build ./...` no longer reports zap errors.
**Effort:** S / 2-3h
**Risk refs:** R4

---

### P2a-T07 — Move `NewTrackingFromContext` (67) + `NewLoggerFromContext` (3) to root `lib-observability`
**Description:** These two symbols moved OUT of the root `lib-commons/v5/commons` package into the root `lib-observability` package in the split (verified at midaz HEAD: `*.NewTrackingFromContext` resolves from root `lib-observability`). For each call site, add the import `github.com/LerianStudio/lib-observability` aliased `libObservability` (see the canonical alias decision above — `libObservability`, chosen deliberately to avoid the overloaded-`libCommons` ambiguity) and change `<qual>.NewTrackingFromContext(...)` → `libObservability.NewTrackingFromContext(...)` and `<qual>.NewLoggerFromContext(...)` → `libObservability.NewLoggerFromContext(...)`.
**The symbol appears under TWO qualifiers — the rewrite must handle both:**
- bare `commons.` — e.g. `internal/cache/package_cache.go:145`, `internal/cache/billing_package_cache.go:282` (`commons.NewLoggerFromContext`), and the bulk of the 67 `NewTrackingFromContext` sites.
- `libCommons.` — **`pkg/net/http/withRecover.go:51`** (`libCommons.NewLoggerFromContext`). This is the 3rd `NewLoggerFromContext` site the prior draft missed.
**CRITICAL — do NOT drop the root-commons import where it is still needed:** files that ALSO use root-commons symbols that STAY (IsNilOrEmpty, Response, RunApp, NewLauncher, etc.) MUST retain their `lib-commons/v5/commons` import (under whatever alias — `commons` or `libCommons`); only the two relocated symbols flip to `libObservability`. `withRecover.go` is the canonical example: it keeps `libCommons "github.com/LerianStudio/lib-commons/v5/commons"` for root-commons symbols that stay, gains `libObservability "github.com/LerianStudio/lib-observability"` for `NewLoggerFromContext`, AND has its `libLog`/`libOpentelemetry` paths repointed by T04/T05 — a triple-split file. Files that needed the root-commons import ONLY for the two moved symbols drop it. This is the highest-volume rename in the phase.
**Files:** the 67/3-site file set from T03 (e.g. `internal/cache/{account_cache,package_cache,billing_package_cache}.go`, `internal/services/*.go`, `pkg/net/http/{withRecover.go,middleware-tracing.go}`, and broadly across `internal/` and `pkg/`).
**Depends on:** P2a-T01, P2a-T03
**Acceptance criteria:**
- Zero `*.NewTrackingFromContext` / `*.NewLoggerFromContext` references resolve through `lib-commons/v5/commons` (any alias).
- All such calls use `libObservability.` against the root `lib-observability` import.
- Files that still need root-commons symbols retain their `lib-commons/v5/commons` import; files that needed it ONLY for the two moved symbols drop it.
- `withRecover.go:51` specifically flips from `libCommons.NewLoggerFromContext` to `libObservability.NewLoggerFromContext` while retaining its `libCommons` import for staying root-commons symbols.
**Tests:** `grep -rEn '\.NewTrackingFromContext|\.NewLoggerFromContext' --include='*.go'` shows every match qualified by `libObservability.` (zero `commons.`/`libCommons.`-qualified survivors); `go build ./...` resolves the symbols.
**Effort:** L / 5-7h
**Risk refs:** R4

---

### P2a-T08 — Split `commons/net/http`: telemetry MIDDLEWARE → `lib-observability/middleware`, keep HTTP helpers (R13 trap)
**Description:** The R13-class split, present in fees. In `internal/http/in/routes.go` (the only file using the moved **middleware** symbols), `NewTelemetryMiddleware`, `WithHTTPLogging`, `WithCustomLogger` moved to `lib-observability/middleware` (verified at midaz HEAD: `libObsMiddleware.NewTelemetryMiddleware(telemetry)` and `app.Use(libObsMiddleware.WithHTTPLogging(libObsMiddleware.WithCustomLogger(logger)))`). Add import `github.com/LerianStudio/lib-observability/middleware` aliased `libObsMiddleware` and repoint those 3 call sites (`routes.go:63` `NewTelemetryMiddleware(tl)`, `routes.go:80` `WithHTTPLogging(WithCustomLogger(lg))`). NOTE: `routes.go:66` calls `tlMid.WithTelemetry(tl)` — a METHOD on the `TelemetryMiddleware` value returned by `NewTelemetryMiddleware`. It moves automatically with the constructor's return type (no extra import, no separate repoint); the reviewer should expect `WithTelemetry` to resolve from the moved type and NOT flag it as a stray symbol. KEEP the `commonsHttp "github.com/LerianStudio/lib-commons/v5/commons/net/http"` import (`routes.go:12`) in the same file for `Respond`/`RespondStatus`/`FiberErrorHandler`/`DecodeCursor`/`JSONResponse`/`Version`, which did NOT move (verified staying-symbol uses in routes.go: `FiberErrorHandler` L60, `Version` L87, `Respond` L100). Do not touch the other 10 `commonsHttp` files — they use only the helpers that stay.
> **Framing tightened (no-shims lens):** T08 is the only file using the moved **middleware** symbols — it is NOT the sole net/http-adjacent split. `withRecover.go` is also net/http-adjacent but its splits (libLog/libOpentelemetry paths + the `NewLoggerFromContext` root-symbol move) are owned by T04+T05+T07, not T08.
**Files:** `/Users/fredamaral/repos/lerianstudio/plugin-fees/internal/http/in/routes.go` (and any other file the T03 audit flags as using a moved middleware symbol).
**Depends on:** P2a-T01, P2a-T03
**Acceptance criteria:**
- `NewTelemetryMiddleware`/`WithHTTPLogging`/`WithCustomLogger` resolve from `lib-observability/middleware` via `libObsMiddleware`; `WithTelemetry` resolves from the moved `TelemetryMiddleware` type.
- `commonsHttp` import retained for the staying helpers; `Respond`/`FiberErrorHandler`/`Version` etc. still resolve from `lib-commons/v5/commons/net/http`.
- No whole-file blind path swap (the helpers must not point at lib-observability).
**Tests:** `go build ./internal/http/in/...` compiles; `grep -n 'libObsMiddleware\|commonsHttp' internal/http/in/routes.go` shows both imports coexisting.
**Effort:** S / 1-2h
**Risk refs:** R13, R4

---

### P2a-T09 — Compile gate: whole tree builds with zero lib-commons observability imports
**Description:** Converge T04-T08. Run `go build ./...` and `go vet ./...` across the full module (incl. test files). Confirm zero remaining imports of any relocated symbol/package from lib-commons. This is the structural correctness gate before style normalization and test runs. Use alias-agnostic patterns so a `libCommons.`-qualified survivor cannot hide (the prior draft's `commons\.` regex would substring-match it by luck but a reviewer must not rely on luck).
**Files:** none (gate); may surface stragglers in any `*.go`.
**Depends on:** P2a-T04, P2a-T05, P2a-T06, P2a-T07, P2a-T08
**Acceptance criteria:**
- `go build ./...` exits 0.
- `go vet ./...` exits 0.
- All of these greps return empty: `lib-commons/v5/commons/log`, `lib-commons/v5/commons/opentelemetry`, `lib-commons/v5/commons/zap`; the alias-agnostic `\.NewTrackingFromContext|\.NewLoggerFromContext` matches resolve only through `libObservability.`; and the 3 middleware symbols are sourced from `lib-observability/middleware`, not `commons/net/http`.
**Tests:** `go build ./... && go vet ./...` exit 0; `grep -rn 'lib-commons/v5/commons/log\|lib-commons/v5/commons/opentelemetry\|lib-commons/v5/commons/zap' --include='*.go'` empty; `grep -rEn '(commons|libCommons)\.NewTrackingFromContext|(commons|libCommons)\.NewLoggerFromContext' --include='*.go'` empty.
**Effort:** S / 1-2h
**Risk refs:** R4, R13

---

### P2a-T10 — Normalize import aliases to midaz canonical across the tree (path-anchored completeness)
**Description:** Sweep for residual non-canonical aliases left by T04-T08 and converge every observability import to its canonical alias: `libLog`, `libOpentelemetry`, `libZap`, `libObservability`, `libObsMiddleware`. The prior draft's completeness check hunted only `clog`/`libCommonsLog`/`libCommonsOtel` — that is an incomplete kill-list. Replace it with a **path-anchored** completeness assertion: for every touched file, verify the import path → alias mapping matches the canonical alias for that observability path, enumerating the FULL real alias zoo to converge: `clog`, `libCommonsLog`, `libCommonsOtel`, the `libCommons`-on-root-lib-observability form (must become `libObservability`), and the **unaliased** `log`/`zap`/`opentelemetry` forms (frequently the import is plain, e.g. `log` in some sites — these must gain the `libLog`/`libZap`/`libOpentelemetry` alias). Run `goimports`/`gofmt` to settle ordering (stdlib → external → internal). This guarantees the fees tree presents a single consistent observability alias convention before the Phase 4 embed (noting per the canonical-alias decision that `libObservability` deliberately diverges from midaz's overloaded-`libCommons` habit; the whole-tree midaz reconciliation is P9's job).
> **Do NOT opportunistically "fix" out-of-scope style issues in this sweep.** `pkg/fee/distribute.go:45` contains `logger.Log(ctx, log.LevelInfo, fmt.Sprintf(...))` — a CLAUDE.md logging-style violation (fmt.Sprintf inside a logger call). It is PRE-EXISTING fees code and explicitly OUT of P2a scope (P2a is a behavior-preserving migration, not a style cleanup). Leaving it untouched keeps T13's coverage delta clean and keeps the migration bisectable. It is a conscious deferral to the build-harmonization phase, not an oversight.
**Files:** any `*.go` touched in T04-T08 plus any straggler the path-anchored check finds.
**Depends on:** P2a-T09
**Acceptance criteria:**
- No non-canonical observability alias survives: for each observability import path, the file's alias equals the canonical alias (verified by the path→alias mapping check, not a hardcoded alias grep). `clog`/`libCommonsLog`/`libCommonsOtel` return empty; no plain-unaliased `log`/`zap`/`opentelemetry` import of a lib-observability path remains; no `libCommons`-aliased import of root `lib-observability` remains.
- `gofmt -l` reports no unformatted files; imports grouped per project convention.
- The `distribute.go:45` Sprintf-in-logger violation is left untouched (deferred), confirming no scope creep.
**Tests:** `gofmt -l . | grep -c .` is 0; `grep -rn 'clog \|libCommonsLog\|libCommonsOtel' --include='*.go'` empty; the path→alias mapping check passes for every observability import; `go build ./...` still 0.
**Effort:** S / 1-2h
**Risk refs:** R4

---

### P2a-T11 — `go mod tidy`, assert orphan indirect drops, drop stale beta
**Description:** Run `go mod tidy`. Confirm `lib-observability` is now a USED direct dep (it had 0 source refs today; after T04-T08 it is heavily used). Confirm the stale `v1.1.0-beta.5` is fully gone and the pin is `v1.0.1`. **Assert** `lib-commons/v2 v2.9.1 // indirect` drops out of go.mod — it is genuinely orphaned at fees HEAD (`go mod why github.com/LerianStudio/lib-commons/v2` returns "main module does not need package"); this is pre-existing dead cruft, not introduced by this phase, and a clean tidy should evict it. Do NOT leave a "document it as a remaining transitive" escape hatch — that would let dead indirect cruft survive the no-shim end state. (If a transitive genuinely re-pulls it after the bumps, that is a surprise to investigate, not a default to accept.) Confirm no `lib-observability` beta leaks back in via any transitive. Ensure `go.sum` is minimal and consistent.
**Files:** `/Users/fredamaral/repos/lerianstudio/plugin-fees/go.mod`, `/Users/fredamaral/repos/lerianstudio/plugin-fees/go.sum`
**Depends on:** P2a-T10
**Acceptance criteria:**
- `go mod tidy` leaves the tree buildable; `lib-observability v1.0.1` present and used; no `v1.1.0` anything.
- `lib-commons/v5` at the v5.2.x GA pin; `lib-auth/v2 v2.8.0`.
- `lib-commons/v2 v2.9.1` is ABSENT from go.mod after tidy (orphan evicted). If — against expectation — it persists, the exact transitive that pulls it is identified via `go mod why` before accepting it.
**Tests:** `go mod tidy && git diff --stat go.mod go.sum` reviewed; `go list -m all | grep -E 'lib-observability|lib-commons|lib-auth'` shows only the target pins; `grep -c 'v1.1.0' go.mod go.sum` is 0; `grep -c 'lib-commons/v2' go.mod` is 0.
**Effort:** S / 1-2h
**Risk refs:** R4, R3

---

### P2a-T12 — `make lint` and `make sec` green under the strict config
**Description:** Run fees' own `make lint` (golangci-lint v2.11.3 per CI) and `make sec`. Fix any lint introduced by the alias changes/import grouping. The migration must not regress fees' strict `.golangci.yml` floor (this config is slated to become the monorepo lint floor in the build phase). Security scan must stay clean — the lib bumps (lib-commons v5.1.0→v5.2.x, lib-observability beta→v1.0.1, lib-auth v2.7.0→v2.8.0) could surface a new advisory, possibly via a transitive (e.g. a `lib-commons/v2` indirect if it survives, though T11 evicts it); triage and pin/upgrade within the v5.2.x / v1.0.1 / v2.8.0 lines if so. The "or triage" branch can blow the upper effort bound if a transitive carries a flagged CVE — budget accordingly.
**Files:** any `*.go` requiring lint fixes; no `.golangci.yml` relaxation permitted.
**Depends on:** P2a-T11
**Acceptance criteria:**
- `make lint` exits 0 with the existing strict config (no rule disabled to pass).
- `make sec` exits 0, or any new finding is triaged with a justified resolution within the pinned lib lines.
**Tests:** `make lint` and `make sec` exit 0.
**Effort:** M / 2-4h (upper bound elastic if a new advisory must be triaged)
**Risk refs:** R11

---

### P2a-T13 — Unit tests pass AND 85% coverage hard gate holds
**Description:** Run `make coverage-unit` (mirrors CI `coverage_threshold: 85`, `fail_on_coverage_threshold: true`, honoring `.ignorecoverunit`). The alias/import rewrites touch many test files (the `commons/zap` and tracking sites live partly in `tests/property/*`, `tests/fuzzy/*`, `*_test.go`); confirm they still compile and pass, and that coverage did not drop below 85% (e.g. if a logger/tracing helper that was exercised got refactored). The new conservation test from P2a-T17 also lands here. Backfill tests only if the migration itself dropped coverage — do not expand scope.
**Files:** test files under `pkg/fee/`, `internal/`, `tests/property/`, `tests/fuzzy/` as needed.
**Depends on:** P2a-T12
**Acceptance criteria:**
- `make test-unit` / `make coverage-unit` pass.
- Total coverage ≥ 85% (the CI gate does not fail).
**Tests:** `make coverage-unit` exits 0 and prints `Total coverage: >=85%`; `go test ./...` (unit) passes.
**Effort:** M / 3-5h
**Risk refs:** R11

---

### P2a-T14 — Integration / property / fuzz suites pass against migrated code
**Description:** Run fees' integration tests (testcontainers Mongo) plus the property tests (`tests/property/fee_monotonicity_test.go`, `package_idempotency_test.go`, `fee_calculation_consistency_test.go`, plus the NEW `fee_conservation` test from P2a-T17) and fuzz tests (`tests/fuzzy/fee_calculation_fuzz_test.go`, `pkg/fee/calculate-fee_fuzz_test.go`). These exercise the fee engine and its logging/tracing instrumentation — the highest-value proof that the observability rewrite did not alter behavior (logging/tracing are cross-cutting; a botched `NewTrackingFromContext` swap that nils a tracer would surface here). Note: this phase does NOT touch fee-on-revert (PD-5) — that correctness work belongs to the Phase 4 embed (P4-T14/T16); here we only prove the migration is behavior-preserving against the existing suite plus the new conservation invariant.
**Files:** none (test execution); fixes land in the relevant `*.go` if a test breaks.
**Depends on:** P2a-T13
**Acceptance criteria:**
- `make test-integration` passes (real Mongo via testcontainers).
- Property and fuzz suites pass (no new failures vs the pre-migration baseline), including the P2a-T17 conservation test.
**Tests:** `make test-integration` exits 0; `go test ./tests/property/... ./tests/fuzzy/... ./pkg/fee/...` pass.
**Effort:** M / 2-4h
**Risk refs:** R4, R11
**Note:** PD-5 fee-on-revert refund correctness is explicitly OUT of this phase (Phase 4). The reversal-balances (sum==0) guarantee P4 must prove rests on the leg-sum conservation invariant frozen by P2a-T17; that artifact is the input, the revert proof is P4's.

---

### P2a-T15 — Full plugins-fees CI green on the migration branch
**Description:** Push the migration branch in the plugins-fees repo and confirm the FULL shared-workflow CI is green: `go-combined-analysis.yml` (lint v2.11.3 + sec + tests + 85% coverage + build), `build.yml`, `pr-validation.yml`, `pr-security-scan.yml`. This is the PD-6 gate: the migration is validated against fees' OWN CI before any code moves. Do NOT merge into midaz here. The branch is the artifact Phase 4 consumes.
**Files:** none (CI run); `.github/workflows/*` unchanged (do NOT bump the shared-workflow version here — that is the build-harmonization phase, P8).
**Depends on:** P2a-T14
**Acceptance criteria:**
- All required CI checks pass on the branch, including the 85% coverage hard gate and the build.
- No workflow file altered to make CI pass.
**Tests:** GitHub Actions: all required checks green on the plugins-fees migration branch.
**Effort:** S / 1-2h (+ CI wait)
**Risk refs:** R11, R4

---

### P2a-T16 — Cross-phase pin reconciliation note for Phase 1 / Phase 4
**Description:** Record, for the Phase 1 (midaz GA bump, P1-T06) and Phase 4 (fees embed) owners, the exact lib pins fees landed on (lib-commons v5.2.x GA chosen, lib-observability v1.0.1, lib-auth v2.8.0). midaz HEAD in this working tree is on `lib-commons/v5 v5.2.0-beta.12` — P1-T06 lands the canonical GA pin. If fees pinned to a v5.2.x GA that differs from whatever P1-T06 lands midaz on, flag the one-line reconciliation needed at embed-time (`go mod tidy` on the unified module will MVS-resolve to the higher v5.2.x, which is fine as long as both are GA on the same minor — no code change, but the embed PR must re-tidy). Confirm no `replace`/shim is implied by any pin gap. This closes the loop so the embed is a mechanical fold, not a re-migration.
**Files:** this plan doc / phase handoff; `docs/monorepo/plan/artifacts/P2a-pin-reconciliation.md` (NEW, optional) (no source change).
**Depends on:** P2a-T15
**Acceptance criteria:**
- The chosen v5.2.x GA pin is documented and cross-referenced to the Phase 1 (P1-T06) target.
- Any minor-version gap between fees' pin and midaz's P1-T06 pin is flagged with the (no-shim) `go mod tidy` resolution.
**Tests:** N/A (coordination); verifiable by the Phase 4 owner finding the recorded pins.
**Effort:** S / <1h
**Risk refs:** R3, R4

---

## Exit criteria (phase done when ALL hold)
- plugins-fees `go.mod`: `lib-commons/v5` at v5.2.x GA, `lib-observability v1.0.1`, `lib-auth/v2 v2.8.0`; no `v1.1.0` anything; `lib-commons/v2 v2.9.1` evicted; no `replace`.
- Zero imports of `lib-commons/v5/commons/{log,opentelemetry,zap}`; zero `NewTrackingFromContext`/`NewLoggerFromContext` resolving through `lib-commons/v5/commons` (any alias); telemetry middleware sourced from `lib-observability/middleware`; HTTP helpers still from `lib-commons/v5/commons/net/http`.
- All observability imports use the canonical aliases (`libLog`, `libOpentelemetry`, `libZap`, `libObservability`, `libObsMiddleware`), verified by the path-anchored completeness check — `libObservability` for root lib-observability is the DELIBERATE choice (see canonical-alias decision), with the whole-tree midaz `libCommons`→`libObservability` reconciliation owned by P9.
- **P2a-T17 frozen:** the fee-engine correctness spike artifact exists, proves the leg-sum conservation invariant (sum(legs)==fee total, decimal-exact), records that GetRouteFrom/GetRouteTo are label strings NOT UUIDs (resolving P4-T02), and confirms Send.Value is the sole top-level amount mutation. P4-T01/P4-T02 depend on it.
- `go build ./...`, `go vet ./...`, `make lint`, `make sec` all green; unit + integration + property + fuzz pass; coverage ≥ 85%.
- Full plugins-fees CI green on the migration branch (PD-6 in-place validation gate met).
- Cross-phase pin reconciliation recorded for P1/P4.
- NO code moved into midaz; NO shim/replace/go.work introduced anywhere.

## Risks addressed
- **R4** (reporter+fees ALSO need observability migration; 86+ sites fail to compile in unified module) — the core of this phase; done in-repo first as the Phase 4 precondition.
- **R13** (telemetry middleware split — naive path swap won't compile) — explicitly handled in fees by P2a-T08 (`commons/net/http` split), which the dossier flagged only for tracer but is present in fees.
- **R3** (no lib-commons coexistence / no replace bridge) — fees converges to a single v5.2.x GA + lib-observability with zero shims; orphan indirect evicted.
- **R5** (fee-engine semantic conflicts at the embed — route shape, leg balancing, amount mutation) — front-loaded by P2a-T17's pre-move spike, resolving P4-T02 before the move so P4 inherits decided facts, not open conflicts.
- **R11** (85% coverage hard gate turns CI red) — T13/T15 hold the gate as a phase exit criterion.

## Open items / flags for Fred
1. **Scope was understated AND the canonical-alias claim was wrong.** The brief's "86 sites" is the floor (3 subpackages). Real surface ≈ 159 sites once `NewTrackingFromContext` (67), `NewLoggerFromContext` (**3**, not 2), and the `commons/net/http` middleware split (3) are counted. The long pole is T07 (67+3-site root-symbol move). Separately, the prior draft's claim that `libObservability` is midaz's canonical/dominant root-lib-observability alias is FALSE — midaz HEAD aliases it `libCommons` in 143 files vs `libObservability` in 45, and overloads `libCommons` onto both roots. We standardize fees on `libObservability` DELIBERATELY (to kill the overload ambiguity), accepting that this diverges from midaz's dominant habit; the whole-tree midaz reconciliation is P9's misleading-alias sweep. Confirm you want the deliberate divergence (recommended) vs matching midaz's `libCommons`-on-lib-observability majority.
2. **The `commons/net/http` R13 trap exists in fees, not just tracer.** `internal/http/in/routes.go` uses `commonsHttp.NewTelemetryMiddleware` + `WithHTTPLogging` + `WithCustomLogger` (these moved to `lib-observability/middleware`) alongside `Respond`/`FiberErrorHandler`/`Version` (which stayed). Handled in T08; flagging because the dossier missed it.
3. **`lib-observability/metrics` is NOT needed by fees.** Fees' `internal/metrics/tenant_metrics.go` uses raw `go.opentelemetry.io/otel/metric`, not the lib-commons metrics subpackage (verified 0 imports). The objective listed `metrics` as a target; it is a no-op for fees (confirmed in T03). No action — just narrower than stated.
4. **Pin coordination with Phase 1.** midaz HEAD (this working tree) is on `lib-commons/v5 v5.2.0-beta.12`; the Phase 1 GA bump (P1-T06) has not landed. fees will pin to v5.2.1 (latest v5.2.x GA) by default. If P1-T06 lands on a different v5.2.x GA, the embed re-tidies (MVS, no code change) — recorded in T16. No shim implied. Decision needed only if Phase 1 chooses a NON-v5.2.x line (e.g. jumps to v5.3.x), which would change this phase's target — flag for confirmation.
5. **github_token / GOPRIVATE machinery stays this phase.** Fees still imports `midaz/v3 v3.5.2` (the repoint to in-tree `pkg/mtransaction` is Phase 4). The CI's `go_private_modules: "github.com/LerianStudio/*"` and the Dockerfile's BuildKit secret remain — their removal is a Phase 4/build-harmonization task, not P2a. Touching them here would break the in-place CI validation.
6. **P2a-T17 resolves the P4-T02 open conflict before the move.** The spike's route-shape determination (label strings, not UUIDs — verified at fees HEAD) converts P4.md's "Open behavioral conflict (must resolve, not paper over)" into a decided input: synthetic route values cannot be written to `RouteID`'s uuid-validated field without a `name→ID` resolution step or staying on the passive `Route` field. P4-T02 inherits the answer; it does not rediscover the question at embed time. This is the audit's #2-most-important de-risking after the third-rail validate-reassignment (which lives in P4-T12).
