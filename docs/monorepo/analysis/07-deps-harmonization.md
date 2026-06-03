# 07 — Cross-Repo Dependency Harmonization

**Scope:** Dependency surface of the monorepo consolidation of `tracer`, `reporter`, `plugin-fees` into `midaz`.
**Target end state (Fred):** liso e final — no shims, no workarounds, no compatibility abstractions.
**Date:** 2026-06-02. **Author:** dependency-harmonization analysis pass.

This document is the dependency layer only. Runtime/deploy collapse, service merge mechanics, and integration points are scoped in sibling dossiers. Here the question is narrow and load-bearing: **can these four `go.mod` files become one (or a clean set) without version conflicts, and what is the ordering of upgrades to get there.**

---

## 0. TL;DR — the three facts that drive everything

1. **The `lib-commons` split is the whole ballgame.** As of `lib-commons/v5 v5.2.0`, the packages `commons/log`, `commons/opentelemetry`, and `commons/zap` were **removed** from lib-commons and re-homed in a new module `github.com/LerianStudio/lib-observability`. Verified directly against the module cache:
   - `lib-commons/v5@v5.1.3/commons/{log,opentelemetry,zap}` → **present**.
   - `lib-commons/v5@v5.2.0/commons/{log,opentelemetry,zap}` → **absent** (gone).
   - `lib-observability@v1.0.1` exports `{log, tracing, metrics, zap, assert, runtime, redaction, middleware}` + root `observability.go` (holds `NewTrackingFromContext`). It has **no dependency on lib-commons** (clean module, `go 1.25.10`).

   midaz is already on the far side of this split (`lib-commons/v5 v5.2.0-beta.12` + `lib-observability v1.0.1`, the "observability-migration" merged at HEAD). **reporter (v5.1.3) and plugin-fees (v5.1.0) are on the near side** — they still import `commons/log` and `commons/opentelemetry`. **tracer (v4.6.3) is two boundaries back.**

2. **There is no `/vN`-path escape for the lib-commons conflict.** All four repos target the *same* major (`v5` for three of them; tracer is on `v4` but its v4 surface must move). Same major = same import path = a single module cannot hold two versions. You cannot have `commons/log` (v5.1) and "no `commons/log`" (v5.2) simultaneously. **Unification, not coexistence, is the only path.** Every incoming repo must migrate to `lib-observability` before/as it lands.

3. **tracer's module is structurally broken for import AND already carries a v4/v5 dual-import shim.** Module path is the bare string `module tracer` (non-qualified, un-importable from any other module). And `internal/bootstrap/config.go` already imports **both** `lib-commons/v4/commons/log` and `lib-commons/v5/commons/log` + `v5/commons/zap` side-by-side, with a comment: *"lib-auth v2.7.0 takes a lib-commons/v5 \*log.Logger; the rest of the codebase is v4."* That is exactly the kind of compatibility shim the end state forbids. It must die in the migration, not survive it.

---

## 1. Module identity & Go toolchain

| Repo | Module path | `go` | `toolchain` | Importable? |
|------|-------------|------|-------------|-------------|
| tracer | `module tracer` | 1.26.3 | — | **No** — bare path, not a URL |
| reporter | `github.com/LerianStudio/reporter` | 1.26 | go1.26.2 | Yes |
| plugin-fees | `github.com/LerianStudio/plugins-fees/v3` | 1.26.3 | — | Yes |
| midaz | `github.com/LerianStudio/midaz/v3` | 1.26.3 | — | Yes (target root) |

**Go version unification:** trivial. All are 1.26.x; the only laggard is reporter at `go 1.26` / `toolchain go1.26.2`. Co-locating under midaz's single `go.mod` forces `go 1.26.3` on all code. No language-level breakage between 1.26.0→1.26.3 (patch releases). **Action: drop the `toolchain` directive, inherit `go 1.26.3` from the unified module.** Zero risk.

