# P2b Execution Note — corrections to plan/P2b.md (2026-06-03)

P2b.md was authored against an assumed lib-commons v5.2.0 pin and is **inverted on the mongo-driver
direction**. P1 executed on **v5.4.1** (mongo-driver-v2-only). Verified live against
`/Users/fredamaral/repos/lerianstudio/reporter` + lib-commons v5.4.1 cache.

## Target (corrected)
lib-commons/v5 **v5.4.1**, lib-observability **v1.0.1** (direct; absent today), lib-auth/v2 **v2.8.0**
(from v2.7.0), mongo-driver **v2 v2.6.0** (NOT v1). lib-streaming v1.4.0 is in the frozen line but reporter
uses NONE — no streaming work in P2b. Reporter currently: lib-commons **v5.1.3** (single jump, NO two-jump),
mongo-driver BOTH v1.17.9 (direct, ~46 sites) + /v2 v2.5.0 (1 test file), go 1.26 / toolchain go1.26.2.
Module `github.com/LerianStudio/reporter`. Two binaries (manager :4005 REST, worker :4006 chromedp) over a
shared `pkg/` (23 packages).

## THE INVERSION (biggest correction)
P2b-T10 says "collapse dual driver ONTO v1.17.9, rewrite the single v2 test file." **WRONG under v5.4.1.**
`pkg/mongodb/connection.go:14` wraps `lib-commons/v5/commons/mongo`; at v5.4.1 that base returns a
**mongo-driver v2** `*mongo.Client`, so the instant lib-commons bumps, `MongoConnection.GetDB()` return type
flips and **all ~46 v1 sites break**. This is a forced repo-wide **v1→v2** migration (same as midaz/fees),
NOT a collapse-onto-v1. Use `docs/monorepo/plan/P1-mongo-v2-migration-map.md` as the template. Surface:
mongo (29) + bson (24) + options (9) + primitive→bson (4) + mtest removal (2, no v2 equivalent). **`bson.D`
decode trap is LIVE** at `pkg/mongodb/deadline/indexes.go:393,398` (`.(bson.M)` on decoded index-spec →
silently `ok=false` in v2; fix representation-agnostic + regression test). No custom Decimal codec in prod
(lighter than fees), but spot-check `datasource_schema_discovery.go` decimal path.

## obs split (confirmed families, counts)
`commons/log`→`lib-observability/log` (159 files), `commons/opentelemetry`→`lib-observability/tracing` (71,
incl. 448 HandleSpanError/HandleSpanBusinessErrorEvent), `commons/zap`→`lib-observability/zap` (11), root
`commons.{NewTrackingFromContext,NewLoggerFromContext,ContextWith*,CustomContextKey,CustomContextKeyValue}`
→ root `lib-observability` (`ContextKey`/`ContextValue` — `.Logger`/`.Tracer`/`.HeaderID` fields survive).
**`commons/runtime`: 0 sites (reporter doesn't use the trident).** net/http telemetry MW (6 files; `routes.go`)
→ `lib-observability/middleware`; HTTP helpers + `FiberErrorHandler` STAY on `commons/net/http` (R13 split).
**Compile-breaker: `pkg/ctxutil/context.go`** depends on deleted `commons.CustomContextKey`/`CustomContextKeyValue`/
`log.NopLogger` → re-source onto `libObservability.ContextKey`/`ContextValue` (62 production callers downstream).
lib-auth `NewAuthClient(addr,enabled,*lib-observability/log.Logger)` at 2 prod sites (manager config.go, worker
config_fetcher.go) — logger vars become lib-observability/log type. Alias hygiene (T12): keep `libCommons`→
lib-commons/v5/commons (lifecycle, 33 files), `libObservability`→lib-observability; do NOT inherit midaz's collision.

## TLS (v5.4.1 default)
Reporter has NO `commons/postgres` (its postgres is raw-pgx customer-datasource clients). But v5.4.1 TLS-default
lands on reporter's **mongo/redis/rabbit** lib-commons connections (13 TLS-surface files). Reporter ALREADY has
an `ALLOW_INSECURE_TLS` knob (manager+worker config + .env.example) — wire `ALLOW_INSECURE_TLS=true` into
`mk/tests.mk` integration recipes (target confirmed: `make test-integration`, Makefile:135). Verify raw-driver
datasource testcontainers (pg/mysql/mssql/oracle) aren't transitively TLS-forced.

## Manager vs worker
No dep-SET divergence (shared pkg/). Depth differs: manager touches mongo directly (11 files); worker via shared
`pkg/mongodb` (20 files) — both break identically when pkg/mongodb migrates. Both wire lib-auth + ALLOW_INSECURE_TLS.

## Gate-0 slices (single-jump; obs first, then mongo-v2+bump)
**PART 1 (lib-commons stays v5.1.3):** add lib-observability v1.0.1 + lib-auth v2.8.0; migrate obs imports
(log 159 → tracing 71 → zap 11 → root tracking/context); ctxutil re-source + propagation round-trip test;
telemetry-MW split. Build green (commons/* still exist at v5.1.3 but unused). Commit.
**PART 2 (the forcing bump):** bump lib-commons v5.1.3→**v5.4.1** + mongo v1→v2 migration (46 sites, primitive→bson,
options builder, mtest rework, the indexes.go bson.M→agnostic fix + regression test) + ALLOW_INSECURE_TLS recipe.
Full gate: build/vet/lint/test-unit/test-integration/sec. Commit. (mongo-v2 MUST land with the bump — v5.4.1
forces it.)

## Flags
- No third-rail risk (no double-entry/balance/license/client-data surface; ctxutil re-source + MW move are genuine
  moves, not shims).
- lib-streaming v1.4.0 in the frozen line but reporter uses none — NO streaming work in P2b (that's P6 go.mod inheritance).
- Toolchain: reporter on go1.26.2; the pending global go1.26.4 CVE bump (GO-2026-5039/5037) applies here too.
