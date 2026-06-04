# F0 — Execution Note

> Live record of v4 phase F0 execution: decisions made and commit SHAs as each task landed, mirroring `docs/monorepo/plan/P2a-EXECUTION-NOTE.md`. Authoritative for the gate-zero SHA, baseline counts, drift resolutions, and the `make ci` matrix composition. The companion Gate-3 artifact is `docs/v4/F0-consumer-coordination-inventory.md`.
> - **Date:** 2026-06-04
> - **Branch:** `feat/monorepo-consolidation` (Q1-RESOLVED — no `feat/v4`)
> - **Gate-zero / v4 epoch SHA:** `46706c9eaff0a85fab80f693b1d4f82de56e239b`

---

## Gate-zero SHA + baseline

**Gate-zero (v4 epoch marker):** `46706c9eaff0a85fab80f693b1d4f82de56e239b` — the F0 tip after F0-T02..T05 fixes and the green baseline capture (F0-T06). Every later phase diffs from this SHA.

**Baseline (FINAL, captured at the epoch SHA with a dedicated `GOCACHE=/tmp/midaz-gocache-f0`):**

| Command | Exit | Result |
|---------|------|--------|
| `make test-unit` | 0 | 15,877 tests, 6 skipped, 104.9s |
| `make test-integration` | 0 | 978 tests, 80 skipped, 1082.8s (33 packages, tag-driven discovery) |
| `make test-property` | 0 | 70 tests, 7 skipped |
| `make test-reporter-chaos` | 0 | 39 tests, 39 skipped (CHAOS=1 opt-in gating by design) |
| `make ci` | 0 | single exit code; all four legs reproduced (15,877 / 978 / 70 / 39) = the Gate-1 reproducibility second run |

`make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor; `make ci` is the single-verdict superset.

---

## Commit ledger (chronological)

| SHA | What it closed |
|-----|----------------|
| `4d6acd431` | v4 plan package (`docs/v4/PLAN.md` + `docs/v4/plan/F0.md`..`F6.md`). |
| `d12b7addf` | **F0-T02 / F0-T03** (Gate 2): `tests/helpers/env.go` `:3000`/`:3001` → `:3002`; `tests/helpers/chaos.go` `up-backend`/`down-backend` → `up`/`down`. Working tree was exactly the two expected files; all Gate-2 verification checks passed. |
| `eb7709603` | **F0-T04 / F0-T05** harness: tag-gated targets `test-property` + `test-reporter-chaos` added; root `ci` target = unit → integration → property → reporter-chaos; live-stack legs (`test-bdd`/`test-chaos-system`) opt-in. Change confined to `mk/tests.mk` — no production Go. |
| `75b62bb88` | Reporter-harness stale component paths. **(VOID baseline — see saga v1.)** |
| `40059a431` | **Tag-driven integration discovery**: grep `//go:build integration` + `go list -tags=integration`. (The earlier name-glob ran ZERO packages after `./tests` widening — silent exit 0; this commit replaces it with execution-verified discovery.) Plus chaos template path fix. |
| `da3ee2583` | **PRODUCT fixes** (real bugs found by the now-running integration suite): GitHub #2139 balance-direction default; #2140 `UpdateMany` ctx guard; #2141 NULL-snapshot scan. All three filed as issues and referenced with `Fixes #N`. |
| `29349b9a5` | Round-1 test repairs. Includes **DELETION** of `create_balance_integration_test.go` — an obsolete v3.5.4 backport (resurrection condition below). |
| `11db17732` | Round-2 repairs: async cold-balance drain, container wait strategy, goleak anchors, bootstrap CWD/env rot. |
| `46706c9ea` | Bootstrap goleak anchor. **= EPOCH SHA.** |

---

## Baseline saga (v1 → v5-retry)

The baseline was not green on the first run. Recording the full saga because the *lesson* is load-bearing for every later phase that captures a baseline.

| Run | SHA | Outcome | Problems |
|-----|-----|---------|----------|
| **v1** | `75b62bb88` | **VOID** | Integration discovery ran **0 packages** — exit 0, **silently**. A green that proved nothing. The name-glob, after `./tests` widening, matched no packages. |
| **v2** | `40059a431` | exposed **67 problems** | Build rot + first-run suites surfaced once tag-driven discovery actually ran the integration matrix. |
| **v3** | `29349b9a5` | **26 problems** | After round-1 repairs + the product-bug fixes. |
| **v4** | `11db17732` | **2 problems** | After round-2 repairs. |
| **v5 (first attempt)** | — | **aborted (environmental)** | Go build cache externally purged to 8KB mid-run; disk had 2.7Ti free. Not a code defect. |
| **v5-retry** | `46706c9ea` | **GREEN** | Re-run with a dedicated `GOCACHE=/tmp/midaz-gocache-f0` (external-purge immunity). All legs green; counts recorded above. |