**Module identity:** tracer and reporter keep their own services/binaries (moves 1 and 2) but lose their own `go.mod`. They become packages under `github.com/LerianStudio/midaz/v3/components/<name>/...`. tracer's internal self-imports (`tracer/internal/...`, `tracer/pkg/...` — see `config.go:27-40`) must be rewritten to `github.com/LerianStudio/midaz/v3/components/tracer/...`. No external module references the `tracer` path (verified: grep across the other three repos returns nothing), so the rename has zero external blast radius — it is purely an internal find/replace.

---

## 2. Lerian shared-library version matrix

| Dependency | tracer | reporter | fees | midaz | **target-unified** |
|---|---|---|---|---|---|
| **lib-commons** | **v4.6.3** (direct) | **v5.1.3** | **v5.1.0** | **v5.2.0-beta.12** | **v5.2.x stable** (≥ the GA of beta.12; cache shows v5.2.0, v5.3.x, v5.4.1 exist) |
| lib-commons/v5 (transitive in tracer) | v5.3.0 *(indirect)* | — | — | — | resolved by unification |
| lib-commons/v2 (transitive in fees) | — | — | v2.9.1 *(indirect)* | — | should drop out after dedupe |
| **lib-observability** | v1.0.0 *(indirect only)* | **absent** | v1.1.0-beta.5 *(direct, but unused in code)* | **v1.0.1** (direct, used) | **v1.0.1** (match midaz) — or latest stable |
| **lib-auth/v2** | v2.8.0 | v2.7.0 | v2.7.0 | v2.8.0 | **v2.8.0** |
| **lib-streaming** | — | — | — | **v1.4.0** | v1.4.0 (midaz-only; unaffected) |
| **lib-license-go/v2** | — | — | **v2.3.4** | — | v2.3.4 (fees-only; carries in) |

### Reading the matrix

- **lib-commons is the only hard conflict**, and it is unavoidable because it spans a package-set change (the observability split), not just a version bump.
- **lib-auth** is a clean minor bump (v2.7.0 → v2.8.0) for reporter and fees. Same major, same import path (`lib-auth/v2/auth/middleware`), used identically across all four. Low risk — bump and rebuild.
- **lib-observability:** midaz uses v1.0.1 directly (244 `log` + 200 `tracing` import-sites). plugin-fees declares v1.1.0-beta.5 in `go.mod` but **does not import it in code** (0 source references) — it is a stale/aspirational require, likely pulled transitively or pre-staged. reporter does not declare it at all. **Target: everyone resolves to midaz's v1.0.1** (or whatever stable midaz advances to). The beta in fees should be dropped to the unified pin.
- **lib-streaming + franz-go** are midaz-exclusive (the streaming subsystem). Nothing incoming touches them. They simply remain in the unified `go.mod`; the incoming code doesn't import them, so no conflict. (Relevant only because plugin-fees embeds into ledger and ledger emits events — but fees itself does not produce events today.)
- **lib-license-go/v2 v2.3.4** is plugin-fees-only (`lib-license-go/v2/middleware`). It carries into the unified module as a new direct dep. midaz does not currently use it; after the fees embed, ledger's binary will link it. Confirm there is no version expectation collision with midaz's licensing posture (midaz uses `lib-commons/v5/commons/license`, a different package — no path clash).

---

## 3. The conflict that breaks a single `go.mod`

### 3.1 lib-commons major/package-set conflict (CRITICAL)

This is the only conflict that *cannot* be resolved by MVS (minimum version selection) picking the highest version, because the highest version (v5.2+) **removed packages that the lower versions import**.

```
reporter:  import "lib-commons/v5/commons/log"          // exists in v5.1.3
plugin-fees: import "lib-commons/v5/commons/log"         // exists in v5.1.0
midaz:     (no such import — moved to lib-observability/log)
unified go.mod resolves lib-commons/v5 → v5.2.0-beta.12  // commons/log GONE → reporter+fees FAIL TO COMPILE
```

Concrete failing import paths if reporter/fees code is dropped into the unified module as-is:
- `github.com/LerianStudio/lib-commons/v5/commons/log` — **159 sites in reporter, 43 in fees**
- `github.com/LerianStudio/lib-commons/v5/commons/opentelemetry` — **71 in reporter, 36 in fees**
- `github.com/LerianStudio/lib-commons/v5/commons/zap` — **11 in reporter, 7 in fees**

