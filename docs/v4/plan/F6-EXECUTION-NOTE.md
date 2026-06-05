# F6 Execution Note — `/v4` Module Bump and Release Alignment

Phase F6 of the Midaz v4 program: the mechanical `github.com/LerianStudio/midaz/v3` → `/v4`
module bump, the non-Go residue sweep, the v3→v4 migration guide, CI/S3 alignment for tracer
migrations, the semantic-release dry-run proof, and the external-lockstep deploy gate.
Executed 2026-06-05 on `feat/monorepo-consolidation`. F6 base: `909b3608c` (F5 tip).
F6 tip at note time: `cb1be73aa`.

This is the final phase. The production tag remains DEPLOY-GATED on the §6 lockstep register.

---

## 1. Commit ledger (chronological, on top of the F6 base `909b3608c`)

| SHA | Subject | Tasks |
|-----|---------|-------|
| `5eaaf8db7` | `refactor(core)!: bump module path to github.com/LerianStudio/midaz/v4` | F6-T02, F6-T03 |
| `6f2d33d77` | `chore(v4): non-Go /v3 sweep — OTEL attribution, docs, governance link` | F6-T04..T08, T10, T11 |
| `71dc97b61` | `docs(v4): v3-to-v4 migration guide — REST and Go importer tables (F6-T13)` | F6-T13 |
| `cb1be73aa` | `ci: publish tracer migrations to S3 alongside ledger (additive, Q15; F6-T14)` | F6-T14 |

`5eaaf8db7` is the **only BREAKING CHANGE carrier in the entire v4 program** — it carries both
the `!` marker and an explicit `BREAKING CHANGE:` footer. No other phase used `!` markers;
release semantics were deliberately centralized on this one commit (PLAN §12 / R5).

Path-only audit of the mechanical commit:

- `go.mod` delta = the module line only; no `replace`/`retract`; `go.sum` zero delta (Gate 2).
- `go mod tidy` was run but **not** absorbed: tidy reclassified `golang.org/x/sync` from
  indirect to direct — pre-existing F1–F5 tidy drift, unrelated to the bump — and was reverted
  to keep the commit strictly path-only.
- 49 files flagged by `gofmt -l` were **already** gofmt-dirty at the F6 base (pre-existing
  formatting debt). The `/v3`→`/v4` token swap is same-length and same-import-group, so the
  rewrite introduced zero new gofmt issues. They were deliberately not formatted: doing so
  would expand the mechanical commit beyond path-only.

## 2. Census (F6-T01, measured at the real HEAD `909b3608c`)

F6.md §0 mandates re-measurement at execution time; the frozen plan baselines were pinned at
`2bdbfe556`, before F1–F5 landed. Execution-time figures govern.

- Go imports of `midaz/v3`: **4,436 occurrences across 1,500 tracked `.go` files** (authoritative,
  via `git grep`). The workflow's first census figure (7,358/1,559) was an artifact of a broken
  grep wrapper (see §4 tooling caveat) and is non-load-bearing — the gate is grep-zero, not a
  count match.
- Non-Go `midaz/v3` tracked files: 57 total; curated sweep surface (excluding `CHANGELOG.md`,
  frozen `docs/monorepo/`, and `docs/v4/` plan sources) = 21 files.
- All F1–F5-owned residue confirmed cleared at census time: `mmodel.Alias` = 0, `plugin-crm`
  RBAC namespace in code = 0, `/v1/.../aliases` routes = 0, `components/crm/api/` deleted,
  tracer `0.1.0` / reporter-manager `1.2.0` spec literals = 0, `v3.7.0` ship surface = 0.
  Zero defects routed back to earlier phases.
- F0 harness fixes survive: `tests/helpers/env.go` defaults `:3002`; `chaos.go` calls
  `up`/`down`; `make ci` chains unit→integration→property→reporter-chaos.

## 3. Sweep and pipeline record (F6-T04..T14)