**Gate lesson (binding for later phases):** discovery logic must be verified by **EXECUTION**, not by a `make -n` dry-run. v1 passed a dry-run and ran nothing. Any later phase that adds or changes test discovery must run the real target and confirm a non-zero package/test count, not trust the recipe text.

---

## Drift resolutions

### F0-T09 — negative-grep drift

The authoritative SDK/console negative is the **tracked-only** form:

```
git grep -niE "midaz-sdk|sdk-golang|sdk-typescript|midaz-console"   # 0 actual source consumers
```

Do **NOT** cite the `grep -rniE ... . --exclude-dir=.git` form: at HEAD it returns **non-zero** because the v4 plan documents (`docs/v4/PLAN.md`, `docs/v4/plan/F0.md`, and now the inventory artifact) literally contain the SDK token strings as plan-text self-references. The count grows as more F-phase plan files land; it is unstable and not load-bearing. The macro plan's earlier claim that the `.git`-excluded form returns 0 is **stale** (predates the self-referencing plan docs). What is load-bearing: none of the surviving hits is a real consumer. The artifact cites the `git grep` form per this resolution.

### F0-T07 — `midazName`/`routingName` named-constant drift

The macro plan §4 cites the `midaz`/`routing` authz namespaces as raw string literals at `routes.go:25-28`. At HEAD they are **named constants** — `midazName = "midaz"` (`routes.go:26`), `routingName = "routing"` (`routes.go:27`) — consumed via the `protectedMidaz` (`:226`) / `protectedRouting` (`:230`) wrappers and direct `auth.Authorize(midazName, ...)` calls at `:75/:82/:89`. A grep for bare `"midaz"`/`"routing"` literals at registration sites will not match; the artifact cites the constant names. (`plugin-crm`/`plugin-fees` were already named constants, consistent with the plan.)

### F0-T08 — contract-comment line drift

Macro plan cites the ALLOW/COMMIT atomic contract at `validation_service.go:158-166`. At HEAD the comment block starts at `:152` (load-bearing ALLOW→COMMIT / non-allow→Rollback at `:160-163`); the `:166` `Validate` signature anchor is exact. The artifact cites `:152-166`. Also sharpened: the net-new delta is narrower than the plan framing — `UpsertAndIncrementAtomic` already carries an `expiresAt` parameter (`:318`) and `DeleteExpiredCounters` already exists (`:663`), so TTL *expiry* plumbing is not greenfield; net-new is the `reserved_usage` bucket + `usage_reservations` table + reaper only.

### F0-T10 — postman merge line drift

Macro plan cites `sync-postman.sh:89-97` as the CRM-spec merge leg. At HEAD `:89-97` is the parallel conversion+wait block; the actual merge is `merge_all_collections()` at `:104-105` (loop `for component in ledger crm` at `:110`, jq slurp at `:137`), invoked at `:185`. Masking mechanism unchanged; line citation corrected in the artifact. The five core divergence facts (holder=0, fee=50, `localhost:4003` at `:12`, broken crm `generate-docs` leg, baked `v3.7.0` vs runtime `v3.8.0`) all reproduce at HEAD.

### F0-T12 — latest-tag drift (load-bearing)

Prompt expected `git describe --tags --abbrev=0` = `v3.8.0-beta.9`. At the F0 tip it returns **`v3.8.0-beta.8`** (`766b555d`, ancestor of HEAD), because `git describe` walks ancestry and `v3.8.0-beta.9` (`1d19f9ff`) is off-branch (on `develop`/`main`, not an ancestor of `feat/monorepo-consolidation`). The expectation conflated "highest tag by version sort" with "latest tag reachable from HEAD". Both are 3.x prereleases, so F6-T18's prerelease-base dry-run framing is unchanged; the recorded latest-reachable tag for F6-T18 is `v3.8.0-beta.8`. Also recorded: `.releaserc.yml` has no `tagFormat` key (default `v${version}`) and richer releaseRules than the plan paraphrase (`chore`/`ci`/`test`/`fix`/`docs` → patch; only `breaking: true` → major).

### F0-T11 — eight-vs-seven count

F0.md says "all eight values verified" (`:311`) but "Gate 4 of F6 checks all seven OTEL values" (`:322`). The authoritative count of distinct file:line touchpoints is **eight** (3 components × 2 files = 6 `/v3` + tracer × 2). The "seven" conflates the F6 gate count; not load-bearing for the F0 enumeration.

---

## `make ci` matrix composition (settled at F0-T05)