All of these must be rewritten to lib-observability *before* the code compiles in the unified module. There is no shim that survives the end-state rule. This is non-negotiable, mechanical, and large.

### 3.2 tracer's bare-module + v4/v5 dual-import shim (CRITICAL)

tracer compiles today against `lib-commons/v4` for its own code (96 files, ~1500 symbol sites) while *also* importing `lib-commons/v5/commons/log` and `v5/commons/zap` in `config.go` purely to satisfy lib-auth v2.7.0's logger type. Two majors of lib-commons coexist in tracer's module **today** — legal only because they are different majors (`/v4` vs `/v5` are distinct import paths). In the unified module, tracer must converge to a single source of truth (lib-observability + lib-commons/v5), and the v4 imports plus the `libLogV5`/`libZapV5` shim aliases must all be deleted.

### 3.3 What is NOT a conflict (handled by MVS)

The third-party skew below all resolves cleanly because every divergence is same-major, and MVS picks the highest — none removed a package the others depend on:

| Dep | tracer | reporter | fees | midaz | resolves to |
|---|---|---|---|---|---|
| go.opentelemetry.io/otel | 1.44.0 | 1.43.0 | 1.43.0 | 1.44.0 | **1.44.0** |
| gofiber/fiber/v2 | 2.52.13 | 2.52.13 | 2.52.13 | 2.52.13 | **2.52.13** (already aligned) |
| Masterminds/squirrel | 1.5.4 | 1.5.4 | — | 1.5.4 | **1.5.4** |
| jackc/pgx/v5 | 5.9.2 | 5.9.2 | 5.9.2 (indirect) | 5.9.2 | **5.9.2** (already aligned) |
| go.mongodb.org/mongo-driver | 1.17.9 | 1.17.9 | 1.17.9 | 1.17.9 | **1.17.9** (aligned) |
| go.mongodb.org/mongo-driver/v2 | 2.6.0 (ind) | 2.5.0 | — | — | **2.6.0** ⚠ see note |
| redis/go-redis/v9 | 9.19.0 | 9.18.0 | 9.18.0 | 9.20.0 | **9.20.0** |
| valyala/fasthttp | 1.71.0 | 1.69.0 | 1.71.0 | 1.71.0 | **1.71.0** |
| testcontainers-go | 0.42.0 | 0.41.0 | 0.41.0 | 0.42.0 | **0.42.0** |
| go-playground/validator/v10 | 10.30.3 | 10.30.1 | 10.30.1 | 10.30.2 | **10.30.3** |
| golang-migrate/migrate/v4 | 4.19.1 | 4.19.1 | 4.19.1 | 4.19.1 | **4.19.1** (aligned) |
| rabbitmq/amqp091-go | 1.11.0 | 1.10.0 | 1.10.0 | 1.11.0 | **1.11.0** |
| google/uuid | 1.6.0 | 1.6.0 | 1.6.0 | 1.6.0 | **1.6.0** (aligned) |
| shopspring/decimal | 1.4.0 | 1.4.0 | 1.4.0 | 1.4.0 | **1.4.0** (aligned) |
| grpc | 1.81.1 | 1.80.0 | 1.81.1 | 1.81.1 | **1.81.1** |

> ⚠ **mongo-driver/v2 note:** tracer carries it only as an indirect (v2.6.0); reporter uses it directly (v2.5.0). Both v1.17.9 (`go.mongodb.org/mongo-driver`) and v2.x (`go.mongodb.org/mongo-driver/v2`) are *distinct import paths* and legally coexist (different majors), which is why all four already carry both. MVS picks v2.6.0. The v1→v2 BSON API differs, but since this is path-distinct coexistence, no rewrite is forced by unification — only if someone later wants to consolidate onto a single driver major (separate, optional cleanup, out of scope for "make it compile").

### 3.4 Net-new dependency surface entering the unified module

These come in via the incoming repos and have **no existing midaz counterpart** — pure additions, no conflict, but they enlarge the build/security/CI surface of the single module:

- **reporter (heaviest contributor):** `chromedp` + `chromedp/cdproto` (headless Chrome — PDF rendering), full `aws-sdk-go-v2` stack (S3 + Secrets Manager), `microsoft/go-mssqldb`, `sijms/go-ora/v2` (Oracle), `go-sql-driver/mysql`, `flosch/pongo2/v6` (templating), `go-resty/resty/v2`, `cloud.google.com/go/*` (GCS auth), plus a large testcontainers module fan-out (mongodb, mssql, mysql, postgres, rabbitmq, redis). This roughly doubles the third-party footprint.
- **tracer:** `google/cel-go` + `cel.dev/expr` (CEL expression engine for rules), `alicebob/miniredis/v2`, `cucumber/godog` (BDD), `prometheus/*` (direct metrics).
- **plugin-fees:** `iancoleman/strcase` (already in midaz), `lib-license-go/v2`, `Shopify/toxiproxy` (already in midaz).

Implication for a **single** `go.mod`: the ledger binary (after fees + crm embed) would transitively *link* chromedp, AWS SDK, Oracle/MSSQL drivers, etc., **if** tracer/reporter share the same module. See §6 for whether that is acceptable or whether reporter/tracer should stay separate `go.mod`s under a workspace.

---

## 4. lib-commons v4 → v5 migration for tracer — API delta

tracer is the deepest migration: a major bump **and** the observability extraction in one move. The delta breaks into three buckets by destination.

### Bucket A — moves to `lib-observability` (the split)
These v4 packages no longer exist under v5/commons; their symbols live in lib-observability:

| v4 import (tracer today) | sites | → unified destination |
|---|---|---|
| `lib-commons/v4/commons/log` | 63 import-stmts / ~1400 symbol-uses | `github.com/LerianStudio/lib-observability/log` |
| `lib-commons/v4/commons/opentelemetry` | 48 | `github.com/LerianStudio/lib-observability/tracing` |
| `lib-commons/v4/commons/opentelemetry/metrics` | 10 | `github.com/LerianStudio/lib-observability/metrics` |
| `lib-commons/v4/commons/zap` | 2 | `github.com/LerianStudio/lib-observability/zap` |

Symbol-level (these are the high-frequency call sites that must keep working post-move):
- `libLog.String / .Int / .Any / .Bool / .Err / .Logger / .Level{Info,Warn,Error,Debug} / .Field / .NewNop` → identical surface in `lib-observability/log` (midaz uses exactly these, alias `libLog`).
- `libOtel.HandleSpanError` (477), `HandleSpanBusinessErrorEvent` (171), `SetSpanAttributesFromStruct` (18), `SetSpanAttributesFromValue` (9), `Telemetry`, `TelemetryConfig`, `NewTelemetry`, `InitializeTelemetryWithError` → all in `lib-observability/tracing` (midaz alias `libOpentelemetry`). midaz's own `HandleSpanError`/`HandleSpanBusinessErrorEvent` usage confirms these names survived intact.
- `NewTrackingFromContext` (301 sites in tracer!) → moved to the **root** `lib-observability` package (`libObservability.NewTrackingFromContext`), confirmed by midaz's `create_account.go:34`. In v4 this lived in `lib-commons/v4/commons`. **This is the single highest-volume rename in the whole consolidation** (301 sites in tracer alone).
- `ContextWithTracer` (8), `ContextWithMetricFactory` (3) → root `lib-observability` (context helpers; `context_helpers.go` in the module).

### Bucket B — stays in `lib-commons/v5/commons` (same package, just major bump)
These v4 `commons` symbols still live in lib-commons under v5 — only the import path major changes `/v4` → `/v5`:

- `ValidateBusinessError` (34), `App` (16), `RunApp` (11), `Launcher` (11), `NewLauncher` (6), `LauncherOption` (3), `SetConfigFromEnvVars` (9), `WithLogger` (3), `ValidateServerAddress` (3), `IsNilOrEmpty` (3), `Contains` (3), `GetMapNumKinds` (3), `Response` (11).
- `lib-commons/v4/commons/postgres` (8) → `v5/commons/postgres`, `runtime` (4) → `v5/commons/runtime`, `net/http` (3) → `v5/commons/net/http`, `server` (1) → `v5/commons/server`.
- `tenant-manager/{postgres,client,event,redis,core}` (29 sites) → `v5/commons/tenant-manager/*`. **Verify subpackage layout did not reshuffle** — v5.2 still lists `tenant-manager/` under commons (confirmed in cache). The MT wiring (`config_multitenant_wiring.go` imports postgres/client/event/redis tenant-manager subpackages) is the riskiest surface here; tenant-manager APIs are the most likely to have evolved across a v4→v5 major. **This needs a focused API-diff pass** — not assumed safe.

### Bucket C — the shim to delete
- Remove the dual `libLogV5`/`libZapV5` imports (`config.go:24-25`) and the v4 `libLog`/`libZap` aliases. After migration everything is one logger type from lib-observability; the lib-auth v2.8.0 logger coupling that motivated the shim must be re-checked — confirm lib-auth v2.8.0's expected logger type matches `lib-observability/log.Logger` (it should, since midaz on lib-auth v2.8.0 + lib-observability v1.0.1 compiles).

**Scope estimate for tracer migration: ~96 Go files touched, ~1500 symbol sites, dominated by `NewTrackingFromContext` (301), `HandleSpanError` (477), and `libLog.String` (830 — but mechanical alias-stable rename).** Most of the volume is package-path rewrites where the symbol surface is byte-identical to midaz's current usage, which makes it largely automatable (gofmt-aware sed / `goimports` re-alias), but the tenant-manager v4→v5 surface and the Telemetry init path need human eyes.

> **Why is this *less* scary than the count suggests?** midaz already completed the exact same observability migration (the merged "observability-migration" PR at HEAD). The destination API is known-good and in-tree. The migration is "make tracer's imports look like ledger's imports," not "discover a new API." reporter and plugin-fees are an *easier* version of the same move (they start from v5.1, so no major bump — only the observability extraction).

---

## 5. The observability migration also hits reporter and plugin-fees

This is the finding most likely to be missed by a planner who only read the "tracer is on v4" headline. **reporter and plugin-fees are NOT safe just because they're on v5.** They are on v5.**1**, the pre-split line.

| Repo | lib-commons today | `commons/log` sites | `commons/opentelemetry` sites | `commons/zap` sites | Migration needed to reach midaz's v5.2+ |
|---|---|---|---|---|---|
| reporter | v5.1.3 | 159 | 71 | 11 | **Yes** — full observability extraction (no major bump) |
| plugin-fees | v5.1.0 | 43 | 36 | 7 | **Yes** — full observability extraction (no major bump) |

For both: rename `lib-commons/v5/commons/log` → `lib-observability/log`, `commons/opentelemetry` → `lib-observability/tracing`, `commons/zap` → `lib-observability/zap`, and `commons.NewTrackingFromContext` → `lib-observability.NewTrackingFromContext`. Same mechanical transform as tracer Bucket A, minus the `/v4`→`/v5` step and minus the tenant-manager major risk. Neither currently imports lib-observability in code, so this is greenfield-but-mechanical.

**plugin-fees subtlety:** it embeds into `components/ledger` as a *service collapse* (move 3), so after the embed its code runs inside the ledger binary and **must** use the exact same lib-observability version and aliases as ledger. Any residual `commons/log` import in fees would fail to compile the moment fees code is in ledger's module. The observability migration of fees is therefore a *precondition* of the embed, not a follow-up.

---

## 6. Single go.mod vs go.work — the structural decision

The end-state rule ("no abstractions de compatibilidade") pushes toward **one `go.mod`**. But the dependency surface argues for nuance, and this is a genuine decision, not a default:

- **Moves 3 & 4 (fees→ledger, crm→ledger) are service collapses.** Their code runs *inside* the ledger binary. They **must** share ledger's module and `go.mod`. No choice. fees + crm dependencies merge into midaz's go.mod unconditionally. (crm is already in midaz's module — same go.mod today — so only fees' net-new deps enter: `lib-license-go/v2`.)
- **Moves 1 & 2 (tracer, reporter) keep their own service/binary.** They are co-located components, like `components/crm` and `components/ledger` are today. midaz's existing pattern is **one module, multiple components, multiple `cmd/app/main.go`** (ledger + crm already live this way under a single root `go.mod`). Following that pattern means tracer and reporter also share the single root `go.mod` — which drags chromedp, AWS SDK, Oracle/MSSQL drivers, CEL, godog, miniredis into the *same* module graph as the ledger binary.
  - Go's linker is per-binary and dead-code-eliminates unused packages, so the ledger binary will **not** ship chromedp unless ledger imports it. The bloat is in the *module graph* (go.sum size, `go mod download` time, vuln-scan surface, CI build matrix), not the ledger artifact.
  - But it does mean a CVE in chromedp or the Oracle driver shows up in `make sec` for the *whole monorepo*, and a `go test ./...` pulls every testcontainer image.

**Recommendation: single root `go.mod`, mirroring the existing ledger+crm pattern.** It is the simplest, matches the "liso e final" intent, and Go's per-binary DCE keeps artifacts clean. Accept the enlarged module graph as the cost of one coherent repo. Only fall back to a `go.work` multi-module layout if the merged vuln-scan / build-time surface becomes operationally painful — and that is a reversible decision made *after* measuring, not a pre-emptive abstraction. Do not introduce `go.work` speculatively; it is itself a form of the compatibility scaffolding the end state rejects.

---

## 7. Recommended unified target versions (the pin list)

```
github.com/LerianStudio/lib-commons/v5        v5.2.0-beta.12  → bump to v5.2.x GA (cache shows v5.2.0 released)
github.com/LerianStudio/lib-observability      v1.0.1
github.com/LerianStudio/lib-auth/v2            v2.8.0
github.com/LerianStudio/lib-streaming          v1.4.0          (midaz-only, unchanged)
github.com/LerianStudio/lib-license-go/v2      v2.3.4          (from fees)
go.opentelemetry.io/otel (+ trace/metric/sdk)  1.44.0
github.com/gofiber/fiber/v2                    v2.52.13
github.com/jackc/pgx/v5                        v5.9.2
go.mongodb.org/mongo-driver                    v1.17.9
go.mongodb.org/mongo-driver/v2                 v2.6.0
github.com/redis/go-redis/v9                   v9.20.0
github.com/valyala/fasthttp                    v1.71.0
github.com/testcontainers/testcontainers-go    v0.42.0
github.com/go-playground/validator/v10         v10.30.3
github.com/golang-migrate/migrate/v4           v4.19.1
github.com/rabbitmq/amqp091-go                 v1.11.0
go 1.26.3   (drop reporter's toolchain directive)
```

Net-new directs carried in: `google/cel-go`, `chromedp` stack, `aws-sdk-go-v2` stack, `go-mssqldb`, `go-ora/v2`, `go-sql-driver/mysql`, `pongo2/v6`, `resty/v2`, `cloud.google.com/go/*`, `lib-license-go/v2`, `miniredis/v2`, `cucumber/godog`.

**Open item to resolve before pinning:** midaz is on a *beta* of lib-commons (`v5.2.0-beta.12`). The cache shows `v5.2.0` (GA), `v5.3.x`, `v5.4.1` exist. Consolidating is the right moment to move midaz off the beta onto a v5.2+ GA. Confirm lib-observability stays compatible (it has no lib-commons dep, so the coupling is one-directional and loose).

---

## 8. Ordering of upgrades

The dependency graph dictates a strict order. Doing it out of order means migrating code twice.

1. **Land midaz off the lib-commons beta first** (optional but clean): `v5.2.0-beta.12` → `v5.2.x` GA, keep `lib-observability v1.0.1`. Establishes the stable target all incoming code will be rewritten *to*. Smallest possible change, lands independently, de-risks everything downstream.

2. **Migrate plugin-fees to the observability split, IN PLACE, before embedding** (move 3 precondition). v5.1.0 → v5.2.x + lib-observability. 86 import-sites (43+36+7). No major bump. Verify against fees' standalone tests while it still has its own `go.mod`. Then embed into ledger — at which point it joins ledger's go.mod and the import paths already match.