- **OTEL attribution (T04, Gate 4)**: all committed `.env.example` values on
  `github.com/LerianStudio/midaz/v4/components/<x>`; tracer's wrong `LerianStudio/tracer`
  value (the trap a `/v3`→`/v4` grep silently skips — it contains no `/v3`) replaced with the
  monorepo path. Gitignored working-tree `.env` files were also updated for developer
  consistency (they cannot reach a commit) and verified directly with `/usr/bin/grep`, since
  `git grep` cannot see untracked files.
- **GOVERNANCE.md (T10)**: malformed `/v3/blob/...` GitHub link stripped; 0 `midaz/v3` matches.
- **coverage_ignore.txt (T11 adjacent)**: 11 module-prefixed entries bumped `/v3`→`/v4` to keep
  the curated grep zero. Their path *stems* still reference pre-consolidation trees
  (`components/onboarding/...`, `components/transaction/...`) that no longer exist — stem
  cleanup is out of F6 scope (module-path-only) and is registered in §7.
- **Migration guide (T13, Gate 9)**: `docs/v4/MIGRATION-v3-to-v4.md` — REST table (54
  `instrument` references; alias→instrument hard cut, holders/instruments routes, composition
  endpoint with `instrumentError` partial-failure block) + Go importer table (4 `/v4` rows).
  Verified against actual HEAD code, not the stale plan baselines. The tracer by-transaction
  reservation endpoints (`/v1/reservations/transaction/{transaction_id}/{confirm,release}`),
  which emerged during F3 execution, are documented as new surfaces.
- **build.yml tracer S3 (T14, Gate 11 support)**: `upload-tracer-migrations` job added,
  `s3_prefix: tracer/postgresql` (two-segment analog of ledger's layout — tracer has one
  migration set), placed in the UPLOAD MIGRATIONS group, `needs: [build]` only. Diff vs the
  pre-T14 build.yml state (`dc50a46b8`) touches **zero** fan-out lines. YAML parses clean.
  **R45 caveat**: the `tracer/postgresql` key layout vs the managed-deploy pipeline's
  expectation is not verifiable in-repo — Ops sign-off item (X2/X3 in §6). A wrong prefix
  ships migrations nobody consumes.

## 4. Grep-zero report (F6-T12, Gate 3) — PASS

Every curated token is grep-zero or fully accounted for by an allowlisted carve-out, at HEAD
`cb1be73aa`. Token-by-token results live in the workflow record; highlights:

1. `midaz/v3` in `.go` — **0** (Gate 1 closed; `go build ./...` exit 0 at HEAD).
2. `midaz/v3` non-Go curated — **0** (full census 37 files, all inside the three carve-outs:
   `CHANGELOG.md`, `docs/monorepo/`, `docs/v4/`).
3. `mmodel.Alias` — 0. `"plugin-crm"` namespace — 0 (`ApplicationName = "midaz"` at
   `crm_routes.go:20`); 4 surviving substring hits are comments + test-fixture container names
   (see §7). `/v1/.*/aliases` — 0.
4. Stale ports — postman env `onboardingPort`/`transactionPort` values both `"3002"`.
5. `v3.7.0` ship surface — 0 (`cmd/main.go @version v4.0.0`; ledger/tracer/reporter-manager
   specs at 4.0.0). Survivors only in `docs/v4/` (bump sources), `CHANGELOG.md`, and
   `docs/monorepo/analysis/03-midaz-host.md:145`.
6. **R30 resolution**: the analysis-tree `v3.7.0` line is EXCLUDED as frozen history (same
   discipline as CHANGELOG), not rephrased — the line accurately documents the pre-v4 state,
   and `docs/monorepo/` is DO-NOT-TOUCH. Gate and scope are thereby consistent.

DO-NOT-TOUCH allowlist verified intact: `Account.Alias` (`account.go:81,245`) and
`Balance.Alias` (`balance.go:47,152,385`) routing handles; the 6 numeric alias sentinels
(`errors.go` 0020/0063/0085/0096/0118/0123); CHANGELOG history; `docs/monorepo/` frozen tree;
`plugin-crm-mongodb` container name; `postman/backups/` (14 files, untracked, never in a gate).