unit (untagged `-race`) → `test-integration` (`-tags=integration -p 1`, discovery widened to reach `./tests`) → `test-property` (`-tags=property`, defaults `./tests/reporter/property/...`) → `test-reporter-chaos` (`-tags=chaos`, defaults `./tests/reporter/chaos/...`; **compile + invocation** verified, run-bodies opt-in via `CHAOS=1`) → `test-bdd` (`-tags e2e`, live tracer — **opt-in**). The env-gated `test-chaos-system` (live docker-compose stack) is an opt-in leg, NOT default CI. `make ci` produces one exit code and reaches `./tests/reporter`.

**F0-T04 mechanism decisions:**
- Widened integration discovery glob `find ./components ./pkg` → `find ./components ./pkg ./tests` in both `test-integration` and `coverage-integration`, so `tests/reporter/integration` (16 files) is no longer orphaned. (Later superseded at `40059a431` by tag-driven `go list -tags=integration` discovery, after the glob-widening was found to run zero packages.)
- Added `test-property` (no invoking target existed for the 9 reporter property suites).
- Added `test-reporter-chaos` (named to avoid conflation with the env-gated `test-chaos-system`).
- `tests/chaos/*_test.go` (11 files) + `tests/helpers/chaos_test.go` left **deliberately untagged**: `tests/chaos` is the live-stack system suite run by `test-chaos-system` via bare `go test` (gated by `CHAOS=1` + `RunChaosTests(m)` TestMain); tagging it `//go:build chaos` would make `test-chaos-system` compile zero tests and silently pass. `chaos_test.go` is a unit test of the helper. All reporter non-unit suites were ALREADY correctly tagged at HEAD — the gap was 100% target/glob coverage, not tagging; no `//go:build` header edits were needed.

---

## Product bugs found + filed (commit `da3ee2583`)

The now-running integration suite (after tag-driven discovery at `40059a431`) surfaced three real PRODUCT defects — not test defects. All filed as GitHub issues and fixed with `Fixes #N`:

- **#2139** — balance direction default.
- **#2140** — `UpdateMany` ctx guard.
- **#2141** — NULL snapshot scan.

These would have stayed hidden under the void v1 baseline (which ran zero integration packages). They are the concrete payoff of the "verify discovery by execution" lesson.

---

## `create_balance_integration_test.go` deletion + resurrection condition

Deleted at `29349b9a5`: it was an obsolete **v3.5.4 backport** that no longer matched the consolidated repo-level error mapping. **Resurrection condition:** if **F2** adds the repo-level `23505` (unique-violation) mapping, resurrect this test — `F2.md` references `ErrDuplicatedAliasKeyValue`, which is exactly the constraint this test exercised.

---

## Harness debt (deferred to F5)

- **(a)** 25 untagged `*_integration_test.go` files in `components/{ledger,tracer}` — mock-based (sqlmock/miniredis); they run inside the **unit floor**. Name lies, tag is truth.
- **(b)** 31 files tagged `//go:build unit` are **invisible to every target** (`test-unit` runs the untagged packages) — zombie tests. The Q16-conform fix is **removing the `unit` tag**, which changes the baseline → F5 scope, not F0.
- **(c)** Internal tags `itestkit` (7 files) / `testhooks` (2 files) exist but are not bound to any make target.
- **Incidental:** `tests/reporter/chaos` `TestMain` references a stale path (`components/manager/cmd/app`; now `reporter-manager`), so a full `CHAOS=1` run fails at infra setup though it compiles. Reporter-suite harness bug for F5/F6.

---

## Design-fact refinement (flagged for F3)

PLAN.md's "balances commit synchronously at HTTP time" means the **Redis hot state** is the synchronous authority; the **PostgreSQL cold row** persists **async** via `BalanceSyncWorker` (Redis sorted set) in **both** sync and async modes. The F3 anchor (`transaction_create.go:1228`, post-`ProcessBalanceOperations`) remains valid, but any F3 task asserting PG balance state immediately after the HTTP response will fail — tests must drain the balance-sync schedule first (helper `drainBalanceSync` now exists in the `http/in` suite).

---

## Environment notes

- Baseline chains use a dedicated `GOCACHE` for external-cache-purge immunity (a purge to 8KB aborted v5's first attempt despite 2.7Ti free disk).
- Known Docker-inspect flake class under sustained load: organization `CountIsolation` failed once in v4 and passes in isolation — **environment, not defect**. A single such flake on re-run is not a baseline regression.

---

## F0-T01 confirmation

D-1..D-12 FROZEN in `docs/v4/PLAN.md` §2 (header `:39`, D-1 Alias→Instrument `:45`, rows `:45-56`); the ledger lives in `docs/v4/PLAN.md` per the Gate 4 retarget, NOT `docs/monorepo/`. Verification-only; no edits.