3. **Migrate reporter to the observability split, IN PLACE.** v5.1.3 → v5.2.x + lib-observability + lib-auth v2.7.0→v2.8.0. 241 import-sites (159+71+11). No major bump. Test against reporter's own go.mod/CI, then co-locate as `components/reporter` (manager + worker).

4. **Migrate tracer LAST — it's the hardest.** Do it in two sub-steps to avoid conflating two failure modes:
   a. **v4 → v5.1** first (pure major bump, packages still co-located in v5.1 — log/otel/zap still under commons). Gets the `/v4`→`/v5` path rewrites and the tenant-manager v4→v5 API delta done while the package *layout* is still familiar. Delete the `libLogV5`/`libZapV5` shim here (now redundant — everything is v5).
   b. **v5.1 → v5.2 + lib-observability** (the split), identical transform to steps 2–3. Then fix the module path (`module tracer` → import-path-correct) and rewrite internal self-imports, and co-locate as `components/tracer`.
   *(Alternative: do the v4→v5.2 jump in one shot. Faster but conflates the major-bump API risk with the split-rename mechanics in a single un-bisectable change. For 96 files / 1500 sites, the two-step is worth the extra commit.)*

5. **Embed crm into ledger** (move 4). crm already shares midaz's go.mod, so this is a code/service merge, **not** a dependency event. No go.mod change beyond whatever crm-only deps fold into ledger's import graph.

6. **Final `go mod tidy` on the unified module.** Collapse the duplicate indirects (e.g. fees' transitive `lib-commons/v2 v2.9.1`, tracer's transitive `lib-commons/v5 v5.3.0`), let MVS settle the third-party skew per §3.3, and run `make lint && make test-unit && make sec` across the whole tree.

**Critical sequencing rule:** steps 2, 3, 4 must each be validated *in their own repo, against their own go.mod and CI*, **before** co-location/embed. Migrating observability and co-locating in the same commit destroys your ability to bisect a regression between "the rename broke it" and "the module merge broke it."

---

## 9. Residual risks & verification gaps

- **tenant-manager v4→v5 API drift (tracer):** the 29 tenant-manager import-sites span postgres/client/event/redis/core subpackages used in MT wiring (`config_multitenant_wiring.go`). A v4→v5 *major* most plausibly changed signatures here. **Not verified at symbol level in this pass** — flagged as the highest-uncertainty surface in the tracer migration. Needs a dedicated API diff against the actual v5.2 tenant-manager package before estimating.
- **lib-auth v2.8.0 logger type coupling:** the entire reason tracer's v4/v5 shim exists. Confirm v2.8.0's middleware logger parameter accepts `lib-observability/log.Logger`. midaz (lib-auth v2.8.0 + lib-observability v1.0.1) compiles, which is strong circumstantial evidence it's fine, but the tracer auth-client wiring (`config.go:1406 buildAuthClientLogger`) should be re-pointed deliberately, not left dangling.
- **plugin-fees stale `lib-observability v1.1.0-beta.5` require:** declared but unused in code (0 source refs). Could be a half-finished migration attempt. Drop to the unified v1.0.1 pin; do not let a beta leak into the monorepo go.mod.
- **midaz on a beta lib-commons:** consolidating onto a stack pinned to `v5.2.0-beta.12` is fragile. Move to GA as step 1.
- **mongo-driver dual-major:** v1.17.9 and v2.x coexist across all repos today (path-distinct, legal). Unification does not force a fix, but the monorepo now carries both BSON APIs indefinitely — a latent cleanup, out of scope here.
- **CI / Makefile unification (out of scope for deps, but adjacent):** each repo has its own `build.yml`, `release.yml`, `go-combined-analysis.yml`, `pr-security-scan.yml`, plus `mk/*.mk` includes. These are not dependency conflicts but they *do* interact with the single-module decision — a single `go.mod` wants a single `go test ./...`/`make lint` surface, which means the per-repo workflow fan-out must be reconciled. Scoped elsewhere; noted because the "one module" recommendation makes it mandatory.