**Binding tooling caveat for anyone re-running the gate**: both the repo shell's `grep` wrapper
(rtk proxy) and macOS BSD `/usr/bin/grep` silently ignore `--include=*.go` under `-r .`,
matching `.md` and `.git` pack files — the spec's literal Token-1 command returns a false 2,883.
The authoritative tool is `command git grep -- '<pathspec>'` (repo-aware, glob-honored, skips
`.git`/untracked/ignored). The spec's raw commands as written will falsely red the gate.

## 5. Release alignment (F6-T18, Gate 10) — dry-run computes 4.0.0

Strategy: local throwaway `release-candidate` branch at `cb1be73aa`; `semantic-release@25.0.3
--dry-run --no-ci` with a version-computation-only plugin set (commit-analyzer +
release-notes-generator carrying the **exact** `releaseRules`/`parserOpts`/preset/noteKeywords
from `.releaserc.yml`; the repo's git/github/backmerge plugins are not installed locally and do
not affect version computation), installed in an isolated temp dir; `--repository-url file://`
to bypass the behind-remote guard. Cleanup verified byte-identical (`.releaserc.yml` untouched,
tree clean, branch deleted, nothing pushed, no tag).

Results — both channels honor the BREAKING CHANGE ("Analysis of 187 commits complete: major
release"):

- **Stable channel: `4.0.0`.**
- **RC prerelease channel (`release-candidate`, prerelease `rc`): `4.0.0-rc.1`.**

Neither computes 3.8.0/3.9.0. The 4.x floor holds.

**Release-engineering finding (surfaced, benign)**: the stable/rc channels base off the last
*stable* tag `v3.7.3`, not `v3.8.0-beta.9` — semantic-release per-channel tag matching feeds
beta tags only to the develop channel. Irrelevant to the v4 cut (a BREAKING CHANGE forces major
from any base) but worth knowing so nobody expects the cut to compute off the beta line.

### Merge mechanics (owner: Fred, at merge time)

semantic-release reads the commit **type** and the **BREAKING CHANGE note** from the release
branch's history — not `go.mod`. When `feat/monorepo-consolidation` merges to a release branch:

- A **plain merge commit** preserves `5eaaf8db7`'s footer automatically. Safe.
- A **squash merge discards individual commit footers** and uses the PR title + body. The PR
  body MUST carry `BREAKING CHANGE:` verbatim (or the PR title must itself be a `!`-marked
  type). A squash that drops it computes a **minor** (refactor → minor per the releaseRules)
  instead of 4.0.0.

## 6. DEPLOY-GATE: external lockstep register (F6-T20, Gate 12)

Per F6-T20, the lockstep is a register, not a repo deliverable. **Every item below is PENDING
at note time. No production v4 tag may fan out before X1–X4 confirm.** The armed fallback is
gate-the-production-tag (P8-T22); both DEPLOY-GATED comments are present and armed in
`build.yml` (helm `:39`, gitops `:64`).

| Item | What must land | Owner | Status |
|------|----------------|-------|--------|
| **X1** | Auth-server RBAC policy migration `plugin-crm` → `midaz` namespace (resources `holders` + `instruments`; related-parties under instruments). NO auth-enabled environment deploys v4 before this confirms. | **Fred** + plugin-auth | **PENDING** |
| **X2** | External `midaz` Helm chart gains tracer/reporter-manager/reporter-worker value blocks; drops crm/fees keys. Includes R45: confirm the `tracer/postgresql` S3 prefix matches the managed-deploy pipeline's expected key layout. | Ops | **PENDING** |
| **X3** | `midaz-firmino-gitops` same reconciliation as X2. Cutting a real tag before X2/X3 fans 4 images into a chart still keying crm/fees → ArgoCD sync fails (R6). | Ops | **PENDING** |
| **X4** | APIDog scenarios updated for the v4 wire surface (alias→instrument routes, composition endpoint, reservations API). | QA/API | **PENDING** |

X5 (docs portal) and X6 (Go importers, served by the T13 migration guide) are informational,
non-blocking (Q14).

## 7. Re-proof baseline at `cb1be73aa` (F6-T15/T16/T17, Gates 6/7/8/13) — ALL GREEN

| Leg | Result | Time | vs F5 baseline |
|-----|--------|------|----------------|
| `make ci` one-exit | exit 0 | — | (Gate 8/13 carrier) |
| unit | 16,138 / 0 fail / 6 skip | 98s | identical |
| integration | 1,005 / 0 fail / 80 skip — **no retry consumed** | 1,112s | identical (F5 absorbed 1 flake) |
| property | 70 / 7 skip | 1.6s | identical |
| reporter-chaos | 39 / 39 skip (env-gated by design) | 43s | identical |
| CHAOS pair (Gate 7) | `TestMultiTenant_Chaos_TenantManagerOutage` PASS 2.38s; `TestMultiTenant_Chaos_RedisOutage` PASS 2.37s | 7.5s | both green |

Zero variation across the count series — exactly what a path-only mechanical bump must produce.

Executed-evidence detail for Gates 6/7: **20 proof tests confirmed PASSED by name inside the
integration leg under `/v4`** — 12 PD-5 double-entry proofs (`TestIntegration_Redis_DoubleEntry*`:
approved/canceled/pending lifecycles, version-chain consistency, insufficient-funds rollback)
and 8 F3 usage-drift proofs (`TestIntegration_ReservationCrashConvergence`,
`TestIntegration_ReservationOverCommit`, reaper cadence ×2, by-transaction confirm/release
idempotency ×4).

Canonical CHAOS-pair command (recorded because both prior forms fail silently or loudly):

```
CHAOS=1 ALLOW_INSECURE_TLS=true go test -tags 'integration chaos' -count=1 \
  ./components/tracer/tests/integration/ \
  -run 'TestMultiTenant_Chaos_TenantManagerOutage|TestMultiTenant_Chaos_RedisOutage'
```

- `ALLOW_INSECURE_TLS=true` is REQUIRED — the make target exports it; a bare `go test` dies in
  `TestMain` at postgres migrate ("TLS required") before any test runs.
- The pair needs a dedicated run because the test names are not `TestIntegration_*`-prefixed,
  so the make chain's `RUN_PATTERN '^TestIntegration'` excludes them.

### Evidence disclosure (recorded honestly)

- The first `make ci` run's output stream was filtered by the rtk shell proxy to 51 lines —
  per-leg counts lost. What survives of that run: exit 0 under `set -e` (the one-exit proof)
  plus a live `pgrep` observation of the integration leg running the full 35-package set under
  `/v4`. The per-leg numbers above come from an immediate re-run of the four legs individually
  with raw per-leg logs (`rtk proxy`, redirect-to-file). Standing rule going forward: gate
  evidence via redirect-to-file + `rtk proxy`, never the proxy-filtered stream.
- The CHAOS pair had two disclosed false starts before the green run: (1) missing
  `ALLOW_INSECURE_TLS` → exit 1 in TestMain pre-test; (2) a stale `-run 'MultitenantChaos'`
  pattern → `[no tests to run]` with exit 0 — the F0 void-trap class, caught by run-count
  verification (a green exit with zero tests run is never accepted as a pass).

## 8. Residue and debt register (recorded, deliberately not silently fixed)

Surfaced by F6, owned elsewhere or accepted:

- **tracer `.env.example:13` carries `VERSION=v0.1.0`** — stale vs the 4.0.0 train; an F5/D-11
  version-train gap with no `/v3` substring for a bump grep to catch. Recorded, not fixed in F6
  (out of the module-path scope).
- **`scripts/coverage_ignore.txt` stale path stems** — entries bumped to `/v4` but referencing
  dead pre-consolidation trees (`components/onboarding/...`, `components/transaction/...`).
  Stem pruning belongs to whoever owns coverage config.
- **`go build -tags=chaos` ALONE fails**: `FindMigrationsPath` definer
  (`tests/utils/postgres/migrations.go`) is tagged `//go:build integration` while the caller
  (`onboarding_migrations.go`) is `integration || chaos`. Pre-existing at pristine HEAD
  (reproduced with the `/v3` module path); the real chaos invocation `-tags 'integration chaos'`
  builds green. Test-tag harness fix, likely F0/F3 lineage.
- **Third `plugin-crm` test-fixture survivor**: `tests/reporter/e2e/shared/infra.go:220`
  (`Name: "plugin-crm"`, the `pluginCRMMongo` fixture) — same non-namespace class as the two
  the spec already allowlisted (`plugin-crm-mongodb` container, `config.go` comment).
  `git grep 'ApplicationName.*plugin-crm'` = 0: the F2 namespace flip is clean.
- **`go vet ./...` exits 1** on ~20 pre-existing `unreachable code` diagnostics in the
  generated DSL parser (`pkg/gold/parser/transaction_parser.go`). Untouched by F6; all other
  vet findings clean.
- **Legacy tracer integration suite** (~144 files, `TestValidation_*` etc.) remains
  `RUN_PATTERN`-excluded from the make chain — the harness-debt decision deferred from F3/F5
  is RECORDED as open debt here, not resolved. Widening `RUN_PATTERN` vs renaming the suite
  needs a runtime-cost measurement first.
- **Inherited register carried forward** (pre-v4 debt, unchanged): 31 `//go:build unit` zombie
  files; 25 untagged mock-based `*_integration_test.go`; gocyclo on
  `FindOrListAllWithOperations`; `bin/` gitignore gap; down-migration linter blind spot.

## 9. Decisions register (affecting future readers)

1. Census measured against real HEAD, not the frozen plan baseline — F6.md §0 makes
   execution-time gates authoritative.
2. `git grep` is the authoritative residue tool (see §4 caveat); broken-wrapper census figures
   superseded.
3. Mechanical commit kept strictly path-only: tidy drift reverted, pre-existing gofmt debt not
   absorbed.
4. R30 resolved by exclusion (frozen-history carve-out), not rephrasing.
5. Tracer OTEL value set to the monorepo module path (`.../midaz/v4/components/tracer`), per
   T04; both `.env` and `.env.example` edited, only the example commits.
6. `postman/backups/` treated as non-residue (untracked, R50 noise source, F6-T07's
   delete/exclude target).
7. Dry-run executed with the version-computation-only plugin set under the prompt's explicit
   escape hatch; both stable and rc channels proven.
8. Note assembly and re-proof orchestration ran in the main session after two consecutive
   server-side 529 failures on agent dispatch; the 13-gate verification remains an independent
   agent (adversarial separation is substance, not form).

## 10. Gate-closure walk (F6.md §3, all 13 gates)

| Gate | Status | Evidence |
|------|--------|----------|
| 1 — `/v4` builds; zero `midaz/v3` in `.go` | **CLOSED** | §1, §4(1); `go build ./...` exit 0 |
| 2 — go.mod-only module change | **CLOSED** | §1 path-only audit |
| 3 — Grep-zero sweep report | **CLOSED** | §4 (with carve-outs + tooling caveat) |
| 4 — OTEL attribution | **CLOSED** | §3; both surfaces verified |
| 5 — chaos.go targets exist | **CLOSED** | §2 (F0 fix survives; census-confirmed) |
| 6 — PD-5 re-proof under /v4 | **CLOSED** | §7: 12 named double-entry proofs PASS in-leg |
| 7 — F3 usage-drift re-proof + CHAOS pair | **CLOSED** | §7: 8 named proofs + 2/2 chaos pair PASS |
| 8 — Full matrix via `make ci` | **CLOSED** | §7: one-exit 0 + per-leg evidence |
| 9 — Migration guide | **CLOSED** | §3 (T13); verified vs actual HEAD |
| 10 — Release alignment, dry-run 4.0.0 | **CLOSED** | §5: 4.0.0 + 4.0.0-rc.1 |
| 11 — Fan-out sanity | **CLOSED** | §3 (T14) + workflow T19: 4 images exact, zero crm/fees keys, T14 diff fan-out-clean |
| 12 — DEPLOY-GATE lockstep sign-off | **ARMED / PENDING** | §6: X1–X4 register, all PENDING, production tag gated |
| 13 — Reporter re-proof reachable + green | **CLOSED** | §7: reporter legs inside `make ci`; reporter-chaos 39 compiled+invocable |

Gate 12 is the only intentionally open gate: it closes outside the repo, at release time, on
the X1–X4 acknowledgements. Everything the repository can prove is proven.
