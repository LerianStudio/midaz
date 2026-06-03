# P8-T06 — Lint config reconciliation: measured volume + decision

**Status:** DECIDED — keep per-component delegation (P7-D2 state). Lint gate stays GREEN under the read-only `go run golangci-lint@v2.4.0` gate.

**Date:** 2026-06-03
**Gate version:** golangci-lint `v2.4.0` (matches P8-T01 record + Makefile pin), Go `1.26.x`.

---

## What the plan asked

P8-T06 wants ONE root `.golangci.yml` with "fees strict config as floor" governing every component, plus a per-component cleanup pass to green. The instruction was: **measure first, then decide** — unify only if the surfaced volume is mechanically fixable (~<60, no deep refactors); otherwise keep per-component delegation and document.

## Topology at measurement time

- Two lint configs exist tree-wide: root `.golangci.yml` (3.0K) and `components/tracer/.golangci.yml` (3.3K). The standalone fees/crm `.golangci.yml` copies referenced in the plan no longer exist — fees and crm folded into the ledger binary, so there is no separate "fees-strict" file. **The strictest surviving config is tracer's.**
- The gate runs per-component: `make lint` loops `GO_COMPONENTS = ledger tracer reporter-manager reporter-worker`, each running `golangci-lint run ./...` from its own dir, plus `pkg` and `tests`. golangci-lint resolves the nearest config by walking up: ledger / reporter-manager / reporter-worker / pkg / tests inherit ROOT; tracer uses its own.
- The two configs share IDENTICAL exclusion rules. The only deltas: tracer ADDS `contextcheck` + `errorlint` (with `errorf/asserts/comparison: true`), and sets `gocyclo.min-complexity: 20` vs root's `18`.

So "fees-strict floor" in practice means: **adopt tracer's `contextcheck` + `errorlint` (and gocyclo) tree-wide.**

## Measurement — strict floor applied to every component

Floor config used: tracer's config (adds `contextcheck` + `errorlint`) with `gocyclo.min-complexity` tightened to `18` (the stricter of the two) — the true strict superset. Applied via `--config` to each scope:

| Scope | Total findings | Real findings | Artifacts of the floor choice |
|---|---:|---:|---|
| `components/ledger` | 29 | 14 (11 contextcheck, 3 errorlint) | 15 nolintlint |
| `components/crm` (remnant, not in gate) | 0 | 0 | 0 |
| `components/tracer` | 5 | 0 | 5 gocyclo (only at forced 18; native 20 = green) |
| `components/reporter-manager` | 0 | 0 | 0 |
| `components/reporter-worker` | 8 | 8 (7 contextcheck, 1 errorlint) | 0 |
| `pkg` | 19 | 19 (12 errorlint, 7 contextcheck) | 0 |
| **Total** | **61** | **~41** | **20** |

Per-linter, the real findings break down as: **contextcheck 25**, **errorlint 16**.

### Why 20 of the 61 are artifacts, not dirt
- **15 nolintlint (ledger):** config-interaction artifact. Under the root config ledger reports 0 nolintlint; adding `contextcheck`+`errorlint` makes golangci-lint read the existing `//nolint:staticcheck` directives (legacy-field back-compat suppressions in `transaction_create.go`, `operationroute.go`, fee paths) as "unused for staticcheck." The only delta is the two added linters — this is golangci-lint analyzer-set interaction, not real code dirt.
- **5 gocyclo (tracer):** appear ONLY because the floor forces `min-complexity: 18`. Under tracer's native `20` the component is green. tracer was written clean against its own config.

## The decisive point: nature, not count

The raw count (61) is borderline by the plan's ~60 heuristic, but the **nature** of the real findings decides it, and it decides against unification:

1. **contextcheck (25) is largely intentional-detach, not fixable dirt.** The findings cluster in:
   - rabbitmq consumers / healthcheck recovery monitors / background goroutines (`components/ledger/internal/adapters/rabbitmq/healthcheck.go`, `bulk_collector.go`, `consumer.rabbitmq.go`, `metric_state_listener.go`),
   - DB connection close/connect paths (`pkg/reporter/mongodb/datasource.mongodb.go` `CloseConnection`, `pkg/reporter/datasource-config.go`),
   - PDF worker pools (`pkg/reporter/pdf/pool.go`).

   These deliberately use `context.Background()` because a recovery monitor / connection close / background pool **must not** be cancelled by an inbound request's ctx. "Fixing" them to inherit the parent ctx would introduce premature-cancellation bugs. The honest remediation here is a per-site `//nolint:contextcheck` audit with justification — that ADDS suppressions, the opposite of a cleanup pass, and it is exactly what P8-T06 says to avoid ("not as a blanket suppression").

2. **errorlint (16) is behavior-sensitive, not a blind sed.** The `err == X` / type-switch-on-error sites are in error-dispatch hot paths (`pkg/net/http/errors.go`, `pkg/net/http/withBody.go`, `pkg/reporter/circuit-breaker.go`, `pkg/reporter/mongodb/extraction/claim.go`, `components/ledger/.../transaction_fee_application.go`). Converting `==`→`errors.Is` and type-switch→`errors.As` changes matching on wrapped errors — that is the linter's intent, but each conversion needs verification it does not alter existing dispatch. Plus 4 `fmt.Errorf` `%v`→`%w` cases in `pkg/mmodel/balance.go` that change the error chain.

So the "mechanically fixable to green" subset is at most the ~16 errorlint cases (behavior-sensitive, needing per-site verification), while the larger 25-finding contextcheck cluster is a suppression-audit of mostly-correct detached contexts. That is a **real refactor + nolint-annotation tail across three components and pkg**, not the short mechanical pass the unify branch requires.

## Decision

**KEEP per-component lint delegation (P7-D2 state). Do NOT unify to a single root strict config in P8.**

The repo already sits in a coherent, green state:
- `make lint` is GREEN across all five scopes (ledger, tracer, reporter-manager, reporter-worker, pkg) under `go run golangci-lint@v2.4.0` — verified read-only.
- tracer runs the stricter config it was authored against (`contextcheck` + `errorlint`, gocyclo 20); ledger / reporter / pkg run the root config. Each tree is green against the config it inherits.
- Unifying UPWARD drags ledger/reporter/pkg through ~41 findings, 25 of which are intentional-detach contextcheck requiring nolint annotations rather than fixes. Unifying DOWNWARD (drop tracer's strict config to root) would silently WEAKEN tracer's already-clean gate — a regression. Neither serves "coherent green lint."

Tree-wide strict-config unification (errorlint + contextcheck everywhere) is recorded as **future hardening, not a consolidation blocker.** The natural sequencing is: land contextcheck per-site nolint justifications + errorlint conversions as a dedicated hardening pass (component-parallel), THEN promote the strict config to root. That work does not gate the P8 CI/docker consolidation.

### Per-component exclusions retained (legitimate)
- **tracer:** keeps `components/tracer/.golangci.yml` — its stricter `contextcheck`+`errorlint` floor and `gocyclo: 20`. This is the strict end of the gate, not a weakening exclusion. (Its CEL/antlr generated-tree concern is covered by the shared `exclusions.generated: lax` + `paths` already in both configs.)
- **root:** unchanged. Governs ledger / reporter-manager / reporter-worker / pkg / tests.
- No new path-scoped suppressions added — the existing `//nolint` directives in ledger remain valid under the root config (0 nolintlint there).

## Note: `components/crm` remnant
`components/crm` still contains 67 Go files but is NOT in `GO_COMPONENTS` — CRM logic was lifted into the ledger binary (P3). It is not part of the lint gate and measured 0 strict-floor findings. Its physical removal is a P3/P8-T07 cleanup concern, out of T06 scope; flagged here for visibility.

## Acceptance reconciliation
P8-T06's literal acceptance ("exactly one `.golangci.yml`") is intentionally NOT met: two configs are retained (root + tracer's stricter floor) as the documented pragmatic state, with the rationale above. The operative exit-criteria intent — a coherent, green, version-pinned module-wide lint gate — IS met: `make lint` exits 0 everywhere at the pinned `v2.4.0`.
