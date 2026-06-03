# P2c Execution Note — corrections to plan/P2c.md (2026-06-03)

P2c.md was authored against lib-commons **v5.2.1**. P1 actually landed on **v5.4.1**, now authoritative at
`plan/P1.md#p1-frozen-target-pin`. This note records the verified corrections; the implementing agent
targets v5.4.1, NOT v5.2.1. Map verified live against `lib-commons v4@v4.6.3 / v5@v5.1.3 / v5@v5.4.1`,
`lib-observability@v1.0.1`, `lib-auth/v2@v2.8.0`, and the tracer checkout.

## Corrections to apply mentally over P2c.md
- **Target = lib-commons/v5 `v5.4.1`** (every `v5.2.1` in T11/T19/T22/T23/exit/DAG → `v5.4.1`). The two-jump
  *structure* is unaffected (v5.1.3 intermediate is independent of the terminus); only the final-version
  strings change.
- **postgres subpackage sites = 7** (plan says 8).
- **total v4 sites = 221** (plan says 222).
- **mongo-driver v1→v2: N/A** — tracer is postgres-only, zero mongo imports. The biggest v5.4.1 hazard for
  midaz does not exist here.

## Tracer current state (verified)
- `module tracer` (bare; rename to `.../components/tracer` is P5, NOT this phase). `go 1.26.3`.
- lib-commons `v4 v4.6.3` (direct) + `v5 v5.3.0` (indirect via lib-auth). **HEAD is RED** — `config.go:24-25`
  import `v5/commons/{log,zap}` which v5.3.0 removed. T00 repairs to green by pinning lib-auth back to
  **v2.7.0** (holds lib-commons ≤ v5.1.3, pre-split). lib-observability v1.0.0 indirect, zero `.go` imports
  today. lib-streaming absent.

## Two-jump plan (target corrected)
- **Jump 1 (v4 → v5.1.3, lib-auth v2.7.0, delete shim):** T00 → {T05,T06,T07} → T08 → T09 → T10. Commit boundary.
  Shim to delete (`libLogV5`/`libZapV5`/`buildAuthClientLogger`) lives in 4 files: `config.go`,
  `routes_test.go`, `routes_multitenant_test.go`, `middleware/auth_guard_test.go`.
- **Jump 2 (v5.1.3 → v5.4.1 + obs split + lib-auth v2.8.0):** T11(bump v5.4.1) → T12 → {T13,T14,T15,T16} →
  T17 → T18(telemetry-MW move) → T19(assert v5.4.1) → T20 → T21(hash-chain) → T24 → T22. Commit boundary.

## lib-observability re-homing map (exact, v5.4.1)
| From (v5 co-located) | To (lib-observability v1.0.1) | Sites |
|---|---|---|
| `v5/commons/log` | `lib-observability/log` | 63 |
| `v5/commons/opentelemetry` | `lib-observability/tracing` | 48 |
| `v5/commons/opentelemetry/metrics` | `lib-observability/metrics` | 10 |
| `v5/commons/zap` | `lib-observability/zap` | 2 |
| `commons.NewTrackingFromContext` | root `lib-observability` | 105 (46 files) |
| `commons.ContextWithTracer` / `ContextWithMetricFactory` | root `lib-observability` | 6 / 1 |
| telemetry MW (`NewTelemetryMiddleware`, `(*TelemetryMiddleware).WithTelemetry/EndTracingSpans`), `WithHTTPLogging`, `WithCustomLogger` | `lib-observability/middleware` | routes.go |
| `FiberErrorHandler` | **STAYS** in `v5/commons/net/http` | routes.go |

R13: telemetry middleware is a real code move — the single `libHTTP` alias must split into `libHTTP`
(FiberErrorHandler) + `libObsMiddleware` (telemetry+logging). Naive swap reports `FiberErrorHandler` missing.

## Tenant-manager logger-type convergence (the key v5.4.1 semantic)
tmpostgres/client/event/middleware are line-identical v4→v5.4.1 EXCEPT `WithLogger` takes
`lib-observability/log.Logger` at v5.4.1. The T13 `commons/log → lib-observability/log` sweep simultaneously
satisfies tenant-manager `WithLogger`, lib-auth v2.8.0 `NewAuthClient`, and tm event/client/middleware logger
options — one type-converged commit, no adapter.

## FLAG-2 — testcontainers TLS (verify at T00, not assume)
Tracer builds its own DSN with explicit `sslmode` and owns `ValidateSaaSTLS` (`config.go:1376`, gated on
`DEPLOYMENT_MODE=saas`). But verify whether v5.4.1 `libPostgres.New`/`NewMigrator` gates TLS at the CLIENT
layer independent of the DSN sslmode. If yes, tracer's `make test-integration` (plaintext testcontainers
Postgres) needs `ALLOW_INSECURE_TLS=true` in its test recipes — same as P1 did for midaz. Green-build/red-test
trap if missed. Production TLS posture stays tracer's `DEPLOYMENT_MODE=saas` path (a P8 deploy decision).

## Guards
- **Hash-chain byte-unchanged (R19):** migrations `000001/000002/000003/000017` + `VerifyHashChain` in
  `audit_event_repository.go`. Include `000003_prevent_truncate` in the diff guard.
- **Coverage is NOT the net for the riskiest code:** `.ignorecoverunit` excludes `/bootstrap/` + `/cmd/app/`
  (tenant-manager + auth wiring). The real safety net is the MT chaos integration tests
  (`16_*`, `17_*`) + godog e2e. 44 integration tests, 9 `.feature` files, 34 migrations.

## Deliverable
Migrated `../tracer` on a tracer branch, green at v5.4.1, ready for P5 import. Do NOT import into midaz here
and do NOT rename the module path (both are P5).
