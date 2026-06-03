# Midaz Monorepo Consolidation — Master Plan (PLAN.md)

**Status:** Authoritative plan-of-record. Synthesizes the twelve corrected phase files under
`docs/monorepo/plan/{P0,P1,P2a,P2b,P2c,P3,P4,P5,P6,P7,P8,P9}.md`. Where this file summarizes, the per-phase
file is the canonical source for that phase; every task below is reproduced verbatim from its phase file so
PLAN.md is self-contained.

**Module:** `github.com/LerianStudio/midaz/v3` · **Go:** `1.26.3` (the real toolchain; CLAUDE.md's "1.25+" is
a stale cosmetic ref) · **License:** Elastic License 2.0.

---

## 1. Executive summary

The midaz codebase today is one Go module (`components/{crm,infra,ledger}`) plus three external repos
(`tracer`, `reporter`, `plugin-fees`) that must fold into a single root `go.mod` monorepo with four deploy
units:

- **ledger** (`:3002`) — absorbs **crm** (holder/alias routes) and the **plugin-fees** engine in-process.
- **tracer** (`:4020`) — co-located component, keeps its own binary; independent of infra's `otel-lgtm`.
- **reporter-manager** (`:4005`) + **reporter-worker** (`:4006`) — two top-level components (the shared
  `build.yml` `path_level: 2` fan-out emits one image per level-2 dir, so a single `components/reporter` dir
  would emit one image, not two).

The consolidation is **dependency-first, then move, then collapse, then unify, then harmonize, then sweep**.
The single hardest verifiable-now claim — that the incoming repos reconcile to one `lib-commons/v5` line under
PD-1 (no `go.work`, no `replace`, no shim) — is gated up front: the tracer `lib-commons/v4 → v5` migration is a
HARD prerequisite proven in P0-T17 (dry-run) and executed in P2c. The most dangerous correctness surface is the
fee third rail (P4): a SINGLE `validate` reassignment in `executeCreateTransaction` must cover all 11 downstream
consumers, and fee reversal on revert/pending-cancel must balance to exactly zero (PD-5) under the ledger's own
exact-`decimal.Equal` validator.

**Sequence:** P0 (pre-flight) → P1 (lib-commons GA pin) → P2a/P2b/P2c (in-place dep migration of fees /
reporter / tracer, each validated against its own CI) → P3 (crm collapse), P4 (fees collapse), P5 (tracer
move), P6 (reporter move) — the four moves are siblings, all gating P7 (go.mod unification + unified
third-rail re-proof) → P8 (CI/Docker/release harmonization) → P9 (final shim/alias/dead-code sweep + docs).

**Totals:** 239 tasks across 12 phases. Readiness: **ready-with-caveats** — the DAG is clean; the prior
fees-spike placeholder-id mismatch is now RESOLVED (P4-T01/T02/T11 reference the concrete spike id `P2a-T17`
directly). Remaining caveats are execution-time only: external-owner sign-offs and cross-phase
prerequisites (lib-commons GA bump not yet landed on `develop`, pg16→17 late-stall risk, P7 assumptions to
re-verify) that are correctly gated, not plan defects.

---

## 2. Locked Decisions Ledger (PD-1 .. PD-7 + Tracer Role)

These are authoritative and NOT re-litigated by any phase.

| ID | Decision | Binding consequence |
| --- | --- | --- |
| **PD-1** | **Single root `go.mod` (Option A).** NO `go.work`, NO `replace`, NO temporary fences/shims. tracer `lib-commons/v4 → v5` is a **HARD prerequisite gate** (P0-T17 dry-run, P2c executes) — a co-located tracer cannot keep v4 import paths beside midaz's v5 without a forbidden shim. | Any `replace`/`go.work`/fence is a plan defect. Exactly ONE `go` directive (`1.26.3`) and ≤ ONE `toolchain` line in the merged root go.mod. P7-T07 asserts one resolved `/v5`, zero `/v4`. |
| **PD-2** | **DELETE the CRM `ErrorCodeTransformer` shim + prune 12 dead `CRM-00xx` codes.** Shim set = exactly `error_transformer.go`, `error_mapping.go`, `error_transformer_test.go` + the `routes.go:38` wiring. **`backward_compat_test.go` is a legit MT test — do NOT delete it** (its single-tenant invariants migrate to ledger in P3-T13b). | Authoritative deletion site is **P3** (P3-T03/T04). P9 = verification + orphan `ErrMissingHeadersInRequest` (CRM-0018) prune + canonical-contract regression test. Clients get canonical midaz codes (incl. `0094` for the non-1:1 CRM-0004 path). |
| **PD-3** | **Fresh git import; origins archived read-only; exclude reporter `ast-before-*`.** Move mechanic = ALLOWLIST `git archive <SHA> \| tar -x` honoring `.gitignore` by construction (the `ast-before-*` snapshots are untracked/gitignored, NOT tracked). | Archival happens at the END of each move phase, after in-module green, during a deploy-first rollback grace window (P9-T14). |
| **PD-4** | **Bump midaz to lib-commons `v5.2.x` GA first (verify GA exists); lib-observability `v1.0.1`.** GA verified live: `v5.2.0` (first GA) + `v5.2.1` exist. P1 pins **`v5.2.0`** (lowest GA, minimal surface); `v5.2.1` is the equally-safe documented fallback. lib-observability held at `v1.0.1` (1.1.x beta-only). | P1-T06 records the canonical frozen pin; every downstream phase cites `docs/monorepo/plan/P1.md#p1-frozen-target-pin` by reference and depends on P1-T06. Incoming repos may default-pin `v5.2.1`; MVS reconciles at merge. |
| **PD-5** | **REFUND original fees on revert AND pending-cancel; reversal must balance (sum == 0).** VERIFY-not-REBUILD: `TransactionRevert` is already fee-aware; injecting refund legs would double-reverse. The DEDUCTIBLE-fee case is load-bearing (`sum(reconstructed legs) == persisted t.Amount`). | First proven P4-T16 (in-phase); re-proven unified in P7-T18; HARD-asserted again in P9-T12. Double-entry correctness is a third rail. |
| **PD-6** | **Migrate deps in-place per repo FIRST, then move; observability + co-location never share a commit.** | P2a (fees), P2b (reporter), P2c (tracer) each migrate against their OWN CI before P4/P6/P5 move the code. Bisectability is the reason. |
| **PD-7** | **Fees persist as NEW Mongo collections in ledger's existing MongoDB (11 compound indexes).** NOT Postgres; no separate `plugin-fees-mongodb`. | P4-T05 ports 11 indexes as code-created `mongo.IndexModel` (no migration files); `WithMB(mgr, ModuleFees)`. |
| **Tracer Role** | **tracer is INDEPENDENT of infra's `otel-lgtm`.** otel-lgtm stays untouched; tracer points at shared `postgres:17` (verify 16→17 + logical-replication compat). | No telemetry coupling. tracer drops its own `tracer-postgres`; a `tracer` DB is provisioned on the shared instance (P5-T10a). |

---

## 3. Global phase sequence + dependency gates

```
P0  Pre-flight verification & out-of-repo coordination       (no host code change)
        │
        ▼
P1  lib-commons GA pin (P1-T06 = canonical frozen pin GATE)
        │
   ┌────┼───────────────┬───────────────────┐
   ▼    ▼               ▼                   ▼
P2a   P2b              P2c                  (parallel; each gated on P1-T06 for its pin;
fees  reporter         tracer               each validated against its OWN repo CI, PD-6)
in-place  in-place     in-place (LONG POLE)
   │      │               │
   │      │               └── P2c-T22 = tracer READY-TO-MOVE gate
   │      └── P2b-T13 ∧ P2b-T14 = reporter prep exit gate
   └── P2a-T17 = fees-engine correctness spike (gates P4)
        │
   ┌────┴──────────┬───────────────┬───────────────┐
   ▼               ▼               ▼               ▼
P3 crm collapse   P4 fees collapse  P5 tracer move  P6 reporter move
(P2a gates P4 · P2c-T22 gates P5 · P2b gates P6 · the four moves are SIBLINGS, none gates another)
   └───────────────┴───────────────┴───────────────┘
        │ (ALL FOUR MOVES GATE P7)
        ▼
P7  go.mod unification & final tidy + unified PD-5 third-rail re-proof (P7-T18)
        │ (P7 GATES P8)
        ▼
P8  Build / CI / Docker / Release harmonization
        │ (P8 GATES P9)
        ▼
P9  Cleanup, dead-code/shim sweep, alias rename, docs ("liso e final")
```

**Dependency gates (authoritative):**

- **P1-T06** is the single canonical-pin gate; `P2a-T00/T16`, `P2b-T01`, P2c (reconciled at merge via P2c-T23),
  `P5-T00`, `P6-T01`, `P7-T01/T03`, `P8-T01`, `P9-T01` read/depend on it.
- **P2a gates P4** — P4-T01/T02/T11 depend on the fees-engine correctness spike **P2a-T17** (referenced in P4
  by its concrete id `P2a-T17`; the prior placeholder label mismatch is resolved — see DAG issues §6).
- **P2c-T22 gates P5** — `P5-T00 depends_on P2c-T22, P1-T06`.
- **P2b gates P6** — `P6-T01 depends_on P1-T06, P2b gate task`.
- **All four moves gate P7** — P3 (P3-T21), P4 (P4-T19), P5 (P5-T15), P6 (P6-T17/T20) precede P7.
- **P7 gates P8** — P8 entry tied to the full P7 exit-criteria set (P7-T17 records the handoff).
- **P8 gates P9** — P9-T01 inventory + P9-T12 final gate depend on P8-T18 (and P7-T03/T10/T11/T18).
- **Cross-phase teardown gate:** irreversible standalone-service teardowns (P3-T13/T17/T18, P4-T19b) are gated
  on in-phase green (P3-T20/T21, P4-T16) AND the unified third-rail re-proof (P7-T18) AND out-of-repo lockstep,
  mirroring P5-T16 abort discipline. P7-T18 is a BACKSTOP re-proof, not a precondition of teardown (inverting
  it would create a cycle, since P7 is gated on the moves landing).

---

## 4. Critical path

```
P0-T17 (prove tracer v4→v5 reconciles)
  → P1-T01 → P1-T03 → P1-T04 → P1-T06 (canonical GA pin merged)        [GATE]
  → P2c-T00 → P2c-T01 → P2c-T06 (tenant-manager v4→v5, highest uncertainty)
  → P2c-T09 → P2c-T10 (Jump-1 green) → P2c-T11 → P2c-T13..T18 (obs split)
  → P2c-T19 → P2c-T20 → P2c-T21 → P2c-T22 (tracer READY-TO-MOVE)        [LONG POLE]
  → P5-T00 → P5-T02 → P5-T04 → P5-T06 → P5-T10/T10a → P5-T15 (tracer in-module green)
  → P7-T06 → P7-T08 → P7-T10 → P7-T11 → P7-T18 (unified PD-5 third-rail re-proof)
  → P8-T11 → P8-T18 (real-tag fan-out builds exactly 4 images)
  → P9-T01 → P9-T12 (final unified gate incl. PD-5 balance proof) → P9-T14 (origins archived)
```

The **P2c tracer migration is the long pole** (v4→v5 tenant-manager rewrite + observability split as two
bisectable jumps). **P4's fee third rail is the highest-correctness-risk parallel branch** (concurrent with P5,
but its in-phase proof P4-T16 and unified re-proof P7-T18 are both on the path to a defensible "done"). P4 is
gated on **P2a-T17**, which runs from P2a phase start in parallel with the observability chain.

---

## 5. Phases (verbatim tasks)

> Each phase reproduces its tasks (id / title / description / files / depends_on / acceptance / tests / effort /
> risk_refs) verbatim from `docs/monorepo/plan/<phase>.md`. The per-phase file remains canonical.


---

<a id="phase-0"></a>

# Phase 0 — Pre-flight Verification & Out-of-Repo Coordination (17 tasks)

_Verbatim from `docs/monorepo/plan/P0.md`._


**Phase ID:** P0
**Objective:** Resolve the remaining VERIFIABLE unknowns and lock out-of-repo coordination before any
code moves. Decisions PD-1..PD-7 are already made; this phase is verification + setup, NOT re-deciding.
Concretely: confirm the lib-commons v5.2.x GA tag exists on the proxy (else pick latest stable, note it);
prove the incoming repos can reconcile to that unified pin (the tracer `lib-commons/v4 → v5` migration is the
single hardest verifiable-now claim and is gated HERE, not at move time); confirm the four origin repos can be
archived read-only; establish `docs/monorepo/plan` as the tracked source of truth; identify and
confirm-lockstep the owners of the external Helm chart `midaz`, `midaz-firmino-gitops`, and APIDog e2e (R12)
WITH a defined no-sign-off fallback; lock the exact CRM shim/dead-code deletion surface (PD-2); and capture a
baseline green build/lint/test/sec snapshot of midaz HEAD as the regression baseline that every later phase is
measured against.

**Locked decisions in scope:** PD-1 (single root go.mod, Option A, NO go.work/replace/fence), PD-2 (delete
CRM `ErrorCodeTransformer` shim + prune 12 dead CRM-00xx codes), PD-3 (fresh import, archive origins, exclude
reporter `ast-before-*`), PD-4 (lib-commons GA-first: midaz → `v5.2.x`, lib-observability `v1.0.1`),
PD-6 (in-place-deps-first sequencing). TRACER ROLE (independent of otel-lgtm).

**Locked phase numbering (referenced throughout, for cross-phase coordination):**
P1 = unified pin / canonical bump · P2a = fees pre-move spike · P2b = reporter prep · P2c = tracer prep ·
**P3 = crm collapse** · **P4 = fees move** · **P5 = tracer move** · **P6 = reporter move** ·
P7 = unified third-rail proof · P8 = CI harmonization · P9 = final shim/alias sweep. P0 introduces no
numeric phase labels in prose that contradict this scheme.

**Hard rule:** ZERO shims. No `replace`, no `go.work`, no temporary fences. Any such step is a plan defect.
This phase introduces NO code changes to the host; it verifies, snapshots, documents, and coordinates.
The one exception class — disposable throwaway branches for dry-run probes (P0-T02, P0-T17) — is never merged
and must leave the tree clean.

---

## Verified ground truth (gathered at plan time, real anchors — re-grep before later phases trust line numbers)

- `go.mod:1` `module github.com/LerianStudio/midaz/v3`; `go 1.26.3`; **no** `go.work`, **no** `replace`.
  (Project CLAUDE.md header still says "Go: 1.25+"; the real toolchain is `1.26.3` — cosmetic stale ref, not
  a P0 defect; the baseline snapshot pins the real toolchain.)
- `go.mod:101` `lib-commons/v5 v5.2.0-beta.12`; `go.mod:102` `lib-observability v1.0.1`.
- **lib-commons v5 GA tags on proxy** (`go list -m -versions`): `v5.2.0`, `v5.2.1` (latest in v5.2.x line),
  plus `v5.3.0..v5.3.3`, `v5.4.0`, `v5.4.1`. `@latest` = `v5.4.1`. → **PD-4 GA target = `v5.2.1`** (latest
  GA of the v5.2.x line; no fallback to a pre-release needed). GA existence VERIFIED, not assumed.
- `go build ./...` → **Success** (baseline compiles green today).
- `mk/tests.mk:105 test-unit:`, `mk/tests.mk:224 test-integration:`; root `Makefile`: `LEDGER_DIR:10`,
  `CRM_DIR:11`, `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` at `:16` (LEDGER_DIR deliberately absent → special-cased),
  set-env crm `.env` check at `:62-63`, build fan-out `:183/:185`, lint hardcodes `:225-227`, crm
  `generate-keys`.
- Workflows present: `.github/workflows/{build,go-combined-analysis,gptchangelog,pr-security-scan,pr-validation,release-notification,release}.yml`.
- `build.yml:19` filter_paths; `:35` `helm_chart: "midaz"`; `:37`
  `helm_values_key_mappings '{"midaz-crm": "crm", "midaz-ledger": "ledger"}'`;
  `:52` `gitops_repository: "LerianStudio/midaz-firmino-gitops"`;
  `:57` `yaml_key_mappings '{"midaz-crm.tag": ".crm.image.tag", "midaz-ledger.tag": ".ledger.image.tag"}'`;
  `:103-106` APIDog e2e job using `MIDAZ_APIDOG_TEST_SCENARIO_ID` / `MIDAZ_APIDOG_DEV/STG_ENVIRONMENT_ID` /
  `APIDOG_ACCESS_TOKEN`.
- `go-combined-analysis.yml:31` `filter_paths '["components/crm", "components/ledger"]'`; `:37`
  `coverage_threshold: 85`; `:38` `fail_on_coverage_threshold: true`.
- `pr-validation.yml:30-32` `pr_title_scopes:` lists ONLY `crm` and `ledger` (the prior P0 draft wrongly
  claimed a longer scope list at line 38 — corrected here). `crm` is OBSOLETE post-collapse; `tracer` and
  `reporter` are MISSING.
- `release.yml:9` `paths-ignore:` includes `:13 '**/*.md'` and `:14 '*.md'` → doc-only changes do NOT trigger
  release/gitops.
- `.gitignore:32 .env` → **`components/crm/.env` is gitignored and NOT tracked** (`git ls-files` shows only
  `components/crm/.env.example`). A live `.env` on disk carries real 64-hex `LCRYPTO_*` values; the
  `.env.example` (`:53-54`) holds placeholders. The tracked surface leaks NO key material.
- **CRM error shim surface** (PD-2 deletion target):
  - `components/crm/internal/adapters/http/in/error_transformer.go` (middleware + `transformResponseCode`).
  - `components/crm/internal/adapters/http/in/error_mapping.go` (`CRMErrorMapping` table + `TransformErrorCode`;
    its own header declares it a post-migration backward-compat shim). The table has **exactly 12 entries**.
  - `components/crm/internal/adapters/http/in/routes.go:38` `f.Use(ErrorCodeTransformer())` (wiring).
  - `components/crm/internal/adapters/http/in/error_transformer_test.go` (shim test — deleted WITH the shim).
  - `pkg/constant/errors.go:223-238` holds the 16 CRM-00xx codes the shim references; only the **12 dead**
    ones (the 11 `*CRM`-suffixed targets + `ErrInvalidFieldTypeInRequest` = `CRM-0004`, the non-1:1 case) are
    pruned. **Live domain codes that MUST survive:** `CRM-0006` (`ErrHolderNotFound`), `CRM-0008`
    (`ErrAliasNotFound`), `CRM-0010` (`ErrDocumentAssociationError`), `CRM-0013` (`ErrAccountAlreadyAssociated`),
    and all of `CRM-0017..CRM-0029` (Holder/Alias/RelatedParty domain errors). `CRM-0006/0008/0010/0013/0024`
    are asserted in live integration/service tests.
  - `components/crm/internal/bootstrap/backward_compat_test.go` is a LEGIT multi-tenant test — **NOT deleted**.
- Incoming-repo go.mod state (the unified-pin reconciliation surface — VERIFIED locally):
  - `tracer/go.mod:1` `module tracer` (bare, non-namespaced — every internal import is `tracer/...`, no domain
    prefix; rewrite cost is large). `:3` `go 1.26.3`. `:8` **`lib-commons/v4 v4.6.3` DIRECT**; `:51`
    `lib-commons/v5 v5.3.0 // indirect`; `:52` `lib-observability v1.0.0 // indirect`. Under PD-1 a co-located
    tracer cannot keep `lib-commons/v4` import paths alongside midaz's `v5` without a rewrite or a forbidden
    shim. **This is the HARD prerequisite gate.**
  - `reporter/go.mod:1` `module github.com/LerianStudio/reporter`; `:9` `lib-commons/v5 v5.1.3`. No root
    `.env.example` (component-level envs under `components/{infra,manager,worker}/.env.example`).
  - `plugin-fees/go.mod:1` `module github.com/LerianStudio/plugins-fees/v3` (remote is
    `LerianStudio/plugin-fees` — **name mismatch**); `:7` `lib-commons/v5 v5.1.0`; `:9`
    `lib-observability v1.1.0-beta.5` (AHEAD of midaz's `v1.0.1` GA → potential downgrade/API-loss); `:42`
    `lib-commons/v2 v2.9.1 // indirect` straggler.
  - `reporter/docs/codereview/ast-before-3807876316/` and `.../ast-before-3371843854/` exist (PD-3
    import-exclusion targets).
- Origin midaz repo: `github.com/lerianstudio/midaz`. Misleading alias
  `libCommons "github.com/LerianStudio/lib-observability"` is **pervasive in PRE-EXISTING ledger code** (20+
  sites, e.g. `components/ledger/internal/adapters/http/in/get_all_accounts.go`,
  `.../redis/transaction/consumer.redis.go`), not just CRM — flagged for P9's whole-tree sweep (SS1).

---

## Tasks

### P0-T01 — Verify lib-commons v5.2.x GA exists on the proxy and pin the canonical GA target
**Description:** Run `go list -m -versions github.com/LerianStudio/lib-commons/v5` against the public proxy
(`GOPROXY=https://proxy.golang.org`) and record the full GA tag list. Confirm a `v5.2.x` GA exists. It does:
`v5.2.0` and `v5.2.1` are GA (latest in the v5.2.x line is `v5.2.1`). Record `v5.2.1` as the canonical
GA target for PD-4 (the version Phase 1 bumps midaz to and that ALL incoming code is rewritten against).
Also confirm `lib-observability v1.0.1` is still resolvable and has no `lib-commons` dependency (clean module).
If — contrary to the cache — no v5.2.x GA were present, the fallback rule is: pick the latest stable
(non-beta, non-rc) tag in the lowest line ≥ v5.2.0 and note it explicitly. Write the chosen pin into this
plan and into `docs/monorepo/plan/deps-pin.md` (NEW) as the single authoritative pin reference for Phase 1.
**Files:** `docs/monorepo/plan/P0.md`, `docs/monorepo/plan/deps-pin.md` (NEW).
**Depends on:** none.
**Acceptance:** `deps-pin.md` records `lib-commons/v5 v5.2.1` as the GA target with proxy evidence (the
version list output) and a one-line rationale; `lib-observability v1.0.1` confirmed resolvable with no
lib-commons require in its own go.mod.
**Tests:** `GOPROXY=https://proxy.golang.org go list -m -versions github.com/LerianStudio/lib-commons/v5`
(must list `v5.2.1`); `GOPROXY=https://proxy.golang.org go list -m github.com/LerianStudio/lib-observability@v1.0.1`
(must resolve); `go mod download github.com/LerianStudio/lib-observability@v1.0.1 && grep -L lib-commons "$(go env GOMODCACHE)/github.com/!lerian!studio/lib-observability@v1.0.1/go.mod"`.
**Effort:** S (1-2h).
**Risk refs:** R3, R4.

### P0-T02 — Dry-run the midaz v5.2.x GA bump in a throwaway branch (de-risk Phase 1, do NOT land)
**Description:** In a disposable local branch (NOT to be merged in P0), edit `go.mod:101` from
`v5.2.0-beta.12` to the P0-T01 GA pin (`v5.2.1`), run `go mod tidy`, then `go build ./...`,
`make test-unit`, `make lint`. Capture any compile/lint deltas between beta.12 and GA. This is a
verification probe to confirm the GA bump is the "smallest possible change" PD-4 assumes and to surface any
hidden API drift BEFORE Phase 1 commits to it. Record findings (clean / N deltas with file:line) in
`deps-pin.md`. This task de-risks midaz's OWN bump only; the incoming-repo reconciliation (the real drift
source) is P0-T17. Discard the branch; Phase 1 owns the real bump. NO commit to develop/main from this task.
**Files:** `docs/monorepo/plan/deps-pin.md`.
**Depends on:** P0-T01.
**Acceptance:** `deps-pin.md` states whether `beta.12 → v5.2.1` is a clean bump or lists exact drift sites;
throwaway branch deleted; develop tree unmodified (`git status` clean after).
**Tests:** on the throwaway branch: `go mod tidy && go build ./...` (record result); `make test-unit`
(record pass/fail); `make lint` (record findings count). Verify `git branch -D <throwaway>` and `git status`
clean at end.
**Effort:** M (2-4h).
**Risk refs:** R3, R4.

### P0-T03 — Capture the baseline regression snapshot of midaz HEAD (build/unit/integration/lint/sec/coverage)
**Description:** Record the authoritative "before" state of midaz `develop` HEAD that every later phase is
measured against. Run and capture, with the exact commit SHA: `go build ./...`; `make test-unit` (capture
pass/fail + count); `make test-integration` (capture pass/fail + which testcontainers ran, or note if
infra-gated/skipped and why); `make lint` (capture finding count under golangci `v2.4.0`); `make sec`
(gosec/govulncheck findings); and the per-package unit coverage numbers vs the CI `85%`
`fail_on_coverage_threshold` gate. Write all of it to `docs/monorepo/plan/baseline-snapshot.md` (NEW) with
the HEAD SHA, date, toolchain (`go version` — pin the real `1.26.3`, NOT the stale "1.25+" CLAUDE.md ref),
and golangci version pinned at top. This is the regression oracle: any later phase that turns a green check
red is caught by diffing against this file.
**Files:** `docs/monorepo/plan/baseline-snapshot.md` (NEW).
**Depends on:** none.
**Acceptance:** `baseline-snapshot.md` records the HEAD SHA, `go build ./...` result, unit test
pass/fail+count, integration test result (or documented skip reason), lint finding count, sec finding
count, and coverage % against the 85% gate — each with the exact command used; toolchain recorded as `1.26.3`.
**Tests:** `git rev-parse HEAD`; `go version`; `go build ./...`; `make test-unit`; `make test-integration`
(or document the gating dependency if it cannot run in this environment); `make lint`; `make sec`.
**Effort:** M (3-5h, dominated by integration test runtime / infra spin-up).
**Risk refs:** R11.

### P0-T04 — Establish docs/monorepo/plan as the tracked single source of truth and commit the phase plans
**Description:** The analysis dossiers live in `docs/monorepo/analysis/` (committed). Establish
`docs/monorepo/plan/` as the tracked plan-of-record: ensure the phase plan files (P0..P9) are committed on a
tracked branch, add a `docs/monorepo/plan/README.md` (NEW) index mapping phase IDs → files → status using the
LOCKED numbering (P3=crm, P4=fees, P5=tracer, P6=reporter), and cross-link the locked decisions PD-1..PD-7 +
risk register R1..R25 so the plan is navigable without re-reading the dossiers. Confirm `docs/` is not
gitignored and that doc-only changes do not trigger the build/release workflows (`release.yml:9 paths-ignore`
covers `**/*.md` and `*.md` — verify). This task makes the plan auditable and prevents plan drift; it changes
NO code.
**Files:** `docs/monorepo/plan/README.md` (NEW), `docs/monorepo/plan/P0.md`.
**Depends on:** none.
**Acceptance:** `plan/README.md` indexes every phase plan file (P0..P9) with a status column under the locked
numbering and links PD-1..PD-7 and R1..R25; `git check-ignore docs/monorepo/plan/README.md` returns nothing
(tracked); a doc-only push is confirmed not to trigger build/gitops (inspect `paths-ignore` in `release.yml`).
**Tests:** `git check-ignore -v docs/monorepo/plan/` (expect no match); `grep -nE "paths-ignore" -A6 .github/workflows/release.yml`
(confirm docs excluded from release trigger); markdown link-check of `plan/README.md` references resolves to
existing files.
**Effort:** S (1-2h).
**Risk refs:** R25.

### P0-T05 — Confirm the four origin repos can become read-only archives + record PD-3 import-exclusion (fresh-import precondition)
**Description:** PD-3 mandates fresh import with origin repos kept as read-only archives. Verify each origin
(`tracer`, `reporter`, `plugin-fees`, and the future-collapsed surfaces) is in a state where it can be frozen
read-only AFTER its code lands in midaz: confirm remotes and default branches (`gh repo view`), confirm no
open release/hotfix branch mid-flight that would be orphaned, and confirm ownership/admin rights to set
`archived: true` (or branch protection lockdown) once the corresponding move phase completes. Produce a
`docs/monorepo/plan/archive-plan.md` (NEW) listing each repo, its remote, default branch, current open PR
count, the move-phase that gates its archival (tracer→P5, reporter→P6, fees→P4, crm-collapse→P3), and the
exact `gh` command to archive it. Do NOT archive anything in P0 — archival happens at the END of each repo's
move phase. This task only proves it is possible and records the runbook. Record TWO verified open items:
(1) the plugin-fees naming mismatch — remote `LerianStudio/plugin-fees` but module
`github.com/LerianStudio/plugins-fees/v3` — note which name downstream references (gitops/helm/docker) use so
archival/redirect is correct; (2) per PD-3, the import MUST exclude reporter's `ast-before-*` artifacts —
record the exclusion glob `reporter/docs/codereview/ast-before-*` (two dirs confirmed present:
`ast-before-3807876316`, `ast-before-3371843854`) so the fresh-import phase has it on record. Also flag
tracer's bare `module tracer` path: every internal tracer import is `tracer/...` with no domain prefix, so the
import-path rewrite cost on co-location is larger than the plugin-fees mismatch — note it as an open item the
tracer-move phase (P5) must budget.
**Files:** `docs/monorepo/plan/archive-plan.md` (NEW).
**Depends on:** none.
**Acceptance:** `archive-plan.md` lists all four repos with remote URL, default branch, open-PR count, the
gating move-phase, and the archival command; the plugin-fees remote-vs-module name mismatch is documented with
the canonical downstream name; the `ast-before-*` import-exclusion glob is recorded; tracer's bare-module
import-rewrite cost is flagged for P5; confirms admin/archive rights exist (or names the owner who holds them).
**Tests:** `gh repo view LerianStudio/tracer --json name,defaultBranchRef,isArchived,viewerCanAdminister`
(and reporter, plugin-fees); `gh pr list --repo LerianStudio/<repo> --state open` for each;
`git -C /Users/fredamaral/repos/lerianstudio/<repo> remote get-url origin` cross-check;
`find /Users/fredamaral/repos/lerianstudio/reporter -maxdepth 3 -name "ast-before-*"` (confirm the exclusion
targets exist).
**Effort:** S (1-2h).
**Risk refs:** R12.

### P0-T06 — Identify owners of and confirm lockstep extension for the external Helm chart `midaz` (R12)
**Description:** `build.yml:35` dispatches to Helm chart `midaz` with `helm_values_key_mappings`
`{"midaz-crm": "crm", "midaz-ledger": "ledger"}` (`build.yml:37`). The end-state renames/deletes images
(crm + plugin-fees images DELETED; tracer/reporter-manager/reporter-worker images ADDED under the `midaz-`
prefix). A co-located component whose Helm values are not updated builds but never deploys; deleting crm/fees
mappings without a chart update breaks ArgoCD sync. Locate the chart repo (the `helm_chart: "midaz"` target —
find the repo backing it via `gh` / org search), identify its CODEOWNERS/maintainers, and obtain written
confirmation that the chart will extend in lockstep with the image topology change: ADD `midaz-tracer`,
`midaz-reporter-manager`, `midaz-reporter-worker` value keys; REMOVE `midaz-crm` and `plugin-fees` keys.
Record owner, repo, the exact value-key delta, and the sequencing constraint (chart change must merge in the
same window as the `build.yml` filter_paths change) in `docs/monorepo/plan/out-of-repo-coordination.md` (NEW).
The no-sign-off fallback for this owner is defined in P0-T15 (Helm = HARD-BLOCKING).
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (NEW, Helm section).
**Depends on:** none.
**Acceptance:** coordination doc names the Helm chart repo, its owner/maintainer, the precise value-key
add/remove delta, and a lockstep sequencing statement with a recorded sign-off status (signed / pending /
refused — feeding P0-T15); no chart change is made in P0 (P0 confirms readiness only).
**Tests:** `gh search repos --owner LerianStudio helm midaz` / `gh repo view <helm-repo> --json name,owner`;
inspect the chart's `values.yaml` for the current `crm`/`ledger` keys to confirm the delta is accurate.
**Effort:** S — active work 2-3h; WALL-CLOCK gated on human owner response (separate the two; phase completion
is governed by P0-T15's fallback, not by this estimate).
**Risk refs:** R12.

### P0-T07 — Confirm lockstep extension for `midaz-firmino-gitops` and the APIDog e2e suite (R12)
**Description:** `build.yml:52` updates `LerianStudio/midaz-firmino-gitops` with `yaml_key_mappings`
`{"midaz-crm.tag": ".crm.image.tag", "midaz-ledger.tag": ".ledger.image.tag"}` (`build.yml:57`), and
`build.yml:103-106` runs APIDog e2e using `MIDAZ_APIDOG_TEST_SCENARIO_ID` /
`MIDAZ_APIDOG_DEV/STG_ENVIRONMENT_ID` / `APIDOG_ACCESS_TOKEN`. Identify owners of (a) the gitops repo and
(b) the APIDog scenario, and confirm both extend in lockstep: gitops must ADD `.tracer.image.tag`,
`.reporter-manager.image.tag`, `.reporter-worker.image.tag` and REMOVE `.crm.image.tag` + the fees tag;
APIDog must add scenario coverage for the newly co-located API surfaces (tracer :4020, reporter-manager :4005)
and keep CRM/fees scenarios working post-collapse (their routes now live behind ledger :3002). Record the
gitops yaml-key delta, the APIDog scenario delta, owners, and the same-window sequencing constraint into the
coordination doc. The no-sign-off fallback for these owners is defined in P0-T15 (gitops = HARD-BLOCKING;
APIDog = NON-BLOCKING / downgrade-not-block).
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (gitops + APIDog sections).
**Depends on:** none.
**Acceptance:** coordination doc names the gitops repo owner and the APIDog scenario owner; lists the exact
gitops `yaml_key_mappings` add/remove delta and the APIDog scenario add/keep delta; both carry a recorded
sign-off status (feeding P0-T15); CRM/fees post-collapse endpoint relocation (now under ledger :3002) is
explicitly noted for APIDog.
**Tests:** `gh repo view LerianStudio/midaz-firmino-gitops --json name,owner,viewerCanAdminister`;
inspect the gitops repo's values file for the current crm/ledger tag keys; confirm the APIDog secret names
exist in the midaz repo settings (`gh secret list` if permitted) so the e2e job is wired.
**Effort:** S — active work 2-3h; WALL-CLOCK gated on human owner responses (governed by P0-T15).
**Risk refs:** R12.

### P0-T08 — Confirm port non-collision and net deploy-unit map for the post-merge topology
**Description:** Verify the final port map has no collisions on the shared host network before any compose
unification: ledger `:3002` (absorbs fees+crm), tracer `:4020`, reporter-manager `:4005`, reporter-worker
`:4006`; the disappearing services are crm `:4003` and plugin-fees `:4002`. Cross-check each origin repo's
declared default port against this map and against midaz's current `:3002`/`:4003`. Use each repo's ACTUAL
layout — reporter has NO root `.env.example`; its ports are declared in
`reporter/components/{manager,worker}/.env.example`, in `reporter/tests/e2e/shared/constants.go`, and via the
worker `HEALTH_PORT` default (4006). The reporter-manager(:4005)/reporter-worker(:4006) split is a P0-T09
layout consequence — cross-check both reporter `cmd/` entrypoints (or component configs) actually declare
those ports, else the collision-free claim rests on a planned, not observed, split. Confirm the TRACER ROLE
decision: tracer is INDEPENDENT of infra's `otel-lgtm` sink — verify midaz infra's `otel-lgtm` service stays
as-is and tracer enters as an isolated component pointing at the shared Postgres (NO telemetry coupling, NO
otel-lgtm replacement). Record the verified port map and the tracer-independence confirmation in
`out-of-repo-coordination.md` (topology section). Verification only; the compose superset is built later.
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (topology section).
**Depends on:** none.
**Acceptance:** doc records the 4-unit port map with each port traced to its origin repo's ACTUAL declaration
(reporter via component-level env + `tests/e2e/shared/constants.go` + worker `HEALTH_PORT`, not a nonexistent
root `.env.example`), confirms zero collisions, confirms the reporter manager/worker port split is observed in
both entrypoints/component configs, and confirms tracer enters pointing at shared Postgres with otel-lgtm
untouched (no telemetry coupling).
**Tests:** `grep -rniE "PORT|4020|4002|4003|3002" /Users/fredamaral/repos/lerianstudio/{tracer,plugin-fees}/.env.example 2>/dev/null`;
`grep -rniE "PORT|4005|4006" /Users/fredamaral/repos/lerianstudio/reporter/components/{manager,worker}/.env.example /Users/fredamaral/repos/lerianstudio/reporter/tests/e2e/shared/constants.go 2>/dev/null`;
`grep -niE "otel-lgtm|otel_lgtm" /Users/fredamaral/repos/lerianstudio/midaz/components/infra/docker-compose.yml`;
`grep -rniE "postgres" /Users/fredamaral/repos/lerianstudio/tracer/components/*/docker-compose*.yml` (confirm
tracer's own pg is a separate service to be dropped, not a sink dependency).
**Effort:** S (1-2h).
**Risk refs:** R20, R21.

### P0-T09 — Verify the shared build.yml fans out one image per top-level component (reporter two-component layout)
**Description:** The locked layout is TWO top-level reporter components (`components/reporter-manager` +
`components/reporter-worker`) because the shared `build.yml` fans out one image per top-level
`filter_path` at `path_level: 2`. Before any move, VERIFY this is how the shared workflow behaves: that two
distinct level-2 dirs produce two distinct images, and that a single component dir with two `cmd/` entrypoints
would NOT cleanly emit two images under one `app_name_prefix` (which would force a shim — forbidden).
Inspect the `LerianStudio/github-actions-shared-workflows` `build.yml` at the version midaz pins and at the
newest validated version, and confirm the auto-discovery semantics. Record the confirmation (and the
shared-workflow version that guarantees it) in `out-of-repo-coordination.md`. This locks the two-component
reporter layout as defect-free rather than a guess; its output feeds the reporter port split in P0-T08.
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (CI fan-out section).
**Depends on:** none.
**Acceptance:** doc cites the shared `build.yml` discovery logic (with the workflow ref/line) proving one
image per top-level `filter_path` dir, and confirms the two-component reporter layout needs no multi-binary
workaround.
**Tests:** `gh api repos/LerianStudio/github-actions-shared-workflows/contents/.github/workflows/build.yml?ref=<pinned-ref> --jq .content | base64 -d | grep -niE "path_level|filter_paths|app_name|matrix|discover"`;
repeat for the newest tag; confirm the component-discovery axis is the level-2 path segment.
**Effort:** M (2-4h, depends on shared-workflow repo access).
**Risk refs:** R12.

### P0-T10 — Verify the unified module needs NO github_token / private-module machinery (clean go mod download)
**Description:** PD/topology assumes the `github_token` BuildKit secret + `.secrets/` + `go_private_modules`
(used by tracer and plugin-fees) can be DROPPED because midaz/reporter prove the common Lerian libs resolve
publicly, and fees' only private import (`midaz/v3`) vanishes on merge. Confirm this empirically: in a clean
environment with NO `~/.netrc`, NO `GOPRIVATE` for `github.com/LerianStudio/*`, and the public proxy, run
`go mod download` against midaz's go.mod and against each incoming repo's go.mod. The goal: prove every
NON-`midaz/v3` Lerian dependency in tracer/reporter/fees (lib-commons, lib-observability, lib-auth,
lib-license-go, lib-streaming) resolves from the PUBLIC proxy without a token. NOTE: this task proves
modules DOWNLOAD; it does NOT prove the v4→v5 import surface migrates (that is P0-T17). Any module that
genuinely requires a token is a blocker that must be surfaced now (open item), not discovered at
Dockerfile-delete time. Record per-repo results in `out-of-repo-coordination.md` (private-module section).
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (private-module section).
**Depends on:** P0-T01.
**Acceptance:** doc confirms `GOPRIVATE=""` + public-proxy `go mod download` succeeds for midaz and resolves
every Lerian lib in tracer/reporter/fees go.mod EXCEPT the `midaz/v3`/`plugins-fees/v3` self/cross references
that disappear on merge; any token-requiring module is listed as a blocker; explicitly states this proves
download-resolvability only, with migration-compat deferred to P0-T17.
**Tests:** `env -u GOPRIVATE -u GONOSUMDB GOPROXY=https://proxy.golang.org GOFLAGS=-mod=mod go mod download -x` in midaz (must succeed);
for each incoming repo: `GOPRIVATE="" GOPROXY=https://proxy.golang.org go list -m all 2>&1 | grep -i lerian`
to enumerate Lerian deps and confirm each resolves publicly (excluding the vanishing self-imports).
**Effort:** M (2-4h).
**Risk refs:** R3, R4.

### P0-T11 — Inventory the exact host-file edit surface for component add/remove (Makefile + 4 workflows)
**Description:** Produce the authoritative checklist of every host file and line that a co-locate/collapse
will touch, so later phases edit a known surface rather than re-deriving it. Capture, with file:line, EACH
annotated ADD/REMOVE/NORMALIZE with target value:
- root `Makefile`: the ledger-special-casing (`LEDGER_DIR:10` deliberately absent from `COMPONENTS`),
  `CRM_DIR:11`, `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` at `:16`, set-env crm `.env` check at `:62-63`, build
  fan-out at `:183/:185`, lint hardcoded `LEDGER_DIR/CRM_DIR` at `:225-227`, and the crm `generate-keys` call
  (NORMALIZE the ledger special-casing; ADD tracer/reporter component dirs; REMOVE the standalone crm fan-out
  once collapsed).
- `build.yml`: filter_paths (`:19`), helm mappings (`:37`), gitops mappings (`:57`), APIDog (`:103-106`).
- `go-combined-analysis.yml`: filter_paths (`:31`) + coverage gate (`:37-38`).
- `pr-security-scan.yml`: filter_paths.
- `pr-validation.yml`: `pr_title_scopes` at `:30-32` — the live list is ONLY `crm` and `ledger`; pre-annotate
  `crm` as REMOVE (obsolete post-collapse) and `tracer` + `reporter` as ADD so the scope delta is not silently
  missed. (Correct the prior draft's false claim of a longer scope list at line 38.)
Record into `docs/monorepo/plan/host-edit-surface.md` (NEW). Re-grep all anchors live before recording — the
doc must match current line numbers. Changes no code in P0.
**Files:** `docs/monorepo/plan/host-edit-surface.md` (NEW).
**Depends on:** none.
**Acceptance:** `host-edit-surface.md` lists every file:line above with an ADD/REMOVE/NORMALIZE annotation and
target value, including the crm `generate-keys` migration, the ledger-special-casing normalization, and the
`pr_title_scopes:30-32` crm→REMOVE / tracer,reporter→ADD delta; no host file is modified in P0; every cited
line matches live grep output.
**Tests:** `grep -nE "COMPONENTS|LEDGER_DIR|CRM_DIR|generate-keys" Makefile`;
`grep -nE "filter_paths|helm_values_key_mappings|yaml_key_mappings|pr_title_scopes|coverage_threshold" .github/workflows/*.yml`
— every line cited in the doc must match current grep output (the doc is verified against live line numbers).
**Effort:** M (2-3h).
**Risk refs:** R12, R25.

### P0-T12 — Confirm coverage-gate baseline of each incoming repo against the 85% hard-fail gate (R11)
**Description:** The midaz CI gate is `coverage_threshold: 85, fail_on_coverage_threshold: true`
(`go-combined-analysis.yml:37-38`). Incoming code (tracer, reporter, fees) that lands under-covered turns CI
red. Measure each incoming repo's CURRENT unit coverage in its OWN repo (`make test-unit` or its coverage
target) and record the gap to 85% per package, so the test-backfill cost is quantified BEFORE the move phases
commit to a date. This is a measurement/coordination task: no tests are written in P0; the output is a
budget. Record per-repo, per-package coverage and the 85% gap into `docs/monorepo/plan/coverage-baseline.md`
(NEW). Note: crm is already in-module and already under the gate — verify its current coverage too as the
control.
**Files:** `docs/monorepo/plan/coverage-baseline.md` (NEW).
**Depends on:** none.
**Acceptance:** `coverage-baseline.md` records current unit coverage % for tracer, reporter, plugin-fees, and
midaz/components/crm, with the per-package gap to 85% and an aggregate backfill estimate flagged for the
relevant move phases (tracer→P5, reporter→P6, fees→P4, crm→P3).
**Tests:** in each origin repo: its coverage command (e.g. `make test-unit` / `go test -cover ./...`),
capturing the per-package and aggregate %; in midaz: `make test-unit` filtered to `components/crm` for the
control number.
**Effort:** M (3-5h, dominated by running three foreign test suites).
**Risk refs:** R11.

### P0-T13 — Confirm no external consumer parses CRM-00xx error codes (PD-2 delete-shim precondition)
**Description:** PD-2 deletes the CRM global `ErrorCodeTransformer` shim and prunes the dead 1:1 CRM-00xx
codes, on the stated basis that "no external consumer parses CRM-00xx." This task VERIFIES that basis (the
EXTERNAL blast radius) before the deletion phase relies on it; the INTERNAL deletion surface and exact prune
set are owned by P0-T16. Search the org for any consumer that string-matches `CRM-00` (SDKs, gateway, APIDog
scenarios, gitops, the Helm chart, docs/openapi published contracts, postman collections). The deletion is
locked; this task de-risks it by confirming the external blast radius is zero (or surfacing the one consumer
that must be coordinated). Record findings in `out-of-repo-coordination.md` (CRM-codes section). If a consumer
is found, it becomes an open item routed to the PD-2 owner — the decision is NOT relitigated, but the lockstep
coordination is added.
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (CRM-codes section).
**Depends on:** none.
**Acceptance:** doc states the search scope (repos + APIDog + published contracts) and the result: zero
external consumers of `CRM-00xx`, OR a named list of consumers requiring lockstep coordination; either way
PD-2's delete is unblocked or blocked with a named owner.
**Tests:** `gh search code --owner LerianStudio "CRM-00"` (and variants `CRM-0014`, `CRM-0001`);
`grep -rniE "CRM-00" /Users/fredamaral/repos/lerianstudio/midaz/{postman,docs,api} 2>/dev/null`;
inspect APIDog scenario payloads for CRM code assertions if accessible.
**Effort:** S (1-2h).
**Risk refs:** R6.

### P0-T14 — Confirm CRM PII crypto key material (LCRYPTO_*) carry-over with NO leak into the tracked surface (PD-2/R7)
**Description:** CRM encrypts holder/alias PII with `libCrypto.Crypto` keyed by `LCRYPTO_HASH_SECRET_KEY` /
`LCRYPTO_ENCRYPT_SECRET_KEY`. On collapse into ledger, these keys MUST be carried with EXACT values or existing
holder/alias documents become permanently undecryptable (data-integrity, not config). VERIFY the tracked-secret
posture FIRST: `.gitignore:32` ignores `.env`, and `git ls-files` confirms only `components/crm/.env.example`
(placeholders at `:53-54`) is tracked — the live `components/crm/.env` (real 64-hex values at `:43-44`) is
UNTRACKED. Confirm this remains true (a regression where `.env` becomes tracked would leak the keys). The
production values are recoverable from the existing secret store (NOT any committed file). Document the
carry-over contract: the unified ledger config must read the SAME key values under a `CRM_`-namespaced var,
sourced from the secret store. This is a precondition for the crm-collapse phase (P3) and a coordination point
with whoever owns the CRM secret in the deploy environment. Record in `out-of-repo-coordination.md`
(crypto-key section). Do NOT print or store actual key values in any plan file. If the verification ever finds
`components/crm/.env` tracked (or any committed file carrying real `LCRYPTO_*` values), that is a FINDING and
open item — block the carry-over until the value is rotated/removed and the secret store becomes the sole
carry-over source; do NOT propagate live key material into the ledger config surface from a tracked file.
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (crypto-key section).
**Depends on:** none.
**Acceptance:** doc names the `LCRYPTO_*` keys, asserts `components/crm/.env` is confirmed
untracked/git-ignored (or, if found tracked, raises it as a blocking finding with a rotate/remove path),
states the production values live in a secret-store reference (not the value), and documents the exact
carry-over contract (same value, `CRM_`-namespaced in ledger config, sourced from the secret store); the
secret owner is identified; no actual key material appears in any committed file.
**Tests:** `git ls-files | grep -E "components/crm/\.env$"` (expect NO match — only `.env.example` tracked);
`git check-ignore -v components/crm/.env` (expect a match on `.gitignore:32`);
`grep -nE "LCRYPTO_(HASH|ENCRYPT)_SECRET_KEY" components/crm/.env.example` (placeholders only);
`git log --all --full-history -- components/crm/.env` (confirm the real `.env` was never committed historically);
`grep -rn LCRYPTO components/crm/internal/bootstrap` (confirm the bootstrap loader references the keys);
confirm the production secret store entry exists (owner-attested, not read into the plan).
**Effort:** S (1-2h).
**Risk refs:** R7.

### P0-T15 — Define the external-owner no-sign-off fallback: blocks-vs-proceeds, decision owner, timebox (CGap2)
**Description:** P0-T06/T07 require lockstep sign-off from three external owners (Helm chart `midaz`,
`midaz-firmino-gitops`, APIDog), and Exit criterion 5 makes that a HARD gate. Without a defined failure mode
the gate is a checkbox: a move phase could proceed without the chart/gitops delta merged in the same window,
silently breaking deploy (build-but-never-deploy / broken ArgoCD sync — the exact R12 failure P0 exists to
prevent), OR the whole consolidation could stall indefinitely on one silent stakeholder. Define the fallback
explicitly in `out-of-repo-coordination.md` (fallback section), per external surface:
- **Named decision owner** for lockstep coordination and for authorizing any move-phase date slip: the
  consolidation/release-platform lead (name the human, not a role placeholder). This owner arbitrates a
  silent/refusing external owner.
- **Response timebox:** a stated deadline per owner; on expiry the owner is treated as non-responsive and the
  rule below applies.
- **Per-owner blocks-vs-proceeds rule:**
  - Helm chart `midaz` (P0-T06) → **BLOCKS** the dependent move phase. There is NO "proceed anyway" lane for a
    deploy-breaking value-key delete: the host-side `build.yml` filter_paths/helm/gitops edit and the external
    chart edit MUST land in the same merge window. A move phase (P3 crm-collapse, P4 fees-move, P5 tracer-move,
    P6 reporter-move) cannot START until its external chart delta carries written sign-off.
  - `midaz-firmino-gitops` (P0-T07) → **BLOCKS**, same reasoning (yaml_key delete without gitops update breaks
    ArgoCD sync).
  - APIDog scenario coverage (P0-T07) → **PROCEEDS** (does NOT block the move): on non-sign-off, the e2e job is
    temporarily marked non-required with a tracked follow-up to restore coverage post-move. APIDog is
    post-deploy validation, not a deploy gate.
- **Escalation path:** if a HARD-BLOCKING owner is silent past the timebox, the decision owner escalates and
  formally slips the move-phase date; no move proceeds on a soft promise.
Record per-external-surface (Helm, gitops, APIDog) with the current sign-off status feeding from P0-T06/T07.
**Files:** `docs/monorepo/plan/out-of-repo-coordination.md` (fallback section).
**Depends on:** P0-T06, P0-T07.
**Acceptance:** coordination doc carries a fallback section naming the single decision owner, a per-owner
response timebox, and an explicit blocks-vs-proceeds rule (Helm=block, gitops=block, APIDog=proceed/downgrade)
with the escalation path; Exit criterion 5 is restated to require EITHER signed lockstep OR the fallback
applied per surface (HARD-BLOCKING owners block the corresponding move phase; APIDog downgrades to a tracked
follow-up).
**Tests:** doc review — confirm each of the three external surfaces from P0-T06/T07 has a named owner, a
timebox, and a block/proceed verdict; confirm the decision owner is a named human; confirm no HARD-BLOCKING
surface has a "proceed anyway" lane.
**Effort:** S (1-2h, plus owner-naming confirmation).
**Risk refs:** R12.

### P0-T16 — Lock the exact CRM shim + dead-CRM-00xx deletion surface, file:line, separating dead from live codes (PD-2)
**Description:** PD-2 deletes the CRM `ErrorCodeTransformer` shim and prunes 12 dead CRM-00xx codes. The later
PD-2 phase (P3-T21, crm-collapse) must delete against a LOCKED map, not re-derive it — re-derivation risks
(a) deleting the two files but leaving the `routes.go:38` wiring → broken build, (b) missing `error_mapping.go`
(the half holding the mappings), or (c) over-pruning `pkg/constant/errors.go` into a LIVE domain code.
Produce a `crm-shim-delete-surface` section in `host-edit-surface.md` listing, with file:line and a DELETE
annotation, the exact surface (VERIFIED at plan time):
- `components/crm/internal/adapters/http/in/error_transformer.go` — DELETE whole file (middleware +
  `transformResponseCode`).
- `components/crm/internal/adapters/http/in/error_mapping.go` — DELETE whole file (`CRMErrorMapping` table +
  `TransformErrorCode`; exactly **12** map entries).
- `components/crm/internal/adapters/http/in/routes.go:38` — REMOVE the `f.Use(ErrorCodeTransformer())` wiring.
- `components/crm/internal/adapters/http/in/error_transformer_test.go` — DELETE WITH the shim (shim test).
- `pkg/constant/errors.go:223-238` — PRUNE exactly the **12 dead** codes the shim owns: the 11 `*CRM`-suffixed
  targets (`CRM-0001/0002/0003/0005/0007/0009/0011/0012/0014/0015/0016`) + `ErrInvalidFieldTypeInRequest`
  (`CRM-0004`, the non-1:1 mapping — `error_mapping.go` maps `ErrInvalidRequestBody` → `ErrInvalidFieldTypeInRequest`,
  so it is structurally different from the 11 and must be called out explicitly).
- **MUST SURVIVE (live domain codes, do NOT touch):** `CRM-0006` (`ErrHolderNotFound`), `CRM-0008`
  (`ErrAliasNotFound`), `CRM-0010` (`ErrDocumentAssociationError`), `CRM-0013` (`ErrAccountAlreadyAssociated`),
  and `CRM-0017..CRM-0029` (Holder/Alias/RelatedParty domain errors). `CRM-0006/0008/0010/0013/0024` are
  asserted in live integration/service tests — deleting them breaks tests and changes API responses.
- **RETAIN:** `components/crm/internal/bootstrap/backward_compat_test.go` — a legit MT single-tenant test, NOT
  part of the shim. Name this RETAIN explicitly so the deletion phase does not over-reach.
Cross-check the count by grepping `error_mapping.go` (12 map entries), the `*CRM` sentinels in
`pkg/constant/errors.go`, and the `CRM-00` literals asserted in CRM tests. Bind this surface to P3-T21.
**Files:** `docs/monorepo/plan/host-edit-surface.md` (crm-shim-delete-surface section).
**Depends on:** P0-T13.
**Acceptance:** `host-edit-surface.md` carries a `crm-shim-delete-surface` section listing every file above
with DELETE/REMOVE/PRUNE/RETAIN annotations and file:line; the prune set is exactly the 12 dead codes with
`CRM-0004`'s non-1:1 mapping called out; the surviving live codes (`CRM-0006/0008/0010/0013` + `0017..0029`)
are enumerated as MUST-SURVIVE; `error_transformer_test.go`=DELETE and `backward_compat_test.go`=RETAIN are
explicit; the section is bound to phase task P3-T21.
**Tests:** `cat components/crm/internal/adapters/http/in/error_mapping.go` (confirm 12 map entries and the
`ErrInvalidFieldTypeInRequest` non-1:1 line); `grep -nE "CRM-00" pkg/constant/errors.go` (confirm `:223-238`
block and the survivors at `:228/230/232/235/239-251`); `grep -n "ErrorCodeTransformer" components/crm/internal/adapters/http/in/routes.go`
(confirm `:38` wiring); `grep -rnE "CRM-00(06|08|10|13|24)" components/crm --include="*.go"` (confirm live
assertions on survivors); `ls components/crm/internal/bootstrap/backward_compat_test.go` (confirm the RETAIN
target exists).
**Effort:** S (1-2h).
**Risk refs:** R6, R7.

### P0-T17 — Dry-run the incoming-repo dependency reconciliation against the unified pin (tracer v4→v5 HARD gate)
**Description:** The single hardest verifiable-now claim in the whole consolidation is that the incoming repos
can reconcile to midaz's unified pin under PD-1 (single root go.mod, NO go.work/replace/shim). The locked
decisions name the tracer `lib-commons/v4 → v5` migration a HARD PREREQUISITE GATE. VERIFIED state: tracer
requires `lib-commons/v4 v4.6.3` DIRECT (`tracer/go.mod:8`) with `v5.3.0`/`lib-observability v1.0.0` only
indirect; reporter is on `lib-commons/v5 v5.1.3`; plugin-fees is on `lib-commons/v5 v5.1.0` +
`lib-observability v1.1.0-beta.5` (AHEAD of midaz's `v1.0.1` GA) with a `lib-commons/v2 v2.9.1` indirect
straggler. Under PD-1 a co-located tracer CANNOT keep v4 import paths alongside midaz's v5 without a rewrite
or a forbidden shim. Prove the migration NOW, while tracer/reporter/fees are checked out locally — mirror
P0-T02's structure but for incoming repos, in throwaway branches that are NEVER merged:
- **tracer:** rewrite `go.mod` from `lib-commons/v4 v4.6.3` to `v5.2.1` + `lib-observability v1.0.0 → v1.0.1`,
  drop the v4 require, `go mod tidy`, `go build ./...`; record the exact v4→v5 API drift sites (file:line) and
  a clean / N-deltas verdict. If the migration depends on v5 API that diverged from v4 (auth, tenant-manager,
  observability split), that is the cost that determines the P5 tracer-move date.
- **reporter:** bump `v5.1.3 → v5.2.1`, `go build ./...`, record deltas.
- **plugin-fees:** bump `v5.1.0 → v5.2.1`, reconcile `lib-observability v1.1.0-beta.5 → v1.0.1` (flag this as a
  potential DOWNGRADE / API-loss — a beta ahead of GA may export surface the GA lacks), drop the
  `lib-commons/v2 v2.9.1` straggler, `go build ./...`, record deltas. Note the `midaz/v3` self-import will not
  resolve until merge — exclude it from the verdict (it vanishes on co-location).
Record per-repo results (clean / N-deltas with file:line) in `deps-pin.md`. Discard all branches; leave every
repo tree clean. This is the verifiable-now blocker the prior draft omitted; it converts the "smallest change"
PD-4 assumption from asserted to verified.
**Files:** `docs/monorepo/plan/deps-pin.md`.
**Depends on:** P0-T01.
**Acceptance:** `deps-pin.md` records, per incoming repo, the reconciliation verdict against `lib-commons/v5
v5.2.1` + `lib-observability v1.0.1`: tracer's v4→v5 migration is either clean or has an enumerated file:line
drift list with an effort verdict feeding P5; reporter and fees verdicts recorded; the
`lib-observability v1.1.0-beta.5 → v1.0.1` fees downgrade is explicitly assessed (clean / API-loss with sites);
all throwaway branches deleted and `git -C <repo> status` clean for each.
**Tests:** per incoming repo on a throwaway branch:
`go mod edit -droprequire=github.com/LerianStudio/lib-commons/v4 -require=github.com/LerianStudio/lib-commons/v5@v5.2.1 -require=github.com/LerianStudio/lib-observability@v1.0.1`
(tracer; adjust the edits per repo), `GOPROXY=https://proxy.golang.org go mod tidy`, `go build ./...` (record
result + drift sites); then `git checkout . && git branch -D <throwaway> && git status` clean. For tracer,
additionally `grep -rn "lib-commons/v4" .` after the edit to prove no v4 import path survives.
**Effort:** L (4-8h, dominated by the tracer v4→v5 API drift hunt).
**Risk refs:** R3, R4, R12.

---

## Exit criteria (Phase 0 is done when)

1. lib-commons v5.2.x GA target is confirmed-present-on-proxy and pinned as `v5.2.1` in `deps-pin.md`
   (PD-4 verified), with a clean/drift verdict from the midaz dry-run bump (P0-T01, P0-T02).
2. A baseline regression snapshot of midaz HEAD (build/unit/integration/lint/sec/coverage) exists in
   `baseline-snapshot.md` with the exact SHA and the real `1.26.3` toolchain (P0-T03).
3. `docs/monorepo/plan/` is the tracked, indexed source of truth with PD/risk cross-links under the locked
   phase numbering (P0-T04).
4. The four origin repos are confirmed archivable read-only with a per-repo archival runbook, the plugin-fees
   name mismatch, the reporter `ast-before-*` import-exclusion glob, and tracer's bare-module rewrite cost all
   documented (P0-T05).
5. Helm chart `midaz`, `midaz-firmino-gitops`, and the APIDog e2e suite each have a named owner AND EITHER a
   signed-off lockstep extension plan with exact key deltas (P0-T06, P0-T07) OR the P0-T15 fallback applied:
   HARD-BLOCKING owners (Helm, gitops) without sign-off BLOCK the corresponding move phase; APIDog without
   sign-off DOWNGRADES to a tracked follow-up but does not block. A named decision owner and per-owner timebox
   exist (P0-T15).
6. The post-merge 4-unit port map is collision-free against each repo's ACTUAL port declaration (reporter via
   component-level env, not a root `.env.example`) and tracer's independence from otel-lgtm is confirmed
   (P0-T08).
7. The shared build.yml one-image-per-top-level-component behavior is verified, locking the two-component
   reporter layout (P0-T09).
8. A clean public-proxy `go mod download` proves no github_token machinery is needed for download-resolution
   (P0-T10), AND the incoming-repo v4→v5 / pin reconciliation is proven against the unified pin with the tracer
   migration cost enumerated (P0-T17).
9. The exact host-file edit surface (Makefile + 4 workflows, by live line, incl. the corrected
   `pr_title_scopes:30-32` crm→REMOVE / tracer,reporter→ADD delta) is inventoried (P0-T11), and the CRM
   shim + 12-dead-code deletion surface is locked file:line with live codes separated and bound to P3-T21
   (P0-T16).
10. Incoming-repo coverage gaps vs the 85% gate are quantified per repo (P0-T12).
11. PD-2 deletion is unblocked: no external CRM-00xx consumer (or a named lockstep list) (P0-T13), and the
    CRM `LCRYPTO_*` key carry-over contract is documented with an owner and the `.env`-untracked posture
    verified (P0-T14).

NO code change to the midaz host lands in Phase 0. Every output is a verification, a snapshot, a tracked
plan document, or an out-of-repo sign-off. The only branches created (P0-T02, P0-T17 dry-runs) are throwaway
and leave every tree clean.

## Risks addressed

R3, R4, R6, R7, R11, R12, R20, R21, R25.

## Open items (flagged, not resolved in P0)

- **lib-commons GA line beyond v5.2.x:** the proxy already has v5.3.x/v5.4.1 GA. PD-4 locks the v5.2.x line
  (`v5.2.1`) as the immediate target; whether to ride higher later is a Phase-1+ decision, not P0's.
- **plugin-fees remote-vs-module name mismatch** (`plugin-fees` repo / `plugins-fees/v3` module): confirm
  which name downstream gitops/helm/docker references use so archival and any redirect are correct (P0-T05
  records it; the canonical-name decision belongs to the fees-move phase, P4).
- **tracer bare-module import rewrite** (`module tracer`, all internal imports `tracer/...` with no domain
  prefix): the import-path rewrite on co-location is larger than the plugin-fees mismatch; P0-T05 flags it,
  P0-T17 quantifies the v4→v5 API drift, and the P5 tracer-move phase must budget both.
- **plugin-fees lib-observability v1.1.0-beta.5 → v1.0.1 downgrade:** a beta AHEAD of the GA pin may export
  surface the GA lacks; P0-T17 assesses whether the reconciliation is clean or loses API. If API-loss is
  found, it is an open item for the fees-move phase (P4), not relitigation of PD-4.
- **APIDog post-collapse scenario relocation:** CRM and fees endpoints move behind ledger :3002 — the APIDog
  scenario owner must confirm the existing assertions still target the right host/port (P0-T07 surfaces it;
  P0-T15 makes APIDog non-blocking with a tracked follow-up).
- **A CRM-00xx consumer, if found** (P0-T13): routed to the PD-2 owner for lockstep coordination; PD-2 itself
  is not relitigated.
- **Whole-tree misleading `libCommons "github.com/LerianStudio/lib-observability"` alias** (verified pervasive
  in pre-existing ledger code, 20+ sites, not just CRM): NOT a P0 task; flagged here so P9's final sweep greps
  the WHOLE tree for misleading aliases, not just shim files, or it survives "liso e final" (SS1).


---

<a id="phase-1"></a>

# Phase 1 — Stable dependency target (lib-commons GA) (10 tasks)

_Verbatim from `docs/monorepo/plan/P1.md`._


**Phase ID:** P1
**Objective:** Bump midaz off `lib-commons/v5 v5.2.0-beta.12` to the v5.2.x **GA** (PD-4), keep
`lib-observability v1.0.1`, and get the full repo green (build / lint / test / sec). This establishes the
exact dependency line that EVERY incoming repo (tracer, reporter, plugin-fees) will be rewritten *to* in
later phases. Smallest possible change, lands independently, de-risks everything downstream.

**Locked decisions in force:** PD-4 (GA first, hold lib-observability at v1.0.1). PD-1/PD-6 set the
downstream contract this phase pins the target for. No shims, no `replace`, no `go.work`.

**Scope fence (read before editing this file):** P1 is *exclusively* the lib-commons GA dependency bump —
a one-line `go.mod` change + `go mod tidy` + a green full-repo gate + recording the frozen pin. There is
**no fee code, no balance arithmetic, no revert/cancel path, no `ValidateSendSourceAndDistribute`, no
streaming emit re-wiring** in this phase. Adversarial-review items that target fee balancing, the single
`validate` variable, deductible-fee revert, per-mode fee tests, precision/ISO-4217, service teardown,
unified third-rail proof, or DAG renumbering of crm/fees/tracer/reporter phases belong to **P2a/P3/P4/P5/
P6/P7/P8/P9** and are deliberately NOT addressed here. They cannot be satisfied or violated by a file that
contains zero fee/balance logic. Resolving them in P1 would be manufacturing scope to fit a generic brief.
The only cross-phase obligation P1 carries is to expose `P1-T06` as the canonical, citable GA-bump gate
that downstream phases depend on (see the DAG-binding note under P1-T06).

---

## Ground-truth established during planning (verified, not assumed)

These were verified live against the module proxy + module cache + the working tree on the planning date.
They are the evidence the tasks below rest on; re-verification commands are encoded in the tasks. Where a
number is given, it is the planning-time observation an executor should expect to reproduce — treat a
mismatch as a signal to re-audit, not as a hard assertion to copy blindly.

1. **GA exists (verified live).** `go list -m -versions github.com/LerianStudio/lib-commons/v5` returns the
   bare-semver GA tags `v5.2.0 v5.2.1 v5.3.0 v5.3.1 v5.3.2 v5.3.3 v5.4.0 v5.4.1` alongside the beta line
   (`… v5.2.0-beta.12 v5.2.0-beta.13 v5.2.0 …`). **v5.2.0 is the first GA of the v5.2 line; v5.2.1 is an
   additive patch** (adds `commons/secretsmanager/external.go`, a package midaz does not import).
   **Pin target: `v5.2.0`** — first GA, minimal surface, exact PD-4 phrasing. `go mod download …@v5.2.0`
   exits 0 against `GOPROXY=https://proxy.golang.org,direct`. The GA line has since advanced to v5.4.1; we
   deliberately do NOT pin latest-stable (see P1-T01 for the explicit rationale) — v5.2.0 is the lowest GA,
   chosen for minimal surface and exact PD-4 ("GA first") phrasing. v5.2.1 is an equally-safe additive-only
   fallback if v5.2.0 is ever yanked.
2. **The bump compiles and tidies clean.** Applied `go get …@v5.2.0` + `go mod tidy` in the working tree:
   `go.mod` changed exactly one line (the lib-commons pin); `go.sum` changed **8 line-level lines = 4 hash
   entries swapped** (`git --numstat` shows `4 4`). `go build ./...` → exit 0. `go vet` on `bootstrap`,
   `http/in`, `crm` → exit 0. Tree was then restored to committed state; **the actual change is owned by
   P1-T03, not pre-applied.** NOTE: `make sec` and the full `make test-unit` coverage gate were **NOT** run
   at planning time — they are the residual unverified gates (see P1-T04 risk note).
3. **beta.12 → GA is NOT a no-op in the library, but IS effectively a no-op for midaz.** GA removed several
   files from `commons/` and `commons/net/http` — `commons/context.go` (holding `NewTrackingFromContext`,
   `NewLoggerFromContext`, `ContextWithLogger`, `ContextWithHeaderID`, `ContextWithTracer`,
   `ContextWithMetricFactory`, `WithTimeoutSafe`, …) and the `withLogging*.go` / `withTelemetry*.go` /
   `context_span.go` HTTP middleware (`NewTelemetryMiddleware`, `WithHTTPLogging`, `WithCustomLogger`,
   `TelemetryMiddleware`, …). **midaz already consumes every one of those symbols from `lib-observability`
   instead.** The audit method that proves this is **import-target resolution, not qualifier-token
   matching** — and the distinction matters because midaz's per-file aliasing is misleading:
   - `NewTrackingFromContext` appears at **471** call sites: **273 written `libCommons.NewTrackingFromContext`**
     and **198 written `libObservability.NewTrackingFromContext`**.
   - A naive executor grepping the qualifier token `libCommons.` would find 273 hits and wrongly conclude
     the symbol is taken from `lib-commons/v5/commons` (which GA removes) — a false alarm, or worse a "fix"
     that breaks the build.
   - **Verified by import-target resolution:** all **139 files** that call `libCommons.NewTrackingFromContext`
     import `libCommons "github.com/LerianStudio/lib-observability"` — i.e. the `libCommons` identifier in
     those files is aliased to **lib-observability**, NOT to `lib-commons/v5/commons`. So every
     `NewTrackingFromContext` call resolves to lib-observability regardless of its qualifier token.
   - Tree-wide, the `libCommons` identifier is bound to **two different modules across different files**:
     **143 files** alias `libCommons → lib-observability`, **172 files** alias `libCommons →
     lib-commons/v5/commons`. **Zero files mix both** (no DUAL-IN-FILE), which is exactly why the bump is
     safe despite the misleading naming. `NewTelemetryMiddleware`/`WithHTTPLogging`/`WithCustomLogger`
     (`unified-server.go`, ledger `routes.go`, crm `routes.go`) likewise resolve to `libObsMiddleware` /
     lib-observability root. **GA's removal of those files cannot break midaz.**
4. **No exported-symbol add/remove in any changed file midaz imports.** Diffed exported funcs/types/consts
   of every *changed* non-test file in the packages midaz uses (`net/http/handler.go`, `redis/lock.go`,
   all `tenant-manager/*` managers + listener/dispatcher/middleware/loader, `utils.go`): the deltas are
   body/comment-only, zero signature churn. NOTE: GA also dropped lib-commons' **own** test files
   (`context_test.go`, `withLogging_test.go`, `withTelemetry_test.go`, …). Those are the *library's* tests,
   not midaz's — they never counted toward midaz coverage, so their removal cannot move the midaz coverage
   number. The bump touches zero midaz `.go` source line, so midaz coverage is mechanically identical
   (the test corpus is byte-identical post-bump).
5. **No third-party fallout.** GA bumped its own indirect testcontainers floor v0.41→v0.42; midaz is
   already at v0.42.0, so MVS does not move. otel 1.44.0, go-redis 9.20.0, fasthttp 1.71.0, rabbitmq
   1.11.0 all unchanged. midaz's `go.mod` go-directive is `go 1.26.3`; GA lib-commons is also on 1.26.3 and
   lib-observability declares `go 1.25.10` — a lower directive than the consuming module is fine (max
   wins), so MVS/toolchain align and there is no toolchain bump implied.
6. **Blast radius of the pin:** 333 `lib-commons/v5` import sites across 23 subpackages
   (`commons`, `commons/net/http`, `commons/crypto`, `commons/mongo`, `commons/postgres`, `commons/redis`,
   `commons/rabbitmq`, `commons/server`, `commons/circuitbreaker`, `commons/constants`, `commons/pointers`,
   and 11 `tenant-manager/*`). All resolve under GA.

**Net:** this phase is genuinely small and low-risk. The risk it removes is strategic, not mechanical —
it stops every downstream repo from being rewritten to a moving beta target.

---

## Task DAG

```
P1-T01 (verify GA exists) ─┐
P1-T02 (API-drift audit) ──┼─> P1-T03 (apply bump + tidy) ─> P1-T04 (full-repo green gate) ─┐
                           │                                  ├─> P1-T04b (streaming JSONShape regression)
P1-T05 (lib-observability  ┘                                  └─> P1-T08 (go.sum/CI-proxy integrity)
        pin-hold guard, parallel)                                          │
P1-T07 (out-of-repo lockstep note) — parallel, advisory                    ▼
                                              GATE: P1-T06 (merge + record canonical frozen pin)
P1-T09 (rollback/abort path) — standing fallback, referenced by P1-T04/T04b/T06/T08
```

`P1-T06` is the phase's single hard gate: it is the merge-to-`develop` + canonical-pin-record task that all
downstream phases depend on. See the DAG-binding note under P1-T06.

---

## Tasks

### P1-T01 — Verify v5.2.x GA exists on the proxy and choose the pin
**Description:** Confirm a non-prerelease v5.2.x tag of `github.com/LerianStudio/lib-commons/v5` is
resolvable from the configured proxy (`GOPROXY=https://proxy.golang.org,direct`). Run
`go list -m -versions github.com/LerianStudio/lib-commons/v5` and assert a bare `v5.2.x` (no `-beta`,
`-rc`, etc.) appears. If a GA exists, pin the **lowest** GA of the v5.2 line (`v5.2.0`) unless it is
yanked, in which case fall to the next patch (`v5.2.1`). The GA line has advanced well past v5.2.0 at
planning time (v5.2.1, v5.3.0–v5.3.3, v5.4.0, v5.4.1 all exist as bare-semver GA tags); pinning v5.2.0 is a
**deliberate** choice (lowest GA, minimal surface, exact PD-4 "GA first" phrasing) — **not** a stale
artifact and **not** an endorsement of jumping to latest-stable. Record that rationale in one line so a
reviewer does not read v5.2.0 as outdated. If — contrary to the verified planning finding — NO v5.2.x GA
exists, this is a PD-4 escalation, not a silent fallback: prefer the lowest bare-semver GA of the *next*
minor (v5.3.0) over latest-stable, and record the substitution in the phase log with justification before
proceeding; do NOT pin a beta. (This branch is dead in practice — v5.2.0 is confirmed live.)
**Files:** none (verification only). Record outcome in `docs/monorepo/plan/P1.md` phase log.
**Depends on:** —
**Acceptance criteria:**
- `go list -m -versions github.com/LerianStudio/lib-commons/v5` output captured and shows `v5.2.0` (and at
  least `v5.2.1`, `v5.3.x`, `v5.4.1`) as bare-semver GA tags.
- `go mod download github.com/LerianStudio/lib-commons/v5@v5.2.0` succeeds (module + go.sum hash fetch
  works against the proxy).
- The chosen pin string is recorded as `v5.2.0` (or the documented escalation outcome), with a one-line
  note explaining why v5.2.0 over the newer v5.3/v5.4 GA tags.
**Tests:** `go list -m -versions github.com/LerianStudio/lib-commons/v5` (real command, verified at planning
to list the GA tags above); `go mod download …@v5.2.0` exits 0 (verified at planning).
**Effort:** S — <1h.
**Risk refs:** R3 (establishes the single v5 line all incoming code converges to), R24.

---

### P1-T02 — Audit beta.12 → GA API drift against midaz's actual usage (by import-target, not token)
**Description:** Diff the GA module against beta.12 for every `lib-commons/v5` subpackage midaz imports,
and prove no symbol midaz consumes was removed or had its signature changed. **Audit method is
import-target resolution, NOT qualifier-token matching** — this is the load-bearing instruction: midaz's
per-file aliasing is misleading, so a token grep produces the wrong answer.

Concretely:
- (a) `diff -rq` the two cached module trees to enumerate added/removed/changed files.
- (b) For every REMOVED non-test file in `commons/` and `commons/net/http`, extract its exported
  funcs/types/consts. For each such symbol that appears in the midaz tree, **resolve what the calling
  file's alias actually imports** (grep the file's import block) — do NOT classify by the qualifier token.
  Assert every call resolves to `lib-observability` (root, `/middleware`, `/log`, or `/tracing`) and NOT to
  `lib-commons/v5/commons*`.
- (c) For every CHANGED non-test file in a midaz-imported package, diff exported funcs/types/consts and
  confirm zero add/remove.

**Expect these planning-time numbers (reproduce them; a mismatch means re-audit):** `NewTrackingFromContext`
has **471** call sites — **273** written `libCommons.NewTrackingFromContext`, **198** written
`libObservability.NewTrackingFromContext`. All **139 files** that call `libCommons.NewTrackingFromContext`
import `libCommons` aliased to `github.com/LerianStudio/lib-observability` (NOT `lib-commons/v5/commons`).
Tree-wide, **143 files** alias `libCommons → lib-observability` and **172 files** alias `libCommons →
lib-commons/v5/commons`, with **zero** files mixing both. The known sharp edges to assert explicitly:
`commons/context.go` and the `withLogging*/withTelemetry*/context_span` files are gone in GA, holding
`NewTrackingFromContext`, `NewTelemetryMiddleware`, `WithHTTPLogging`, `WithCustomLogger`, etc. — and midaz
must be shown to take ALL of these from lib-observability already, by import-target resolution.
**Files:** none (analysis only). Record the symbol-resolution table in the phase log.
**Depends on:** P1-T01.
**Acceptance criteria:**
- Documented list of files removed in GA from `commons/` + `commons/net/http`, with each exported symbol
  classified: (i) midaz does not use it, or (ii) midaz uses it but the calling file's alias resolves to
  `lib-observability`.
- For `NewTrackingFromContext`: a grep that resolves, for each file calling `X.NewTrackingFromContext`,
  what `X` imports — asserting every such file imports `X → github.com/LerianStudio/lib-observability`.
  Zero file resolves a GA-removed symbol through `lib-commons/v5/commons` or `lib-commons/v5/commons/net/http`.
- Exported-symbol diff of all CHANGED midaz-imported files shows no add/remove (body/comment-only).
- The `libHTTP` (`commons/net/http`) symbols midaz uses — `CursorPagination`, `Cursor`, `DecodeCursor`,
  `CalculateCursor`, `PaginateRecords`, `FiberErrorHandler`, `Ping`, `Version`, `ExtractTokenFromHeader`,
  `EncodeCursor`, `ErrInvalidCursor`, cursor-direction consts — all still exist in GA's `commons/net/http`.
**Tests:** `diff -rq` of cached trees + import-target resolution greps (commands recorded in phase log; the
resolution sweep, not a qualifier-token count, is the proof); no midaz code change, so the audit table is
validated by P1-T04's clean build.
**Effort:** S/M — 1–2h.
**Risk refs:** R3, R4 (locks the exact lib-observability boundary every incoming repo will be migrated to).

---

### P1-T03 — Apply the GA bump and tidy
**Description:** On a branch off `develop`, set `github.com/LerianStudio/lib-commons/v5` to the GA pin from
P1-T01 via `go get github.com/LerianStudio/lib-commons/v5@v5.2.0`, then `go mod tidy`. Do NOT touch
`lib-observability` — it must stay at `v1.0.1` (PD-4). Verify the resulting diff is minimal: exactly one
changed line in `go.mod` (the lib-commons pin) and a hash-only `go.sum` change. If `go mod tidy` pulls any
unexpected indirect (anything beyond the lib-commons hash swap), stop and reconcile against P1-T02's
finding before proceeding — an unexpected indirect means a transitive surfaced and must be explained, not
absorbed silently. NO `replace`, NO `go.work`, NO temporary pin of anything else.
**Files:** `go.mod`, `go.sum` (modified).
**Depends on:** P1-T01, P1-T02.
**Acceptance criteria:**
- The `lib-commons/v5` require line in `go.mod` reads `github.com/LerianStudio/lib-commons/v5 v5.2.0`
  (no `-beta`). (Asserted by content, not line number — at planning this is line 101, but any unrelated
  edit above it would shift the number; match the require line itself.)
- The `lib-observability` require line in `go.mod` still reads
  `github.com/LerianStudio/lib-observability v1.0.1` (unchanged; planning line 102), and the
  `lib-streaming` line still reads `v1.4.0` (planning line 103, untouched by this phase).
- `git diff go.mod` shows ONLY the lib-commons line changed; `git diff --numstat go.sum` shows `4 4`
  (4 hash entries swapped = 8 line-level changes), hash-only.
- No `replace` directive, no `go.work`, no other dependency version moved.
**Tests:** `go mod verify` exits 0; `go mod tidy` is idempotent (running it twice produces no further diff).
**Effort:** S — <1h.
**Risk refs:** R3.

---

### P1-T04 — Full-repo green gate (build / vet / lint / unit / sec)
**Description:** Prove the bumped tree is green across the whole repo using the project's own commands, the
same ones CI runs (`go-combined-analysis.yml` → shared `go-pr-analysis.yml@v1.27.5`, golangci-lint
`v2.4.0`; `pr-security-scan.yml` → `make sec`). Run `go build ./...`, `go vet ./...`, `make lint`,
`make test-unit`, `make sec`. Pay special attention to the most lib-commons-coupled packages
(`components/ledger/internal/bootstrap`, `components/ledger/internal/adapters/http/in`, `components/crm`,
and the `tenant-manager` wiring) — these are where any GA drift would surface. All must pass with the GA
pin in place.

**Residual-risk note (the two unverified gates):** `go build`, `go vet`, and the go.mod/go.sum diff were
genuinely executed at planning time and confirmed green. **`make sec` and the `make test-unit` coverage
gate were NOT run at planning** — they are the slowest, most environment-sensitive checks and the only two
in this task with zero planning-time evidence. They are the classic late-stall risk. Mitigations baked into
the acceptance below: (1) the bump touches **zero midaz `.go` source line**, so the unit-test corpus is
byte-identical and coverage is mechanically unchanged — GA's removal of lib-commons' *own* `_test.go` files
does not touch midaz coverage; (2) the only thing that can move is `make sec` flagging a new advisory on
the GA module hash. Budget for one `make sec` remediation loop if a new advisory surfaces, and on red,
invoke P1-T09 (rollback). Do NOT phrase coverage as "not regressed" against an undefined baseline — record
the actual coverage % so the claim is checkable, and note that the expected delta is zero.
**Files:** none (verification of P1-T03's change).
**Depends on:** P1-T03.
**Acceptance criteria:**
- `go build ./...` exits 0 (planning baseline: confirmed 0).
- `go vet ./...` exits 0 (planning baseline: confirmed 0 on the coupled packages).
- `make lint` passes under golangci-lint v2.4.0 with no new findings attributable to the bump.
- `make test-unit` passes; the recorded coverage % is captured and equals the pre-bump number (expected
  delta zero, since no midaz `.go` source line changed). If for any reason it differs, treat as a finding.
- `make sec` passes; no new vuln introduced by the GA module hash. If a new advisory surfaces, run the
  remediation loop or invoke P1-T09.
**Tests:** `go build ./...`, `go vet ./...`, `make lint`, `make test-unit`, `make sec` — all real commands,
all exit 0. (`make sec` + coverage gate are run for the first time HERE, not at planning.)
**Effort:** S/M — 1–2h (dominated by test/lint/sec wall-clock; budget a remediation loop for sec).
**Risk refs:** R3, R11 (coverage gate), R23-adjacent (sec gate — unverified residual risk).

---

### P1-T04b — Streaming JSONShape wire-contract regression check
**Description:** Run the streaming-events JSON-shape lock tests to confirm the CloudEvents wire contract is
byte-identical post-bump. This is the genuinely valuable asset: `pkg/streaming/events/*_test.go` holds the
`JSONShape` / Definition-key / `ToEvent` locks that exist to catch any drift in the emitted CloudEvents
shape, plus `pkg/streaming/*_test.go` (`emit_test.go`, `tenant_test.go`, `mock_test.go`).

**Premise correction (verified):** midaz does **NOT** import or compose `commons/outbox` — `grep
'commons/outbox'` and `grep 'WithOutboxRepository'` across `components/` and `pkg/` both return **zero**
hits. midaz's own streaming code states the outbox is not yet wired (e.g.
`pkg/streaming/events/transaction_lifecycle.go:46` "The outbox subsystem is NOT yet wired in midaz", and
the same follow-up note across `account_created.go`, `account_updated.go`, `account_deleted.go`,
`emit.go`). GA lib-commons does ship `commons/outbox/{dispatcher,mongo,postgres}`, but **midaz composes
none of it**. There is therefore NO outbox-composition unit test to run — that acceptance criterion would
test a vacuum. This task is scoped to the real streaming JSONShape regression only; the outbox claim is
removed.
**Files:** none (runs existing tests under `pkg/streaming/...`).
**Depends on:** P1-T03.
**Acceptance criteria:**
- `go test ./pkg/streaming/...` passes — all `pkg/streaming/events/*_test.go` JSONShape / Definition-key /
  `ToEvent` locks green, and `pkg/streaming/*_test.go` green (verified green at planning: both
  `pkg/streaming` and `pkg/streaming/events` = ok).
- No change to emitted CloudEvents shape (the JSONShape field-count locks hold).
- (Removed: no "commons/outbox composition" criterion — midaz composes no outbox code; outbox is
  integration-tested with testcontainers if/when it lands, out of P1-T04b unit scope.)
**Tests:** `go test ./pkg/streaming/... -count=1` exits 0.
**Effort:** S — <1h.
**Risk refs:** R3.

---

### P1-T05 — lib-observability pin-hold guard (do NOT let tidy drift it)
**Description:** PD-4 mandates `lib-observability` stays at `v1.0.1`. lib-observability has no dependency on
lib-commons (verified: clean module, `go 1.25.10` directive — lower than midaz's `go 1.26.3`, which is
fine; max wins), so the GA bump cannot transitively pull it forward — but `go get`/`go mod tidy` ordering
mistakes or a later careless `go get -u` could. Add an explicit guard: confirm `go.mod` still pins
`lib-observability v1.0.1` after the tidy, and document in the phase log that v1.0.1 is the deliberate hold
(so a future engineer doesn't "fix" it to a newer tag). No `replace`, no artificial constraint — just
verification + a recorded decision. (Lightweight, parallel to T03/T04.)
**Files:** none (verification + phase-log note). Optionally a one-line comment beside the
`lib-observability` require in `go.mod` is acceptable but not required.
**Depends on:** —
**Acceptance criteria:**
- Post-tidy `go.mod` shows `github.com/LerianStudio/lib-observability v1.0.1` (direct, unchanged).
- `go list -m github.com/LerianStudio/lib-observability` reports `v1.0.1` as the selected version.
- Phase log records "lib-observability held at v1.0.1 per PD-4" so the hold is intentional, not stale.
**Tests:** `go list -m github.com/LerianStudio/lib-observability` outputs `v1.0.1`.
**Effort:** S — <30m.
**Risk refs:** R4.

---

### P1-T08 — go.sum / CI-proxy hash-integrity verification
**Description:** Local module-cache success does not guarantee the CI runner (potentially a different
`GOPROXY` / `GONOSUMCHECK` / `GONOSUMDB` posture) resolves the *same* module hash. Close the "green on my
machine" vs "green in CI" gap: after P1-T03, run `go mod verify` and confirm the merged commit's `go.sum`
hash for `lib-commons/v5 v5.2.0` matches what the CI proxy will fetch, and that the CI runner's `GOPROXY`/
`GONOSUMCHECK`/`GONOSUMDB`/`GOFLAGS` posture matches the local resolution path used in P1-T01/T03. This is
the integrity bridge between the local download (P1-T01) and the CI green (P1-T06 merge).
**Files:** none (verification). Phase-log note recording the CI proxy posture confirmation.
**Depends on:** P1-T03.
**Acceptance criteria:**
- `go mod verify` exits 0 on the bumped tree.
- `GOFLAGS=-mod=readonly go build ./...` exits 0 (proves go.sum is complete and the build needs no further
  module resolution — what CI does with a read-only mod).
- Recorded confirmation that CI's `GOPROXY` (and `GONOSUMCHECK`/`GONOSUMDB` if set) match the local posture
  used to fetch v5.2.0, so the CI runner resolves the identical hash recorded in `go.sum`.
**Tests:** `go mod verify` exits 0; `GOFLAGS=-mod=readonly go build ./...` exits 0.
**Effort:** S — <30m.
**Risk refs:** R3, R24.

---

### P1-T06 — GATE: merge the GA bump and record the pinned target as the canonical downstream version
**Description:** This phase's deliverable is not just a green build — it is a *contract*: the version string
every later phase rewrites tracer/reporter/fees TO. **P1-T06 is the single hard gate of this phase and the
canonical handle downstream phases depend on.** Record the canonical pin at a fixed, addressable heading
(`## P1 frozen target pin`, anchor `#p1-frozen-target-pin`) in this file: `lib-commons/v5 v5.2.0` +
`lib-observability v1.0.1` (held) + `lib-streaming v1.4.0` + the unchanged third-party floors (otel 1.44.0,
fiber 2.52.13, pgx 5.9.2, mongo-driver 1.17.9, go-redis 9.20.0, fasthttp 1.71.0, testcontainers 0.42.0,
validator 10.30.x, migrate 4.19.1, rabbitmq 1.11.0, lib-auth/v2 2.8.0). Merge the P1-T03 branch into
`develop` once T04/T04b/T08 green.

**DAG binding (cross-phase — load-bearing):** `P1-T06` is the resolved target for every placeholder/bare
edge that pointed at "P1 final" / "P1 GA-bump merged":
- DAG-3: `P5-T00 depends_on P1-T-final` → resolves to `P5-T00 depends_on P1-T06`.
- DAG-4: `P2b-T01 depends_on P1-T-libcommons-ga-bump` → resolves to `P2b-T01 depends_on P1-T06`.
- DAG-5: bare phase-level edge `… depends_on P1` → resolves to `… depends_on P1-T06`.
Downstream phase files MUST cite `docs/monorepo/plan/P1.md#p1-frozen-target-pin` **by reference** for the
version list and declare `Depends on: P1-T06` for the gate — they must NEVER re-list or re-derive the
versions (re-listing is the exact drift P1 exists to prevent). Editing those downstream files to bind the
ids is the owning task of P5/P2b/etc., not P1; P1's obligation is to make `P1-T06` the stable, named,
single-source-of-truth gate, which it now is.
**Files:** `docs/monorepo/plan/P1.md` (the `## P1 frozen target pin` section below); the merge of the
go.mod/go.sum change into `develop`.
**Depends on:** P1-T04, P1-T04b, P1-T05, P1-T08.
**Acceptance criteria:**
- The exact pin list (including `lib-streaming v1.4.0`) is written into `docs/monorepo/plan/P1.md` under the
  fixed `## P1 frozen target pin` heading as the authoritative downstream target.
- The go.mod/go.sum change is merged to `develop` with a conventional-commit message
  (`chore(deps): bump lib-commons to v5.2.0 GA`), CI green.
- Downstream phase plans cite `docs/monorepo/plan/P1.md#p1-frozen-target-pin` by reference and declare
  `Depends on: P1-T06`, never re-listing versions.
**Tests:** CI on the merge commit (`go-combined-analysis.yml`, `pr-security-scan.yml`) green; `git log`
shows the single deps commit on `develop`.
**Effort:** S — <1h.
**Risk refs:** R3, R24.

---

### P1-T07 — Confirm no out-of-repo lockstep needed for a pure deps bump (advisory)
**Description:** A dependency-version bump is NOT a deploy-topology change — no image renames, no Helm
`values_key_mappings`, no gitops `yaml_key_mappings`, no APIDog e2e change. This task explicitly confirms
that and records it, so the phase is not blocked waiting on cross-team coordination that the LATER phases
(image renames/deletes when crm/fees collapse) genuinely require. The only thing to sanity-check: the
shared CI workflow version (`go-pr-analysis.yml@v1.27.5`) and golangci version (`v2.4.0`) are unchanged by
this phase — confirm no workflow file edit is implied by the bump.

**Note on the broader owner-unavailable fallback:** the "external owner does not sign off / Helm chart
rejected / APIDog suite unavailable" stall path is a REAL risk for the LATER phases that touch deploy
topology — but it does NOT apply to P1, which touches none of that surface (confirmed below). Defining that
fallback is the obligation of the first phase that actually requires an external-owner sign-off (the
crm/fees collapse and image-rename phases), not P1. P1 records here only that it carries zero such
dependency, so a missing external owner cannot stall this phase.
**Files:** none. Phase-log note.
**Depends on:** —
**Acceptance criteria:**
- Recorded confirmation that P1 touches no Helm chart, no `midaz-firmino-gitops`, no APIDog suite, no
  Dockerfile, no compose file, no CI workflow file — hence no external-owner sign-off can block P1.
- `.github/workflows/go-combined-analysis.yml` and `pr-security-scan.yml` are byte-unchanged by P1.
**Tests:** `git diff --name-only develop` after P1-T03 lists ONLY `go.mod`, `go.sum`, and
`docs/monorepo/plan/P1.md` — no `.github/`, no `components/*/Dockerfile`, no `*.mk`, no chart files.
**Effort:** S — <30m.
**Risk refs:** R12 (confirms it does NOT apply here), R24.

---

### P1-T09 — Rollback / abort path (standing fallback for the green gates)
**Description:** Every downstream phase depends on P1 merging to `develop`. If a green gate reds — `make
sec` flags a new advisory, the coverage % drops unexpectedly, `make lint` finds a new issue, `go mod
verify` fails, or CI on the merge commit reds — there must be a defined, executable revert procedure rather
than an improvised scramble. This task is the standing fallback referenced by P1-T04, P1-T04b, P1-T08, and
P1-T06. It is not "work" in the normal sense; it is the named decision point and procedure.
**Files:** none (procedure; executed only on a red gate). Phase-log note recording the trigger and outcome
if invoked.
**Depends on:** — (standing; activates on a red in P1-T04 / P1-T04b / P1-T08 / P1-T06).
**Acceptance criteria:**
- A recorded procedure exists: on a pre-merge red (T04/T04b/T08), `git restore go.mod go.sum` (or drop the
  deps commit) returns the branch to `lib-commons v5.2.0-beta.12`; the branch is not merged; downstream
  phases stay blocked until P1 is re-pinned and re-greened.
- On a post-merge red (CI on the `develop` merge commit), the deps commit is reverted on `develop` with
  `git revert` (conventional-commit `revert: ...`), `develop` returns to beta.12, and the re-pin is retried
  on a fresh branch.
- The trigger (which gate red, why) and the resolution (remediated-and-retried vs reverted-and-blocked) are
  recorded in the phase log so the failure path is auditable, not improvised.
**Tests:** dry-run check that `git restore go.mod go.sum` cleanly returns the working tree to the committed
beta.12 pin (no residual `go.sum` drift); on actual invocation, the post-revert tree builds green
(`go build ./...` exit 0).
**Effort:** S — <30m (only material if triggered).
**Risk refs:** R3, R11, R23-adjacent, R24.

---

## P1 frozen target pin

This is the **single source of truth** for the dependency line every downstream phase
(P2a/P2b/P2c/P3/P4/P5/P6) rewrites incoming repos TO. Downstream phase files MUST cite this section by
reference (`docs/monorepo/plan/P1.md#p1-frozen-target-pin`) and declare `Depends on: P1-T06` — never
re-list or re-derive these versions.

| Dependency | Frozen pin | Note |
| --- | --- | --- |
| `github.com/LerianStudio/lib-commons/v5` | `v5.2.0` | First GA of the v5.2 line (PD-4 "GA first"). |
| `github.com/LerianStudio/lib-observability` | `v1.0.1` | Held per PD-4 (do not advance). |
| `github.com/LerianStudio/lib-streaming` | `v1.4.0` | Untouched by P1; relevant to streaming-instrumentation phases. |
| `github.com/LerianStudio/lib-auth/v2` | `v2.8.0` | Unchanged. |
| otel | `1.44.0` | Unchanged. |
| fiber | `2.52.13` | Unchanged. |
| pgx | `5.9.2` | Unchanged. |
| mongo-driver | `1.17.9` | Unchanged. |
| go-redis | `9.20.0` | Unchanged. |
| fasthttp | `1.71.0` | Unchanged. |
| testcontainers | `0.42.0` | Unchanged (GA floor v0.41→v0.42 already satisfied). |
| validator | `10.30.x` | Unchanged. |
| migrate | `4.19.1` | Unchanged. |
| rabbitmq (amqp091) | `1.11.0` | Unchanged. |

Go toolchain: `go 1.26.3` (midaz go-directive). lib-observability's lower `go 1.25.10` directive is fine —
the consuming module's higher directive wins under MVS.

---

## Exit criteria (phase done when ALL hold)

1. `go.mod` pins `lib-commons/v5 v5.2.0` GA (no beta) and `lib-observability v1.0.1` (held) and
   `lib-streaming v1.4.0` (untouched), merged to `develop`.
2. `go build ./...`, `go vet ./...`, `make lint`, `make test-unit`, `make sec`, `go test ./pkg/streaming/...`
   all green on the merged commit; `go mod verify` and `GOFLAGS=-mod=readonly go build ./...` green.
3. `go.mod`/`go.sum` diff is minimal (lib-commons pin + 4 hash-entry swaps only); no `replace`, no
   `go.work`, no other version moved.
4. The frozen target pin list (`## P1 frozen target pin`) is recorded as the canonical version every
   downstream phase rewrites to, citable by anchor; `P1-T06` is the named gate downstream phases depend on.
5. Confirmed: zero out-of-repo (Helm/gitops/APIDog) or CI-workflow change implied by this phase, hence no
   external-owner sign-off can block P1.
6. A recorded rollback procedure (P1-T09) exists for the red-gate failure path.

## Risks addressed
- **R3** — establishes the single, stable v5 lib-commons line that the tracer v4→v5 and reporter/fees
  v5.1→v5.2 migrations all converge ONTO. A moving beta target would force re-work.
- **R4** — locks the exact `lib-observability v1.0.1` boundary (held, verified) that reporter/fees/tracer
  observability migrations target.
- **R11** — green-gate task records the coverage % (expected delta zero) so the bump doesn't silently red CI.
- **R23-adjacent** — `make sec` is the unverified residual gate; P1-T04 budgets a remediation loop and
  P1-T09 the revert if it reds.
- **R24** — single conventional-commit deps change keeps the repo-wide semantic-release classification clean.

## Open items
- **v5.2.0 vs v5.2.1 vs the newer GA lines (v5.3.x/v5.4.x):** plan pins v5.2.0 (first GA, smallest surface).
  v5.2.1 is additive-only (`secretsmanager/external.go`, unused by midaz) and is the documented fallback if
  v5.2.0 is yanked. The GA line has advanced to v5.4.1; we deliberately do NOT pin latest-stable —
  v5.2.0 minimizes the surface that every downstream rewrite must converge to. If ops later prefers tracking
  a newer GA, that is a single re-pin in a follow-up, not a P1 blocker. Flagged, not blocking.
- **Misleading `libCommons` alias is tree-wide pre-existing debt, NOT created or fixed by P1.** The repo
  binds the `libCommons` identifier to TWO modules across different files — 143 files alias it to
  `lib-observability` (e.g. `components/ledger/internal/services/query/get_all_accounts.go:13`,
  `components/ledger/internal/adapters/redis/transaction/consumer.redis.go:18`) and 172 files alias it to
  `lib-commons/v5/commons`. This is a genuine misleading-alias, not a compatibility shim. The bump survives
  only because **no single file mixes both** (zero DUAL-IN-FILE, verified). Touching 143+ files for a
  cosmetic rename inside a "smallest possible change" deps bump would itself violate discipline, so it is
  correctly scoped OUT of P1. **This deferral is explicit, not silent:** the misleading-alias sweep across
  the WHOLE tree (ledger + crm, not just shims) is owned by the P9 reference-hygiene sweep — P9 must
  grep-for-misleading-aliases tree-wide so this debt does not survive "liso e final." (P9 reference; binding
  is P9's task, not P1's.)
- **No shim anywhere in this phase, and no deletion owed by P1.** P1 is a pure single-line deps bump that
  obsoletes no code, no file, no shim. The PD-2 CRM `ErrorCodeTransformer` shim (`error_transformer.go` +
  its `error_transformer_test.go`) and the 12 dead `CRM-00xx` codes are real and still present, but they
  belong to PD-2 / the crm-collapse phase (P3), NOT to PD-4/P1 — correctly out of scope here.
  `backward_compat_test.go` is the legitimate MT test and must survive; neither it nor the shim is touched
  by P1. Verify the PD-2 deletion tasks land in their own phase file; they are not a P1 gap.
- The `lib-license-go/v2 v2.3.4` (fees) and the net-new reporter/tracer dep surface do NOT enter in P1 —
  they arrive with their respective in-repo migration phases (PD-6). P1 deliberately stays a single-line bump.
- **Out-of-scope-by-design (adversarial brief items targeting other phases):** fee balancing / `sum(legs)==
  fee total` (P4), single `validate` reassignment (P4-T12), deductible-fee revert/cancel refund + no-
  double-reverse (P4-T14/T16, PD-5), Postgres unbounded-DECIMAL vs JSONB-body precision hunt (P4-T23),
  applyFeeCorrection/ISO-4217 balancing independence (P4-T11), pre-move fees-engine correctness spike
  (P2a), per-mode fee tests (P4-T16), unified third-rail proof + service-teardown abort paths (P3/P4/P7),
  pg16→17 logical-replication compat (P5), godog shared-vs-bespoke CI ownership (P5/P7), DAG renumbering of
  crm=P3/fees=P4/tracer=P5/reporter=P6 (P7/P8/P9). None involve fee/balance/streaming-emit code, which P1
  does not touch. Recorded here so a reader does not mistake their absence for an omission.


---

<a id="phase-2a"></a>

# Phase 2a — plugins-fees IN-PLACE dependency migration (18 tasks)

_Verbatim from `docs/monorepo/plan/P2a.md`._


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


---

<a id="phase-2b"></a>

# Phase 2b — Reporter In-Place Dependency Migration (15 tasks)

_Verbatim from `docs/monorepo/plan/P2b.md`._


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


---

<a id="phase-2c"></a>

# Phase 2c — tracer in-place dependency migration (THE LONG POLE) (25 tasks)

_Verbatim from `docs/monorepo/plan/P2c.md`._


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


---

<a id="phase-3"></a>

# Phase 3 — crm → ledger service collapse (23 tasks)

_Verbatim from `docs/monorepo/plan/P3.md`._


**Phase ID:** P3
**Objective:** Collapse the `components/crm` service into the `components/ledger` binary. CRM stops being a
standalone deploy unit (:4003 dies); its 11 holder/alias routes mount on ledger's shared `UnifiedServer`
(:3002) via a 4th `RouteRegistrar`. The global `ErrorCodeTransformer` shim is DELETED (PD-2) along with the
12 dead 1:1 `CRM-00xx` codes. A 3rd `tmmongo.Manager(WithModule("crm-api"))` is wired reusing ledger's
tenant client/cache/loader; a CRM-scoped tenant middleware injects the crm-api Mongo into CRM routes only;
CRM pool eviction folds into ledger's ONE `TenantEventListener.WithOnTenantRemoved`. `ModuleCRM` tenant
constant + provisioning added (R8). Crypto keys (`LCRYPTO_*`) carried with EXACT values (R7). The CRM
standalone bootstrap, Dockerfile, compose, Makefile, swagger, and CI/build entries are deleted — but ONLY
after the unified binary proves the collapse end-to-end (P3-T20) and the abort/rollback path is recorded
(P3-T22). Until then the standalone CRM deploy unit stays buildable and deployable.

**No shims. No replace directives. No go.work.** CRM is already in-module (`github.com/LerianStudio/midaz/v3/components/crm`),
so there is zero dependency skew — this is a pure runtime/service collapse.

## Phase numbering (locked, per DAG-1)
The collapse/move phases are: **P3 = crm collapse** (this phase), **P4 = fees embed**, **P5 = tracer move**,
**P6 = reporter move**. All four gate **P7** (unified third-rail / system suite); **P7 gates P8** (CI
harmonization); **P9** is the final liso/cleanup sweep. Any earlier prose that called the crm collapse "P5"
or routed the shim deletion to "P9" is stale; this phase (P3) is the **authoritative deletion site** for the
`ErrorCodeTransformer` shim and the 12 dead `CRM-00xx` codes. P9-T02 (and any sibling reference) is
DOWNGRADED to a verification-only check that the deletion already happened here — it must not re-delete or
dangle.

## Locked decisions touching this phase
- **PD-2:** DELETE the global `ErrorCodeTransformer` + `error_mapping.go` + `error_transformer_test.go` ONLY.
  These three files under `components/crm/internal/adapters/http/in/` are the shim set. **Do NOT delete
  `components/crm/internal/bootstrap/backward_compat_test.go`** — it is a legitimate single-tenant MT-compat
  regression test (`TestMultiTenant_BackwardCompatibility`) with ZERO reference to the shim. (It is, however,
  coupled to the standalone CRM bootstrap symbols `Config{}`/`initTenantMiddleware`/`newMockLogger` that
  P3-T13 deletes; its three invariants are MIGRATED to ledger's bootstrap test surface in P3-T13b, not
  dropped.) Prune the 12 dead 1:1 `CRM-00xx` codes. Clients receive canonical midaz codes. No
  scoped-transformer fallback.
- **R6:** never mount the error transformer globally on the unified app (it would rewrite ledger's own codes).
- **R7:** carry `LCRYPTO_HASH_SECRET_KEY` / `LCRYPTO_ENCRYPT_SECRET_KEY` with EXACT values or holder/alias PII
  becomes undecryptable.
- **R8:** add `ModuleCRM` constant + `WithMB` registration + per-tenant provisioning or MT requests fail
  silently at DB resolution.
- Preserve the `crm-api` tenant-manager module identity (commit 3f38a8c8c aligned crm→crm-api with
  provisioning). Do NOT rename to `ledger`.
- Tenant SERVICE identity stays `ledger` (`APPLICATION_NAME=ledger`); only the MODULE key is `crm-api`.
- **TF4/CGap3 (abort/rollback gate):** the standalone CRM deploy unit (main/Dockerfile/compose/CI image/
  ArgoCD app) MUST stay intact and deployable until the in-module proof (P3-T20) AND the full quality gate
  (P3-T21) AND the unified third-rail/system suite (P7) are green. The irreversible teardown tasks
  (P3-T13/T17/T18) are GATED on that green signal, mirroring P5-T16.

## Grounded architecture facts (verified against code)
- `NewUnifiedServer(addr, logger, telemetry, readyz, routeRegistrars ...RouteRegistrar)` is variadic
  (`unified-server.go:40`). CRM mounts as a 4th registrar; zero server change.
- **Panic recovery is NOT applied anywhere in the unified path.** Verified: `unified-server.go` applies
  `WithTelemetry` (L57), `cors.New` (L58), `WithHTTPLogging` (L59), `EndTracingSpans` (L89), and the
  public/health/readyz endpoints — but NO Fiber recover middleware. Ledger's own `http/in/routes.go` ALSO
  applies no `WithRecover`. CRM's standalone `NewRouter` is the ONLY registrar that applies
  `http.WithRecover(http.WithRecoverLogger(lg))` (`routes.go:39`). Therefore stripping CRM's `WithRecover`
  during the collapse would DROP the only panic recovery in the process and ship a regression invisibly. P3-T07
  resolves this by HOISTING `WithRecover` into `NewUnifiedServer` once (fixing the pre-existing ledger gap),
  not by silently dropping it.
- The unified MT middleware uses module-keyed `WithMB(mgr, constant.Module)` (`config.go:1048-1049`). CRM today
  uses single-arg `WithMB(mongoManager)` (`config.tenant.go:80`). Both repos read the SAME context key
  `tmcore.GetMBContext(ctx)` (`holder.mongodb.go:75,82`). **Therefore the CRM tenant DB injection MUST be a
  separate middleware instance attached ONLY to CRM routes via a dedicated `ProtectedRouteOptions`** — mounting
  CRM's `WithTenantDB` globally would overwrite ledger handlers' tenant Mongo. This is the one real architectural
  subtlety of the collapse.
- Repos need NO change: `getDatabase` reads `tmcore.GetMBContext(ctx)` then falls back to the static
  connection (`holder.mongodb.go:72-87`).
- Ledger's single eviction closure is `config.go:513-566`; add a crm-api Mongo `CloseConnection` block there.
- 12 dead 1:1 codes (from `error_mapping.go`): CRM-0001, -0002, -0003, -0004, -0005, -0007, -0009, -0011,
  -0012, -0014, -0015, -0016. Domain codes kept: CRM-0006, -0008, -0010, -0013, -0017..-0029. The 12 dead
  sentinels are INTERLEAVED with KEEP domain codes in `pkg/constant/errors.go` (~L223-249, e.g. CRM-0006 at
  L228, CRM-0008 at L230, CRM-0010 at L232, CRM-0013 at L235) — prune by NAME, not by line block.
- The comment at `pkg/constant/errors.go:221` ("Errors with 'CRM' suffix have generic equivalents and are
  used for error code mapping") describes the migration-compat mapping being deleted; after the prune it is
  false (the remaining CRM codes are domain codes with no generic equivalent and no mapping) — fix it in P3-T04.
- `ApplicationName = "plugin-crm"` is a CONSTANT in `routes.go:21` with a guard test
  `TestApplicationNameConstant` in `routes_test.go:22`. Carry the constant (and its test) with the handlers;
  never reintroduce a `"plugin-crm"` string literal (project no-bare-string discipline).
- Auth key drift: CRM `PLUGIN_AUTH_ADDRESS` vs ledger `PLUGIN_AUTH_HOST` (`config.go:69`). Both build the
  identical `middleware.NewAuthClient(<host>, cfg.AuthEnabled, nil)` (crm `config.go:174` vs ledger
  `config.go:716`), so reconciling to ledger's key is behaviorally safe — but it is a BREAKING env-var rename
  for CRM operators; flag to ops alongside `LCRYPTO_*` (P3-T18).
- Swagger generation is INLINE in the ledger Makefile recipe: `swag init -g cmd/app/main.go -o api
  --parseDependency --parseInternal` (`components/ledger/Makefile:255`). There is NO `.swaggo` file for
  ledger. `--parseDependency` makes swag follow the import graph, so once ledger bootstrap imports the CRM
  handler package the holder/alias annotations are scanned automatically (P3-T19).
- The 12 dead CRM-00xx codes are also documented in `llms-full.txt` (~L307-323, the "CRM Error Codes (all
  29)" block) and the README/architecture sections that mention port 4003. These doc references go stale on
  prune; P3-T04 reconciles the error-code entries, the broader doc sweep is P9.

---

## Tasks

### P3-T01 — Verify CRM compiles/tests green in-module before any change (baseline)
**Description:** Establish a clean baseline. Run `make test-unit` scoped to `components/crm/...` and
`go build ./components/crm/...` from repo root to confirm the current standalone CRM builds and tests pass on
`develop`. Capture the current CRM unit coverage number (it has a strict gate; see R11). This is the bisect
anchor — every later task is diffed against this green state. NOTE: this is a read-only baseline capture, NOT
a preserved fallback; the deployable-fallback invariant is owned by P3-T22.
**Files:** `components/crm/` (read-only baseline)
**Depends on:** —
**Acceptance:** `go build ./components/crm/...` succeeds; `go test ./components/crm/...` passes; coverage
number recorded.
**Tests:** `go test ./components/crm/...`; `go vet ./components/crm/...`
**Effort:** S (1-2h)
**Risk refs:** R11

### P3-T02 — Confirm no external consumer parses CRM-00xx (PD-2 precondition)
**Description:** Coordination/verification task. PD-2 deletes the wire-contract translation, so confirm with the
API/product owner and a search of APIDog scenarios + the external `midaz` Helm/gitops consumers that no
client asserts on `CRM-00xx` codes for the 12 dead 1:1 mappings. The domain codes (HolderNotFound=CRM-0006
etc.) are NOT translations and stay. Record the confirmation; if a consumer DOES parse them, escalate to Fred
before P3-T03 lands (it does not change the engineering path — clients get canonical midaz codes — but it is a
breaking API change that needs sign-off). **Owner-unavailable fallback (CGap2):** if the API/product owner
does not sign off within the planned window, do NOT proceed to P3-T03; the locked decision authorizes deletion
but P3-T02 is the surprise-dependency check. If the owner is unreachable, escalate to Fred for a go/no-go
rather than stalling the whole phase indefinitely — Fred makes the breaking-change call directly.
**Files:** `components/crm/internal/adapters/http/in/error_mapping.go` (reference list of the 12)
**Depends on:** —
**Acceptance:** Written confirmation that no external consumer depends on the 12 translated CRM-00xx codes,
OR an escalation logged with Fred's sign-off on the breaking change, OR (owner unavailable) a Fred go/no-go
recorded.
**Tests:** grep APIDog scenario exports for `CRM-00` (manual); n/a automated
**Effort:** S (1-3h, mostly waiting on a human)
**Risk refs:** R6

### P3-T03 — Delete the global ErrorCodeTransformer shim and its tests (authoritative deletion site)
**Description:** Delete the THREE shim files under `components/crm/internal/adapters/http/in/`:
`error_transformer.go`, `error_mapping.go`, and `error_transformer_test.go`. **Do NOT touch
`components/crm/internal/bootstrap/backward_compat_test.go`** — it is a legit single-tenant MT-compat test,
not a shim (PD-2). Remove the `f.Use(ErrorCodeTransformer())` line at `routes.go:38` from CRM's `NewRouter`
(it is being replaced by a registrar in P3-T07 anyway, but remove the reference now so the package compiles
after the files are gone). This is the AUTHORITATIVE site for the shim deletion (P9-T02 is verification-only,
per the phase-numbering block). Sequence P3-T03 then P3-T04 in one logical change so the tree never has a
dangling reference. NOTE: deleting `error_mapping.go` removes the ONLY consumer of the 12 dead sentinels; Go
tolerates unreferenced package-level vars, so the tree still BUILDS with them present — they become dead, not
dangling. The only build break from this task is the deleted `error_transformer_test.go` references, which go
away with the file. Document a one-line note that the "migration-compat mapping" framing is the thing being
killed, so a future reader does not "helpfully" re-add a mapping.
**Files:** `components/crm/internal/adapters/http/in/error_transformer.go` (DELETE),
`components/crm/internal/adapters/http/in/error_mapping.go` (DELETE),
`components/crm/internal/adapters/http/in/error_transformer_test.go` (DELETE),
`components/crm/internal/adapters/http/in/routes.go` (remove `f.Use(ErrorCodeTransformer())` at `:38`)
**Depends on:** P3-T02
**Acceptance:** No file references `ErrorCodeTransformer`, `CRMErrorMapping`, or `TransformErrorCode`
(`grep -rn` returns nothing). The three shim files are gone from git; `backward_compat_test.go` is UNTOUCHED.
**Tests:** `grep -rn "ErrorCodeTransformer\|CRMErrorMapping\|TransformErrorCode" .` returns empty; build
verified jointly with P3-T04.
**Effort:** S (1-2h)
**Risk refs:** R6

### P3-T04 — Prune the 12 dead 1:1 CRM-00xx error sentinels + reconcile docs/comment
**Description:** Remove these 12 sentinels by NAME from `pkg/constant/errors.go` (they are interleaved with
KEEP domain codes across ~L223-249, NOT a contiguous block):
`ErrInvalidMetadataNestingCRM` (CRM-0001), `ErrMetadataKeyLengthExceededCRM` (CRM-0002),
`ErrMissingFieldsInRequestCRM` (CRM-0003), `ErrInvalidFieldTypeInRequest` (CRM-0004),
`ErrInvalidPathParameterCRM` (CRM-0005), `ErrUnexpectedFieldsInTheRequestCRM` (CRM-0007),
`ErrPaginationLimitExceededCRM` (CRM-0009), `ErrInvalidSortOrderCRM` (CRM-0011),
`ErrMetadataValueLengthExceededCRM` (CRM-0012), `ErrInternalServerCRM` (CRM-0014),
`ErrBadRequestCRM` (CRM-0015), `ErrInvalidQueryParameterCRM` (CRM-0016). KEEP the domain sentinels:
CRM-0006, CRM-0008, CRM-0010, CRM-0013, CRM-0017 through CRM-0029. Before deleting each, `grep -rn` the
identifier across the whole repo to PROVE it is referenced only by the now-deleted `error_mapping.go` — if any
domain handler/service still references one, do NOT delete it (flag the discrepancy). Renumber is NOT
required; gaps in the CRM-00xx sequence are acceptable (the codes are independent sentinels, not an array).
Also FIX the now-false comment at `pkg/constant/errors.go:221` ("Errors with 'CRM' suffix have generic
equivalents and are used for error code mapping") — after the prune the remaining CRM codes are domain codes
with no generic equivalent and no mapping; reword to reflect that. Reconcile `llms-full.txt` (~L307-323): the
12 dead CRM-00xx entries in the "CRM Error Codes" block describe codes the binary can no longer emit; remove
those 12 lines and fix the "(all 29)" count to "(all 17)". Run the reference-proving grep over BOTH `*.go`
AND `llms-full.txt` so the published API reference no longer advertises pruned codes.
**Files:** `pkg/constant/errors.go` (12 sentinels + the L221 comment), `llms-full.txt` (12 dead CRM-00xx
entries + the count)
**Depends on:** P3-T03
**Acceptance:** The 12 sentinels are gone; `go build ./...` succeeds for the whole module; the kept CRM
domain sentinels remain and still resolve; the L221 comment no longer claims a mapping exists; `llms-full.txt`
lists only the 17 remaining CRM codes. No dangling reference anywhere.
**Tests:** `go build ./...`; `go test ./pkg/constant/...`; `grep -rn "Err.*CRM\b" --include=*.go` shows only
expected residual (none of the 12); `grep -n "CRM-0001\|CRM-0002\|CRM-0003\|CRM-0004\|CRM-0005\|CRM-0007\|CRM-0009\|CRM-0011\|CRM-0012\|CRM-0014\|CRM-0015\|CRM-0016" llms-full.txt` returns empty.
**Effort:** S-M (2-4h incl. reference-proving grep per identifier + doc reconcile)
**Risk refs:** R6, R25

### P3-T05 — Add ModuleCRM tenant constant
**Description:** Add `ModuleCRM = "crm-api"` to `pkg/constant/module.go` alongside `ModuleOnboarding` and
`ModuleTransaction`. The value MUST be exactly `crm-api` to match tenant-manager provisioning (commit
3f38a8c8c). Replace the bare `const moduleName = "crm-api"` literal that lives in CRM's
`config.tenant.go:29` with this shared constant when the CRM bootstrap folds into ledger (P3-T06) — the
literal must not survive in two places.
**Files:** `pkg/constant/module.go` (add `ModuleCRM`)
**Depends on:** —
**Acceptance:** `constant.ModuleCRM == "crm-api"`; referenced by the new initCRM/middleware wiring;
no remaining bare `"crm-api"` string literal in ledger bootstrap code (only the constant).
**Tests:** `go test ./pkg/constant/...`; a unit assertion `assert.Equal(t, "crm-api", constant.ModuleCRM)`
**Effort:** S (<1h)
**Risk refs:** R8

### P3-T06 — Add CrmPrefixed Mongo + crypto + auth config fields to ledger Config
**Description:** Extend ledger's `Config` struct (`components/ledger/internal/bootstrap/config.go`, after the
`TxnPrefixed*` block ~line 165) with a `CRM_`-namespaced Mongo block + crypto keys, mirroring the
`OnbPrefixed*`/`TxnPrefixed*` pattern. Fields and env tags:
`CrmPrefixedMongoURI` (`MONGO_CRM_URI`), `CrmPrefixedMongoDBHost` (`MONGO_CRM_HOST`),
`CrmPrefixedMongoDBName` (`MONGO_CRM_NAME`), `CrmPrefixedMongoDBUser` (`MONGO_CRM_USER`),
`CrmPrefixedMongoDBPassword` (`MONGO_CRM_PASSWORD`), `CrmPrefixedMongoDBPort` (`MONGO_CRM_PORT`),
`CrmPrefixedMongoDBParameters` (`MONGO_CRM_PARAMETERS`), `CrmPrefixedMaxPoolSize` (`MONGO_CRM_MAX_POOL_SIZE`),
`CrmPrefixedMongoTLSCACert` (`MONGO_CRM_TLS_CA_CERT`), `CrmHashSecretKey` (`LCRYPTO_HASH_SECRET_KEY`),
`CrmEncryptSecretKey` (`LCRYPTO_ENCRYPT_SECRET_KEY`). Reuse ledger's existing single MT block and
`PLUGIN_AUTH_*`/auth fields — do NOT add CRM-specific MT or auth keys (CRM's `PLUGIN_AUTH_ADDRESS` reconciles
to ledger's existing `PLUGIN_AUTH_HOST`). Add programmatic defaults in `applyConfigDefaults` if CRM relied on
any (max pool default 100, mirroring onboarding). Crypto keys keep the bare `LCRYPTO_*` env names (no CRM
prefix) so the EXACT existing key VALUES carry over unchanged (R7) — document this in a field comment.
**Files:** `components/ledger/internal/bootstrap/config.go` (Config struct + applyConfigDefaults)
**Depends on:** —
**Acceptance:** `Config` has all 11 CRM fields with correct env tags; `LCRYPTO_*` env names are byte-identical
to CRM's so existing values decrypt; build succeeds.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run Config`; extend `config_test.go` to assert the
new fields load from env.
**Effort:** S-M (2-4h)
**Risk refs:** R7, R17

### P3-T07 — Convert CRM NewRouter into RegisterCRMRoutesToApp + crmRouteRegistrar; hoist WithRecover
**Description:** Replace CRM's standalone `NewRouter` (`components/crm/internal/adapters/http/in/routes.go`)
with a `RegisterCRMRoutesToApp(f fiber.Router, auth *middleware.AuthClient, hh *HolderHandler,
ah *AliasHandler, routeOptions *libhttp.ProtectedRouteOptions)` function plus a `CreateCRMRouteRegistrar`
returning `func(fiber.Router)`, mirroring ledger's `RegisterMetadataRoutesToApp`/`CreateRouteRegistrar`
(`ledger/.../routes.go:71-101`). STRIP the duplicated app-global middleware that `UnifiedServer` already
applies once: `WithTelemetry`, `cors.New`, `WithHTTPLogging`, `/health`, `/version`, `/swagger`, `/readyz`,
`EndTracingSpans`, and the global tenant `f.Use(tenantMw)`. The `ErrorCodeTransformer` line is already removed
(P3-T03).
**Panic recovery — do NOT silently drop (verified gap):** CRM's `http.WithRecover(http.WithRecoverLogger(lg))`
is the ONLY panic recovery in the unified process — neither `NewUnifiedServer` nor ledger's `routes.go`
applies it. Do NOT strip it on the (false) assumption the platform provides it. Instead HOIST `WithRecover`
into `NewUnifiedServer` ONCE (in `unified-server.go`, before the existing `WithTelemetry`/`cors` chain) so
onboarding + transaction + crm ALL gain Fiber-level panic recovery (this also closes the pre-existing ledger
gap). Use ledger's `http.WithRecover(http.WithRecoverLogger(logger))` so the recover logger matches the
unified logger.
**Authz constant — keep, don't re-literal:** carry the `const ApplicationName = "plugin-crm"` (routes.go:21)
and its guard test `TestApplicationNameConstant` (routes_test.go:22) with the handlers. Keep the 11 routes
EXACTLY: same paths, `auth.Authorize(ApplicationName, resource, verb)` namespace (R9 — do NOT rename, do NOT
substitute a string literal), same `ParseUUIDPathParameters`/`WithBody`. Attach the CRM-scoped tenant
middleware via `routeOptions.PostAuthMiddlewares` (built in P3-T09), NOT as a global `f.Use`.
**Package placement (pinned, was an open "OR"):** KEEP the route definitions in the CRM package
(`components/crm/internal/adapters/http/in`) and have ledger bootstrap IMPORT it — this is legal in-module,
minimizes the diff, and preserves the `plugin-crm` authz locality. Do NOT relocate routes into the ledger
http/in package.
**Files:** `components/crm/internal/adapters/http/in/routes.go` (rewrite to registrar; drop `NewRouter`,
`fiber.New`, middleware stack, public endpoints; keep `ApplicationName` const),
`components/ledger/internal/bootstrap/unified-server.go` (hoist `WithRecover` once)
**Depends on:** P3-T03
**Acceptance:** No `fiber.New`, no duplicated telemetry/cors/logging/health/swagger/readyz in CRM routing
code; the 11 routes register on a passed-in `fiber.Router`; `plugin-crm` authz namespace preserved via the
`ApplicationName` constant (no string literal); `NewUnifiedServer` applies `WithRecover` exactly once so a
forced handler panic returns a 500 via `FiberErrorHandler` instead of killing the connection.
**Tests:** `go test ./components/crm/internal/adapters/http/in/ -run Routes`; `TestApplicationNameConstant`
still passes (carried with the handlers); an integration test mounting the registrar on a bare Fiber app and
asserting all 11 routes resolve with correct methods; a panic-recovery test (folded into P3-T20) asserting a
forced handler panic returns 500.
**Effort:** M (4-6h)
**Risk refs:** R6, R9, R22

### P3-T08 — Extract initCRM() helper (single-tenant Mongo + cipher + repos + UC + handlers)
**Description:** Create `components/ledger/internal/bootstrap/config.mongo.crm.go` (NEW) with an `initCRM(opts
*Options, cfg *Config, logger libLog.Logger) (*crmComponents, error)` helper mirroring `initOnboardingMongo`
(`config.mongo.onboarding.go`). It must: (a) in single-tenant mode build a `libMongo.Client` from the
`CrmPrefixed*` config (reuse `resolveMongoURI`-equivalent logic), build the `libCrypto.Crypto{HashSecretKey:
cfg.CrmHashSecretKey, EncryptSecretKey: cfg.CrmEncryptSecretKey}` and call `InitializeCipher()`, construct the
`holder.NewMongoDBRepository(conn, cipher)` + `alias.NewMongoDBRepository(conn, cipher)`, assemble the CRM
`services.UseCase{HolderRepo, AliasRepo}`, and build the `HolderHandler`/`AliasHandler`; (b) in multi-tenant
mode build a 3rd `tmmongo.Manager(WithModule(constant.ModuleCRM))` reusing `opts.TenantClient` +
`opts.TenantServiceName` (mirror `initOnboardingMultiTenantMongo`), with repos constructed against a nil
connection (per-request context provides the DB). Return a `crmComponents` struct holding
`{connection *libMongo.Client, cipher *libCrypto.Crypto, holderHandler, aliasHandler, mongoManager
*tmmongo.Manager}`. Register the single-tenant connection for graceful close in `InitServersWithOptions` via
the existing `addCleanup` pattern. Call `initCRM` in `InitServersWithOptions` immediately after
`initTransactionMongo` (step 4) with the same `doCleanup()`-on-error discipline. Do this BEFORE wiring routes
so the composition root stays reviewable (per dossier-01 §9.7).
**Files:** `components/ledger/internal/bootstrap/config.mongo.crm.go` (NEW),
`components/ledger/internal/bootstrap/config.go` (call site after `initTransactionMongo`, cleanup registration)
**Depends on:** P3-T05, P3-T06
**Acceptance:** `initCRM` returns wired handlers in both ST and MT modes; cipher initialized with carried key
values; single-tenant Mongo client registered for graceful close; build succeeds; no logic inlined in the
god-function beyond the `initCRM(...)` call + cleanup.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run CRM`; a unit/integration test (testcontainers
Mongo) asserting `initCRM` ST mode returns a working holder repo that round-trips an encrypted holder doc.
**Effort:** M (4-7h)
**Risk refs:** R7, R8, R16

### P3-T09 — Build CRM-scoped tenant middleware + crmRouteOptions in buildUnifiedRouteSetup
**Description:** Extend `buildUnifiedRouteSetup` (`config.go:1013`) to accept the CRM `*tmmongo.Manager` and
build a SEPARATE `tmmiddleware.NewTenantMiddleware(tmmiddleware.WithMB(crmMongoManager, constant.ModuleCRM),
WithTenantCache(tenantCache), WithTenantLoader(tenantLoader))` instance. Add a
`crmRouteOptions *ProtectedRouteOptions` field to the `unifiedRouteSetup` struct (`config.go:1007`) and set
`crmRouteOptions.PostAuthMiddlewares = []fiber.Handler{authAssertion, crmTenantMiddleware.WithTenantDB}`.
**Critical (R6/architecture):** this CRM `WithTenantDB` must be a DISTINCT middleware instance attached ONLY to
CRM routes — NOT added to the onboarding/transaction middleware and NOT mounted globally — because all repos
read the same `tmcore.GetMBContext` key and a global CRM injection would overwrite ledger handlers' tenant
Mongo. Reuse the SAME `tenantCache`/`tenantLoader` already built at `config.go:387-390` (do not create a 2nd
cache/loader). Update the `buildUnifiedRouteSetup` call site (`config.go:720`) to pass `crmMgo.mongoManager`.
**Files:** `components/ledger/internal/bootstrap/config.go` (`unifiedRouteSetup` struct +
`buildUnifiedRouteSetup` signature/body + call site)
**Depends on:** P3-T08
**Acceptance:** A CRM-only tenant middleware exists; `crmRouteOptions` carries it as a PostAuthMiddleware;
onboarding/transaction route options are byte-unchanged; in MT mode a request to a CRM route resolves the
crm-api Mongo while a concurrent ledger request resolves onboarding/transaction Mongo (no cross-contamination).
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run RouteSetup`; an MT integration test asserting
a CRM route and a ledger route in the same process resolve different tenant Mongo DBs from context.
**Effort:** M (4-6h)
**Risk refs:** R6, R8

### P3-T10 — Mount crmRouteRegistrar on the UnifiedServer
**Description:** In `InitServersWithOptions` (`config.go:728-765`), add a `crmRouteRegistrar := func(router
fiber.Router) { httpin.RegisterCRMRoutesToApp(router, auth, crmMgo.holderHandler, crmMgo.aliasHandler,
routeSetup.crmRouteOptions) }` (mirror `onboardingRouteRegistrar`) and pass it as a 4th argument to
`NewUnifiedServer(...)` after `ledgerRouteRegistrar`. The auth client is ledger's existing
`middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, nil)` — CRM uses the same auth client, only a
different authz resource namespace (`plugin-crm`), which is encoded in the route definitions (P3-T07).
**Files:** `components/ledger/internal/bootstrap/config.go` (registrar closure + `NewUnifiedServer` call)
**Depends on:** P3-T07, P3-T09
**Acceptance:** `NewUnifiedServer` is called with 4 registrars; CRM's 11 routes are reachable on `:3002`;
ledger routes unchanged; `go build ./...` succeeds.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run Unified`; integration test hitting
`POST /v1/holders` and `GET /v1/aliases` on the unified server and asserting 401 (auth on) / handler reached.
**Effort:** S-M (2-4h)
**Risk refs:** R6, R9

### P3-T11 — Fold CRM Mongo pool eviction into ledger's single TenantEventListener
**Description:** In ledger's existing `WithOnTenantRemoved` closure (`config.go:513-566`), add a
`crmMgo.mongoManager.CloseConnection(ctx, tenantID)` block alongside the existing onboarding/transaction
Mongo eviction (with the same nil-guard + Warn-on-error logging pattern). Do NOT create a second
`TenantEventListener` or second dispatcher — the CRM standalone listener (`crm/.../config.tenant.go`) is
deleted in P3-T13. One listener evicts onboarding + transaction + crm-api pools on tenant suspend/delete.
The `tmClient.InvalidateConfig` call already present covers config-cache invalidation for the shared service.
**Files:** `components/ledger/internal/bootstrap/config.go` (`WithOnTenantRemoved` closure ~L513-566)
**Depends on:** P3-T08
**Acceptance:** The single eviction closure closes the crm-api Mongo manager; no second listener/dispatcher
exists; on a tenant-removed event all three module pools are evicted.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run Evict`; MT lifecycle test (mirror
`balance_sync.mt_lifecycle_test.go`) asserting crm-api `CloseConnection` is invoked on tenant removal.
**Effort:** S (1-3h)
**Risk refs:** R8

### P3-T12 — Fold CRM Mongo readyz checker into ledger's buildReadyzHandler
**Description:** Extend ledger's `buildReadyzHandler` (`readyz.go:358`) to accept the CRM
`*crmComponents` (or its `connection` + Mongo URI) and, in single-tenant mode when `crmMgo.connection != nil`,
append `NewMongoChecker("mongo_crm", crmMgo.connection, crmMongoURI)` to the checkers slice (mirror
`mongo_onboarding`/`mongo_transaction` at `readyz.go:407-415`). Build the CRM Mongo URI for TLS detection from
the `CrmPrefixed*` config the same way `onbMongoURI`/`txnMongoURI` are built. In MT mode CRM has no static
connection, so no checker is added (consistent with onboarding/transaction). Update the `buildReadyzHandler`
call site (`config.go:748`).
**Files:** `components/ledger/internal/bootstrap/readyz.go` (signature + checker append),
`components/ledger/internal/bootstrap/config.go` (call site)
**Depends on:** P3-T08
**Acceptance:** `/readyz` reports a `mongo_crm` dependency in single-tenant mode; TLS validation (ValidateSaaSTLS)
covers the CRM Mongo connection; MT mode adds no CRM checker.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run Readyz`; extend `readyz_test.go` to assert the
`mongo_crm` checker appears when a CRM connection is present.
**Effort:** S-M (2-4h)
**Risk refs:** R7, R16

### P3-T13 — Delete CRM standalone bootstrap runtime (main/Server/Service/config/listener/readyz)
**Description:** Delete the CRM standalone process surface now subsumed by ledger:
`components/crm/cmd/app/main.go`, `components/crm/internal/bootstrap/server.go`,
`components/crm/internal/bootstrap/service.go`, `components/crm/internal/bootstrap/config.go`,
`components/crm/internal/bootstrap/config.tenant.go`, `components/crm/internal/bootstrap/readyz.go`,
`components/crm/internal/bootstrap/readyz_checkers.go`, `components/crm/internal/bootstrap/tls_detection.go`,
and their tests (`config_test.go`, `readyz_test.go`, `readyz_integration_test.go`, `tls_detection_test.go`,
`backward_compat_test.go`). NOTE: `backward_compat_test.go` is deleted HERE — not as a shim (it is not one;
PD-2 keeps it OUT of the P3-T03 shim set) but because the standalone-bootstrap symbols it exercises
(`Config{}`, `initTenantMiddleware`, `newMockLogger` from `config_test.go`) vanish with this task; the file
cannot compile once they are gone. Its three single-tenant invariants are MIGRATED to ledger's bootstrap test
surface in P3-T13b BEFORE this deletion lands — do NOT delete `backward_compat_test.go` until P3-T13b is
green. The holder/alias Mongo repos, the `services/` use cases, and the (rewritten) `http/in` handlers +
registrar REMAIN (they are imported by ledger bootstrap). After deletion `components/crm/cmd/` is empty →
remove it. Verify the remaining CRM package (`internal/adapters/...`, `internal/services/...`, rewritten
`http/in`) still builds as a library imported by ledger.
**IRREVERSIBLE — gated on the unified green signal (P3-T22 invariant):** this task deletes the standalone
`package main` / `Server` / `Service.Run`. It MUST NOT land until P3-T20 (in-module end-to-end proof) and
P3-T21 (full quality gate) are green AND the abort/rollback runbook (P3-T22) is recorded. The library
extraction (keeping repos/services/http-in) and P3-T13b can land early; the destruction of the runnable
standalone waits for the green signal.
**Files:** `components/crm/cmd/app/main.go` (DELETE), `components/crm/internal/bootstrap/*` (DELETE all listed,
incl. `backward_compat_test.go` after P3-T13b migrates its invariants), associated `*_test.go` (DELETE)
**Depends on:** P3-T08, P3-T10, P3-T11, P3-T12, P3-T13b, P3-T20, P3-T21, P3-T22
**Acceptance:** No CRM `main`, `Server`, `Service.Run`, `InitServersWithOptions`, standalone `config`/tenant
listener, or standalone readyz remains; the remaining CRM packages build as a library; `go build ./...`
succeeds; the ledger binary serves CRM routes; the three single-tenant invariants from `backward_compat_test.go`
live in ledger's bootstrap tests (P3-T13b) and pass.
**Tests:** `go build ./...`; `go test ./components/crm/...` (remaining lib tests pass); confirm no
`package main` under `components/crm`.
**Effort:** M (3-5h)
**Risk refs:** R8, R11

### P3-T13b — Migrate single-tenant MT-compat invariants to ledger's bootstrap tests
**Description:** Before P3-T13 deletes CRM's `backward_compat_test.go`, preserve its three single-tenant
regression invariants on ledger's bootstrap test surface so the single-tenant-mode guarantee mandated by
`multi-tenant.md` is not silently dropped. The invariants are: (1) a zero-value `Config{}` has multi-tenant
DISABLED (all `MultiTenant*` fields default to Go zero values); (2) `initTenantMiddleware` (ledger's analog)
returns a nil handler AND nil listener with NO Tenant Manager contact when MT is disabled; (3) the Config
struct carries the required `MULTI_TENANT_*` fields with correct env tags. Ledger ALREADY has a
`components/ledger/internal/bootstrap/backward_compat_test.go` covering MT single-tenant invariants — RECONCILE
CRM's three invariants against it: if ledger's existing test already asserts all three (it covers the same
single-tenant contract for the unified binary, which now subsumes CRM), record that the coverage is preserved
and ADD any CRM-specific assertion ledger's lacks (notably the crm-api MT path, if relevant) rather than
duplicating. Do NOT create a parallel test file — extend the existing ledger one. Since the merged binary
serves CRM routes, ledger's single-tenant backward-compat test IS the post-collapse home for this guarantee.
**Files:** `components/ledger/internal/bootstrap/backward_compat_test.go` (extend with any CRM-specific
single-tenant invariant ledger's existing test lacks)
**Depends on:** P3-T06, P3-T08
**Acceptance:** Every invariant from CRM's `backward_compat_test.go` is asserted by ledger's bootstrap test
suite (existing coverage confirmed and/or extended); a zero-value unified `Config{}` is proven single-tenant;
ledger's `initTenantMiddleware` returns nil/nil when MT disabled; no parallel/duplicate compat test file
created.
**Tests:** `go test ./components/ledger/internal/bootstrap/ -run BackwardCompat`; the migrated invariants pass.
**Effort:** S (1-2h)
**Risk refs:** R11

### P3-T14 — Fix misleading libCommons-aliasing-lib-observability imports (whole-tree sweep)
**Description:** The misleading alias `libCommons "github.com/LerianStudio/lib-observability"` (pointing the
`libCommons` name at the OBSERVABILITY module) exists not only in CRM files but in PRE-EXISTING ledger code
too — e.g. `components/ledger/.../get_all_accounts.go:13` (SS1). Grep the WHOLE TREE for the misleading alias,
not just shim/CRM files, and rename every occurrence to `libObs` (or the correct `libLog`/`libOpentelemetry`
import as appropriate) so the import name reflects the module. This is cruft the "liso e final" end-state
forbids and would otherwise survive into P9; doing the full-tree sweep here makes P9's grep clean. It does not
change behavior. Confirm via `grep -rn 'libCommons "github.com/LerianStudio/lib-observability"' .` that ZERO
occurrences remain anywhere in the repo.
**Files:** `components/crm/internal/adapters/mongodb/holder/*.go`,
`components/crm/internal/adapters/mongodb/alias/*.go`, `components/crm/internal/services/*.go`,
`components/crm/internal/adapters/http/in/*.go`, AND any `components/ledger/**` / `pkg/**` files carrying the
same misleading alias (grep-discovered, e.g. `get_all_accounts.go`)
**Depends on:** P3-T13
**Acceptance:** No file ANYWHERE in the repo aliases the observability module as `libCommons`; build + tests
still pass.
**Tests:** `go build ./...`; `grep -rn 'libCommons "github.com/LerianStudio/lib-observability"' .` returns empty.
**Effort:** S-M (2-3h, larger scope than CRM-only)
**Risk refs:** R25

### P3-T15 — Merge CRM env into ledger .env.example; carry LCRYPTO_* exact values
**Description:** Add the new `MONGO_CRM_*` keys + `LCRYPTO_HASH_SECRET_KEY` + `LCRYPTO_ENCRYPT_SECRET_KEY` to
`components/ledger/.env.example`, matching the field/env-tag set defined in P3-T06. Do a 3-way diff of CRM's
`.env.example` against ledger's so no silent-missing var slips through (R17 — namespace collisions: CRM's flat
`MONGO_*`/`SERVER_PORT`/`OTEL_*` must NOT leak into ledger's; only the prefixed CRM Mongo + crypto carry over;
CRM's `SERVER_PORT=4003`, `OTEL_RESOURCE_SERVICE_NAME=crm`, `OTEL_LIBRARY_NAME=.../components/crm` are
DROPPED — ledger's `:3002` and `ledger` service identity win). The crypto values carried into the example are
the LOCAL/TEST placeholders (`my-hash-secret-key`/`my-encrypt-secret-key`); production values are managed in
the secrets manager and MUST equal CRM's current production values (R7 — flag to ops in P3-T18).
**Files:** `components/ledger/.env.example`
**Depends on:** P3-T06
**Acceptance:** Ledger `.env.example` has the `MONGO_CRM_*` + `LCRYPTO_*` keys; no `SERVER_PORT=4003` or
`crm` OTEL identity leaked; 3-way diff documented showing every CRM var is either carried-prefixed, merged
(MT block), or intentionally dropped.
**Tests:** `make set-env` produces a ledger `.env` that loads cleanly via `SetConfigFromEnvVars` into the
P3-T06 Config (smoke: `go run` boots in single-tenant with a local Mongo and CRM routes respond).
**Effort:** S-M (2-4h)
**Risk refs:** R7, R17

### P3-T16 — Fold CRM crypto-key generation into ledger set-env; rework root Makefile COMPONENTS
**Description:** The root `set-env` target (`Makefile:406-409`) calls `make -C $(CRM_DIR) generate-keys` to
inject `LCRYPTO_*` placeholders. When CRM's standalone Makefile is deleted (P3-T17), migrate that
`generate-keys` logic (the `inject_crypto_key LCRYPTO_HASH_SECRET_KEY` / `LCRYPTO_ENCRYPT_SECRET_KEY` lines)
into ledger's `set-env`/Makefile so local crypto keys land in ledger's `.env`. Rework the root Makefile's
component list: remove `CRM_DIR := ./components/crm` and drop `$(CRM_DIR)` from `COMPONENTS := $(INFRA_DIR)
$(CRM_DIR)` (`Makefile:11,16`). Audit every loop over `$(COMPONENTS)` and every explicit `$(CRM_DIR)`
reference (build at L185, set-env at L407-409, clear-envs at L407, etc.) so lint/test/format/build still cover
the merged ledger code and no target references the deleted CRM dir.
**Files:** `Makefile` (root), `components/ledger/Makefile` (add crypto-key generation)
**Depends on:** P3-T15
**Acceptance:** `make set-env` injects `LCRYPTO_*` into ledger's `.env` without invoking the deleted CRM
Makefile; root Makefile has no `CRM_DIR`/`$(CRM_DIR)`; `make lint`, `make test-unit`, `make build` all run and
cover the merged ledger code.
**Tests:** `make set-env` then `grep LCRYPTO_ components/ledger/.env`; `make build` succeeds; `make lint`
exits 0.
**Effort:** M (3-5h)
**Risk refs:** R17, R25

### P3-T17 — Delete CRM Dockerfile, docker-compose, Makefile, swagger, scripts, env files
**Description:** Delete CRM's standalone build/deploy/doc surface: `components/crm/Dockerfile`,
`components/crm/docker-compose.yml`, `components/crm/Makefile`, `components/crm/.env`,
`components/crm/.env.example`, `components/crm/.swaggo`, `components/crm/api/` (openapi.yaml, swagger.json,
swagger.yaml, docs.go — the standalone CRM swagger), `components/crm/scripts/` (validate-api-docs.js,
validate-api-implementations.js), `components/crm/reports/`. The holder/alias swagger annotations on the
handlers stay (they fold into ledger's unified swagger in P3-T19). Verify nothing in the repo references the
deleted compose service `midaz-crm` or port `4003`.
**IRREVERSIBLE — gated on the unified green signal (P3-T22 invariant):** deleting the Dockerfile/compose/
Makefile removes the standalone CRM build artifact. MUST NOT land until P3-T20 + P3-T21 are green AND P3-T22's
abort runbook is recorded. Sequence: collapse + mount (T05-T12) → unified suite green (T20/T21 + the external
APIDog/MT e2e in P7) → THEN delete (T13/T17) → THEN CI/gitops lockstep (T18).
**Files:** `components/crm/Dockerfile` (DELETE), `components/crm/docker-compose.yml` (DELETE),
`components/crm/Makefile` (DELETE), `components/crm/.env` (DELETE), `components/crm/.env.example` (DELETE),
`components/crm/.swaggo` (DELETE), `components/crm/api/*` (DELETE), `components/crm/scripts/*` (DELETE),
`components/crm/reports/*` (DELETE)
**Depends on:** P3-T13, P3-T16, P3-T20, P3-T21, P3-T22
**Acceptance:** None of the listed files exist; `grep -rn "midaz-crm\|:4003\|4003" components/ Makefile` returns
no live deploy reference; the CRM image is no longer buildable (intentional, and only after the unified proof).
**Tests:** `grep -rn "components/crm/Dockerfile\|midaz-crm" .` returns empty; `make build` (ledger only) still
succeeds.
**Effort:** S (1-2h)
**Risk refs:** R12, R25

### P3-T18 — Remove CRM from CI build/gitops/helm fan-out (lockstep with ops)
**Description:** Edit `.github/workflows/build.yml`: remove `components/crm` from `filter_paths` (line 20);
remove the `"midaz-crm": "crm"` entry from `helm_values_key_mappings` (line 37); remove
`"midaz-crm.tag": ".crm.image.tag"` from `yaml_key_mappings` (line 57). Also audit
`.github/workflows/pr-security-scan.yml` and `.github/workflows/go-combined-analysis.yml` for CRM-specific
filter paths and prune them (the merged code is covered under `components/ledger` + `pkg/`). **R12 — cross-team
blast radius:** the external `midaz` Helm chart and `midaz-firmino-gitops` repo still reference the `midaz-crm`
image/values key; the CRM Deployment/Service, ingress :4003, and ArgoCD app must be removed in lockstep or
ArgoCD sync breaks. This task COORDINATES that removal (confirm ownership, sequence the gitops PR with this
build.yml change, re-point any API gateway from the CRM host to the ledger host); do not merge the build.yml
change until the gitops/helm side is staged. **Ops flag:** the `PLUGIN_AUTH_ADDRESS`→`PLUGIN_AUTH_HOST`
env-var rename (P3-T06) and the `LCRYPTO_*` exact-value carry-over (R7) are BREAKING operator-facing changes —
include both in the ops handoff alongside the gitops/helm window.
**Owner-unavailable fallback (CGap2):** if the external Helm/gitops owner cannot confirm a deploy window, or
the chart change is rejected, do NOT merge `build.yml` (the standalone CRM stays deployable per P3-T22).
Escalate to Fred for a go/no-go on the deploy window rather than merging a half-staged change that breaks
ArgoCD sync. The standalone-intact invariant (P3-T22) means a stalled ops handoff degrades to "collapse done,
teardown deferred", not "production broken".
**IRREVERSIBLE deploy change — gated on the unified green signal + ops lockstep:** depends on P3-T17 (artifacts
deleted) which itself is gated on P3-T20/T21/T22.
**Files:** `.github/workflows/build.yml`, `.github/workflows/pr-security-scan.yml`,
`.github/workflows/go-combined-analysis.yml`; (out-of-repo) `midaz` Helm chart, `midaz-firmino-gitops`
**Depends on:** P3-T17
**Acceptance:** `build.yml` builds only `components/ledger` (no `midaz-crm` image, no crm helm/gitops keys);
external Helm/gitops CRM entries removed in the same coordinated window OR a Fred-recorded go/no-go if the
owner is unavailable; ArgoCD does not error on a missing `crm` image; APIDog e2e re-pointed if it targeted the
CRM host; ops handoff includes the auth-key rename + crypto-key carry-over.
**Tests:** CI dry-run on a tag shows no `midaz-crm` build job; `gh workflow` run green; gitops PR linked.
**Effort:** M (3-6h incl. cross-team coordination)
**Risk refs:** R12, R24

### P3-T19 — Fold holder/alias endpoints into ledger unified swagger
**Description:** Regenerate ledger's unified swagger (`make generate-docs`) so the holder/alias endpoints
(carried by the swagger annotations on the surviving `HolderHandler`/`AliasHandler` methods) appear in the
single `/swagger` served by `UnifiedServer`. **Mechanism (verified — no `.swaggo` file exists for ledger):**
ledger's swagger is generated by the inline Makefile recipe `swag init -g cmd/app/main.go -o api
--parseDependency --parseInternal` (`components/ledger/Makefile:255`). The `--parseDependency` flag makes swag
follow the import graph, so once ledger bootstrap IMPORTS the CRM handler package (P3-T07/T10), the
holder/alias annotations are scanned automatically — no swag-config edit is needed. Do NOT hunt for a
`.swaggo` file; there is none. The standalone CRM swagger (deleted in P3-T17) is replaced by the unified one.
Verify `/v1/holders`, `/v1/aliases` and the related-party endpoints render in the ledger swagger UI.
**Files:** `components/ledger/api/*` (regenerated)
**Depends on:** P3-T10, P3-T17
**Acceptance:** Ledger's `/swagger` lists all 11 holder/alias endpoints with correct params (incl.
`X-Organization-Id` header) and the `plugin-crm` security scope; no separate CRM swagger exists.
**Tests:** `make generate-docs` then grep the generated `swagger.json` for `/v1/holders` and `/v1/aliases`;
load `/swagger` against a running unified server and confirm the holder/alias group renders.
**Effort:** S-M (2-4h)
**Risk refs:** R22, R25

### P3-T20 — End-to-end integration test: CRM routes on unified ledger binary (ST + MT)
**Description:** Add an integration test (testcontainers Mongo + the unified bootstrap via
`InitServersWithOptions`) that exercises the collapsed CRM surface inside the ledger binary: (1) single-tenant
— create a holder, read it back (proving cipher round-trip with the carried `LCRYPTO_*` keys), create an alias
under it, list aliases, delete; (2) multi-tenant — the same flow with a tenant JWT context, asserting the
crm-api Mongo manager resolves the per-tenant DB and that a concurrent ledger (onboarding) request resolves a
DIFFERENT Mongo without cross-contamination (the R6 architecture risk); (3) error path — a validation failure
returns the CANONICAL midaz code (NOT a translated CRM-00xx), proving the shim is gone (PD-2); (4) tenant
removal evicts the crm-api pool (the P3-T11 fold); (5) panic recovery — a forced handler panic returns 500 via
`FiberErrorHandler` instead of killing the connection (proving the P3-T07 `WithRecover` hoist). This is the
in-module proof the collapse is correct end-to-end and is the GREEN SIGNAL that (together with P3-T21 and the
P7 unified suite) authorizes the irreversible teardown (P3-T13/T17/T18 per P3-T22).
**Files:** `components/ledger/internal/bootstrap/crm_collapse_integration_test.go` (NEW)
**Depends on:** P3-T10, P3-T11, P3-T12
**Acceptance:** All five scenarios pass against real Mongo; holder PII decrypts with carried keys; ledger and
CRM tenant DBs do not cross-contaminate in MT; validation errors return canonical midaz codes; a handler panic
returns 500 (not a dropped connection).
**Tests:** `make test-integration` (the new test); CI green.
**Effort:** M-L (1-2 days)
**Risk refs:** R6, R7, R8, R22

### P3-T21 — Full-module verification gate (build, lint, unit, integration, sec, coverage)
**Description:** In-module quality gate. Run the full suite against the merged tree: `make lint` (the strict
golangci floor), `make test-unit`, `make test-integration`, `make sec`. Confirm the ledger binary boots in
single-tenant with a local Mongo and serves both ledger and CRM routes (smoke via `make up` + curl
`/v1/holders` and an existing ledger endpoint). Verify the 85% coverage gate (R11) still passes for the merged
package set — the CRM tests that survived (services, repos, http/in) must keep coverage above the floor;
confirm no coverage cliff from deleting the standalone bootstrap tests (config/readyz/tls were CRM-specific and
are now subsumed by ledger's bootstrap tests; the single-tenant MT invariants are migrated in P3-T13b).
**Scope note (phase tail / archive gate — binds PHASE-crm-collapse, DAG-2):** P3-T21 is the in-module quality
floor and the phase-tail completion marker for the crm collapse (the id `PHASE-crm-collapse` referenced by
later phases resolves to **P3-T21**). It is NOT the cross-phase system suite — the unified third-rail/system
proof is **P7**, and the irreversible teardown is additionally gated on P7 per P3-T22. P3-T21 green +
P3-T20 green + P3-T22 runbook recorded is the local authorization for teardown; the P7 unified-suite green is
the final cross-phase authorization.
**Files:** whole module (verification only)
**Depends on:** P3-T13b, P3-T14, P3-T19, P3-T20
**Acceptance:** `make lint`, `make test-unit`, `make test-integration`, `make sec` all green; coverage gate
passes; `make up` boots a unified binary serving ledger + CRM routes on :3002; no :4003.
**Tests:** `make lint && make test-unit && make test-integration && make sec`; `make up` smoke curl.
**Effort:** M (3-6h, more if coverage backfill needed)
**Risk refs:** R11, R16

### P3-T22 — Define the CRM-collapse abort/rollback path (standalone stays deployable until unified green)
**Description:** This phase mutates a LIVE production deploy unit (:4003 dies, ArgoCD app removed) and the
irreversible deletions (P3-T13 main/Server/Service/config/listener; P3-T17 Dockerfile/compose/Makefile/
swagger; P3-T18 CI/gitops/helm) are unrecoverable in-tree once landed. Define the revert path BEFORE those
deletions so failure is recoverable, mirroring P5-T16. Document the abort as a `git revert` of the
teardown commit range `X..Y` on the midaz branch — and keep the WIRING commits (T03-T12: shim delete, config,
initCRM, middleware, mount) SEPARATE from the TEARDOWN commits (T13/T17/T18: delete standalone runtime +
artifacts + CI) so the teardown range is independently revertable and bisectable. **HARD INVARIANT
(TF4/CGap3):** the standalone CRM deploy unit — `cmd/app/main.go`, `Dockerfile`, `docker-compose.yml`, the
`midaz-crm` CI image, and the gitops/ArgoCD app — MUST remain intact, buildable, and deployable until ALL of:
P3-T20 (in-module e2e), P3-T21 (full quality gate), and the P7 unified third-rail/system suite are GREEN.
Until that combined green, the collapse is "wired but standalone still primary"; the registrar mount is live
on :3002 but :4003 stays deployable as the rollback target. The rollback procedure is: `git revert` the
teardown range, re-enable the standalone `build.yml` job + gitops ArgoCD app, redeploy :4003. Record the exact
commit range and abort command in the runbook once T03-T12 land. This task GATES P3-T13/T17/T18 — they may not
merge until this runbook exists and the combined green signal is achieved.
**Files:** `docs/monorepo/plan/P3.md` (runbook abort section)
**Depends on:** P3-T20, P3-T21
**Acceptance:** Abort runbook entry exists with the exact `git revert X..Y` teardown range; the
standalone-stays-deployable-until-(P3-T20 ∧ P3-T21 ∧ P7)-green invariant is stated and wired into P3-T13/T17/T18
sequencing (each depends on P3-T22); the wiring and teardown commit ranges are documented as separate for
bisectability.
**Tests:** Dry-run the abort on a throwaway branch: `git revert` the teardown range restores a compiling
midaz tree with the standalone CRM main/Dockerfile/CI present and the ledger registrar mount reverted.
**Effort:** S (1-2h)
**Risk refs:** R12, R16

---

## Exit criteria
- The ledger binary serves all 11 CRM holder/alias routes on `:3002`; the CRM `:4003` standalone service no
  longer exists (no `package main`, no Dockerfile, no compose, no CI image) — but ONLY after the abort/rollback
  runbook (P3-T22) is recorded and P3-T20 + P3-T21 + the P7 unified suite are green.
- The global `ErrorCodeTransformer` shim and its THREE files (`error_transformer.go`, `error_mapping.go`,
  `error_transformer_test.go`) are deleted (this phase is the authoritative deletion site); the 12 dead 1:1
  `CRM-00xx` codes are pruned; clients receive canonical midaz codes (PD-2). The 17 CRM domain codes remain.
  `backward_compat_test.go` is NOT a shim file and is NOT in the shim-delete set; its single-tenant invariants
  are migrated to ledger's bootstrap test surface (P3-T13b) before the file is dropped with the standalone
  bootstrap (P3-T13).
- `unified-server.go` applies Fiber `WithRecover` exactly once, so onboarding + transaction + crm all have
  panic recovery (closing the pre-existing ledger gap); a handler panic returns 500, not a dropped connection.
- A 3rd `tmmongo.Manager(WithModule("crm-api"))` is wired reusing ledger's tenant client/cache/loader; CRM
  routes get a route-scoped tenant middleware; CRM pool eviction is folded into ledger's ONE
  `TenantEventListener` (no second listener).
- `ModuleCRM = "crm-api"` constant added; provisioning identity preserved; MT CRM requests resolve the crm-api
  Mongo (R8).
- `LCRYPTO_*` keys carried with EXACT values; existing holder/alias PII decrypts (R7) — proven by the
  integration round-trip test.
- No misleading `libCommons "github.com/LerianStudio/lib-observability"` alias survives ANYWHERE in the tree
  (whole-tree sweep, not just CRM/shim files — SS1).
- `llms-full.txt` advertises only the 17 remaining CRM codes; the `errors.go:221` mapping comment is corrected.
- No shim, no replace directive, no go.work introduced anywhere.
- Full quality suite green; coverage gate held; external Helm/gitops/APIDog updated in lockstep (R12) OR a
  Fred-recorded go/no-go if an external owner is unavailable (CGap2).

## Risks addressed
R6 (error transformer global rewrite — deleted, not scoped; panic-recovery hoist), R7 (crypto key carry-over),
R8 (ModuleCRM + provisioning + eviction fold), R9 (preserve plugin-crm authz namespace via the
`ApplicationName` constant), R11 (coverage gate + single-tenant invariant migration), R12 (helm/gitops
lockstep + abort/rollback), R16 (migration/startup path — CRM is Mongo-only, no Postgres migration added; plus
abort runbook), R17 (env merge, namespace collisions, auth-key rename), R22 (X-Organization-Id header
convention documented in unified swagger), R24 (conventional-commit scope), R25 (stale docs / misleading
aliases / Makefile rework / llms-full reconcile).

## Open items
1. **PD-2 external-consumer confirmation (P3-T02)** is a product/API gate, not engineering. If a consumer DOES
   parse the 12 translated CRM-00xx codes, deleting the shim is a breaking change needing Fred's explicit
   sign-off. The plan assumes "delete" is approved (the locked decision says so); P3-T02 only confirms no
   surprise external dependency exists. No scoped-transformer fallback is permitted (PD-2). Owner-unavailable
   path: escalate to Fred for go/no-go (CGap2).
2. **R12 cross-team lockstep (P3-T18)** depends on the external `midaz` Helm chart + `midaz-firmino-gitops`
   ownership. Sequencing the build.yml change with the gitops PR requires ops coordination outside this repo;
   confirm the owner and the deploy window before merging P3-T18. If the owner is unavailable or the chart
   change is rejected, do NOT merge build.yml — the standalone CRM stays deployable (P3-T22) and the teardown
   degrades to "deferred", not "production broken". Name a deploy-window owner before P3 starts.
3. **CRM Mongo instance topology** — whether CRM gets a dedicated Mongo connection (the `MONGO_CRM_*` block) or
   shares ledger's onboarding/transaction Mongo instance is a DEPLOY decision. The plan provisions a dedicated
   `CrmPrefixed*` connection block (cleanest, matches dossier-02 §5.2 recommendation); pointing it at the same
   physical Mongo host as ledger is then a pure env-config choice with no code change.
4. **X-Organization-Id vs path-based scoping (R22)** — CRM scopes by the `X-Organization-Id` header while
   ledger scopes by `/v1/organizations/:id/...` path. This API-shape divergence persists post-collapse (no
   collision, no functional issue). Reworking CRM to path-based scoping is explicitly OUT of scope; documented
   in the unified swagger only.
5. **Tenant-manager provisioning of the crm-api module per tenant** is an operational prerequisite for MT (R8):
   each tenant must have the crm-api module provisioned in tenant-manager or MT CRM requests fail at DB
   resolution. The code wiring (P3-T05/T08/T09) assumes provisioning exists (the crm-api module already runs in
   prod under the standalone service); confirm the provisioning identity survives the service collapse
   unchanged.
6. **P3/P9 shim-deletion overlap** — this phase (P3-T03) is the AUTHORITATIVE deletion site for the
   `ErrorCodeTransformer` shim and the 12 dead codes. P9-T02 (or any sibling reference) is verification-only:
   it must assert the deletion already happened, NOT re-delete (a no-op or dangling-reference hazard). The plan
   owner should confirm P9-T02 is downgraded accordingly (DAG-1).


---

<a id="phase-4"></a>

# Phase 4 — plugin-fees → ledger service collapse (THE THIRD-RAIL MOVE) (27 tasks)

_Verbatim from `docs/monorepo/plan/P4.md`._


**Phase ID:** P4
**Objective:** Embed the plugin-fees engine into `components/ledger`, wire it into the
transaction-create funnel, persist fee/billing-package state in ledger's MongoDB, refund fees on
revert and pending-cancel, and tear down the standalone fees service — with zero shims and proven
double-entry balance under ledger's own validator.

This phase depends on **P2a** (fees observability + lib-commons migrated in-repo, PD-6) and **P1**
(midaz on lib-commons v5.2.x GA, PD-4). The fee code does not compile inside ledger's module until
P2a lands — that is the hard precondition gate for everything here.

**Locked phase numbering (authoritative, per DAG-1):** P3 = crm embed, **P4 = plugin-fees collapse
(this file)**, P5 = tracer co-location, P6 = reporter co-location. All four moves gate P7; P7 gates
P8. This file binds to the locked scheme and references the concrete sibling task ids
(`P7-T18`, `P2a-T17`) directly so an implementer reading P4's dependency blocks never has to
reconcile drifted labels. (Sibling prose is now aligned to the locked numbering as well.)

---

## Locked decisions this phase executes against

- **PD-5 (THIRD RAIL):** refund original fees on transaction revert AND pending-cancel by reversing
  the fee legs. Integration tests must prove the reversal also balances (sum of all legs incl.
  reversed fees == 0). PD-5 is a VERIFY-not-REBUILD here (see P4-T14): `TransactionRevert` is already
  fee-aware by construction; injecting refund legs would double-reverse.
- **PD-6:** fees observability + lib-commons migrated IN-PLACE in the fees repo first, validated
  against fees' own CI, THEN the code moves. Observability and co-location never share a commit.
- **PD-7:** fee/billing-package state persists as NEW collections in ledger's existing MongoDB
  (port the 11 compound indexes). NOT Postgres.

---

## Correctness baseline (verified against HEAD — read before planning the third rail)

Four "facts" carried into earlier drafts were **wrong** and have been corrected here. They were
load-bearing for the third-rail tasks, so they are pinned explicitly:

1. **There is NO per-asset or per-balance `Scale` in the ledger.** Verified at HEAD:
   - `pkg/mmodel/asset.go` has no `Scale` field (`grep Scale` → no match).
   - `pkg/mmodel/balance.go` carries `Available`/`OnHold`/`OverdraftUsed` as raw
     `decimal.Decimal` (arbitrary precision) — **no `Scale` field**.
   - `pkg/mtransaction` `Amount`/`Send` carry only `Value decimal.Decimal` — no `Scale`.
   - The streaming comment at `pkg/streaming/events/balance_created.go` that references
     `mmodel.Asset.Scale` is itself **stale** — that field does not exist. Flag for separate
     cleanup; do not treat it as evidence of a scale model.
   - `Scale` survives only on `assetrate` (FX rates), which is irrelevant to fee leg rounding.

   **Consequence:** the ledger is arbitrary-precision decimal. The validator
   (`pkg/mtransaction/validations.go:621`) enforces balance by **exact** `decimal.Equal`:
   `!sourcesTotal.Equal(destinationsTotal) || !destinationsTotal.Equal(response.Total)` →
   `ErrTransactionValueMismatch`. **Zero rounding/scale logic.** There is no "ledger scale" for the
   fee engine to "agree with." The third-rail task is reframed around decimal exactness and the
   **serialization** precision boundary (NOT a phantom `Asset.Scale` lookup, and NOT the Postgres
   amount column — see baseline #4).

2. **`TransactionRevert()` is already fee-inclusive by construction.** Verified at
   `components/ledger/internal/adapters/postgres/transaction/transaction.go:293`: it iterates
   `t.Operations`, swapping `CREDIT`→from and `DEBIT`→to (skipping `#overdraft` companions at L325 /
   L347 via `BalanceKey == OverdraftBalanceKey`), and sets reverse `Send.Value = *t.Amount` (L386).
   Because the fee seam (P4-T12) persists fee legs as **real operations** on the parent transaction,
   `TransactionRevert()` reverses the fee legs automatically. The revert refund is therefore a
   **verification** task, not a build-refund-legs task. Manually injecting refund legs on top of this
   would double-reverse and fail the validator's exact-equality check — a self-inflicted third-rail
   violation. Corrected in P4-T14 (CG2).

3. **`FromTo.Route` is NOT formally deprecated — it is "passive, kept for backward compatibility."**
   Verified: `Route` is `validate:"omitempty,max=250"`; `RouteID` is `validate:"omitempty,uuid"`.
   The ledger's own operation builder writes **both** `ft.Route` (with `//nolint:staticcheck //
   legacy field kept for backward compatibility; RouteID is canonical`) and `ft.RouteID`
   (`transaction_create.go:1201-1202`), and so does `TransactionRevert` (L342-343, L364-365). RouteID
   is canonical; Route is a passive fallback the ledger populates everywhere. The fee engine should
   mirror this dual-write convention, not drop Route — this is the ledger-wide convention, NOT a
   fees-local shim. The `FromTo.Route` passive-compat field and its `//nolint:staticcheck` markers
   (`transaction.go:342/364`, `transaction_create.go:1201`) are an **EXPLICIT accepted exception** to
   the no-shims mandate (SS2): they are pre-existing ledger-wide debt, mirrored — not introduced — by
   this phase, and their eventual removal is out-of-phase (see Open items). Corrected in P4-T02.

4. **The persistence-decimal boundary is NOT the Postgres `amount`/`available` columns — they are
   UNBOUNDED `DECIMAL`.** Verified at HEAD (TF2):
   - `migrations/transaction/000005_update_balance.up.sql`: `ALTER COLUMN available TYPE DECIMAL`,
     `ALTER COLUMN on_hold TYPE DECIMAL`, then `DROP COLUMN IF EXISTS scale`.
   - `migrations/transaction/000006_update_operation.up.sql`: `ALTER COLUMN amount TYPE DECIMAL`
     (and `available_balance`, `on_hold_balance`, `*_after`) then `DROP COLUMN IF EXISTS amount_scale,
     balance_scale, balance_scale_after`.
   - Bare `DECIMAL` in Postgres = `NUMERIC` with no `(p,s)` = arbitrary precision, **non-lossy**. The
     `*_scale` companion columns are GONE.

   **Consequence:** the Postgres column cannot truncate. The only places a `decimal.Decimal` can lose
   precision on the round-trip are the **serialization** seams, verified at HEAD:
   - The JSONB `body` column (`migrations/transaction/000000_create_transaction_table.up.sql:14`:
     `body JSONB NOT NULL`) — the marshalled `Transaction` payload.
   - The **msgpack `TransactionQueue`** struct serialized to RabbitMQ
     (`internal/adapters/postgres/transaction/transaction.go:411-438`): `Validate *mtransaction.Responses
     msgpack:"Validate"`, `Input *mtransaction.Transaction msgpack:"ParseDSL"`). This is the
     async/backup-recovery round-trip.
   - The Mongo metadata mirror (if it carries any decimal-derived value).

   The third-rail precision check (P4-T23) is therefore redirected at the **JSONB body / decimal-msgpack
   serialization / Mongo metadata mirror** round-trip, NOT the unbounded-DECIMAL column. And — critically
   — the `sum(legs) == fee total` balancing invariant does NOT hinge on resolving this boundary at all
   (see P4-T11): it is held by `applyFeeCorrection`'s residual-to-max reconciliation under exact
   `decimal.Equal`, independent of any precision rule.

---

## Tasks

### P4-T01 — Diff `pkg/transaction@v3.5.2` vs `pkg/mtransaction@HEAD` and repoint 18 imports

With fees observability-migrated in-repo (P2a), produce a field-by-field diff of the symbols fees
consumes (`Amount`, `FromTo`, `Transaction`, `Send`, `Source`, `Distribute`, `Responses`, `Share`,
`ValidateSendSourceAndDistribute`) between published `midaz/v3` v3.5.2 `pkg/transaction` and HEAD
`pkg/mtransaction`. Repoint all 18 import sites. **The 18 imports live in the SEPARATE plugin-fees
working tree** (at v3.5.2's published `pkg/transaction`), NOT in this midaz tree — `pkg/transaction`
does not exist at midaz HEAD (only `pkg/mtransaction` does). The grep gate therefore runs **inside the
fees working tree**, not the midaz tree, or it returns a vacuous empty result. Capture shape drift
(`FromTo.Route` "passive backward-compat" semantics, `RouteID *string`, `Amount.OverdraftAmount`,
`Responses.TransactionRouteID`) as explicit follow-ups; do not silently absorb behavioral change.
Diff+repoint only; the tree move is P4-T03.

- **Files:** (plugin-fees working tree)
  `internal/services/{calculate-fee,estimate-fee-calculation,payload_builder}.go`,
  `pkg/net/http/body_validator.go`, `pkg/fee/{distribute,calculate-fee}.go`, `pkg/model/fees.go`
  (production) + the test/mock sites surfaced by grep (incl. `pkg/model/billing_calculation_test.go`,
  `pkg/net/http/{with-body_test,validator_v10_migration_test}.go`);
  `docs/monorepo/plan/artifacts/P4-transaction-mtransaction-diff.md` (NEW, in midaz)
- **Depends on:** P1-T06, P2a-T17
- **Acceptance:** diff doc enumerates every consumed symbol with v3.5.2 vs HEAD shape; **`grep -rn
  'midaz/v3/pkg/transaction' .` run from the fees working-tree root returns empty** (this is the gate,
  not the enumerated file list); follow-ups for Route/RouteID and overdraft filed.
- **Tests:** `grep -rn 'midaz/v3/pkg/transaction' .` empty **in the fees working tree**; fees packages
  compile against in-tree `mtransaction` once moved (validated in P4-T04).
- **Effort:** S + 4–8h · **Risks:** R5

### P4-T02 — Re-point fee engine synthetic `Route` writes to `RouteID` (dual-write, mirror ledger)

`pkg/fee/distribute.go::updatedAmountsFromFee` builds synthetic keys
`credit->fee_sourceN->payer->routeId` and writes `fromTo.Route = route`. At HEAD `RouteID` is
canonical (`validate:"omitempty,uuid"`); `Route` is a passive backward-compat field. The ledger's
own op-builder (`transaction_create.go:1201-1202`) and `TransactionRevert` (L342-343/L364-365) write
**both** Route and RouteID (verified). Mirror that: write `fromTo.RouteID = &route` AND keep
`fromTo.Route = route` as the passive fallback — this is the ledger-wide convention (SS2 accepted
exception), not a fees-local shim, so it does not violate no-shims.

**Open behavioral conflict — RESOLVED-BEFORE-START via the P2a pre-move spike (P2a-T17, TF3):**
the synthetic route values come from `feeModel.GetRouteFrom()/GetRouteTo()`. `RouteID` carries
`validate:"omitempty,uuid"`. If those configured values are NOT UUID-shaped, writing them to `RouteID`
fails uuid validation on any route-validation-enabled path — a real behavioral conflict, not a
mechanical rename. **This is a genuine unknown** because the fee engine internals are not in this tree
yet (verified: no `applyFeeCorrection`/`getAssetPrecision`/`pkg/fee` under `components/ledger` or
`pkg` at HEAD). P2a-T17 MUST determine the route-value shape in the fees repo and decide the
resolution BEFORE this task starts: if non-UUID, P4-T02 expands from M to L to add a `name→ID`
resolution step at the seam (rippling into the P4-T06 resolver). Do NOT start the dual-write until the
spike has reported. Audit every fee-engine site reading/writing `.Route` (`distribute.go`,
`calculate-fee.go`, `filter.go::FindPackageToCalculateFee` which reads `cf.Transaction.Route`).

- **Files:** `pkg/fee/distribute.go`, `pkg/fee/filter.go`, `internal/services/calculate-fee.go`
  (fees working tree, pre-move)
- **Depends on:** P4-T01, P2a-T17
- **Acceptance:** the route-value shape is KNOWN (from P2a-T17) before any code change; synthetic
  routes flow through `RouteID` (canonical) with `Route` retained as the passive fallback exactly as
  the ledger writes it; if route source values are non-UUID, the `name→ID` resolution step is built
  here (not deferred); route-validation-enabled path still resolves fee legs' routes; no
  `validate:"omitempty,uuid"` failure on the route-validation path.
- **Tests:** unit test in `pkg/fee` asserting `updatedAmountsFromFee` populates `RouteID` (and Route
  fallback) for fee_source legs; existing `calculate-fee_test.go` route assertions updated and green;
  a test exercising the route-validation-enabled path with the actual route-value shape the spike found.
- **Effort:** M→L + 1–2d (L if non-UUID resolution needed) · **Risks:** R5

### P4-T03 — Move the fee engine + service + persistence tree into `components/ledger`

Git-based move (PD-3 fresh import, one `import plugin-fees` commit) of surviving fee code into
ledger. Target layout: fee engine → `components/ledger/pkg/fee/`; service/use-case layer →
`components/ledger/internal/services/fees/`; Mongo repos →
`components/ledger/internal/adapters/mongodb/fees/{pack,billing_package}/`; models fold into
`internal/services/fees/model` or `pkg/mmodel` (DO NOT create `/internal/domain`). Rewrite every
`plugins-fees/v3` self-import to the new in-tree path. Exclude `cmd/`, `internal/bootstrap/`,
`internal/m2m/`, `internal/metrics/`, `internal/cache/account_cache.go`,
`pkg/net/http/midaz-service.go` and standalone HTTP middleware/health/readiness — those are deleted
(P4-T08/T19), not moved. **Package cache:** default to DELETE under the no-shims aesthetic; KEEP only
if P4-T11/T16 hot-path profiling produces evidence it earns its keep (burden of proof is on keeping it,
not deleting it).

- **Files:** `components/ledger/pkg/fee/*.go` (NEW), `components/ledger/internal/services/fees/*.go`
  (NEW), `components/ledger/internal/adapters/mongodb/fees/{pack,billing_package}/*.go` (NEW)
- **Depends on:** P4-T01, P4-T02
- **Acceptance:** moved tree contains no `plugins-fees/v3` import; package boundaries respect the
  inward-dependency rule (engine/services do not import `internal/adapters/http/in`); no
  `/internal/domain` dir created; account cache not moved; package cache moved only with profiling
  justification (else deleted).
- **Tests:** `grep -rn 'plugins-fees/v3' components/ledger` empty; moved unit tests discoverable by
  `go test`.
- **Effort:** L + 2–3d · **Risks:** R5

### P4-T04 — `go mod tidy` + compile the embedded fee tree in the unified module

After the move, run `go mod tidy` on midaz's root module. **Resolve `lib-license-go/v2` now, not
later (decision pinned, not deferred):** the license middleware is deleted in P4-T19 (ledger already
enforces license), so the fees-carried `lib-license-go/v2 v2.3.4` direct require is DROPPED in this
phase. P7.md L169 expecting it present must reconcile to the post-collapse state — surface that to the
P7 reviser; in P4 the dep is removed. Drop fees' stale `lib-observability` v1.1.0-beta.5 (P2a already
pinned v1.0.1). Resolve transitive oddities (`lib-commons/v2` v2.9.1 indirect). Whole tree must
compile (`go build ./...`) and unit tests run (`make test-unit`). Confirm a clean `go mod download`
resolves all Lerian libs publicly with no private module / `github_token` — gate for the
Dockerfile/secret deletion in P4-T19.

- **Files:** `go.mod`, `go.sum`
- **Depends on:** P4-T03
- **Acceptance:** `go build ./...` green; `make test-unit` green for moved packages; `go mod
  download` succeeds with no `GOPRIVATE`/token; `go.mod` has no beta `lib-observability`; no external
  `pkg/transaction` dep on midaz; `lib-license-go/v2` removed (license middleware deleted in P4-T19).
- **Tests:** `go build ./...`; `make test-unit`; `GOFLAGS=-mod=readonly go mod download` in a clean env;
  `grep 'lib-license-go' go.mod` empty after teardown gate.
- **Effort:** M + 1d · **Risks:** R4, R5

### P4-T05 — Wire fee Mongo collections: connection, repos, 11 indexes, ModuleFees, WithMB, provisioning

Per PD-7, fee/billing packages live as new collections in ledger's existing MongoDB. (a) Add
`ModuleFees` constant to `pkg/constant/module.go` (coordinate the EXACT tenant-manager provisioning
name; mirror the CRM `crm→crm-api` footgun, see P4-T22). (b) Add an `initFees` bootstrap helper
(mirroring `initTransactionMongo`/`initOnboardingMongo`) that opens the fee Mongo manager and
constructs `pack.Repository` + `billing_package.Repository`. (c) Port the 11 compound indexes via the
moved `EnsureIndexes` functions (`pack/indexes.go` + `billing_package/indexes.go`) — these are
code-created `mongo.IndexModel`, NOT migration files, so no Postgres/golang-migrate change. Call
`EnsureIndexes` at startup in single-tenant mode (mirror metadata index ensure). (d) Register the fee
Mongo manager in the unified tenant middleware via `WithMB(mgr, constant.ModuleFees)` in
`buildUnifiedRouteSetup`. (e) Ensure per-tenant provisioning creates the fee module DB or MT requests
fail silently at DB resolution (R8).

**Scheduling risk (call out, not silently assume):** the MT-mode acceptance below depends on
out-of-repo tenant-manager provisioning (P4-T22 confirms this is cross-team). The in-repo task cannot
fully self-verify the MT path until that external provisioning exists; sequence the MT integration
assertion AFTER P4-T22 confirms the `ModuleFees` provisioning name, or stub the provisioning in the
testcontainer to keep the in-repo test deterministic.

- **Files:** `pkg/constant/module.go`, `components/ledger/internal/bootstrap/config.go`,
  `components/ledger/internal/bootstrap/config.mongo.fees.go` (NEW),
  `components/ledger/internal/adapters/mongodb/fees/{pack,billing_package}/indexes.go`
- **Depends on:** P4-T04
- **Acceptance:** `ModuleFees` constant exists; fee Mongo manager registered with `WithMB`; all 11
  indexes created on startup (verifiable on real Mongo); single-tenant and MT-enabled boot both
  resolve the fee module DB; the MT-path provisioning dependency on P4-T22 is documented.
- **Tests:** integration test (testcontainers Mongo) asserting all 11 named indexes exist on
  `pack` + `billing_package` after startup; MT-mode test that a tenant request reaches the fee repo
  without a DB-resolution failure (with provisioning stubbed if P4-T22 has not landed).
- **Effort:** L + 2–3d · **Risks:** R8, R16

### P4-T06 — Replace account/segment resolution HTTP reads with direct `query.UseCase` calls

Re-point `internal/adapters/midaz/account_resolver.go` and the segment-exemption path
(`pkg/fee/segment-resolution.go::isAccountExemptWithSegments`) from the outbound `MidazClient` to
ledger's in-process `query.UseCase`. Map: `GetAccountDetailsByAlias` →
`query.UseCase.GetAccountByAlias(ctx, org, ledger, portfolioID=nil, alias)`; `ListAccounts(filters)`
→ `query.UseCase.GetAllAccount(ctx, org, ledger, portfolioID, segmentID, filter http.QueryHeader)`.

**Correctness correction (pagination):** `GetAllAccount` takes a `QueryHeader` carrying pagination —
it does **NOT** return the full filtered set unconditionally. The resolver MUST paginate through
`GetAllAccount` with an explicit loop respecting the max-limit-100 constraint, or segment-membership
checks on segments with >100 accounts silently truncate and wrongly charge/exempt accounts. Replace
`SegmentContext.MidazClient` with a thin internal resolver interface backed by `query.UseCase`.
Preserve active-status filter and alias dedup. `GetAccountFromMidazByAlias` collapses into a
`GetAccountByAlias` not-found check.

- **Files:** `components/ledger/internal/services/fees/account_resolver.go` (moved),
  `components/ledger/pkg/fee/segment-resolution.go`, `components/ledger/pkg/fee/calculate-fee.go`
- **Depends on:** P4-T04
- **Acceptance:** no fee-side code references `http.MidazClient`; account existence, alias detail, and
  segment-membership exemption resolve via `query.UseCase`; active-status filtering and dedup
  preserved; **the resolver paginates `GetAllAccount` to full coverage with no silent truncation on
  segments larger than the page limit**.
- **Tests:** unit tests for the resolver using a fake `query.UseCase`; segment-exemption integration
  test proving an account in a waived segment is exempted and one outside is charged; **explicit test
  that a segment with >100 accounts is fully traversed (no truncation)**.
- **Effort:** L + 2–3d · **Risks:** R5

### P4-T07 — Replace transaction-count read with `query.UseCase.CountTransactionsByFilters`

Re-point `internal/adapters/midaz/transaction_counter.go::CountByRoute` from
`MidazClient.CountTransactionsByRoute` to
`query.UseCase.CountTransactionsByFilters(ctx, org, ledger, transaction.CountFilter)`. Map the fees
`CountParams` (route + window) onto ledger's `CountFilter`. This count feeds billing-package volume
calc (`billing-calculate-service.go`); if billing is deferred (see open items) the re-point still must
compile but can be exercised by billing tests only.

- **Files:** `components/ledger/internal/services/fees/transaction_counter.go` (moved),
  `components/ledger/internal/services/fees/billing-calculate-service.go`
- **Depends on:** P4-T04
- **Acceptance:** no fee-side code references `MidazClient.CountTransactionsByRoute`; count resolves
  via `CountTransactionsByFilters`; `CountParams`→`CountFilter` mapping is total or dropped fields are
  documented.
- **Tests:** unit test mapping `CountParams`→`CountFilter`; billing-calculate integration test
  asserting count matches a seeded transaction set.
- **Effort:** M + 1d · **Risks:** R5

### P4-T08 — Delete `internal/m2m`, the MidazService HTTP client, and the account cache

With all four outbound reads now in-process (P4-T06/T07), delete the outbound HTTP surface:
`internal/m2m/*` (AWS Secrets Manager per-tenant creds + `TenantAwareAuthGetter`),
`pkg/net/http/midaz-service.go` (+ `midaz-service_test.go`, `midaz_service_mock.go`),
`internal/cache/account_cache.go` (+ test). Remove dead config: `CLIENT_ID`, `CLIENT_SECRET`,
`M2M_TARGET_SERVICE`, `MIDAZ_*` URL vars, AWS-Secrets-Manager config.

**Hard grep-zero acceptance for the AWS surface (decoupled from `go mod tidy` outcome):** because
reporter brings its OWN `aws-sdk-go-v2` surface, the `go.mod` may still carry AWS deps after tidy for
reasons unrelated to fees. The acceptance is therefore a **grep-zero of any fees-path import of
`aws-sdk-go-v2/service/secretsmanager` and `aws-sdk-go-v2/config` in the fee tree** — NOT a `go.mod`
absence claim (which reporter muddies).

- **Files:** `internal/m2m/*` (delete), `pkg/net/http/midaz-service.go` (+ test, + mock) (delete),
  `internal/cache/account_cache.go` (+ test) (delete)
- **Depends on:** P4-T06, P4-T07
- **Acceptance:** no references to `m2m`, `MidazService`, `M2MCredentialProvider`, `account_cache`
  remain in the fee tree; unified module builds; **`grep -rn 'aws-sdk-go-v2/service/secretsmanager'`
  over the fees paths in `components/ledger` is empty** (independent of reporter's AWS deps in `go.mod`).
- **Tests:** `go build ./...` green; `grep -rn 'm2m|MidazService|M2MCredential|account_cache'` over
  fees paths in `components/ledger` empty; `grep -rn 'secretsmanager'` over the fee tree empty.
- **Effort:** M + 1d · **Risks:** R5

### P4-T09 — Construct `fees.UseCase` (holding a `query.UseCase` ref) and add `initFees` to bootstrap

Create a dedicated `fees.UseCase` (recommended architecture: NOT extending the command/query
god-structs). Fields: `PackageRepo`, `BillingPackageRepo`, `Query *query.UseCase` (for in-process
account/segment/count resolution), `DefaultCurrency`, optional package cache (default OFF per P4-T03).
Add an `initFees(opts, mongoMgr, queryUC)` bootstrap helper constructing repos + UC + `RouteRegistrar`,
mirroring the `initTransactionPostgres` extraction discipline so `InitServersWithOptions` stays
reviewable. Hold the UC on the handlers that need it: the fee CRUD handler (P4-T10) and the
`TransactionHandler` (for the create-fee seam, P4-T12) — pass `fees.UseCase` into the transaction
handler constructor.

- **Files:** `components/ledger/internal/services/fees/usecase.go` (NEW),
  `components/ledger/internal/bootstrap/config.go`,
  `components/ledger/internal/bootstrap/config.fees.go` (NEW)
- **Depends on:** P4-T05
- **Acceptance:** `fees.UseCase` constructed once in bootstrap and injected;
  `InitServersWithOptions` delegates fee init to `initFees` (no inline sprawl); command/query
  god-structs are NOT extended with fee fields.
- **Tests:** bootstrap wiring test (or `InitServersWithOptions` smoke test with Options) confirming a
  non-nil `fees.UseCase` reachable by the transaction handler and the fee CRUD handler.
- **Effort:** M + 1–2d · **Risks:** R8

### P4-T10 — Mount fee/billing-package CRUD as a `RouteRegistrar` with preserved auth namespace

Convert fees' standalone routes into a `RegisterFeesRoutesToApp` `RouteRegistrar` passed variadically
to `NewUnifiedServer` (zero `UnifiedServer` changes). Mount CRUD + estimate endpoints (the
`POST /fees` calculate endpoint is NOT mounted — fees now run in-process via the seam; keep
`POST /estimates` as dry-run): `/packages` (CRUD), `/estimates`, `/billing-packages` (CRUD),
`/billing/calculate`. Drop fees' own recover/telemetry/CORS/logging/health/readyz/limiter
(`UnifiedServer` provides them). Preserve the plugin-fees auth resource namespace strings in
`auth.Authorize(...)` (R9 — do not silently rename; tenant-manager RBAC policies key on them). Attach
`[authAssertion, tenantMiddleware.WithTenantDB]` for `ModuleFees` as `PostAuthMiddlewares`. DELETE the
standalone `POST /fees`. Verify no route-prefix collision with ledger's `/v1/organizations/...` tree.

- **Files:** `components/ledger/internal/adapters/http/in/fees_routes.go` (NEW),
  `components/ledger/internal/adapters/http/in/fees_package_handler.go` (NEW/moved),
  `components/ledger/internal/adapters/http/in/billing_package_handler.go`,
  `components/ledger/internal/bootstrap/config.go`
- **Depends on:** P4-T09
- **Acceptance:** fee CRUD/estimate/billing routes mounted on the unified `:3002` app; plugin-fees
  auth namespace preserved verbatim; standalone `POST /fees` removed; no duplicate middleware; no
  route collision.
- **Tests:** route-table test asserting the fee endpoints register; auth test asserting
  `Authorize('plugin-fees','packages',...)` is still applied; HTTP integration test hitting
  `POST /packages` then `GET /packages` on the unified server.
- **Effort:** L + 2–3d · **Risks:** R9

### P4-T11 — THIRD RAIL (RE-SCOPED): lock the `applyFeeCorrection` residual-to-max balancing invariant; delete the hard-coded precision table

**This task was re-scoped because its original premise was a phantom AND because the balancing
invariant was wrongly entangled with the precision table.** There is NO `Asset.Scale`/`Balance.Scale`
to look up (Correctness baseline #1). The ledger is arbitrary-precision decimal; the validator checks
balance by exact `decimal.Equal` with zero rounding. So the third rail here is NOT "make the fee engine
round to the same scale as the ledger" — there is no ledger scale.

**The balancing invariant is INDEPENDENT of the ISO-4217 table (TF2 re-scope).** The invariant
`sum(fee legs) == fee total` is held **exactly** by the fee engine's `applyFeeCorrection`: it computes
the decimal residual `delta = feeValue.Sub(newFeeTotalPaying)` and dumps it on the max account
(residual-to-max reconciliation), and `CalculateFee` adjusts `f.Transaction.Send.Value` to match. This
reconciliation holds `sum(legs) == fee total` under exact `decimal.Equal` **regardless of any per-leg
precision rule** — it depends on no scale and no table. **Deleting the ISO-4217 table cannot break the
third rail**; it only changes fee QUOTING granularity (the `RoundCeil`/`RoundFloor`/`Round` calls in
`updatedAmountsFromFee`), which is a product/presentation matter, not a balance matter.

> **FG1/TF3 caveat — verify before relying.** `applyFeeCorrection`, `getAssetPrecision`, and
> `CalculateFee`'s `Send.Value` mutation are NOT in this tree at HEAD (verified: no `pkg/fee` under
> `components/ledger`/`pkg`). The "residual-to-max holds sum==total exactly" claim is therefore
> **asserted, not yet verified in-tree**. P2a-T17 (TF3) MUST confirm-and-document, in the fees
> repo BEFORE the move, that (1) `applyFeeCorrection` holds `sum(legs) == fee total` under exact
> `decimal.Equal`, and (2) `CalculateFee`'s `Send.Value` mutation is the only top-level amount change.
> P4-T11 LOCKS the in-tree behavior against the spike's findings; it does not re-discover them.

The real correctness work here is twofold:

1. **Pre-rounding source of truth — delete the table, branch on the serialization boundary (P4-T23).**
   Today `getAssetPrecision(assetCode)` pre-rounds each leg against a hard-coded ISO-4217/crypto table
   (`pkg/fee/asset_precision.go`). Decide ONE source of truth and **delete the hard-coded table** — no
   known/unknown fallback (a scoped-fallback table is itself a shim under the no-shims mandate). The
   acceptance **branches explicitly on P4-T23's finding** (no unsatisfiable "a boundary must exist"
   criterion):
   - **IF P4-T23 finds a lossy serialization boundary** (JSONB/msgpack/Mongo round-trip truncates at
     some decimal place — see baseline #4): the source of truth is that boundary; the engine pre-rounds
     to it; `asset_precision.go` is deleted and replaced by the boundary-derived rule.
   - **IF P4-T23 finds NO lossy boundary** (serialization round-trips full precision): the source of
     truth is "emit whatever `applyFeeCorrection` produces, unrounded"; `asset_precision.go` is deleted
     with **NO replacement rounding**. The residual-to-max reconciliation alone guarantees exactness.
   After this task, `pkg/fee/asset_precision.go` and `getAssetPrecision` are deleted, not demoted to a
   fallback.

2. **Fee-asset vs transaction-asset denomination** is split into its own task (P4-T24) because it can
   produce `ErrTransactionValueMismatch` independently and deserves first-class treatment.

State explicitly in the chosen-rule doc that the `sum(legs) == fee total` invariant is independent of
any precision rule, so the third-rail balance guarantee does NOT hinge on resolving the boundary. Most
correctness-sensitive task in the phase — pair-review + exhaustive tests (P4-T16) mandatory.

- **Files:** `components/ledger/pkg/fee/asset_precision.go` (DELETE after the source of truth is
  chosen), `components/ledger/pkg/fee/distribute.go`, `components/ledger/pkg/fee/calculate-fee.go`
- **Depends on:** P4-T03, P4-T06, P4-T23, P2a-T17
- **Acceptance:** the `sum(fee legs) == fee total` invariant holds **exactly** (decimal) under
  `applyFeeCorrection`'s residual-to-max reconciliation for every fixture, **proven independent of the
  table** (the table-deleted suite still balances); the hard-coded ISO-4217 precision table is
  **deleted**, not retained as a fallback — a single source of truth governs leg precision; the
  source-of-truth choice **branches on P4-T23's finding** (boundary-rounded if lossy, unrounded if
  not); the chosen rule and its independence-from-balancing are documented.
- **Tests:** property/fuzz test (port `asset_precision_property_test.go` + `_fuzz_test.go`) asserting
  `sum(legs) == fee total` exactly across random splits **with the table deleted**; explicit cases for
  typical scales (JPY/0, BRL/2, BTC/8, ETH/18) all balancing under the exact-equality validator; a test
  proving (a) no leg exceeds the persistence serialization boundary IF P4-T23 found one, or (b) full
  precision round-trips IF it did not.
- **Effort:** XL + 3–5d · **Risks:** R1, R2

### P4-T12 — THIRD RAIL: fee seam in `executeCreateTransaction` via a SINGLE `validate` reassignment covering ALL downstream consumers

**Rewritten per TF1 — this is the #1 third-rail gap.** The earlier draft enumerated only 5 of the
11 verified `validate` consumers and proposed a per-site "replace the first validate at five
consumers" splice. That is WRONG: `validate` is `*mtransaction.Responses` (a pointer); EVERY
downstream site reads the SAME `validate` variable. The correct shape is a **single
`validate, err = ValidateSendSourceAndDistribute(...)` reassignment of the existing variable** after
`applyFees`, with NO per-site splicing — the reassignment by construction covers every downstream
reader. A per-site patch would leave the persistence path (BuildOperations, ProcessBalanceOperations
Lua, WriteTransaction) running on the PRE-fee `validate`: fee legs would mutate balances but vanish
from persisted operations, producing a permanent `sum != 0` in the operation table that the in-memory
validator never catches. That is the exact corruption the second validate exists to prevent.

**Verified insertion point (correctness-lens blocking issue, resolved):** the seam + second validate
MUST run **BETWEEN L1045 (first validate) and L1057 (`GetParsedLedgerSettings`)**. Reason verified at
HEAD:
- `PropagateRouteValidation(ctx, validate, ...)` at **L1068** MUTATES the `validate` pointee IN PLACE
  (verified `pkg/mtransaction/validations.go:379` takes `validate *Responses`), gated on
  `ledgerSettings.Accounting.ValidateRoutes` (L1067), which is loaded at L1057.
- `ValidateSendSourceAndDistribute` returns a FRESH `*Responses`. If the seam reassigned `validate`
  AFTER L1068, the route-propagation enrichment applied to the OLD pointer would be DISCARDED on a
  route-validation-enabled ledger — fee legs would reach `SendTransactionToRedisQueue`/`GetBalances`/
  `buildBalanceOperations` without propagated route metadata (silent regression on exactly the path
  P4-T02 wired RouteID for).
- Placing the seam BEFORE L1057 makes the single `validate` reassignment happen upstream of
  `GetParsedLedgerSettings`; PropagateRouteValidation at L1068 then decorates the POST-fee `validate`
  pointer, and ALL downstream consumers (which read that same pointer) see the fee-inclusive,
  route-propagated state. **L1068 is therefore a MUTATOR running against the already-reassigned
  post-fee validate — NOT a passive "reader to replace."** Do not list it in any "replace" enumeration.

Verified sequencing (HEAD line numbers):

1. Keep idempotency hash over raw input (L1020–1034, PD-6). Do NOT move it after fee mutation.
2. First `ValidateSendSourceAndDistribute` (L1045) → `validate`.
3. **[NEW]** `handler.applyFees(ctx, &transactionInput, &validate, params)` — a private method
   mirroring `enrichOverdraftOperations`: loads packages via `fees.UseCase`, drives the engine on the
   `validate` `Responses`, mutates `transactionInput.Send.*` (incl. `Send.Value`, which moves on
   deductible fees), returns mutated input or a business error. **On `isRevert=true`, `applyFees` is a
   no-op** — the reverse transaction already carries reversed fee legs from `TransactionRevert`
   (P4-T14); do not re-charge or inject.
4. **[NEW, R2]** RE-RUN `validate, err = ValidateSendSourceAndDistribute(ctx, transactionInput,
   transactionStatus)` on the MUTATED input — a `=` reassignment of the EXISTING `validate` variable,
   NOT a `:=` (which would fork the binding). This is the ONLY mechanic; there is no per-consumer splice.
5. Everything from L1057 onward then runs against the reassigned `validate` automatically. The
   complete verified consumer set that MUST read the post-fee `validate` (none may read a pre-fee
   snapshot): **`GetParsedLedgerSettings`-gated `PropagateRouteValidation` (L1068, mutator),
   `SendTransactionToRedisQueue` (L1076), `GetBalances(validate.Aliases)` (L1086),
   `rejectInternalScopeBalances` via `balances` derived from `validate.Aliases` (L1103),
   `buildBalanceOperations` (L1113), `enrichOverdraftOperations` (L1129), `ValidateAccountingRules`
   (L1140), `ProcessBalanceOperations{Validate: validate}` (L1151/1156 — the Lua balance mutation that
   MOVES MONEY), `BuildOperations` (L1210 — persisted Operation rows), `tran.Source/tran.Destination
   = filterCompanionAliases(validate.Sources/Destinations)` (L1230-1231), `WriteTransaction` (L1249).**

**Idempotency-slot release on fee failure (no-shims correctness hole):** the existing first-validate
failure path calls `handler.deleteIdempotencyKey(ctx, idempotencyResult.InternalKey)` (L1052). The
NEW `applyFees` error branch AND the NEW second-validate error branch MUST do the same before
returning, or the claimed idempotency slot is poisoned and the client's corrected retry replays the
cached failure forever.

Fees run BEFORE overdraft enrichment; the second validate restores the totals invariant overdraft
enrichment relies on. Fees applied once pre-queue so async is unaffected; the L1076 backup seed carries
the post-fee `validate` and post-fee `transactionInput` (proven structurally in P4-T27 and behaviorally
in P4-T25).

- **Files:** `components/ledger/internal/adapters/http/in/transaction_create.go`,
  `components/ledger/internal/adapters/http/in/transaction_fee_application.go` (NEW)
- **Depends on:** P4-T09, P4-T11
- **Acceptance:**
  - the seam + second validate are inserted BETWEEN L1045 and L1057 (so PropagateRouteValidation at
    L1068 decorates the post-fee validate);
  - the second validate is a SINGLE `validate, err = ValidateSendSourceAndDistribute(...)` `=`
    reassignment of the existing variable — no `:=`, no shadow copy, no per-consumer splice;
  - **grep-zero-pre-fee-reuse gate (executable, see P4-T27):** there is exactly one `validate :=`
    binding (the first validate) and exactly one `validate =` reassignment (the second), and NO
    additional `validate :=` reintroduces a fork between the seam and `WriteTransaction` (L1249);
  - ALL 11 verified consumers above read the post-fee `validate` (the reassignment makes this
    automatic; the acceptance enumerates them so a reviewer can confirm the blast radius reaches
    `WriteTransaction`/`ProcessBalanceOperations`, not just L1128);
  - **on a `ValidateRoutes`-enabled ledger, the fee legs carry propagated route metadata into
    `buildBalanceOperations`** (proving L1068 ran against the post-fee validate);
  - `validate.Aliases` after the second validate includes the fee credit-account and fee_source
    aliases; the backup-seed payload round-trips to a fee-inclusive transaction;
  - idempotency hash remains over raw input; **both the `applyFees` error branch and the
    second-validate error branch call `deleteIdempotencyKey` before returning**;
  - overdraft enrichment runs after fees; `isRevert` path does not re-charge or inject fees.
- **Tests:** integration: create a transaction matching a fee package → response includes fee legs,
  transaction balances, `metadata.packageAppliedID` set; replay with same idempotency key returns the
  same fee-inclusive transaction; a no-package case is unchanged; **a fee-engine business error
  releases the idempotency slot so a corrected retry succeeds**; **after the second validate,
  `GetBalances` is invoked with the fee_source/fee credit aliases present**; **a route-validation-enabled
  ledger carries propagated route metadata onto the fee legs into `buildBalanceOperations`**; the
  structural grep-zero gate (P4-T27) is green.
- **Effort:** XL + 3–5d · **Risks:** R1, R2, R5

### P4-T13 — Apply the second seam in the commit/cancel state handler (no double-charge)

The pending commit/cancel path in `transaction_state_handlers.go` (L407–433) has its own
`ApplyDefaultBalanceKeys` + `ValidateSendSourceAndDistribute` (L433) over `tran.Body`. Fees were
already applied when the pending transaction was created (P4-T12), so the persisted `tran.Body`
already contains fee legs — DO NOT re-apply fees on commit. Confirm and lock: the commit/cancel path
must NOT call `applyFees` again (double-charge), and the validate at L433 operates on the
already-fee-inclusive `tran.Body`. Add a guard/test proving commit of a pending fee-bearing
transaction does not add fees a second time. Cancel triggers the PD-5 refund path (P4-T14), not a
re-charge.

- **Files:** `components/ledger/internal/adapters/http/in/transaction_state_handlers.go`
- **Depends on:** P4-T12
- **Acceptance:** commit of a pending fee-bearing transaction produces no additional fee legs; the
  validate at L433 runs over fee-inclusive `tran.Body`; cancel routes to refund (P4-T14), not
  re-charge.
- **Tests:** integration: create PENDING with fees → commit → committed legs == pending legs (no
  double fee) + balance check (explicit leg-count parity assertion).
- **Effort:** M + 1–2d · **Risks:** R2

### P4-T14 — PD-5 (THIRD RAIL): VERIFY fee refund on revert AND pending-cancel — do NOT rebuild legs (CG1/CG2)

**This task is VERIFY-not-REBUILD because the original design would double-reverse fees (CG2).**
Verified at `transaction.go:293`: `TransactionRevert()` reconstructs the reverse transaction from ALL
persisted parent operations (CREDIT→from, DEBIT→to, swapping sides; skipping `OverdraftBalanceKey`
companions at L325/L347) and sets the reverse `Send.Value = *t.Amount` (L386). Because P4-T12 persists
fee legs as real operations on the parent, `TransactionRevert()` **already reverses the fee legs
automatically**. Manually injecting refund legs on top (the earlier draft's plan) would reverse the fee
legs twice → over-refund → `ErrTransactionValueMismatch` under the exact-equality validator → a
self-inflicted third-rail violation. **`TransactionRevert` needs NO change; it is already fee-aware by
construction.**

**Deductible-fee revert is the load-bearing case (CG1).** `tran.Amount` is derived from the MUTATED
`Send.Value` (L1188 region in `transaction_create.go`, post-fee) for DEDUCTIBLE fees. `TransactionRevert`
reconstructs froms/tos from `t.Operations` and sets reverse `Send.Value = *t.Amount` (L386). The revert
therefore balances ONLY IF `sum(reconstructed legs) == persisted t.Amount`. A NON-deductible-fee revert
can pass while a deductible-fee revert breaks, because deductible fees move `Send.Value` itself. The
P4-T16 proof MUST pin the DEDUCTIBLE-fee revert case specifically.

Corrected scope — VERIFY, do not REBUILD:

(a) **Revert.** `RevertTransaction` builds `tran.TransactionRevert()` then funnels through
`createRevertTransaction → executeCreateTransaction(..., isRevert=true)` (which sets
`action = constant.ActionRevert`, L1072-1074). The `isRevert` branch in P4-T12 makes `applyFees` a
**no-op** and passes `transactionReverted` through UNCHANGED. The work here is proving (integration)
that `transactionReverted` already carries the reversed fee legs and `sum(all legs incl. reversed fees)
== persisted t.Amount` and nets to zero under ledger's own machinery — **for the DEDUCTIBLE-fee case
specifically**. Confirm the reversed fee legs carry the correct `Route`/`RouteID` plumbing
(`TransactionRevert` copies `op.Route`+`op.RouteID` from the persisted fee ops, L342-343/L364-365 —
verify this matches what P4-T02 wrote).

(b) **Pending-cancel.** The cancel path at `transaction_state_handlers.go` drops the distribute side
when `transactionStatus == constant.CANCELED` (L417-419) and releases `OnHold` mechanically, reversing
ALL held legs including fees with NO new code. Note the cancel-path also runs the cancel-specific
overdraft enrichment (L479-481): a fee leg that drove an overdraft on the pending tx must release its
companion correctly on cancel — exercise this. Verify and add an explicit balance assertion that holds
are released incl. fees and `sum == 0`.

**No `pkg/mtransaction/` change is in scope** (the earlier draft listed it "if `TransactionRevert`
needs fee-leg awareness" — it does not).

- **Files:** `components/ledger/internal/adapters/http/in/transaction_state_handlers.go`,
  `components/ledger/internal/adapters/http/in/transaction_fee_application.go` (the `isRevert` no-op
  branch only)
- **Depends on:** P4-T12, P4-T13
- **Acceptance:**
  - revert of a fee-bearing transaction produces a reverse transaction that refunds the fee legs via
    the EXISTING `TransactionRevert` machinery (NO manually injected legs) and balances (`sum == 0`),
    **proven for the DEDUCTIBLE-fee case** where `sum(reconstructed legs) == persisted t.Amount`;
  - **explicit no-double-reverse assertion:** the reverse transaction does NOT contain doubled fee legs
    (the leg count and per-account net match a single reversal, not two);
  - **explicit applyFees-skipped-on-isRevert assertion:** a spy/counter on the fee engine proves
    `applyFees` was short-circuited when `isRevert=true` (output balance can pass for the wrong reason
    if reversed legs happen to net to the persisted amount — so the no-op must be proven structurally,
    not inferred from balance);
  - pending-cancel of a fee-bearing transaction reverses all legs incl. fees and balances (`sum == 0`),
    including the cancel-path overdraft-companion release (L479-481);
  - no fee is charged on the reversal itself; reversed fee legs carry the Route/RouteID plumbing P4-T02
    established.
- **Tests:** integration: create DEDUCTIBLE-fee-bearing tx → revert → assert reverse tx contains the
  refund legs AND `sum(all legs incl. reversed fees) == 0` AND payer net effect is zero AND
  `sum(reconstructed legs) == persisted t.Amount`; integration: create PENDING fee-bearing tx (incl. an
  overdraft-driving fee) → cancel → assert holds released incl. fees and companion released, `sum == 0`;
  **a regression assertion that the reverse transaction does NOT contain doubled fee legs**; **a spy
  assertion that `applyFees` is not invoked on the `isRevert=true` path**.
- **Effort:** L + 2–3d (verification, not rebuild) · **Risks:** R18, R1

### P4-T15 — Lock idempotency-hash-over-raw-payload semantics + document the assumption

The idempotency hash is computed over the raw user input BEFORE fee mutation
(`transaction_create.go` L1020–1034). This is correct: fees are deterministic given the same input +
same packages. Add a test proving two identical requests with the same idempotency key return the
same fee-inclusive transaction (replay path). Document the package-config-churn assumption: if package
config changes between two identical-key requests, the replay returns the FIRST result (idempotency
wins over recomputation). **Also document the sharper near-miss case (deliberate decision, not
oversight):** a NON-replay request (different idempotency key, same body) issued AFTER the package is
DELETED recomputes against the now-deleted package and produces a different fee outcome — this is
correct-by-design (the hash keys on raw body, not package version), but call it out in one sentence so
it reads as a deliberate decision. Add a code comment at the hash site stating the hash is intentionally
over the pre-fee payload and why. Do NOT add package-version to the key (out of scope; flagged as a
deliberate accepted limitation).

- **Files:** `components/ledger/internal/adapters/http/in/transaction_create.go`,
  `components/ledger/internal/adapters/http/in/transaction_fee_idempotency_test.go` (NEW)
- **Depends on:** P4-T12
- **Acceptance:** comment documents the raw-payload hash decision; replay returns the original
  fee-inclusive transaction; the package-config-churn limitation AND the deleted-package near-miss are
  both documented (comment + plan open_item).
- **Tests:** integration: same idempotency key + same body twice → identical fee-inclusive response,
  `IdempotencyReplayed=true` on the second.
- **Effort:** S + 4–8h · **Risks:** R2

### P4-T16 — Integration test suite: prove fee + reversal balance under ledger's validator (per-rule AND per-MODE)

Correctness gate for the third rail. Build a real-dependency (testcontainers: Postgres + Mongo +
Redis) integration suite exercising the full transaction-create path with fee packages seeded in the
new Mongo collections. Mandatory proof classes:

1. `sum(fee legs) == fee total` (exact decimal) for non-deductible, deductible, flat, percentual, and
   maxBetweenTypes RULE types (R1).
2. The whole fee-augmented transaction balances under ledger's own `ValidateSendSourceAndDistribute` +
   balance machinery (R2).
3. Proportional multi-account splits with repeating decimals (1/3) reconcile and the residual lands on
   the max account (R1) — proven with the ISO-4217 table DELETED (P4-T11).
4. Segment + alias exemptions resolve via the in-process query layer; **a segment >100 accounts is
   fully traversed (no pagination truncation, P4-T06)**.
5. **Revert refund balances (`sum == 0`) for the DEDUCTIBLE-fee case specifically (CG1)** —
   `sum(reconstructed legs) == persisted t.Amount` — AND does NOT double-reverse fee legs, AND the
   `applyFees`-skipped-on-isRevert spy holds (P4-T14/CG2); pending-cancel refund balances (`sum == 0`)
   incl. the overdraft-companion release (PD-5).
6. Assets at typical precisions (JPY, BRL, BTC) each balance under exact-equality.
7. **Fee-asset != Send.Asset denomination case balances or is correctly rejected (P4-T24)** — a fee
   default currency different from the transaction's `Send.Asset` does not silently produce a
   multi-asset imbalance.
8. **Overdraft-from-fee:** a fee that pushes the payer's source into overdraft produces a correct
   `#overdraft` companion leg via `enrichOverdraftOperations` (which runs AFTER fees on the second
   validate) AND the whole transaction still balances.
9. **Per-MODE fee tests (CGap1):** all five transaction creation modes — **JSON, DSL, inflow, outflow,
   annotation** — exercised through the seam, since each builds `transactionInput.Send.*` differently
   (`applyFees` mutates `Send.Value`/`Source.From`/`Distribute.To` which the modes populate via distinct
   paths) while all funnel `executeCreateTransaction`. Required assertions:
   - **annotation (NOTED status):** emits NO fee legs and `applyFees` does not mutate `Send.Value`
     (annotation is one-sided / no real balance movement — charging it would violate its invariants);
   - **inflow / outflow:** the fee charges the CORRECT side given the asymmetric source/distribute
     construction (an inflow with no `Source.From` has nowhere to charge a deductible fee — assert the
     defined behavior: reject with a clean business error or charge the configured side);
   - **JSON / DSL:** identical fee legs and identical balance result for the same logical transaction
     (parsed `Send` vs literal `Send` must not diverge).
10. **Fee-leg op-shape reversibility (missing-task, correctness lens):** assert the persisted fee-leg
    operations carry `Type ∈ {CREDIT, DEBIT}` and `BalanceKey != OverdraftBalanceKey`, so
    `TransactionRevert`'s exact filter (which SKIPS `OverdraftBalanceKey` ops at L325/L347) actually
    picks them up on revert. A fee leg persisted with an overdraft-like balance key would be silently
    skipped on revert → under-refund → third-rail break on revert only.
11. **Streaming wire-contract guard (CRITICAL event):** After a fee-bearing transaction create, assert the
    emitted `transaction_lifecycle` CloudEvents payload (built via `SendTransactionEvents` from the post-fee
    `tran`) carries the fee legs in `Operations[]` and the post-fee `tran.Amount` — asserted against an
    event-spy/fake emitter. Guards the CRITICAL streaming wire contract so a future pre-fee-snapshot refactor
    cannot silently ship wrong financial events.

Run under `make test-integration`.

- **Files:** `components/ledger/internal/adapters/http/in/transaction_fee_integration_test.go` (NEW),
  `components/ledger/internal/services/fees/testdata/` (NEW)
- **Depends on:** P4-T11, P4-T12, P4-T14, P4-T24
- **Acceptance:** all eleven proof classes pass against real Postgres+Mongo+Redis; suite is part of
  `make test-integration`; coverage of the moved fee code meets the 85% gate (R11).
- **Tests:** `make test-integration` green including the new suite; explicit assertions on leg
  `sum == 0` and `fee total == sum(fee legs)`; explicit deductible-revert `sum == persisted t.Amount`;
  explicit no-double-reverse + applyFees-spy assertions; explicit overdraft-from-fee companion-leg
  assertion; explicit per-mode assertions incl. annotation-emits-no-fee; explicit fee-leg op-shape
  (CREDIT/DEBIT, non-overdraft BalanceKey) assertion; explicit streaming wire-contract assertion that the
  emitted `transaction_lifecycle` payload carries the fee legs in `Operations[]` and the post-fee
  `tran.Amount` against an event-spy/fake emitter.
- **Effort:** XL + 6–8d (raised from 4–6d for per-mode + op-shape + spike-lock classes) · **Risks:** R1, R2, R18, R11

### P4-T17 — Fold fee/billing endpoints into unified ledger Swagger

Fees' standalone swagger annotations must merge into ledger's `make generate-docs` output, or the
combined OpenAPI loses the fee CRUD/estimate/billing endpoints. Port swagger annotations onto the
moved handlers (P4-T10) and regenerate. Drop fees' standalone `swagger.go` serving (`UnifiedServer`
owns `/swagger`).

- **Files:** `components/ledger/internal/adapters/http/in/fees_package_handler.go`,
  `components/ledger/internal/adapters/http/in/billing_package_handler.go`, ledger swagger docs output
- **Depends on:** P4-T10
- **Acceptance:** `make generate-docs` produces a unified spec including `/v1/packages`,
  `/v1/estimates`, `/v1/billing-packages`, `/v1/billing/calculate`; standalone fees swagger serving
  removed.
- **Tests:** `make generate-docs` succeeds; generated spec contains the fee paths (grep the output).
- **Effort:** M + 1d · **Risks:** R25

### P4-T18 — Relicense moved fee source headers to Elastic License 2.0

Apply EL2.0 headers to every moved `.go` file (`scripts/check-license-header.sh` enforces this).
plugin-fees source currently lacks the EL2.0 header (verified: `pkg/fee/asset_precision.go` has only
a package doc comment). Add the canonical midaz EL2.0 header to all moved files and run the
license-header check.

- **Files:** `components/ledger/pkg/fee/*` (moved), `components/ledger/internal/services/fees/*`
  (moved), `components/ledger/internal/adapters/mongodb/fees/*` (moved)
- **Depends on:** P4-T04
- **Acceptance:** `scripts/check-license-header.sh` passes for the moved tree; every moved `.go` file
  carries the EL2.0 header.
- **Tests:** `bash scripts/check-license-header.sh` (or the make target wrapping it) exits 0.
- **Effort:** S + 2–4h · **Risks:** R23

### P4-T19 — Teardown: delete fees code (reversible) and deploy units (gated) — re-gated on the unified third-rail proof

**Re-gated per TF4.** Teardown is a one-way door: it deletes the standalone fees binary/Dockerfile/
compose/CI — the ONLY rollback target. It must NOT proceed until the **unified cross-phase third-rail
proof passes (P7-T18 — the fee-on-revert balance integration proof gate, P7.md L52)**, not merely the
in-phase P4-T16. To make the door survivable, this task is SPLIT into a reversible step and an
irreversible step, with the irreversible step gated on the cross-phase proof AND the gitops lockstep:

**P4-T19a (reversible — delete code):** delete the standalone composition/runtime code that does not
survive the collapse: `cmd/app/main.go`, `internal/bootstrap/*` (standalone composition root),
`internal/metrics/*`, `internal/http/in/{health,readiness,middlewares,security-headers,routes}.go`
(`UnifiedServer` provides these), `pkg/net/http/{middleware-tracing,withRecover,httputils,errors}.go`
standalone HTTP plumbing. This is recoverable via a documented revert commit (see P4-T26).

**P4-T19b (irreversible — delete deploy units):** delete `Dockerfile`, `docker-compose.yml`,
`.github/workflows/*` and remove the `github_token` BuildKit secret + `.secrets/` + `go_private_modules`
(gated by P4-T04's clean `go mod download` — fees' only private import was `midaz/v3`, which vanished).
This step runs ONLY after BOTH (1) P7-T18 is green and (2) the gitops/Helm/APIDog repoint (P4-T21) has
landed, so a failed cutover has a standing fallback service to route back to.

First-class deletion task, not cleanup.

- **Files:** `cmd/app/main.go` (delete), `internal/bootstrap/*` (delete), `internal/metrics/*`
  (delete), `internal/http/in/{health,readiness,middlewares,security-headers,routes}.go` (delete),
  `Dockerfile` (delete), `docker-compose.yml` (delete), `.github/workflows/*` (delete)
- **Depends on:** P4-T16, P4-T17, P4-T18, **P7-T18** (cross-phase fee-on-revert proof gate), **P4-T21**
  (gitops/Helm/APIDog lockstep, precondition of P4-T19b only), **P4-T26** (rollback runway documented)
- **Acceptance:** no fees standalone binary/Dockerfile/compose/CI survives after P4-T19b; the unified
  ledger image builds and runs with fees embedded; no `github_token`/`.secrets/` machinery in the fees
  build path; net deploy units reflect fees folded into `ledger:3002`; **P4-T19b did not run until
  P7-T18 was green AND P4-T21 had landed** (the irreversible deploy-unit delete strictly follows the
  cross-phase proof and the gitops repoint).
- **Tests:** `make up` brings up the stack without a plugin-fees container; `ledger:3002` serves the
  fee CRUD routes; `docker build` of ledger succeeds without BuildKit secrets; a CI/checklist assertion
  that P7-T18 and P4-T21 are both green before P4-T19b's commit.
- **Effort:** M + 1–2d · **Risks:** R23, R12

### P4-T20 — Merge fee-specific config into ledger Config (namespaced) + 3-way env diff

Only fee-specific config survives the bootstrap deletion: `DEFAULT_CURRENCY`, package cache
toggles/TTLs (only if the cache survived P4-T03), and (if the fee Mongo is a separate logical DB) its
connection vars — namespace them `FEES_*` / `MONGO_FEES_*` in ledger's Config struct +
`applyConfigDefaults`. Do a careful 3-way diff of fees vs ledger `.env.example` to catch collisions
(`SERVER_PORT`, `MONGO_*`, `DB_*`, `APPLICATION_NAME`) — a silent missing var passes CI build but
fails at runtime (R17). `APPLICATION_NAME` stays `ledger` (the auth namespace `plugin-fees` is
per-route, not the app name). Update `.env.example`.

- **Files:** `components/ledger/internal/bootstrap/config.go`, `components/ledger/.env.example`
- **Depends on:** P4-T05
- **Acceptance:** all surviving fee config is `FEES_`/`MONGO_FEES_`-namespaced with defaults;
  `.env.example` updated; no env-var collision with ledger's existing surface; `DEFAULT_CURRENCY`
  default documented.
- **Tests:** a config-load test asserting fee defaults applied when env unset; startup with the
  merged `.env.example` succeeds.
- **Effort:** M + 1d · **Risks:** R17

### P4-T21 — Out-of-repo lockstep: remove plugin-fees image from Helm/gitops/APIDog

Coordination task (cross-team blast radius, R12). The plugin-fees image is DELETED, not renamed — its
`filter_paths`, `helm_values_key_mappings`, and midaz-firmino-gitops `yaml_key_mappings` entries must
be removed, with ArgoCD/Helm + APIDog e2e updated in lockstep. A co-located/embedded component without
chart updates builds but the old fees image deploy reference breaks ArgoCD sync. This task is a
**precondition of the irreversible deploy-unit delete (P4-T19b)** — it must land BEFORE the standalone
image is removed, so the embedded path serves cluster traffic before the fallback disappears. Confirm
ownership and sequencing with the chart/gitops owners.

**Owner-unavailable / chart-rejected fallback (CGap2):** if the Helm/gitops/APIDog owner does not sign
off in lockstep, or the chart change is rejected, the documented stall path is: HOLD at P4-T19a (code
deleted, deploy units intact, standalone image still deployable), keep the standalone fees image
publishing from its archived read-only origin's last good build, and do NOT execute P4-T19b. The
embedded ledger continues to serve fee CRUD in-process; the standalone image remains the deploy unit
until the chart lands. This is the likeliest real-world stall and must not block the rest of P4.

- **Files:** (out-of-repo) Helm chart midaz, midaz-firmino-gitops, APIDog e2e config
- **Depends on:** P4-T19a
- **Acceptance:** plugin-fees removed from Helm values + gitops yaml mappings; ArgoCD sync green
  post-removal; APIDog suite no longer targets the standalone fees service; fee endpoints tested
  against `ledger:3002`; the owner-unavailable/chart-rejected fallback path is documented and is the
  defined behavior on non-sign-off.
- **Tests:** ArgoCD dry-run/diff shows no dangling fees image; APIDog run against the unified ledger
  passes the fee endpoint cases.
- **Effort:** M + 1–2d (cross-team) · **Risks:** R12

### P4-T22 — Reconcile fees auth/RBAC namespace into the merged binary

Ledger authorizes under `midaz/routing`; fees under `plugin-fees` (resources
`packages/fees/estimates/billing-packages/billing-calculate`). Route merge != authz merge (R9).
Preserve the plugin-fees namespace strings initially (tenant-manager RBAC policies key on them); do
not silently rename or authorization breaks. Confirm with the auth/tenant-manager owners that the
plugin-fees application/resource policies remain provisioned for tenants now served by the ledger
binary. Confirm the `ModuleFees` tenant-manager module name chosen in P4-T05 matches what provisioning
expects (the CRM `crm→crm-api` footgun precedent). Document the namespace map.

- **Files:** `components/ledger/internal/adapters/http/in/fees_routes.go`, (out-of-repo)
  tenant-manager RBAC policy config
- **Depends on:** P4-T10
- **Acceptance:** plugin-fees auth namespace preserved verbatim on the merged routes; tenant-manager
  confirms the policies + `ModuleFees` provisioning name; namespace map documented.
- **Tests:** auth integration test: a token with `plugin-fees:packages:post` can create a package on
  `ledger:3002`; a token without it is denied; MT tenant resolves the fee module DB.
- **Effort:** M + 1d (partly cross-team) · **Risks:** R9, R8

### P4-T23 — Locate the SERIALIZATION decimal-precision boundary (NOT the unbounded DECIMAL column)

**Refocused per TF2.** The Postgres `amount`/`available` columns are UNBOUNDED `DECIMAL` (verified:
migrations `000005`/`000006` `ALTER ... TYPE DECIMAL` and `DROP COLUMN *_scale` — see baseline #4).
A bare `DECIMAL` column cannot truncate, so the earlier draft's hunt for `NUMERIC(p,s)` column
truncation searches a boundary that **does not exist**. The REAL serialization seams where a
`decimal.Decimal` can lose precision are, verified at HEAD:

1. **The JSONB `body` column** (`migrations/transaction/000000_create_transaction_table.up.sql:14`:
   `body JSONB NOT NULL`) — the marshalled `Transaction` payload.
2. **The msgpack `TransactionQueue`** struct serialized to RabbitMQ
   (`internal/adapters/postgres/transaction/transaction.go:411-438`): `Validate *mtransaction.Responses
   msgpack:"Validate"`, `Input *mtransaction.Transaction msgpack:"ParseDSL"`). This is the
   async/backup-recovery round-trip P4-T25 worries about.
3. **The Mongo metadata mirror**, if it carries any decimal-derived value.

Find the boundary precisely on each of these three seams: round-trip a high-precision
`decimal.Decimal` (e.g. a repeating-decimal residual from a 1/3 split) through (a) JSONB marshal/unmarshal
of `body`, (b) the actual `TransactionQueue` msgpack encode/decode, (c) the Mongo metadata write/read,
asserting `decimal.Equal` on the way back. If any seam normalizes/truncates, document the exact decimal
place it tolerates — that is the boundary P4-T11 branches on. If all three round-trip full precision,
document "no lossy boundary" and P4-T11 emits unrounded (the residual-to-max reconciliation alone
guarantees exactness). This is the genuine third-rail check P4-T11 was groping toward — aimed at the
real target this time.

- **Files:** `components/ledger/internal/adapters/postgres/transaction/transaction.go` (the
  `TransactionQueue` msgpack struct + the JSONB `body` marshal path),
  `components/ledger/internal/adapters/mongodb/*` (metadata mirror),
  `docs/monorepo/plan/artifacts/P4-decimal-precision-boundary.md` (NEW)
- **Depends on:** P4-T04
- **Acceptance:** the serialization precision boundary is documented for each of the three seams (JSONB
  `body`, msgpack `TransactionQueue`, Mongo metadata mirror) — either the exact decimal place each
  tolerates, OR an explicit "round-trips full precision, no lossy boundary" finding; the result is fed
  into P4-T11 as the branch condition (boundary-round vs emit-unrounded); the Postgres `amount` column
  is explicitly noted as unbounded/non-lossy and OUT of scope.
- **Tests:** a round-trip test through the actual `TransactionQueue` msgpack encode/decode asserting
  `decimal.Equal`; a JSONB `body` marshal/unmarshal round-trip test asserting `decimal.Equal`; a Mongo
  metadata round-trip test if metadata carries decimal-derived values; the boundary value (or the
  "no boundary" finding) is referenced by the P4-T11 precision rule.
- **Effort:** M + 1–2d · **Risks:** R1, R2

### P4-T24 — Define and enforce the fee-asset vs transaction-asset denomination rule

`calculate-fee.go` computes fee amounts in `DefaultCurrency` (often BRL) but the ledger validator
requires a single `transaction.Send.Asset` and aggregates totals per-asset. If a transaction is in USD
but fees default to BRL, the fee legs carry a different `Asset` than the transaction → the validator's
per-asset total logic rejects (`ErrTransactionValueMismatch`) or silently mis-aggregates a multi-asset
imbalance. Define the explicit rule — **fees are always denominated in the transaction's `Send.Asset`**
(the safe default) — and enforce it: the fee engine must read `Send.Asset` for fee leg denomination,
not a global default, OR explicitly reject a transaction whose asset has no configured fee package in
that asset. This is a first-class correctness item because it can break the third rail independently of
leg rounding.

- **Files:** `components/ledger/pkg/fee/calculate-fee.go`, `components/ledger/pkg/fee/distribute.go`,
  `components/ledger/internal/services/fees/usecase.go`
- **Depends on:** P4-T11
- **Acceptance:** fee legs are denominated in the transaction's `Send.Asset` (rule documented); a
  transaction whose `Send.Asset` differs from the fee `DefaultCurrency` still balances (single-asset)
  OR is rejected with a clear business error rather than silently producing a multi-asset imbalance;
  no path emits a fee leg in an asset different from `Send.Asset`.
- **Tests:** unit: fee on a USD transaction produces USD-denominated legs (not BRL); integration: a
  USD transaction with a configured fee balances under the validator; integration: the
  no-fee-package-in-asset case returns a clean business error, not `ErrTransactionValueMismatch`.
- **Effort:** M + 1–2d · **Risks:** R1, R2

### P4-T25 — HARD GATE: prove the async (RABBITMQ_TRANSACTION_ASYNC) path persists the fee-inclusive transaction

**Hardened per SP3/TF4 — this is a HARD gate.** P4-T12 asserts "fees applied once pre-queue so async is
unaffected" and "backup seed carries fee legs." The risk is concrete and verified: `SendTransactionToRedisQueue`
(L1076) seeds the crash-recovery backup BEFORE `GetBalances` (L1086) and runs with nil balances; fees
mutate `Send.Value` (deductible) between the first validate (L1045) and the second validate. A worker
(or crash-recovery replay) reconstructing from a PRE-fee payload would silently under/over-charge.

This task has TWO parts:

(a) **Structural assertion (the HARD gate, delegated to P4-T27):** a compile-time / test-time positional
check that the fee seam mutation (`applyFees` + the second `ValidateSendSourceAndDistribute`) provably
PRECEDES the `SendTransactionToRedisQueue` call at L1076 in `executeCreateTransaction`. A behavioral
async test is a flaky guardian of ordering; a future refactor that reorders the seam after the seed must
be caught at lint/test time, NOT in a non-deterministic async integration run. This structural assertion
is the gate; the behavioral test below is corroboration, not the primary guarantee.

(b) **Behavioral proof:** end-to-end with `RABBITMQ_TRANSACTION_ASYNC=true`, prove the queued/backup
payload is the fee-inclusive one and the worker re-validates and persists fee legs correctly, including a
simulated crash-recovery replay reconstructing from the backup seed.

- **Files:** `components/ledger/internal/adapters/http/in/transaction_fee_async_integration_test.go`
  (NEW)
- **Depends on:** P4-T12, P4-T16, P4-T27
- **Acceptance:**
  - **(HARD) the structural assertion (P4-T27) proving `applyFees` + second validate precede the L1076
    `SendTransactionToRedisQueue` seed is green** — a reorder fails the build/test, not the async run;
  - with async enabled, a fee-bearing transaction queued via Redis/RabbitMQ is persisted with the fee
    legs and balances under the validator;
  - the backup-seed payload reconstructs to the fee-inclusive transaction (not the pre-fee payload) on a
    simulated crash-recovery replay.
- **Tests:** the P4-T27 structural seam-precedence assertion; integration (testcontainers + async flag):
  create fee-bearing tx in async mode → assert persisted operations include fee legs AND `sum == 0`; a
  backup-recovery reconstruction test asserting the recovered transaction is fee-inclusive.
- **Effort:** L + 2–3d · **Risks:** R2, R1

### P4-T26 — NEW: Teardown rollback runway — keep the standalone fees deploy unit recoverable

**Added per TF4 (missing-task, all three lenses).** The teardown (P4-T19) deletes the only rollback
target. There must be a documented, tested recovery path so a failed embedded-fee cutover after teardown
is survivable. Mirror the P5-T16 abort/rollback discipline (keep standalone services intact until the
unified-suite balance proof is green).

Deliverable: a documented rollback procedure that (a) identifies the exact revert commit for P4-T19a
(code deletion) and the deploy-unit deletion in P4-T19b, (b) keeps the standalone fees image buildable
from the archived read-only origin's last good state (PD-3 origins archived read-only — the fees image
can be rebuilt from the archived tag without the live repo), and (c) defines the trigger conditions:
if P7-T18 (cross-phase third-rail proof) goes red after P4-T19a, or if the embedded fee seam regresses in
staging, HOLD at P4-T19a, do NOT execute P4-T19b, and re-deploy the standalone fees image from the
archived tag while the regression is fixed. The rollback must be exercised once (dry-run) so it is not
paper-only.

- **Files:** `docs/monorepo/plan/artifacts/P4-fees-teardown-rollback.md` (NEW)
- **Depends on:** P4-T16, P4-T17, P4-T18
- **Acceptance:** the rollback runway is documented with concrete revert commits and the
  archived-image rebuild path; the trigger conditions (P7-T18 red, staging regression) are explicit; the
  rollback has been dry-run-exercised (the standalone fees image rebuilt from the archived tag and shown
  to boot); P4-T19b is gated on this task existing and being green.
- **Tests:** a dry-run that rebuilds the standalone fees image from the archived read-only origin tag and
  boots it (health/readyz green) — proving the rollback target is recoverable.
- **Effort:** S + 4–8h · **Risks:** R12, R23

### P4-T27 — NEW: Structural (lint/AST) gates — single `validate` reassignment + seam-precedes-redis-seed

**Added per TF1 + SP3 (missing-task, feasibility + no-shims lenses).** The runtime integration tests do
not catch a future code reorder or a forked `validate` binding. Two structural assertions are required;
both must fail the build/test, not an async run:

1. **grep-zero-pre-fee-reuse / single-reassignment gate (TF1).** A structural test or AST/grep check over
   `executeCreateTransaction` asserting: exactly one `validate :=` binding (the first validate at L1045)
   and exactly one `validate =` reassignment (the second validate after `applyFees`); NO additional
   `validate :=` between the seam and `WriteTransaction` (L1249) reintroduces a fork; NO snapshot copy of
   the pre-fee `validate` survives past the `applyFees` call. This proves all 11 downstream consumers read
   the post-fee value by construction.

2. **seam-precedes-redis-seed gate (SP3).** A structural assertion (AST positional check or
   anchored-invariant test) that the `applyFees` call and the second `ValidateSendSourceAndDistribute`
   call both appear textually/positionally BEFORE the `SendTransactionToRedisQueue` call (L1076) in
   `executeCreateTransaction`. A reorder that moves the seam after the seed fails this gate.

Implement as a Go test in the `in` package (using `go/ast`/`go/parser` over the source file, or a
deterministic source-scan test) so it runs under `make test-unit` and CI — NOT as a manual checklist.

- **Files:** `components/ledger/internal/adapters/http/in/transaction_fee_seam_structure_test.go` (NEW)
- **Depends on:** P4-T12
- **Acceptance:** both structural gates exist as automated tests under `make test-unit`; gate 1 fails if
  a second `validate :=` or a pre-fee snapshot is introduced; gate 2 fails if `applyFees`/second-validate
  is reordered after `SendTransactionToRedisQueue`; both are green against the P4-T12 implementation.
- **Tests:** the two structural assertions run green; a deliberately-broken fixture (reordered seam or
  forked binding) in a sub-test confirms each gate actually fails (the gate is proven to bite).
- **Effort:** S + 4–8h · **Risks:** R1, R2

---

## Exit criteria

1. Fees engine runs in-process inside `executeCreateTransaction`; `POST /transactions/*` applies fee
   legs via a **single `validate` reassignment placed between L1045 and L1057** so PropagateRouteValidation
   (L1068) decorates the post-fee validate and ALL 11 downstream consumers (through `WriteTransaction`
   L1249 and `ProcessBalanceOperations` L1151) read it; the transaction balances under ledger's own
   exact-equality validator (R1/R2). A structural gate (P4-T27) proves no pre-fee `validate` survives.
2. Revert and pending-cancel refund the original fees via the EXISTING `TransactionRevert`/cancel
   machinery (no manually injected legs, no double-reverse; `applyFees` proven skipped on `isRevert`);
   reversal `sum (incl. reversed fee legs) == 0`, proven for the **DEDUCTIBLE-fee case** by integration
   tests (PD-5/R18/CG1/CG2).
3. Fee/billing-package state persists in ledger's MongoDB with all 11 indexes; CRUD + estimate +
   billing endpoints served on `:3002` under the preserved plugin-fees auth namespace (PD-7/R8/R9).
4. `internal/m2m`, the MidazService HTTP client, and the account cache are deleted; no outbound HTTP
   to ledger remains; no `github_token`/AWS-Secrets machinery in the fee path (grep-zero secretsmanager
   in the fee tree).
5. Fee leg precision is governed by a single source of truth that **branches on the serialization
   boundary** (P4-T23: JSONB body / msgpack / Mongo mirror, NOT the unbounded Postgres column); the
   hard-coded ISO-4217 precision table is DELETED (not a fallback); `sum(fee legs) == fee total` exactly
   for all fixture assets and split rules **proven independent of the table** via `applyFeeCorrection`'s
   residual-to-max reconciliation; fees are denominated in the transaction's `Send.Asset` (P4-T24).
6. The 18 `pkg/transaction` imports point to `pkg/mtransaction`; the fee engine writes `RouteID`
   (canonical) with `Route` as the passive fallback mirroring the ledger's own convention (R5); the
   route-value-shape conflict was resolved by the P2a pre-move spike before P4-T02 started.
7. The async path persists fee-inclusive transactions and the backup seed reconstructs the
   fee-inclusive payload (P4-T25), with a STRUCTURAL gate (P4-T27) proving the seam precedes the L1076
   redis seed.
8. Standalone fees code (P4-T19a) and deploy units (P4-T19b) deleted; the deploy-unit delete ran ONLY
   after P7-T18 (cross-phase proof) was green AND P4-T21 (Helm/gitops/APIDog lockstep) had landed; a
   tested rollback runway (P4-T26) keeps the standalone image recoverable from the archived origin; net
   deploy units = fees folded into `ledger:3002` (R12).
9. Source relicensed to EL2.0 (R23); `make test-unit`, `make test-integration`, `make lint`,
   `make sec` green; 85% coverage gate met for moved code (R11); per-MODE fee tests (json/dsl/inflow/
   outflow/annotation) and fee-leg op-shape reversibility pass (CGap1).
10. Idempotency slot is released on fee-engine and second-validate failures (no poisoned slot).
11. Zero shims, replace directives, or compat fences introduced. The `FromTo.Route` passive-compat
    field and its `//nolint:staticcheck` markers are the EXPLICIT accepted exception (SS2), mirrored not
    introduced; their ledger-wide removal is out-of-phase.

## Risks addressed

R1, R2, R4, R5, R8, R9, R11, R12, R16, R17, R18, R23, R25

## Open items

- **Route value shape (R5/P4-T02) — RESOLVED-BEFORE-START via P2a-T17 (TF3):** the fee engine's
  synthetic route values come from `feeModel.GetRouteFrom()/GetRouteTo()`. `RouteID` carries
  `validate:"omitempty,uuid"`. If those configured values are NOT UUID-shaped, writing them to `RouteID`
  fails validation — a real behavioral conflict, not a mechanical rename. The fee engine is NOT in this
  tree at HEAD, so this is verified by the P2a pre-move spike (in the fees repo) BEFORE P4-T02 starts; if
  non-UUID, P4-T02 builds a `name→ID` resolution step (M→L) and the P4-T06 resolver absorbs it.
- **Billing scope:** `POST /billing/calculate` + billing-package CRUD are configuration/reporting, not
  the transaction hot path. Kept as CRUD endpoints here, but folding billing into the reporter/CRM
  consolidation is a defensible alternative. Decision deferred; default keep-in-ledger.
- **lib-license-go/v2 — RESOLVED (drop):** carried by fees; the license middleware is deleted in
  teardown (ledger already enforces license), so the dep is DROPPED in P4-T04. P7 has been reconciled to
  the post-collapse state — P7-T06 now asserts `lib-license-go/v2` is ABSENT, not present.
- **Idempotency package-config churn (P4-T15):** the hash is over the raw pre-fee payload; if package
  config changes between two identical-key requests, the replay returns the original result. A NON-replay
  request after a package DELETE recomputes against the deleted package and produces a different fee
  outcome — correct-by-design, documented. Accepted limitation — package-version is intentionally NOT in
  the key.
- **Stale `Asset.Scale` comment (cross-phase cleanup):** the streaming comment at
  `pkg/streaming/events/balance_created.go` references a non-existent `mmodel.Asset.Scale`. It is
  stale noise, unrelated to fees. Flag for a separate doc-comment cleanup PR; do NOT fix inside P4
  (out of scope, no behavioral impact).
- **Ledger's own legacy `Route` field (out-of-phase, SS2 accepted exception):** the ledger writes
  `ft.Route` with `//nolint:staticcheck` across its op-builder (`transaction_create.go:1201`) and
  `TransactionRevert` (`transaction.go:342/364`). P4 mirrors this convention for fee legs (P4-T02) rather
  than inheriting it as a fees shim — this is named as an EXPLICIT accepted exception to the no-shims
  mandate, not silently absorbed. The eventual removal of the passive `Route` field ledger-wide is a
  separate, out-of-phase cleanup — flagged so it is not silently attributed to the fees collapse.
- **Package cache (default DELETE):** under the no-shims aesthetic the burden of proof is on KEEPING the
  package/billing-package cache: delete it in P4-T03 unless P4-T11/T16 hot-path profiling shows Mongo
  hits on the transaction hot path are material. Account cache is deleted unconditionally (P4-T08).
- **Cross-phase numbering reconciliation (DAG-1) — RESOLVED:** this file binds to the locked scheme (P3=crm,
  P4=fees, P5=tracer, P6=reporter) and references `P7-T18` directly. P7.md prose now uses the locked
  numbering throughout (P6 = reporter, P4 = fees); the prior pre-lock "P6" mislabel of this collapse has
  been corrected by the P7 reviser. P4's dependency blocks are unambiguous regardless.


---

<a id="phase-5"></a>

# Phase 5 — Tracer Co-location (24 tasks)

_Verbatim from `docs/monorepo/plan/P5.md`._


**Move type:** Co-located COMPONENT (tracer keeps its own service/binary at `:4020`; it does NOT collapse into ledger).
**Source:** `/Users/fredamaral/repos/lerianstudio/tracer` (`module tracer`, go 1.26.3, branch `develop` today; the imported tree is the P2c-validated post-migration commit, NOT `develop` as-is).
**Destination:** `github.com/LerianStudio/midaz/v3/components/tracer`.
**Locked phase numbering (DAG-1):** P3 = crm, P4 = fees, **P5 = tracer**, P6 = reporter. All four moves gate P7 (unified verification); P7 gates P8 (CI harmonization); P8 precedes P9 (final shim sweep). This phase performs the tracer MOVE; the in-repo dep migration is P2c.
**Gated on:** Phase 2c (P2c) — tracer's lib-commons v4→v5 + observability migration validated IN-PLACE in the tracer repo against tracer's own CI, per PD-6 (observability+co-location MUST NOT share a commit; bisectability). **As of this plan, P2c is unstarted on every visible tracer branch** (`develop` and `feat/migrate-libcommons-v4` both still pin `lib-commons/v4` and carry the `libLogV5`/`libZapV5` shim). The P5-T00 gate is therefore RED today and this phase cannot start until P2c lands.

## Locked decisions binding this phase

- **PD-1** Single root `go.mod`. tracer becomes `components/tracer`. NO go.work, NO replace, NO fences/shims anywhere. Exactly ONE `go` directive and at most ONE `toolchain` line survive in the merged root go.mod (P5-T06 gate).
- **PD-3** Fresh import: git-based ALLOWLIST move (copy ONLY git-tracked paths at the recorded SHA via `git archive | tar -x`), one `import tracer` commit. Because the move copies only tracked paths, untracked/gitignored on-disk artifacts (the `docs/codereview/ast-before-*` AST snapshots, `.bin/`, `artifacts/`, `reports/`) are excluded BY CONSTRUCTION — see the correction in "Verified ground-truth facts". Selective `git filter-repo` for tracer ONLY if audit-hash-chain blame is operationally load-bearing — default fresh (decided per-task, see P5-T01).
- **PD-4** lib-commons GA bump is a HARD prerequisite owned by P1. **midaz `develop` currently still pins `lib-commons/v5 v5.2.0-beta.12` + `lib-observability v1.0.1` (verified live on 2026-06-03 — the beta bump has NOT happened yet).** P1 MUST move midaz off the beta tag onto a v5.2.x GA line BEFORE this phase. **Verified on proxy:** v5.2.0, v5.2.1 (GA), v5.3.0–v5.3.3, v5.4.0/v5.4.1 all exist. tracer's v5 surface needs ≥ v5.3.0 (its current indirect pin). The unified pin must be ≥ the higher of (P1's chosen GA pin, v5.3.0). P5-T00 hard-gates on P1-T06 having landed this.
- **PD-6** P2c migrated tracer's deps in-place first. This phase consumes that validated state; P5 does not assert specific patch versions invented here — it carries forward whatever P2c finalized at the recorded SHA.
- **TRACER ROLE** tracer is INDEPENDENT of infra's otel-lgtm. otel-lgtm stays untouched. tracer drops its own `tracer-postgres` and points at shared `postgres:17` (verify 16→17 compat). A `tracer` database MUST be created on the shared instance (P5-T10a) — it is not created today.
- **R19** Preserve audit-hash-chain migrations (000001/000002/000017) verbatim — no renumber, no hash-logic touch.

## Verified ground-truth facts (checked against live source on 2026-06-03)

- tracer: `module tracer`, go 1.26.3, `develop` go.mod = `lib-commons/v4 v4.6.3` (DIRECT), `lib-commons/v5 v5.3.0` (indirect), **`lib-observability v1.0.0` (indirect — NOT v1.0.1)**, `lib-auth/v2 v2.8.0` (exact match with midaz). The v4/v5 dual-import shim is ALIVE in `config.go` (L24-25 `libLogV5`/`libZapV5`, L1417 `buildAuthClientLogger`). **This shim must be dead by the time the code lands (it dies in P2c).**
- **midaz root go.mod (verified live):** `go 1.26.3`, NO `toolchain` line, `lib-commons/v5 v5.2.0-beta.12`, `lib-observability v1.0.1`. The beta→GA bump (PD-4) is a P1 prerequisite, NOT done. Both modules already declare `go 1.26.3` so the fold needs no toolchain bump — but the merged go.mod must carry exactly ONE `go` directive (P5-T06 gate).
- **CORRECTION — the AST snapshots are UNTRACKED, not tracked.** `git ls-files docs/codereview/` returns 0 files; `git check-ignore docs/codereview/` and `ast-before-3320020718`/`ast-before-2558961936` all match (gitignored). The 9.3MB of `docs/codereview/` AST/codereview junk and `.bin/`/`artifacts/`/`reports/` are gitignored on-disk artifacts. A git-tracked (allowlist) move excludes them automatically; the ONLY way they leak in is a raw working-tree `cp` that ignores `.gitignore`. P5-T02 therefore mandates a `git archive`-based copy and the `ast-before-*` acceptance gate is a DEFENSE against an accidental raw cp, not a guard against tracked files.
- **Rename surface (git-tracked, the denominator for an allowlist move):** ~448 `.go` files / ~856 `"tracer/` import sites / ~42 package dirs on `develop`. **These are INDICATIVE only — all gates are count-agnostic (grep-emptiness + compile).** The original dossier's `465/749/38` matches NO inspectable state and is discarded. (On-disk including untracked artifacts: ~603 files / ~1335 sites / ~77 pkgs — these are NOT the move scope and are excluded by the allowlist mechanic.) The exact move scope is recomputed against the P5-T00 recorded SHA and written into the runbook as an indicative working figure, never a gate.
- There are roughly **~90 bare `"tracer"` token occurrences** (measured: 88 exact `"tracer"` literals via `git grep -h '"tracer"' -- '*.go'`, 25 of them `AppName: "tracer"` config literals) that MUST NOT be rewritten — the codemod targets only slash-suffixed import-spec literals `"tracer/...`. (The dossier's `133` was a broader-regex artifact and is dropped; the count is indicative, the codemod rule is structural.)
- `docs/codereview/` carries 9.3MB of UNTRACKED AST/codereview snapshots, including `ast-before-2558961956` (1.3M) and `ast-before-3320020718` (988K) — excluded for free by the allowlist move (P5-T02), with a belt-and-suspenders acceptance grep.
- Telemetry middleware site: `internal/adapters/http/in/routes.go` imports `libHTTP "lib-commons/v4/commons/net/http"`, calls `NewTelemetryMiddleware` / `tlMid.WithTelemetry` / `tlMid.EndTracingSpans`; also imports `lib-commons/v4/commons/tenant-manager/middleware` and `gofiber/contrib/otelfiber/v2`. Relocation to `lib-observability/middleware` is P2c's job; this phase only verifies it compiles AND emits spans in-module (R13).
- **tenant-manager v4 surface:** 96 files import `lib-commons/v4`, the bulk of which is `tenant-manager/{client,postgres,redis,event,core,middleware}`. The v4→v5 signature migration (constructor + middleware relocation + core types) is P2c's job; P5-T00 confirms tracer's own MT integration tests are green at the recorded SHA, and P5-T15 re-runs them in-module.
- Bespoke guard: `routes.go` `guard.With(...)`. KEEP for liso entry; ProtectedRouteChain conformance is non-blocking follow-up.
- Audit hash chain: migrations 000001/000002/000017 + `internal/adapters/postgres/audit_event_repository.go::VerifyHashChain` (R19).
- Migrations: 34 top-level `.sql` (17 up/down pairs) + `migrations/seeds/{001_dev_data.sql,001_dev_data.down.sql}`. **No `migrations/functions/` dir exists** on `develop`. 9 `.feature` godog files under `tests/`. The audit-hash-chain functions (`000001_calculate_audit_event_hash`, `000002_verify_audit_hash_chain`) and `000003_prevent_truncate` are PURE DDL/PL-pgSQL with ZERO dependency on the Go move — so they can be probed against `postgres:17` + logical replication BEFORE any rename (P5-T01a).
- Dockerfile: `WORKDIR /tracer`, builds `./cmd/app`, `COPY --from=builder /tracer/migrations /app/migrations`, distroless static nonroot, `EXPOSE 4020`. **No `github_token`/`.secrets/`/`go_private_modules` in tracer Dockerfiles** (verified absent); the only mention of `go_private_modules` is in tracer's shared CI workflow (`go-combined-analysis.yml:44` carries `go_private_modules: github.com/LerianStudio/*`), NOT the image build — and `.github/` is never imported (P5-T02), so no tracer-scoped CI in midaz carries it.
- tracer `.env.example`: `DB_HOST=tracer-postgres` (L60), `DB_USER=tracer`, `DB_NAME=tracer` (L63), `DB_PORT=5432`. **Both `DB_HOST` and `DB_NAME` must be repointed** — `DB_HOST` to the shared primary's service name, not just `DB_NAME`. **Shared `components/infra/postgres/init.sql` creates ONLY `onboarding` + `transaction` databases** — no `tracer` DB exists, and init.sql runs only on a fresh volume (P5-T10a addresses both).
- Shared postgres is a logical-replication PRIMARY (`init.sql` creates `replicator` role + physical + logical (`pgoutput`) slots) with a streaming replica — not a standalone like tracer's old `postgres:16-alpine`.
- **midaz CI is split across TWO shared-workflow callers:** `build.yml` (image fan-out, helm dispatch, gitops, S3 migration upload — `filter_paths: components/crm + components/ledger`, `app_name_prefix: midaz`, `path_level: 2`) AND `go-combined-analysis.yml` (lint/security/tests/coverage with `coverage_threshold: 85, fail_on_coverage_threshold: true` — separate `filter_paths: ["components/crm","components/ledger"]`). tracer must be added to BOTH. `build.yml` carries `helm_values_key_mappings` + gitops `yaml_key_mappings` IN-REPO (the deploy-key maps live here, not in an external repo). There is NO godog/cucumber precedent anywhere in midaz CI.
- midaz `build.yml` has two `s3-upload.yml` jobs publishing ledger onboarding+transaction migrations to `lerian-migration-files` for out-of-band ops consumption. tracer's audit-hash-chain DDL has no equivalent today — P5-T12b decides include-or-exclude explicitly.
- tracer CI carries `build.yml`, `go-combined-analysis.yml`, `gptchangelog.yml`, `pr-security-scan.yml`, `pr-validation.yml`, `release.yml`, `dependabot-auto-merge.yml` (full set verified present), plus `.releaserc.yml` + `.releaserc.hotfix.yml` + `CHANGELOG.md`. **tracer CI references NO APIDog secrets and NO migration-role ARN** (verified) — so there are no tracer-specific deploy secrets to migrate; only the origin-repo workflows need disabling on archival (P5-T13b enumerates the full set).
- **midaz Makefile structure (verified):** `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` (L16) — `LEDGER_DIR` is invoked DIRECTLY in build/lint/set-env, NOT in the loop; `CRM_DIR` is BOTH in the loop AND referenced directly in `set-env` (L62-63, L407-408). So "add tracer to the normal loop" is right, but `make set-env` also needs a direct `components/tracer/.env` provisioning touch-point mirroring the CRM/LEDGER direct checks (P5-T11).
- Net-new deps entering root go.mod: `google/cel-go v0.28.1`, `cel.dev/expr v0.25.1`, `yuin/gopher-lua v1.1.2`, `cucumber/godog v0.15.1`, `cucumber/gherkin/go/v26 v26.2.0`, `alicebob/miniredis/v2 v2.38.0`, `DATA-DOG/go-sqlmock v1.5.2`. midaz ALREADY has `antlr4-go/antlr/v4 v4.13.1` (matches).
- **There is NO `fees` package in midaz tree** (components = crm, infra, ledger only; no `package fees`). The fees ENGINE is co-located in P4, not P5. P5-T06's cross-component MVS-drift regression set is scoped to `ledger` + `crm` ONLY (the consumers that exist at P5 time).

---

## Tasks

### P5-T00 — Confirm P2c gate is green; record SHA; recompute (indicative) move scope
- **Description:** Verify the P2c gate and capture the import SHA. In the tracer source repo, confirm P2c landed: (a) zero imports of `lib-commons/v4/*` remain (`grep -rn 'lib-commons/v4' --include='*.go'` empty); (b) the `libLogV5`/`libZapV5` dual-import shim in `config.go` is gone and `buildAuthClientLogger` returns a `lib-observability/log.Logger`; (c) the go.mod dep lines for lib-commons/v5, lib-observability, and lib-auth/v2 are READ FROM the tree at the recorded SHA — **do NOT assert invented patch numbers here**; whatever P2c finalized is what P5-T06 carries forward (note: `develop` currently pins lib-observability **v1.0.0** indirect, so P2c MUST bump it to the chosen line — see open-item #1); (d) the telemetry middleware imports `lib-observability/middleware` and the otelfiber + tenant-manager-v5 middleware chain compiles; (e) tracer's own MT integration tests (worker-supervisor lifecycle, per-tenant lazy spawn, tenant-cap 503) are green — proving the v5 tenant-manager signatures behave, not just that the v4 import path is gone; (f) tracer's full CI (lint + unit + integration + godog e2e) is green at the post-migration commit. **Also confirm the PD-4 prerequisite: P1-T06 has merged the canonical lib-commons GA bump onto midaz develop (root go.mod is OFF `v5.2.0-beta.12`).** **Record the exact SHA.** Then recompute the move scope at that SHA: git-tracked `.go` count, `"tracer/` import-site count, package-dir count — write them into the runbook as an INDICATIVE working figure (NOT a gate, NOT the dossier's stale 465/749/38).
- **Files:** `/Users/fredamaral/repos/lerianstudio/tracer/go.mod`, `/Users/fredamaral/repos/lerianstudio/tracer/internal/bootstrap/config.go`, `/Users/fredamaral/repos/lerianstudio/tracer/internal/adapters/http/in/routes.go`, tracer CI run.
- **Depends on:** P2c-T22, P1-T06.
- **Acceptance:** `grep -rn 'lib-commons/v4' tracer/ --include='*.go'` empty; no `libLogV5`/`libZapV5` symbols; telemetry middleware imports `lib-observability/middleware` and compiles; tracer MT integration tests green; lib-observability line at SHA recorded verbatim (and reconciled against the chosen line, not assumed v1.0.1); P1-T06 GA bump confirmed merged on midaz develop; tracer CI green at recorded SHA; SHA + recomputed (indicative) move-scope counts written into the runbook.
- **Tests:** Re-run tracer's `make lint && make test-unit && make test-integration` + godog e2e at the recorded SHA; all green, MT suite included.
- **Effort:** S (2-4h).
- **Risks:** R3, R4, R10, R13.

### P5-T01 — Decide & record git-history strategy for tracer (fresh vs filter-repo)
- **Description:** Make and DOCUMENT the PD-3 call. Default is fresh import (single `import tracer` commit). PD-3 allows selective `git filter-repo` for tracer ONLY IF its audit-hash-chain blame is operationally load-bearing. Assess whether `git blame` on the audit-hash-chain code (migrations 000001/000002/000017, `audit_event_repository.go::VerifyHashChain`) is used in any SOX/GLBA evidence or incident-forensics workflow. If NO documented operational dependency, choose fresh (recommended, matches "the git log carries the past"). **Default-fresh fires automatically if no SOX/forensics sign-off lands by the phase-start deadline** — the phase is NOT blocked on external approval. Record decision + rationale in the runbook. Gates the move mechanic in P5-T02.
- **Files:** `docs/monorepo/plan/P5.md` (runbook section).
- **Depends on:** P5-T00.
- **Acceptance:** Decision recorded with explicit rationale; if filter-repo chosen, the exact path-set to preserve blame for is enumerated; default-fresh-on-no-signoff deadline stated.
- **Tests:** Decision task (no code); the chosen mechanic is the one P5-T02 executes.
- **Effort:** S (1-2h).
- **Risks:** R19.

### P5-T01a — EARLY pg16→17 + logical-replication compat probe for audit-hash-chain (pre-move gate)
- **Description:** Run the cheapest disqualifying check BEFORE the expensive move. The audit-hash-chain functions (`000001_calculate_audit_event_hash`, `000002_verify_audit_hash_chain`) and `000003_prevent_truncate` are pure DDL/PL-pgSQL with ZERO dependency on the Go rename, go.mod fold, or Dockerfile work — they can be exercised against a throwaway `postgres:17` instance configured for logical replication, in the ORIGIN tracer tree at the P5-T00 recorded SHA. Spin a `postgres:17` PRIMARY with `wal_level=logical` (mirroring the shared instance: physical + `pgoutput` logical slots) and a streaming replica. Apply tracer's 34 migrations + the two seeds. Assert: (a) every migration applies cleanly on pg17 (enum evolutions, decimal conversions, the cents→decimal migration 000005, the unbounded-DECIMAL ALTERs); (b) the `prevent_truncate` trigger (000003) fires correctly AND its behavior is correct under logical replication and on the replica; (c) `VerifyHashChain` / `calculate_audit_event_hash` produce a valid chain whose writes are decoded by `pgoutput` without error on the replica. If pg17 or logical decoding breaks the hash-chain functions, the phase ABORTS HERE — before any rename, go.mod merge, or CI fold is sunk. P5-T10 consumes this result (re-verifies end-to-end against the actual shared instance) rather than re-discovering it. This task carries NO dependency on the rename.
- **Files:** `tools/probe/pg17-logical-replication-probe.sh` (or runbook-recorded one-shot script), tracer migrations at the recorded SHA (read-only).
- **Depends on:** P5-T00.
- **Acceptance:** all 34 migrations + 2 seeds apply on a throwaway `postgres:17` `wal_level=logical` primary; `prevent_truncate` fires correctly on primary AND its replication behavior is correct on the streaming replica; audit-hash-chain functions produce a valid chain that `pgoutput` decodes on the replica without error; result recorded in the runbook; on any failure, the phase aborts before P5-T02.
- **Tests:** Executable probe script exits 0 against `postgres:17` primary+replica; manual log inspection confirms hash-chain rows replicate and `prevent_truncate` blocks a TRUNCATE on both nodes.
- **Effort:** S-M (2-4h).
- **Risks:** R16, R19.

### P5-T02 — Execute git-based ALLOWLIST tree move tracer → components/tracer (one import commit)
- **Description:** On a fresh midaz branch off post-Phase-1 `develop` (with the PD-4 GA bump landed), copy the tracer tree at the SHA recorded in P5-T00 into `components/tracer/` using an ALLOWLIST mechanic from the strategy chosen in P5-T01. **MANDATED MOVE MECHANIC (fresh-import):** `git -C <tracer> archive <SHA> | tar -x -C components/tracer` (or `git -C <tracer> ls-files | rsync --files-from=- ...`). This copies ONLY git-tracked paths at the recorded SHA and **honors `.gitignore` by construction** — it structurally CANNOT import the untracked/gitignored `docs/codereview/ast-before-*` AST snapshots, `.bin/`, `artifacts/`, or `reports/`. **DO NOT use a raw working-tree `cp`** — a cp sweeps gitignored junk in and is the precise mechanic that would trip the `ast-before-*` guard. Then `git add components/tracer`, single commit `import tracer (component co-location)`. **Post-archive cleanup (these ARE tracked, so the allowlist copies them — delete after extract):** `go.mod`, `go.sum` (dissolved into root), `.github/` (folded in P5-T12 — never imported into the component), `.releaserc.yml` + `.releaserc.hotfix.yml` (deleted, see P5-T13a), `CHANGELOG.md` (midaz canonical), `dot_coderabbit.yaml`, `coderabbit-instructions.md`. **KEEP (tracked, intentional):** `cmd/`, `internal/`, `pkg/`, `api/`, `migrations/` (incl. `seeds/`), `mk/`, `tests/` (incl. 9 `.feature` files), `Dockerfile`, `Dockerfile.dev`, `docker-compose.yml`, `.env.example`, `.golangci.yml`, `.ignorecoverunit`, `.trivyignore`, `.dockerignore`, `AGENTS.md`, `README.md`, `SECURITY.md`, `LICENSE`, `llms*.txt`. The tree will NOT compile yet (imports still `tracer/...`) — P5-T04 fixes it in the next commit. Do NOT add a `go.mod` under `components/tracer`. The `ast-before-*` acceptance grep below is a DEFENSE against an accidental raw cp, not a guard against tracked files.
- **Files:** `components/tracer/**`.
- **Depends on:** P5-T00, P5-T01.
- **Acceptance:** move executed via `git archive`/`ls-files` (allowlist), NOT raw `cp` — recorded in the runbook; `components/tracer/cmd/app/main.go`, `internal/bootstrap/config.go`, `migrations/000017_audit_actor_in_hash.up.sql`, `Dockerfile` all exist; NO `components/tracer/go.mod`; NO `components/tracer/.github`; `find components/tracer -path '*codereview/ast-before-*'` returns nothing (defense check); `find components/tracer -name '.bin' -o -name 'artifacts' -o -name 'reports'` returns nothing; import commit is a single self-contained commit separate from the rename commit.
- **Tests:** `find components/tracer -name go.mod` returns nothing; `ls components/tracer/migrations` shows 34 sql + `seeds/`; `find components/tracer -path '*codereview/ast-before-*'` empty; `git log --oneline -1` shows the lone import commit.
- **Effort:** M (3-6h).
- **Risks:** R19.

### P5-T03 — Preserve audit-hash-chain migrations + seeds byte-identical (R19 guard)
- **Description:** Verify migrations 000001/000002/000017 (and all 34 top-level files **plus the two `seeds/001_dev_data*.sql` files**) landed byte-identical to source — no renumber, no whitespace/CRLF mangling, no hash-logic edit. Compute and record SHA256 of each migration + seed file pre-move (tracer repo) and post-move (`components/tracer/migrations`); they MUST match. SOX/GLBA integrity gate. Same check for the `audit_event_repository.go::VerifyHashChain` logic body. Add a CI check (or make target) that diffs the migration+seed set against recorded hashes so a future careless edit fails loudly. **Because P5-T02 is an allowlist move (no untracked duplicates copied), the hashed set is clean by construction — but still scope the SHA verification to `components/tracer/migrations` + `seeds` so no stray copy of `audit_event_repository.go`/`routes.go` can pollute the hash set.**
- **Files:** `components/tracer/migrations/*.sql`, `components/tracer/migrations/seeds/*.sql`, `components/tracer/internal/adapters/postgres/audit_event_repository.go`, `components/tracer/migrations/.hashes`.
- **Depends on:** P5-T02.
- **Acceptance:** SHA256 of every `migrations/*.sql` AND `migrations/seeds/*.sql` matches source-repo SHA256; `VerifyHashChain` body unchanged; integrity check wired into CI/make; no `ast-before-*` files in the hashed set.
- **Tests:** `shasum -a 256 components/tracer/migrations/*.sql components/tracer/migrations/seeds/*.sql` matches recorded source hashes; tracer integration tests `07_audit_events_test.go`, `17_audit_actor_test.go`, `10_upgrade_path_test.go`, `09_bootstrap_migrations_test.go` pass post-move.
- **Effort:** S (2-3h).
- **Risks:** R19.

### P5-T04 — Scripted module-path rename tracer/… → components/tracer/… (Go sources)
- **Description:** Deterministic prefix rewrite across the full git-tracked Go surface recorded in P5-T00 (~448 files / ~856 import sites / 42 pkgs on `develop`, INDICATIVE — recompute at the actual SHA): `tracer/` → `github.com/LerianStudio/midaz/v3/components/tracer/` in every Go import-spec literal, INCLUDING the ~half of the tree that is test files (`*_test.go` import `tracer/` heavily). **Codemod scope is import specs only:** target ONLY slash-suffixed literals matching `"tracer/` and MUST NOT touch the ~90 bare `"tracer"` non-import tokens (e.g. `AppName: "tracer"` config literals — measured at 88, of which 25 are AppName literals). Use an import-aware tool (goimports/gofumpt rewrite over import blocks, or a guarded codemod operating on the AST import section), NOT a blanket `sed`/`gofmt -r '"tracer/x" -> ...'` that cannot generalize across all subpaths. SEPARATE commit from P5-T02 (bisectability) and from any dep change (P2c deps are done). After rewrite, run goimports/gofumpt to re-sort import groups (stdlib → external → internal). **All gates are count-agnostic** — rely on grep-emptiness + compile, not on hitting a specific site count. The rename commit is intentionally non-compiling standalone (compile is gated at P5-T06 after the dep fold); the T04→T06 direction is correct, not a missing dependency.
- **Files:** `components/tracer/**/*.go`.
- **Depends on:** P5-T02.
- **Acceptance:** `grep -rn '"tracer/' components/tracer --include='*.go'` returns nothing; `grep -rn 'github.com/LerianStudio/midaz/v3/components/tracer/' components/tracer --include='*.go'` shows the rewritten imports; bare `"tracer"` config literals untouched; import groups ordered correctly; `go build ./components/tracer/...` compiles (after P5-T06 dep fold).
- **Tests:** `gofmt -l components/tracer` clean; `git log` shows the rename commit distinct from the import commit.
- **Effort:** M (6-8h — sized to the real ~856-site surface incl. test files + codemod iteration).
- **Risks:** R3.

### P5-T05 — Rewrite non-Go string references to the tracer module path
- **Description:** After the Go rewrite, grep for the literal `tracer/` (and bare `module tracer`) across ALL non-Go files under `components/tracer` and fix real hits. VERIFIED at plan time: `.golangci.yml` (no `local-prefixes`/`tracer` rule), `.ignorecoverunit` (relative fragments only), `mk/*.mk` (no module-path literals) currently contain NO module-path string refs — likely no-ops, but **re-confirm post-move as a HARD acceptance gate** (a missed `local-prefixes` rule in `.golangci.yml` silently disables import-ordering lint for tracer). Real hits: swagger annotations in `api/` and **`cmd/app/main.go`'s `@host`/`@BasePath`/`@Router` comment block** (hardcodes `localhost:4020`), plus doc refs in `README.md`/`llms*.txt`/`AGENTS.md`. Dockerfile/compose/Makefile path refs are handled by their dedicated tasks (P5-T08/T10/T11) — coordinate, do not double-fix.
- **Files:** `components/tracer/api/**`, `components/tracer/cmd/app/main.go`, `components/tracer/.golangci.yml`, `components/tracer/.ignorecoverunit`, `components/tracer/README.md`, `components/tracer/llms.txt`, `components/tracer/llms-full.txt`, `components/tracer/AGENTS.md`, `components/tracer/internal/adapters/http/in/*.go`.
- **Depends on:** P5-T04.
- **Acceptance:** `grep -rn 'tracer/' components/tracer --include='*.yml' --include='*.yaml' --include='*.json' --include='*.md' --include='*.txt'` shows only intentional/path-correct refs; `.golangci.yml` `local-prefixes` re-confirmed (hard gate); swagger regenerates without stale module path.
- **Tests:** swagger generation target runs clean; `grep -rn 'module tracer' components/tracer` returns nothing.
- **Effort:** S (2-4h).
- **Risks:** R3.

### P5-T06 — Fold tracer deps into root go.mod and tidy (single go/toolchain directive)
- **Description:** Add tracer's net-new deps to root go.mod and let MVS settle shared deps. Ensure present: `google/cel-go v0.28.1`, `cel.dev/expr v0.25.1`, `yuin/gopher-lua v1.1.2`, `cucumber/godog v0.15.1`, `cucumber/gherkin/go/v26 v26.2.0`, `alicebob/miniredis/v2 v2.38.0`, `DATA-DOG/go-sqlmock v1.5.2`. midaz already has `antlr4-go/antlr/v4 v4.13.1`. **Carry forward the EXACT lib-commons/v5 + lib-observability lines recorded at the P5-T00 SHA — do not invent patch numbers.** Confirm the unified lib-commons/v5 pin is ≥ max(P1 GA pin, v5.3.0). Confirm lib-observability is the reconciled line (v1.0.0 vs v1.0.1 resolved per open-item #1 — whichever P1/P2c agreed). Keep lib-auth/v2 v2.8.0. **PD-1 single-module hygiene:** the merged root go.mod MUST carry exactly ONE `go` directive (both modules already declare `go 1.26.3`, so no toolchain bump needed) and at most ONE `toolchain` line — no second `go`/`toolchain` directive may leak from tracer's dissolved go.mod. Run `go mod tidy`; verify no `lib-commons/v4` and no replace/go.work anywhere. **If the MVS bump moves ledger/crm off their P1 lib-commons line, re-run the cross-component regression suite (ledger + crm — there is NO `fees` suite in-tree at P5; if a later phase co-locates the fees engine, add it to this MVS-drift regression set then) and flag any drift before proceeding.**
- **Files:** `/Users/fredamaral/repos/lerianstudio/midaz/go.mod`, `/Users/fredamaral/repos/lerianstudio/midaz/go.sum`.
- **Depends on:** P5-T04.
- **Acceptance:** `go build ./components/tracer/...` compiles; `grep lib-commons/v4 go.sum` empty; no replace directives, no go.work; `grep -c '^go ' go.mod` == 1 and `grep -c '^toolchain' go.mod` ≤ 1, both equal to midaz canonical (1.26.3); cel-go/godog/miniredis/go-sqlmock present; lib-commons/v5 pin ≥ max(P1 pin, v5.3.0); lib-observability matches the reconciled line; cross-component (ledger/crm) regression suite green if the bump moved any consumer.
- **Tests:** `go mod verify`; `go build ./...` (whole module) green; ledger + crm test suites green if their lib-commons line moved.
- **Effort:** M (3-5h + contingency for cross-component MVS drift).
- **Risks:** R3, R4.

### P5-T07 — Consolidate dead pkg/shell duplication only
- **Description:** Per phase objective, the ONLY pkg consolidation in this phase is dead `pkg/shell`. tracer's `pkg/shell` (`ascii.sh`, `colors.sh`, `makefile_colors.mk`, `makefile_utils.mk`) duplicates midaz's `pkg/shell` build scaffolding. Delete `components/tracer/pkg/shell` and repoint references to midaz root `pkg/shell` + fold tracer's `.mk` helpers into midaz `mk/` (needs T11's mk landing). DO NOT touch `components/tracer/pkg/{constant,model,net,opentelemetry,clock,contextutil,hash,logfields,logging,migration,resilience,sanitize,validation}` — domain-specific, co-exist cleanly. `pkg/constant` (tracer error sentinels) MUST stay component-local — merging into midaz `pkg/constant` violates the per-owner error-sentinel model.
- **Files:** `components/tracer/pkg/shell/**`, `components/tracer/mk/*.mk`.
- **Depends on:** P5-T04, P5-T11.
- **Acceptance:** `components/tracer/pkg/shell` gone; no broken references; tracer domain pkgs untouched; `pkg/constant` error sentinels remain component-local.
- **Tests:** `make lint` on tracer component clean; build green; `grep` for `pkg/shell` in tracer shows no dangling refs.
- **Effort:** S (1-3h).
- **Risks:** R3.

### P5-T08 — Repoint tracer Dockerfile build context & migration path
- **Description:** Update `components/tracer/Dockerfile` (and `Dockerfile.dev`): build path `./cmd/app` becomes `./components/tracer/cmd/app` (or keep per-component Dockerfile with build context repo-root, mirroring ledger — pick whichever matches the chosen CI fan-out in P5-T12). Fix `WORKDIR /tracer` and `COPY --from=builder /tracer/migrations /app/migrations` to the new layout. **Confirmed at plan time: tracer Dockerfiles do NOT reference `github_token`/`.secrets/`/`go_private_modules`** — nothing to drop in the image build; P5-T09 still proves the clean `go mod download` so the absence is verified end-to-end. Keep distroless static nonroot, `EXPOSE 4020`, GOMEMLIMIT.
- **Files:** `components/tracer/Dockerfile`, `components/tracer/Dockerfile.dev`, `components/tracer/.dockerignore`.
- **Depends on:** P5-T04, P5-T06.
- **Acceptance:** docker build of the tracer image succeeds from repo root with the merged go.mod; image `EXPOSE 4020`; migrations present at `/app/migrations`; no `github_token`/`.secrets` references (already absent — assert).
- **Tests:** Local `docker build -f components/tracer/Dockerfile .` produces a runnable image; container starts and serves `/readyz`.
- **Effort:** M (2-4h).
- **Risks:** R16.

### P5-T09 — Verify clean go mod download with no private-module machinery
- **Description:** Confirm the no-private-token claim for tracer: run `GOPRIVATE='' go mod download` (no netrc, no github token) against the merged go.mod and prove every dep (all Lerian libs included) resolves from the public proxy. tracer's only non-public import was `module tracer` itself (dissolved). If download is clean, confirm there is no token machinery to delete in tracer's build path (already verified absent in P5-T08) and that CI does not inject one for tracer. **NOTE — the acceptance below is load-bearing on the `.github/` exclusion (P5-T02):** tracer's ORIGIN `go-combined-analysis.yml:44` DOES carry `go_private_modules: github.com/LerianStudio/*`, but `.github/` is never imported into the component, so no tracer-scoped CI in midaz carries it. A future reader must NOT "helpfully" import that workflow — see open-item #6. This is the gate that proves the token machinery is genuinely unused, not assumed.
- **Files:** `components/tracer/Dockerfile`, `components/tracer/Dockerfile.dev`.
- **Depends on:** P5-T06.
- **Acceptance:** `go mod download` succeeds with zero private-auth config; no reference to `github_token`/`.secrets/`/`go_private_modules` in tracer build path or tracer-scoped CI (true because `.github/` was excluded — cross-reference P5-T02).
- **Tests:** A CI job (or local run in a clean credential-free container) runs `go mod download` and exits 0.
- **Effort:** S (1-2h).
- **Risks:** R3.

### P5-T10 — Wire tracer into shared compose; drop tracer-postgres, point at postgres:17 (DB_HOST + DB_NAME)
- **Description:** Per TRACER ROLE (independent of otel-lgtm). Fold tracer into `components/infra/docker-compose.yml` or repoint `components/tracer/docker-compose.yml` to join `infra-network` (not `tracer-network`). DELETE the `tracer-postgres` (`postgres:16-alpine`) service and repoint tracer's DB env at shared `midaz-postgres-primary` (`postgres:17`). **Repoint BOTH `DB_HOST` and `DB_NAME`:** `.env.example` currently sets `DB_HOST=tracer-postgres` (L60) and `DB_NAME=tracer` (L63). `DB_HOST` MUST repoint to the shared primary's service name (`midaz-postgres-primary` or the actual compose service name); `DB_NAME=tracer` stays (the DB itself is created in P5-T10a). Getting `DB_HOST` wrong leaves `/readyz` red even with the DB created. VERIFY 16→17 migration compat END-TO-END against the actual shared instance (P5-T01a already pre-cleared the audit-hash-chain functions on a throwaway pg17): run tracer's 34 migrations + seeds against the shared `postgres:17` and confirm audit-hash-chain Postgres functions (000001/000002), `prevent_truncate` (000003), enum evolutions, decimal conversions all apply cleanly. **Because the shared instance is a logical-replication PRIMARY (wal_level=logical, physical+logical slots, a streaming replica), explicitly verify the `prevent_truncate` trigger and audit-hash-chain functions behave correctly under logical replication and on the replica — not merely that migrations apply.** Add `depends_on` the postgres healthcheck. Leave `midaz-otel-lgtm` entirely untouched (no telemetry coupling).
- **Files:** `components/tracer/docker-compose.yml`, `components/infra/docker-compose.yml`, `components/tracer/.env.example`.
- **Depends on:** P5-T01a, P5-T04, P5-T08, P5-T10a.
- **Acceptance:** `tracer-postgres` service removed; tracer's `DB_HOST` repointed to the shared primary service name AND `DB_NAME=tracer`; tracer connects to `postgres:17` over `infra-network`; all 34 migrations + seeds apply on pg17; behavior verified under logical replication + on the replica; otel-lgtm unchanged; `/readyz` green against shared pg.
- **Tests:** `make up` brings tracer + shared pg17; tracer integration suite green incl. one run against pg17; migration+seed-apply test on pg17 passes; replica/logical-replication behavior spot-checked; `/readyz` resolves DB_HOST to the shared primary.
- **Effort:** M (3-5h).
- **Risks:** R16.

### P5-T10a — Provision the tracer database on shared postgres:17 (init.sql + existing-volume story)
- **Description:** The implementing task behind T10's hand-wave. Today `components/infra/postgres/init.sql` creates ONLY `onboarding` + `transaction`; deleting `tracer-postgres` without creating the `tracer` DB on the shared instance = migrations fail at connect, `/readyz` never green. Edit `components/infra/postgres/init.sql` to add `CREATE DATABASE tracer;` (matching `DB_NAME=tracer`). **init.sql runs ONLY on a fresh empty volume** — so a populated dev volume will NOT get the DB. Either (a) document the volume-reset requirement in the runbook AND add an idempotent bootstrap (a `CREATE DATABASE IF NOT EXISTS`-equivalent `DO $$ ... $$` block or a startup hook) so existing volumes get `tracer` without a wipe, or (b) make tracer's own migration bootstrap create-if-absent its database before running migrations. Pick the idempotent path; never rely on every dev wiping their volume silently.
- **Files:** `components/infra/postgres/init.sql`, runbook (volume-reset note), optionally `components/tracer/internal/bootstrap` migration bootstrap.
- **Depends on:** P5-T02.
- **Acceptance:** `tracer` DB exists after a fresh `make up` (init.sql path) AND on a pre-existing populated volume (idempotent path); no silent connect failure; documented.
- **Tests:** Fresh-volume `make up` → `psql -l` shows `tracer`; pre-existing-volume `make up` → `tracer` appears without manual intervention; tracer `/readyz` green in both.
- **Effort:** S-M (2-4h).
- **Risks:** R16.

### P5-T11 — Fold tracer Makefile/mk into midaz component-delegation model
- **Description:** Add `TRACER_DIR := ./components/tracer` to the root Makefile and into the `COMPONENTS` loop (`COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` at L16). Note the existing footgun: `LEDGER_DIR` is special-cased OUTSIDE `$(COMPONENTS)` — do NOT replicate that for tracer; add tracer to the normal loop. **SECOND TOUCH-POINT (verified required):** `make set-env` references `CRM_DIR`/`LEDGER_DIR` DIRECTLY (Makefile L62-63 missing-check, L407-408 generate-keys) in ADDITION to the loop — so add a direct `components/tracer/.env` provisioning check to `set-env` mirroring the CRM/LEDGER direct blocks, or `make set-env` will not provision tracer's env (the acceptance below requires it). Port tracer's `mk/{database,docker,docs,quality,security,tests}.mk` into the component's local Makefile so `make tracer COMMAND=<target>` delegates correctly. The 18KB `tests.mk` orchestrates the godog BDD suite (a test mode midaz CI does not currently run); preserve its godog/integration/coverage targets. Repoint any `tracer/`-prefixed path filters in the `.mk` files to `components/tracer/`.
- **Files:** `/Users/fredamaral/repos/lerianstudio/midaz/Makefile`, `components/tracer/Makefile`, `components/tracer/mk/*.mk`.
- **Depends on:** P5-T04.
- **Acceptance:** `make tracer COMMAND=build` works; `make lint`/`make test-unit` at root include tracer; `make set-env` provisions `components/tracer/.env` (via the added direct touch-point); godog target invokable via the component Makefile.
- **Tests:** `make tracer COMMAND=test-unit` green; root `make lint` includes the tracer dir; `make set-env` creates `components/tracer/.env`.
- **Effort:** M (3-5h).
- **Risks:** R11, R25.

### P5-T12 — Fold tracer into existing midaz CI (build + go-combined-analysis), deploy maps, S3 decision
- **Description:** Fold tracer into BOTH midaz shared-workflow callers and the in-repo deploy maps. (1) **`build.yml`:** add `components/tracer` to `filter_paths` so the shared `github-actions-shared-workflows/build.yml` (`path_level:2`, `app_name_prefix midaz`) fans out a `midaz-tracer` image; **add `"midaz-tracer": "tracer"` to `helm_values_key_mappings` AND `"midaz-tracer.tag": ".tracer.image.tag"` to the gitops `yaml_key_mappings`** — these maps live IN build.yml, not externally; without them the fanned-out image is silently dropped and deploys nowhere. (2) **`go-combined-analysis.yml`:** add `components/tracer` to ITS separate `filter_paths` JSON array so lint/security/tests/coverage + the **85% `fail_on_coverage_threshold` gate** actually run on tracer (this is the file with the gate — build.yml does NOT gate coverage). (3) Merge tracer's `.golangci.yml`/`.ignorecoverunit`/`.trivyignore` ignore-lists into midaz config (repoint `tracer/` path rules to `components/tracer/`). (4) **Phantom-deletion note:** `.github/` was never imported (P5-T02 excluded it), so there is nothing to delete under `components/tracer/.github` — just assert its absence; do not budget a deletion. Pin ONE shared-workflow version, ONE go (1.26.3), ONE golangci version. **godog e2e and the coverage-backfill are SPLIT OUT to P5-T12a and P5-T12c; the godog shared-vs-bespoke DECISION is P5-T12a-decide; the migration-S3 decision is P5-T12b.**
- **Files:** `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/build.yml`, `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/go-combined-analysis.yml`, `components/tracer/.golangci.yml`, `components/tracer/.ignorecoverunit`, `components/tracer/.trivyignore`.
- **Depends on:** P5-T06, P5-T11.
- **Acceptance:** CI builds a `midaz-tracer` image on tracer-path changes; `build.yml` helm + gitops maps both contain a `midaz-tracer` entry; `go-combined-analysis.yml` `filter_paths` includes `components/tracer` and the 85% gate evaluates a tracer-scoped coverage number (not a whole-module blend); lint/unit/integration jobs run for tracer; `components/tracer/.github` confirmed absent.
- **Tests:** Open a PR touching `components/tracer/**` and observe CI: build fan-out produces `midaz-tracer`, helm/gitops keys present, go-combined-analysis lint/test/coverage gates run and evaluate tracer coverage.
- **Effort:** L (1 day).
- **Risks:** R11, R15.

### P5-T12a-decide — DECIDE godog shared-vs-bespoke CI workflow (no longer deferred)
- **Description:** Stop deferring the godog CI-workflow decision. There is ZERO godog/cucumber precedent in midaz CI. ANSWER, with sign-off recorded in the runbook, ONE question: does the shared `github-actions-shared-workflows` expose a godog/BDD job hook that midaz can call, or must midaz stand up a BESPOKE local workflow for the BDD suite? This is a cross-repo dependency with its OWN review cycle if the shared workflow must change — that lead time must start BEFORE the implementation task (P5-T12a), not be discovered inside it. Also decide here whether godog GATES the P5 merge or ships as a same-week fast-follow (per open-item #3, fast-follow is acceptable but it MUST be green somewhere before P5-T14 declares tracer deployable). Output: a recorded shared-vs-bespoke decision + gating-or-fast-follow decision that P5-T12a consumes as a given, not a question. **Scope boundary (vs P7-T13a):** this task owns the TRACER-MOVE-TIME decision only — the shared-vs-bespoke workflow choice plus gating-vs-fast-follow for standing up godog at tracer co-location; the separate question of how godog ultimately runs in the consolidated monorepo CI is owned by P7-T13a (UNIFIED-MODULE delivery).
- **Files:** `docs/monorepo/plan/P5.md` (runbook decision section), and (if shared-workflow change is required) an opened cross-repo request against the shared-workflows repo.
- **Depends on:** P5-T06, P5-T11.
- **Acceptance:** shared-vs-bespoke decision recorded with rationale + sign-off; if shared-workflow change is required, the cross-repo request is opened (lead time started); gating-or-fast-follow call recorded; P5-T12a no longer carries an open decision.
- **Tests:** Decision task (no code); runbook entry present; cross-repo request linked if applicable.
- **Effort:** S (2-4h).
- **Risks:** R11, R15.

### P5-T12a — Stand up godog BDD e2e as new midaz CI plumbing (fast-follow, non-gating)
- **Description:** STAND UP the godog BDD e2e as new plumbing per the DECISION already made in P5-T12a-decide (shared vs bespoke is no longer an open question at this point — implement the chosen path). The cucumber dep tree is now in go.mod via P5-T06. Wire the 9 `.feature` files under `components/tracer/tests/` and confirm the runner resolves features + step-defs from the new path. **Per the P5-T12a-decide gating call: godog MAY ship as a same-week fast-follow CI job that does NOT gate the P5 merge, but it MUST run somewhere green before tracer is declared deployable (P5-T14).** Do not let this unproven harness block the phase exit.
- **Files:** `.github/workflows/*` (new or extended godog job, per the decided path), `components/tracer/tests/**/*.feature`, `components/tracer/Makefile` (godog target).
- **Depends on:** P5-T06, P5-T11, P5-T12, P5-T12a-decide.
- **Acceptance:** godog job runs the 9 features green against the in-module binary; features + step-defs resolve from `components/tracer/tests/`; gating-or-fast-follow status applied per P5-T12a-decide.
- **Tests:** godog CI job runs green; feature/step-def path resolution confirmed.
- **Effort:** L (1-2 days — unproven harness + possible cross-repo workflow change executed per P5-T12a-decide).
- **Risks:** R11, R15.

### P5-T12b — Decide tracer migration S3 distribution (mirror ledger or explicitly exclude)
- **Description:** `build.yml` has two `s3-upload.yml` jobs publishing ledger's onboarding+transaction migrations to `lerian-migration-files` for out-of-band ops consumption. tracer ships 34 migrations including audit-hash-chain DDL that ops may run out-of-band. **Decide, do not omit:** either (a) add a tracer migrations `s3-upload` job mirroring ledger's (path `components/tracer/migrations/*.sql`, an `s3_prefix` like `tracer/postgresql`, reusing `AWS_MIGRATIONS_ROLE_ARN`), OR (b) explicitly record that tracer applies migrations ONLY via its own bootstrap and is NOT part of the S3 ops-migration distribution, with sign-off and rationale (audit-hash-chain DDL integrity argues for keeping it in one applied-by-bootstrap path). Record the decision in the runbook.
- **Files:** `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/build.yml`, runbook.
- **Depends on:** P5-T12.
- **Acceptance:** Explicit decision recorded; if (a), an s3-upload job for tracer migrations exists and resolves the bucket/prefix/role; if (b), the exclusion + rationale + sign-off is documented. No silent gap.
- **Tests:** If (a): a build run uploads tracer migrations to the bucket. If (b): runbook entry present with sign-off.
- **Effort:** S (1-2h).
- **Risks:** R15.

### P5-T12c — Measure tracer coverage in-module; reserve 85%-gate backfill
- **Description:** Promote the open-item to a task. The `go-combined-analysis.yml` gate is `coverage_threshold: 85, fail_on_coverage_threshold: true`. tracer is a freshly-imported ~448-file/42-pkg component whose in-module coverage is unmeasured. MEASURE tracer-scoped coverage in-module BEFORE flipping the gate live for tracer. If below 85%, reserve an explicit backfill sub-task with budgeted hours and a list of under-covered packages; do NOT let an unmeasured baseline hard-fail the merge with no backfill plan. Confirm the gate evaluates a tracer-scoped number, not a whole-module blend that hides tracer's deficit.
- **Files:** `components/tracer/**` (test files for backfill), coverage report.
- **Depends on:** P5-T06, P5-T11.
- **Acceptance:** tracer-scoped coverage measured and recorded; if <85%, backfill task with hours + package list exists; gate confirmed tracer-scoped.
- **Tests:** `make tracer COMMAND=test-unit` with coverage produces a tracer-scoped percentage; backfill PRs (if needed) bring it ≥85%.
- **Effort:** M (0.5-1 day + backfill contingency).
- **Risks:** R15.

### P5-T13a — Delete tracer standalone release artifacts & assert no own go.mod (in-repo, prereq of T15)
- **Description:** The IN-REPO half of the former T13 — these assertions are a PREREQUISITE of the verification gate (P5-T15) and must NOT touch the origin repo. Confirm deletion of tracer's independent release machinery so it rides midaz's monorepo semantic-release: ensure `.releaserc.yml`, `.releaserc.hotfix.yml`, and tracer's `release.yml`/`gptchangelog.yml` workflows are NOT present under `components/tracer` (they were excluded/cleaned by P5-T02; assert absence). Confirm `components/tracer/go.mod` + `go.sum` do NOT exist (dissolved into root). Add `tracer` to midaz's PR-title conventional-commit scopes so a `feat(tracer):` commit bumps the monorepo version correctly. **Confirmed at plan time: tracer CI has NO APIDog secrets and NO migration-role ARN — there are no tracer-specific deploy secrets to migrate; assert this rather than budget a secret-plumbing task.** This task does NOTHING to the origin tracer repo — origin archival is P5-T13b, gated AFTER P5-T15 green.
- **Files:** `components/tracer/.releaserc.yml` (must be absent), `components/tracer/.releaserc.hotfix.yml` (must be absent), `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/pr-validation.yml`.
- **Depends on:** P5-T02, P5-T12.
- **Acceptance:** no tracer `.releaserc*`/`release.yml`/`gptchangelog.yml` under the component; no `components/tracer/go.mod`; `tracer` is a valid commit scope in midaz pr-validation; no tracer-specific CI secret to migrate (asserted). NO origin-repo action taken.
- **Tests:** `find components/tracer -name 'go.mod' -o -name '.releaserc*'` returns nothing; a `feat(tracer): ...` commit passes midaz pr-title validation.
- **Effort:** S (1-2h).
- **Risks:** R24.

### P5-T13b — Archive origin tracer repo read-only & disable ALL origin workflows (gated AFTER T15 green)
- **Description:** The ORIGIN-repo half of the former T13 — an ops action OUTSIDE this repo that DESTROYS the rollback fallback, so it is gated STRICTLY AFTER the in-module verification gate (P5-T15) is green and the P5-T16 abort invariant has been satisfied. Mark the origin tracer repo read-only and **disable EVERY origin tracer workflow so no orphaned release/build/scan fires post-archival.** Full enumerated set (verified present in tracer `.github/workflows`): `release.yml`, `gptchangelog.yml`, `build.yml`, `go-combined-analysis.yml`, `pr-security-scan.yml`, `pr-validation.yml`, `dependabot-auto-merge.yml`. Archive tracer's `CHANGELOG.md` in the origin repo (midaz canonical takes over). **HARD PRECONDITION (must be checked, not merely depended-on):** the runbook records P5-T15 as GREEN before this task starts; archiving before in-module green removes the only fallback (P5-T16 invariant).
- **Files:** origin `tracer` repo (read-only flag + workflow disabling — outside this monorepo), runbook.
- **Depends on:** P5-T13a, P5-T15, P5-T16.
- **Acceptance:** runbook precondition "P5-T15 green" recorded and checked; origin tracer repo set read-only; ALL seven enumerated origin workflows disabled (no orphaned run can fire); origin `CHANGELOG.md` archived; no origin release/build triggers remain active.
- **Tests:** origin repo shows archived/read-only state; a push attempt is rejected; Actions tab shows all workflows disabled; runbook records the green-gate timestamp.
- **Effort:** S (1-2h).
- **Risks:** R24, R10, R19.

### P5-T14 — Out-of-repo deploy lockstep: external Helm chart / gitops repo / APIDog
- **Description:** A co-located component that builds but isn't deployed is dead weight. The IN-REPO deploy-key maps (build.yml helm + gitops mappings) are handled in P5-T12; this task is the genuinely EXTERNAL surface. Coordinate cross-team: add `midaz-tracer` to the external Helm chart `midaz` (values key `tracer` + deployment on `:4020` pointed at shared pg) and to `midaz-firmino-gitops` (ArgoCD app). Extend APIDog e2e if tracer endpoints are covered (tracer has no APIDog secrets today — adding coverage means provisioning a tracer scenario in midaz's APIDog set, or explicitly deferring with sign-off). tracer ports verified non-colliding (`:4020`). Gated AFTER the image builds (P5-T12) and AFTER P5-T15 proves green (do NOT flip ArgoCD before in-module green). **godog (P5-T12a) must be green somewhere before tracer is declared deployable here.** **OWNER-UNAVAILABLE / CHART-REJECTED FALLBACK (CGap2):** if the external Helm/gitops/APIDog owner does NOT sign off in lockstep, or the chart change is rejected, this is the likeliest real-world stall — the defined path is: (1) the in-module component stays merged and CI-green (it does not regress); (2) tracer is flagged `built-but-not-yet-deployed` in the runbook with the blocking owner named; (3) the origin tracer repo stays canonical and WRITABLE (P5-T13b is NOT executed) so the standalone service keeps deploying from origin until the external surface lands; (4) a tracking item with the owner + ETA is recorded. Deployment-lockstep failure must NOT force-archive the origin or strand the running service.
- **Files:** external `midaz` Helm chart, `midaz-firmino-gitops` repo, APIDog config, runbook (fallback entry).
- **Depends on:** P5-T12, P5-T15.
- **Acceptance:** Helm `midaz` renders a `midaz-tracer` deployment on `:4020` pointed at shared pg; ArgoCD app present; APIDog covers tracer endpoints (or explicitly deferred with sign-off); godog green somewhere; OR — if the owner is unavailable / chart rejected — the built-but-not-yet-deployed fallback is recorded with named owner + ETA and the origin repo is kept writable (P5-T13b held).
- **Tests:** `helm template` of `midaz` includes the tracer deployment; ArgoCD diff is clean; a staging deploy of `midaz-tracer` serves `/readyz`. (Fallback path: runbook entry present, origin still deploying.)
- **Effort:** M (0.5-1.5 days wall-clock).
- **Risks:** R12.

### P5-T15 — Full in-module verification gate (lint/unit/integration green; godog tracked)
- **Description:** The phase exit gate. With everything folded, run the full midaz verification surface and prove tracer is green INSIDE the monorepo, not just in isolation: `make lint`, `make test-unit`, `make test-integration` (testcontainers + the added pg17 run from P5-T10 against the provisioned `tracer` DB from P5-T10a). Re-prove integrity-sensitive paths: audit-hash-chain integration tests; **multi-tenant worker-supervisor lifecycle (per-tenant lazy spawn + tenant-cap 503) — re-run in-module to prove the P2c v5 tenant-manager signatures behave, not merely that the v4 import is gone**; and the **telemetry middleware path: assert the otelfiber + `lib-observability/middleware` + tenant-manager-v5 middleware + `guard.With` chain COMPILES and EMITS spans in-module (concrete span-emission test, not just compile)**. Confirm the tracer binary boots and serves `/health` `/readyz` `/metrics` `/version` `/swagger/*`, and the `guard.With()` auth chain still protects `/v1/*`. godog is tracked via P5-T12a (gating-or-fast-follow per P5-T12a-decide); record the bespoke-guard → ProtectedRouteChain conformance as a NON-BLOCKING follow-up (do not do it here). **This task's GREEN result is the precondition that unlocks the destructive origin archival (P5-T13b) — record it explicitly in the runbook.**
- **Files:** `components/tracer/**`.
- **Depends on:** P5-T03, P5-T05, P5-T06, P5-T07, P5-T08, P5-T09, P5-T10, P5-T10a, P5-T11, P5-T12, P5-T12c, P5-T13a.
- **Acceptance:** all make targets green for tracer in-module; binary boots and serves all public + `/v1` routes; audit-hash-chain + MT-worker (v5 signatures) + telemetry-span-emission tests pass; guard chain enforced; godog status recorded (P5-T12a); ProtectedRouteChain polish logged as non-blocking; GREEN result recorded in the runbook as the P5-T13b unlock precondition.
- **Tests:** `make lint && make test-unit && make test-integration` (root, incl. tracer) green; concrete telemetry span-emission test passes; MT worker-supervisor test passes; smoke `/readyz` + one authed `/v1/rules` call against the running binary.
- **Effort:** L (1 day).
- **Risks:** R10, R11, R13, R15, R16, R19.

### P5-T16 — Define the move abort/rollback path (origin stays canonical until T15 green)
- **Description:** This is the highest-blast-radius phase (~448-file scripted rename + go.mod merge + compose rewrite + CI fold). Define the revert path BEFORE the move so failure is recoverable. The import commit and rename commit are SEPARATE (bisectability) — document the abort as `git revert` of the commit range `X..Y` on the midaz branch, with the origin tracer repo staying canonical and writable until P5-T15 passes. **HARD ORDERING INVARIANT: do NOT archive/read-only the origin tracer repo (P5-T13b) until P5-T15 is green in-module** — archiving the source before the monorepo is proven removes the only fallback. This invariant is ENFORCED structurally by the dependency graph: P5-T13b depends on P5-T15 AND P5-T16, and carries a hard runbook precondition check ("P5-T15 green recorded"); the in-repo release-artifact assertions (P5-T13a) are split out so they can be a PREREQUISITE of P5-T15 WITHOUT dragging origin archival before the gate. Record the exact commit range and abort command in the runbook once P5-T02/T04 land.
- **Files:** `docs/monorepo/plan/P5.md` (runbook abort section).
- **Depends on:** P5-T02, P5-T04.
- **Acceptance:** Abort runbook entry exists with the exact `git revert X..Y` range; origin-stays-canonical-until-T15-green invariant stated and enforced via the P5-T13b dependency edge + precondition check (not prose alone); P5-T13a/P5-T13b split documented as the mechanism that resolves the former T13→T15 prerequisite collision.
- **Tests:** Dry-run the abort on a throwaway branch: `git revert` the range restores a compiling pre-move midaz tree.
- **Effort:** S (1-2h).
- **Risks:** R10, R19.

---

## Exit criteria

- `components/tracer` compiles and tests green inside the single root go.mod — no go.work, no replace, no lib-commons/v4, no `libLogV5`/`libZapV5` shim, exactly ONE `go` directive (1.26.3) and ≤ ONE `toolchain` line.
- Module-path rename complete (zero `"tracer/` imports) in a commit SEPARATE from the import commit and from the P2c dep migration; gates are grep-emptiness + compile, NOT a hardcoded site count (~448/~856/42 are indicative only).
- The tree move was an ALLOWLIST `git archive`/`ls-files` copy (NOT raw cp); untracked/gitignored `docs/codereview/ast-before-*` snapshots excluded by construction (PD-3), with a defense-grep acceptance; audit-hash-chain migrations + seeds byte-identical (R19), SHA-verified and CI-guarded.
- The pg16→17 + logical-replication compat of the audit-hash-chain functions + `prevent_truncate` was pre-cleared EARLY (P5-T01a, pre-move) and re-verified end-to-end against the shared instance (P5-T10).
- tracer runs as an independent component on `:4020` against shared `postgres:17` with a provisioned `tracer` DB (init.sql + idempotent existing-volume path), `DB_HOST` + `DB_NAME` both repointed; 16→17 + logical-replication behavior verified on primary and replica; otel-lgtm untouched.
- CI fans out a `midaz-tracer` image (build.yml) AND runs lint/security/tests with the 85% coverage gate (go-combined-analysis.yml); build.yml helm + gitops deploy maps carry a `midaz-tracer` entry; godog shared-vs-bespoke workflow DECIDED (P5-T12a-decide) before implementation, then runs green (gating or same-week fast-follow); tracer-scoped coverage measured with backfill reserved if <85%.
- tracer migration S3 distribution decided explicitly (mirror ledger or documented exclusion).
- tracer standalone go.mod/go.sum/.releaserc/release workflows deleted or absent (P5-T13a); rides midaz monorepo release; origin repo archived read-only WITH all seven origin workflows disabled (P5-T13b) — only AFTER P5-T15 green.
- External Helm/gitops/APIDog extended in lockstep so `midaz-tracer` actually deploys, OR the owner-unavailable/chart-rejected fallback is recorded (built-but-not-deployed, origin kept writable) with named owner + ETA.
- A documented abort path exists; the origin-stays-canonical-until-T15-green invariant is enforced via the dependency graph (P5-T13b ← P5-T15 + P5-T16 + precondition check), not prose.

## Risks addressed

R3, R4, R10, R11, R12, R13, R15, R16, R19, R24, R25.

## Open items

1. **lib-observability v1.0.0 vs v1.0.1 (cross-phase, P1/P2c):** tracer's `develop` go.mod pins lib-observability **v1.0.0 indirect**; PD-4/CLAUDE.md say keep **v1.0.1**. P2c MUST bump tracer to the agreed line, and P5-T06 carries forward whatever the recorded SHA shows — it does NOT assert a number. Reconcile P1/P2c/P5 on ONE lib-observability line BEFORE P5-T06 folds it into root go.mod. Dependency: P1-T06 + P2c-T22.
2. **lib-commons/v5 unified pin (cross-phase, P1):** unified pin must be ≥ max(P1 GA pin, v5.3.0). **midaz develop STILL pins `v5.2.0-beta.12` as of 2026-06-03** — the GA bump (PD-4) is a P1-T06 prerequisite, NOT done. Proxy verified: v5.2.0/v5.2.1 GA + v5.3.0–v5.3.3 + v5.4.0/v5.4.1 GA exist; tracer carries v5.3.0 indirect. If P1 pins exactly v5.2.x GA (<5.3.0), Phase-5 forces ≥v5.3.0 because tracer's v5 indirect surface needs it. P5-T06 reconciles; cross-component regression suite (ledger + crm — no fees suite in-tree at P5) re-runs if the bump moves any consumer. Dependency: P1-T06.
3. **godog gating decision — NOW OWNED (P5-T12a-decide):** the shared-vs-bespoke-workflow question and the gating-or-fast-follow call are DECIDED in P5-T12a-decide (no longer deferred). godog may ship as a same-week fast-follow CI job WITHOUT gating the merge, but MUST run green somewhere before tracer is declared deployable (P5-T14). Implementation is P5-T12a.
4. **git filter-repo blame decision (P5-T01):** default fresh; only deviate if audit-hash-chain blame is shown operationally load-bearing — needs a human SOX/forensics sign-off if claimed. Default-fresh fires automatically if no sign-off by the phase-start deadline; the phase is not blocked on external approval.
5. **Origin tracer repo archival is an ops action (set read-only + disable ALL workflows) outside this repo (P5-T13b)** — sequenced STRICTLY after P5-T15 is green via the dependency edge + a hard runbook precondition check (P5-T16 invariant). Archiving before in-module green removes the rollback fallback. The former T13 was SPLIT (T13a in-repo prereq of T15 / T13b origin archival after T15) to resolve the prerequisite-vs-after-green collision.
6. **`go_private_modules` in tracer origin CI is intentionally orphaned:** tracer's origin `go-combined-analysis.yml:44` carries `go_private_modules: github.com/LerianStudio/*`, but `.github/` is never imported into the component (P5-T02), so no tracer-scoped CI in midaz carries it and P5-T09's clean-download gate holds. A future reader must NOT import that workflow into the monorepo — cross-referenced in P5-T09.
7. **Bespoke `guard.With()` vs `ProtectedRouteChain()`:** kept for liso entry per phase objective; logged as known divergence and non-blocking polish, not in this phase's scope.
8. **`pkg/model` vs midaz `pkg/mmodel` convention:** tracer keeps `components/tracer/pkg/model` (co-located components may keep their own pkg). Cosmetic; not addressed here.


---

<a id="phase-6"></a>

# Phase 6 — Reporter Co-location (Two Top-Level Components) (21 tasks)

_Verbatim from `docs/monorepo/plan/P6.md`._


**Objective.** Move `github.com/LerianStudio/reporter` into the midaz single-module monorepo as **two
top-level components** — `components/reporter-manager` (:4005 REST) and `components/reporter-worker`
(:4006 headless Chromium renderer) — per the verified `build.yml` `path_level: 2` fan-out (dossier 08
§4.3: each level-2 dir under `filter_paths` becomes one image). Reporter's shared `pkg/` lands at
`components/reporter/pkg/` to avoid the `constant` / `net` / `shell` collisions with `midaz/pkg`. Reporter's
root `tests/` tree (e2e/integration/property/fuzzy/chaos/utils) lands at `components/reporter/tests/` (a
fourth level-2 dir with no `cmd/` and no Dockerfile, so the fan-out ignores it). Module path rewritten
across all reporter `.go` files; `go.mod`/`go.sum` deleted and folded into root; dual mongo-driver
collapsed to midaz's `v1.17.9`; Dockerfile COPY paths/build-context rewritten to repo-root; worker stays
fat alpine+Chromium, manager goes distroless-nonroot. Two binaries, two images preserved.

**Topology note (resolves a dossier disagreement).** Dossier 05 §4 recommends ONE component
`components/reporter/{manager,worker}` with two `cmd/`. That is WRONG for the shared `build.yml`, which
fans out one image per **level-2 directory** — a single `components/reporter` dir would emit ONE image,
not two. Dossier 08 §1/§4.3 and the locked decision are authoritative: **two top-level components**
`components/reporter-manager` + `components/reporter-worker`. The shared, non-image-emitting reporter
trees land under `components/reporter/`: `components/reporter/pkg/` (library code, gated by `shared_paths`)
and `components/reporter/tests/` (cross-component suites). Neither has a `cmd/` or a Dockerfile, so the
fan-out ignores both. This plan executes the two-component layout exclusively.

**Gate.** This phase is GATED on **P1** (midaz lib-commons GA pin — DAG-5: pinned to **P1-T06**, the
canonical-pin-merged gate task) and **P2b** (reporter observability migration: `commons/log` +
`commons/opentelemetry` + `commons/zap` sites → `lib-observability`, done IN reporter's own repo,
validated against reporter's CI, in a commit SEPARATE from co-location — PD-6; DAG-4/DAG-5: pinned to the
P2b gate task). **P2b** must have bumped reporter to that same lib-commons GA + lib-auth v2.8.0 + Go 1.26.3
in-repo. This phase assumes reporter ALREADY compiles clean against `lib-observability` and the Phase-1
lib-commons GA in its origin repo. Phase 6 is the mechanical rename+fold+wire move ONLY.

**Phase 6 is INDEPENDENT of P5 (Tracer) and P3 (CRM) — explicit interim-topology contract.** Under the
locked numbering (P3=crm, P4=fees, P5=tracer, P6=reporter) all four moves are siblings: each gates P7, and
none gates another (DAG-1). The DAG permits P6 to land BEFORE P5 and/or P3. **Therefore P6 must NOT assume
`components/tracer` exists or that `components/crm` is gone.** Ground truth at the time of writing:
`ls components/` = `{crm, infra, ledger}`; midaz `Makefile` L16 `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)`;
`build.yml` `filter_paths` = `components/crm` + `components/ledger`;
`helm_values_key_mappings` = `{"midaz-crm":"crm","midaz-ledger":"ledger"}`. Every task that touches the
component list, the CI fan-out, the port-collision check, or STRUCTURE.md MUST read the **live** state at
execution time and **add** reporter's two components onto whatever is already there — never hardcode a
`ledger tracer` literal or assert against a `midaz-tracer` image that may not exist yet. Net-new infra
(SeaweedFS, KEDA) is owned by Phase 8 — Phase 6 stages reporter's SeaweedFS config files and must not break
Phase 8's compose authoring; it flags the dependency.

**Ground-truth verified against source** (`/Users/fredamaral/repos/lerianstudio/reporter`,
`/Users/fredamaral/repos/lerianstudio/midaz`):
- 582 `.go` files in reporter (excl. `docs/codereview/ast-before-*`); **360** contain the
  `github.com/LerianStudio/reporter` import string (the rest are package-local). The rename script targets
  ALL `.go` files plus the `go.mod` declaration and non-Go string refs, so the exact import-bearing count
  is informational, not load-bearing.
- The root `tests/` tree is real and substantial: `tests/{e2e(19), integration(16), property(9),
  fuzzy(10), chaos(15), utils(6)}` = ~75 `.go` files, **61** of which import a FIFTH module prefix
  `github.com/LerianStudio/reporter/tests`. It lives at the reporter REPO ROOT, NOT under
  `components/manager` or `components/worker`. T16/T17 cite these suites as their verification mechanism,
  so the tree MUST be carried (T02) and rewritten (T03), not phantom-"ported".
- Observability sites are handled in P2b, NOT here (the dossier-09 "raw zap" claim is WRONG — reporter
  uses `commons/log`). T01 gates on the migrated state (== 0 `commons/{log,opentelemetry,zap}` sites).
- **ZERO** `LerianStudio/midaz` imports in reporter — no cross-repo Go coupling to reconcile (R21 is purely
  runtime fetcher connection strings to `midaz_onboarding`/`midaz_transaction`, found in
  `pkg/datasource/types.go`, `pkg/fetcher/types.go`, and tests — config, not imports).
- Dual mongo-driver is small but NOT "two lines": `mongo-driver/v2 v2.5.0` appears in `go.mod`/`go.sum` +
  exactly ONE source file (`pkg/itestkit/infra/mongodb/mongodb_test.go`, `//go:build itestkit`). That file
  has **2 import lines PLUS 4 `mongo.Connect(options.Client().ApplyURI(uri))` call sites** that must move
  to the v1 ctx-first signature `mongo.Connect(ctx, opts...)` (plus the matching Disconnect/Ping shape).
  All production code uses v1 (`go.mongodb.org/mongo-driver/{mongo,bson,...}`) which matches midaz's
  `v1.17.9`.
- `docs/codereview/ast-before-*` is git-ignored and has 0 tracked files — a git-based move excludes it
  automatically (R-exclude). A raw `cp -r` would drag in 35 MB; the move MUST be git-based.
- pkg collisions confirmed real: `pkg/net/http`, `pkg/shell`, `pkg/constant` exist in BOTH with different
  contents. `components/reporter/pkg/` placement dissolves all three via path divergence (no merge).
- Dockerfiles build with repo-root context (`COPY go.mod go.sum`) but use SELECTIVE COPY
  (`COPY pkg/ pkg/` + `COPY components/manager/`), final image `alpine:3.23`, EXPOSE 4005, `wget /health`
  HEALTHCHECK. Worker = alpine + chromium + nss + full noto/dejavu/liberation/freefont stack,
  `CHROME_BIN=/usr/bin/chromium-browser`, `CHROMEDP_USE_SYSTEM_CHROME=true`, EXPOSE 4006. Both Dockerfiles
  carry `LABEL org.opencontainers.image.source="https://github.com/LerianStudio/reporter"` (L29) —
  a stale pointer that MUST be rewritten (T09/T10). Ledger's distroless Dockerfile uses `COPY . .`, no
  HEALTHCHECK (pure orchestrator probe) — the pattern to mirror for the manager.
- Reporter compose reality (T11 load-bearing): each service attaches to `reporter-infra-network` +
  `reporter-{manager,worker}-network`. `infra-network` IS declared `external: true` in the top-level
  `networks:` block BUT is NOT listed in any service's `networks:` — a **dangling declaration**, not a
  working attachment. "Keep the gift" is a no-op that leaves the services on the wrong network. T11 must
  REPOINT the service blocks. Reporter also ships its OWN bundled infra in
  `reporter/components/infra/docker-compose.yml`: `reporter-postgres`, `reporter-mongodb`,
  `reporter-valkey`, `reporter-rabbitmq:4.0`, plus SeaweedFS (`chrislusf/seaweedfs:4.05`) and KEDA
  (`ghcr.io/kedacore/keda:2.16.0`). All bundled backing services must be reconciled to midaz's shared
  infra, not just RabbitMQ.
- SeaweedFS mounted config lives at `reporter/components/infra/seaweedfs/{s3.json, init-bucket.sh}`. The
  service/KEDA DEFINITIONS are authored by Phase 8; the CONFIG FILES must be carried now (T02) so T11/T17
  and Phase 8 have them. This requires carrying a precise allowlist out of reporter's `components/infra/`,
  reconciling the earlier "do NOT carry infra/" blanket statement.
- reporter env surface (T-env load-bearing): worker `.env.example` and manager `.env.example` define
  `OBJECT_STORAGE_*` (SeaweedFS S3 creds/bucket/endpoint), `PDF_POOL_WORKERS`/`PDF_TIMEOUT_SECONDS`,
  `FETCHER_ENABLED`/`FETCHER_URL`, `MULTI_TENANT_ENABLED`, `MONGO_*`/`RABBITMQ_*`/`REDIS_*` connection
  config. None of this is wired into midaz's `make set-env` flow today — a real gap for local `make up`.
- RabbitMQ topology real names (T17): exchange `reporter.generate-report.exchange`, queue
  `reporter.generate-report.queue`, routing key `reporter.generate-report.key`.
- reporter `build.yml`: `app_name_prefix: "reporter"`, `filter_paths: components/manager + components/worker`,
  `helm_chart: "reporter"`, `helm_values_key_mappings: {"reporter-manager":"manager","reporter-worker":"worker"}`,
  ghcr-only. Must become prefix `midaz`, paths `components/reporter-{manager,worker}`, chart `midaz`,
  DockerHub+ghcr.
- lib-auth import sites: **8** `.go` files import `LerianStudio/lib-auth` (excl. ast-before), not 5; the
  v2.7→v2.8 middleware-drift surface is ~1.6x the prior estimate. Still clean call-site work (no shim).
- lib-commons proxy state (PD-4 verification): no literal "v5.2.x GA" beyond `v5.2.1`; the 5.2 line's GA
  is `v5.2.1`, and the line has advanced to `v5.3.x` GA (up to v5.3.3) and `v5.4.0`/`v5.4.1` GA. Phase 1
  owns the pin choice; Phase 6 consumes "the Phase-1-pinned lib-commons GA" as a variable.

**Risks addressed:** R14 (dual mongo-driver collapse), R20 (worker Chromium fat image), R21 (fetcher
external-DB reachability), R11 (85% coverage gate on moved code), R12 (helm/gitops image-rename lockstep),
R25 (STRUCTURE.md staleness). No shims, no replace, no go.work, no fences anywhere.

---

## Task DAG (sequential within phase unless noted)

```
P6-T01 (verify P2b/P1 preconditions) ─┐
P6-T02 (git-based move skeleton incl. tests/ + seaweedfs config) ─┼─→ P6-T03 (module-path rename, 5 prefixes) ─→ P6-T04 (delete reporter go.mod, fold require)
                                       │                                        │
                                       │                                        ├─→ P6-T05 (collapse dual mongo-driver)
                                       │                                        ├─→ P6-T06 (go mod tidy, build both binaries)
P6-T07 (pkg/ placement + collision audit) ── depends P6-T03                     │
P6-T08 (rewrite non-Go path refs) ── depends P6-T03                             │
P6-T09 (Dockerfile manager rewrite + OCI label) ── depends P6-T06               │
P6-T10 (Dockerfile worker rewrite + OCI label) ── depends P6-T06                │
P6-T11 (compose fold, infra-network repoint, shared infra, SeaweedFS/KEDA handoff) ── depends P6-T06
P6-T12 (Makefile component fold) ── depends P6-T06                              │
P6-T13 (build.yml + CI filter_paths fold, relative to live components) ── depends P6-T09,P6-T10
P6-T14 (delete reporter origin CI/release/dependabot dupes) ── depends P6-T13   │
P6-T15 (coverage gate backfill audit) ── depends P6-T06                         │
P6-T16 (R21 fetcher reachability integration test) ── depends P6-T06, P6-T11    │
P6-T17 (end-to-end PDF pipeline verification) ── depends P6-T09,P6-T10,P6-T11,P6-T21
P6-T18 (STRUCTURE.md + docs update, live topology) ── depends P6-T13            │
P6-T19 (out-of-repo Helm/gitops/APIDog lockstep coordination) ── depends P6-T13 │
P6-T20 (origin repo read-only archive) ── depends P6-T17,P6-T19                 │
P6-T21 (reporter env-var wiring into make set-env) ── depends P6-T11            │
```

PHASE-reporter-move (referenced by P9 / DAG-2) resolves to **P6-T17** (move verified end-to-end) and
**P6-T20** (origin archived). DAG-5: every external phase edge is pinned to a gate task — P1 → **P1-T06**,
P2b → its gate task — and every internal task chains back to P6-T01. No bare phase-level edges remain.

---

### P6-T01 — Verify P2b and P1 preconditions before any move
**Description.** Coordination/verification gate. Confirm: (a) P1-T06 has merged a concrete lib-commons GA
version into midaz root `go.mod` (NOT a beta — the live tree currently pins `v5.2.0-beta.12`, which FAILS
this gate; P6 cannot start until P1-T06 lands the GA pin) — capture the exact tag (proxy shows `v5.2.1` as
the 5.2 GA, with `v5.3.x`/`v5.4.x` GA also available; the chosen tag is Phase-1's, not Phase-6's, decision).
(b) P2b has landed in reporter's ORIGIN repo: `grep -rn "lib-commons/v5/commons/\(log\|opentelemetry\|zap\)"`
over reporter `.go` (excl. ast-before) returns **0**; `lib-observability` is imported; reporter `go.mod` pins
the SAME lib-commons GA as midaz + `lib-auth/v2 v2.8.0` + `go 1.26.3` (drop the `toolchain go1.26.2` line);
reporter's own CI is green on that commit; and the observability migration is a SEPARATE commit from any
co-location change (bisectability, PD-6). If any precondition is unmet, STOP — Phase 6 cannot start.
**Files.** `/Users/fredamaral/repos/lerianstudio/midaz/go.mod` (read), `/Users/fredamaral/repos/lerianstudio/reporter/go.mod` (read), reporter `.go` tree (read).
**Depends on.** P1-T06 (lib-commons GA pin merged), P2b gate task (reporter observability migration in-repo).
**Acceptance.** Documented confirmation that all five precondition checks pass; the exact lib-commons GA tag is recorded for use in T04; midaz root `go.mod` lib-commons line is GA (not `-beta`).
**Tests.** `grep -rn "lib-commons/v5/commons/\(log\|opentelemetry\|zap\)" --include=*.go` over reporter excl. ast-before → 0 matches; `go build ./...` green in reporter origin on the P2b HEAD commit; `grep "lib-commons/v5 v5" /Users/fredamaral/repos/lerianstudio/midaz/go.mod | grep -v beta` matches.
**Effort.** S (2-3h).
**Risk refs.** R3, R4 (precondition that the observability/lib-commons unification already happened, so the move PR stays mechanical).

---

### P6-T02 — Git-based move: create reporter-manager / reporter-worker / reporter/pkg / reporter/tests + staged seaweedfs config
**Description.** Using a git-based move (NOT `cp -r`, to auto-exclude the git-ignored 35 MB
`docs/codereview/ast-before-*` snapshots — PD-3 fresh import, one `import reporter` commit). Materialize
the new directories under midaz `components/`:
- `components/reporter-manager/` (from reporter `components/manager/`),
- `components/reporter-worker/` (from reporter `components/worker/`),
- `components/reporter/pkg/` (from reporter `pkg/`),
- **`components/reporter/tests/`** (from reporter ROOT `tests/` — the full e2e/integration/property/
  fuzzy/chaos/utils tree, ~75 `.go` files, 61 importing the `.../reporter/tests` prefix; landed as a single
  shared suite so T16/T17 have a real home to point at — preserves all build tags such as
  `//go:build itestkit` and any e2e tags),
- **`components/infra/seaweedfs/`** (ADDITIVE subdir, from reporter `components/infra/seaweedfs/{s3.json,
  init-bucket.sh}`) — staged so T11/T17 and Phase 8 have the SeaweedFS config. This carry is the SOLE thing
  taken out of reporter's `components/infra/`; it is name-collision-safe (midaz has no `components/infra/
  seaweedfs/` today). Do NOT overwrite midaz's `components/infra/docker-compose.yml` or `components/infra/.env`.

Carry over per-component `templates/examples/`, `.env.example`, `.swaggo`/swagger annotations, and `api/`
docs. Do NOT carry: reporter's root `go.mod`/`go.sum` (handled T04), root `Makefile`/`make.sh`/`mk/`
(handled T12), `.github/` (handled T13/T14), `docs/codereview/`, or reporter's `components/infra/
docker-compose.yml` + `components/infra/rabbitmq/` SERVICE definitions (Phase 8 authors the unified infra
compose; only the seaweedfs config subdir is carried). This is the one `import reporter` commit (origin
becomes read-only archive in T20).
**Files (NEW).** `components/reporter-manager/**` (from `reporter/components/manager/**`),
`components/reporter-worker/**` (from `reporter/components/worker/**`),
`components/reporter/pkg/**` (from `reporter/pkg/**`),
`components/reporter/tests/**` (from reporter root `tests/**`),
`components/infra/seaweedfs/{s3.json,init-bucket.sh}` (from `reporter/components/infra/seaweedfs/**`, additive).
**Depends on.** P6-T01.
**Acceptance.** Four reporter dirs exist under `components/` (`reporter-manager`, `reporter-worker`,
`reporter/pkg`, `reporter/tests`); `components/infra/seaweedfs/{s3.json,init-bucket.sh}` present and
midaz's pre-existing `components/infra/docker-compose.yml` is byte-unchanged; `find components/reporter* components/infra/seaweedfs -path '*ast-before*'` returns 0; the move is a single commit; reporter's
`components/infra/` SERVICE defs (compose, rabbitmq) are NOT folded (Phase 8 owns them).
**Tests.** `git log --oneline -1` shows one `import reporter` commit; `du -sh components/reporter*` shows no 35 MB ast snapshot bloat; `ls components/reporter/pkg` lists the reporter pkgs (auth, constant, crypto, ctxutil, datasource, fetcher, itestkit, model, mongodb, multitenant, net, pdf, pongo, postgres, rabbitmq, readyz, redact, redis, seaweedfs, shell, storage, template_builder, templateutils); `ls components/reporter/tests` lists e2e/integration/property/fuzzy/chaos/utils; `test -f components/infra/seaweedfs/s3.json && test -f components/infra/seaweedfs/init-bucket.sh`; `git diff --quiet HEAD~1 -- components/infra/docker-compose.yml` (midaz infra compose untouched).
**Effort.** M (0.5-1d).
**Risk refs.** R-exclude (ast-before snapshots), R21 (preserve fetcher config files unchanged).

---

### P6-T03 — Rewrite Go module-path prefix across all moved files (FIVE prefixes)
**Description.** Scripted, deterministic prefix rewrite over ALL moved `.go` files. CRITICAL ORDERING:
rewrite the longer, more-specific prefixes BEFORE the bare root prefix so no double-rewrite occurs. The
FIVE rewrite rules, in order:
1. `github.com/LerianStudio/reporter/components/manager` → `github.com/LerianStudio/midaz/v3/components/reporter-manager`
2. `github.com/LerianStudio/reporter/components/worker` → `github.com/LerianStudio/midaz/v3/components/reporter-worker`
3. `github.com/LerianStudio/reporter/pkg` → `github.com/LerianStudio/midaz/v3/components/reporter/pkg`
4. `github.com/LerianStudio/reporter/tests` → `github.com/LerianStudio/midaz/v3/components/reporter/tests`
5. bare `github.com/LerianStudio/reporter` (root package, if any) → `github.com/LerianStudio/midaz/v3/components/reporter`

Use `gofmt -r` / `find ... -exec sed` then `goimports -w` to re-sort. Guard the rewrite so it touches only
`.go` import paths and does NOT mangle non-import GitHub URL strings (e.g. `/releases`, `/compare`,
`/discussions`) found in markdown/comments — those are handled in T08 where applicable, not here.
**Files.** All `.go` under `components/reporter-manager/`, `components/reporter-worker/`, `components/reporter/pkg/`, `components/reporter/tests/`.
**Depends on.** P6-T02.
**Acceptance.** `grep -rn "github.com/LerianStudio/reporter" --include=*.go components/reporter*` → 0 matches; all five new prefixes present and resolvable where they applied; imports re-sorted stdlib→external→internal; no double-rewritten path (e.g. no `reporter-manager/components/manager`).
**Tests.** `grep -rc "LerianStudio/reporter" components/reporter* --include=*.go` → 0; `gofmt -l components/reporter*` → empty (formatting clean); `grep -rn "midaz/v3/components/reporter/tests" --include=*.go components/reporter/tests | head` shows the tests prefix resolved.
**Effort.** M (0.5-1d, mostly scripted + spot review of the mongo-driver/itestkit and tests/ edge files).
**Risk refs.** R14 (clean prefix sets up mongo-driver collapse).

---

### P6-T04 — Delete reporter go.mod/go.sum; fold require block into midaz root go.mod
**Description.** Delete reporter's origin `go.mod`/`go.sum` (they never followed in T02; this confirms no
nested module exists under `components/reporter*`). Merge reporter's `require` block into midaz root
`go.mod`. Reconcile every shared dep to the HIGHER same-major version (MVS resolves upward, no code
change): otel, redis, testcontainers, fasthttp, validator, grpc, rabbitmq, fiber, pgx already aligned per
dossier 07. Pin lib-commons to the Phase-1 GA tag recorded in T01, lib-auth `v2.8.0`. Add reporter-unique
deps: `chromedp/chromedp`+`cdproto`, `flosch/pongo2/v6`, `aws-sdk-go-v2/{config,credentials,s3,secretsmanager}`,
`go-sql-driver/mysql`, `go-resty/resty/v2`, `Shopify/toxiproxy/v2`, `testcontainers-go/modules/{mongodb,mysql,postgres,redis}`,
`go-ora/v2`, `denisenkom/go-mssqldb`. Do NOT yet run tidy (T06).
**Files.** `/Users/fredamaral/repos/lerianstudio/midaz/go.mod`, delete `components/reporter*/go.mod` (assert absent).
**Depends on.** P6-T03.
**Acceptance.** No `go.mod` under `components/reporter*`; midaz root `go.mod` contains the reporter-unique deps; lib-commons/lib-auth pinned to the unified GA versions; module remains single-root (`github.com/LerianStudio/midaz/v3`), no `replace`, no `go.work`.
**Tests.** `find components/reporter* -name go.mod -o -name go.sum` → empty; `grep -c "replace " go.mod` → 0; `find . -name go.work` → none.
**Effort.** M (0.5d).
**Risk refs.** R3, R4 (single module, no shim/replace).

---

### P6-T05 — Collapse dual mongo-driver onto midaz v1.17.9
**Description.** midaz uses `go.mongodb.org/mongo-driver v1.17.9` only. Reporter's sole `/v2` usage is in
`components/reporter/pkg/itestkit/infra/mongodb/mongodb_test.go` (`//go:build itestkit`): **2 import lines**
(`go.mongodb.org/mongo-driver/v2/mongo` and `.../v2/mongo/options`) PLUS **4 `mongo.Connect(options.Client().ApplyURI(uri))`
call sites** (the v2 single-arg signature). Repoint the 2 imports to the v1 equivalents
(`go.mongodb.org/mongo-driver/mongo` + `.../mongo/options`) and rewrite the 4 call sites to the v1 ctx-first
signature `mongo.Connect(ctx, opts...)` (plus the matching `Disconnect(ctx)`/`Ping(ctx, ...)` shape). Remove
`go.mongodb.org/mongo-driver/v2` from root `go.mod` require after tidy (T06). Dossier 05's
runtime-BSON-codec fear is overscoped; the collapse is 2 imports + 4 call-site rewrites in one test helper.
**Files.** `components/reporter/pkg/itestkit/infra/mongodb/mongodb_test.go`, `/Users/fredamaral/repos/lerianstudio/midaz/go.mod`.
**Depends on.** P6-T04.
**Acceptance.** `grep -rn "mongo-driver/v2" components/reporter*` → 0; the itestkit mongo helper compiles against v1 driver; all 4 `mongo.Connect` call sites use the v1 ctx-first signature.
**Tests.** `go vet -tags itestkit ./components/reporter/pkg/itestkit/...`; `go test -tags itestkit ./components/reporter/pkg/itestkit/infra/mongodb/...` green against a real Mongo (testcontainers).
**Effort.** S (2-4h).
**Risk refs.** R14 (mongo-driver collapse, validated against real Mongo).

---

### P6-T06 — go mod tidy and build both binaries
**Description.** Run `go mod tidy` at midaz root, resolving toxiproxy/testcontainers/otel/redis skew
upward (MVS). Then build both reporter binaries from their new paths:
`go build ./components/reporter-manager/cmd/app` and `go build ./components/reporter-worker/cmd/app`.
Resolve any residual lib-auth v2.7→v2.8 middleware signature drift surfaced at compile (reporter has **8**
lib-auth import sites — ~1.6x the prior estimate; check auth-middleware constructor/option drift at each).
Resolve any compile fallout from the unified lib-commons GA that P2b's reporter-CI did not catch under the
new module path. NO new shim code — fix at the call site or flag in open_items.
**Files.** `/Users/fredamaral/repos/lerianstudio/midaz/go.mod`, `go.sum`, any reporter `.go` needing a v2.8 auth-API fix (likely `components/reporter-manager/internal/adapters/http/in/middlewares.go` and bootstrap; up to 8 lib-auth call sites).
**Depends on.** P6-T04, P6-T05.
**Acceptance.** `go build ./components/reporter-manager/... ./components/reporter-worker/... ./components/reporter/pkg/...` all succeed; `go.sum` has no `mongo-driver/v2` entry; both binaries produced.
**Tests.** `go build ./components/reporter-manager/cmd/app ./components/reporter-worker/cmd/app` exit 0; `go vet ./components/reporter-manager/... ./components/reporter-worker/... ./components/reporter/pkg/...` clean.
**Effort.** M (0.5-1d, dominated by lib-auth (8 sites)/lib-commons drift fixes).
**Risk refs.** R3, R4 (compiles as one module with no shim).

---

### P6-T07 — pkg/ placement collision audit
**Description.** Confirm the three known collisions (`constant`, `net`, `shell` — all present in both
`midaz/pkg` and reporter `pkg/`) are fully dissolved by the `components/reporter/pkg/` placement: reporter
code imports `.../components/reporter/pkg/constant`, `.../components/reporter/pkg/net/http`,
`.../components/reporter/pkg/shell`; midaz code is untouched and still imports `.../midaz/v3/pkg/constant`
etc. No package was merged, no symbol renamed. Grep-verify no reporter file accidentally imports a
`midaz/pkg/*` package of a colliding name (which would be a silent wrong-package bug post-rename).
**Files.** `components/reporter/pkg/{constant,net,shell}/**` (read), reporter import sites (read).
**Depends on.** P6-T03.
**Acceptance.** Reporter's `constant`/`net`/`shell` imports all resolve under `components/reporter/pkg/`; zero reporter file imports `github.com/LerianStudio/midaz/v3/pkg/{constant,net,shell}` (those are midaz-only).
**Tests.** `grep -rn "midaz/v3/pkg/\(constant\|net\|shell\)" components/reporter* --include=*.go` → 0; `go build ./components/reporter/pkg/...` green.
**Effort.** S (1-2h).
**Risk refs.** R14 (placement correctness).

---

### P6-T08 — Rewrite non-Go module-path string references (incl. doc-URL guard)
**Description.** Rewrite reporter's non-Go references to the old module path / old layout: swagger
`@host`/`@BasePath` annotations and generated `api/docs.go` (host stays `localhost:4005`/`:4006` — ports
unchanged, so only the title/module refs and any path-derived strings change), `.golangci.yml` path
filters, `.ignorecoverunit` glob patterns (rewriting the moved tests/ + pkg/ paths), and any `.mk`/Makefile
path filters that reference reporter's old `components/manager`/`components/worker` paths. Doc/markdown URL
strings like `https://github.com/LerianStudio/reporter/{releases,compare,discussions}` are intentionally
LEFT untouched here unless they ship inside a component (they are historical references, not module
imports); the Dockerfile OCI source label is handled by T09/T10, NOT here. Reporter's
`.github/ISSUE_TEMPLATE/config.yaml` module-path ref is dropped with the rest of reporter `.github/` (T14),
not rewritten. Regenerate swagger docs if the title/module embed changed.
**Files.** `components/reporter-manager/api/docs.go`, `components/reporter-worker/` swagger if present,
`components/reporter-manager/.golangci.yml` (or root if folded), `.ignorecoverunit` patterns,
`components/reporter-*/.swaggo`.
**Depends on.** P6-T03.
**Acceptance.** No stale `LerianStudio/reporter` or old `components/manager`/`components/worker` path strings in non-Go config that ships with the components (excluding Dockerfile OCI label handled by T09/T10, and intentionally-preserved historical doc URLs); swagger regenerates with correct module embed.
**Tests.** `make reporter-manager COMMAND=generate-docs` (post-T12) produces clean swagger; `grep -rn "LerianStudio/reporter" components/reporter* --include='*.yml' --include='*.yaml' --include='*.swaggo' --include='*.go' | grep -v Dockerfile` over non-Go config that ships → 0 (excluding archived `.github/`).
**Effort.** S (2-4h).
**Risk refs.** R25 (path-ref staleness).

---

### P6-T09 — Rewrite manager Dockerfile (repo-root context, distroless-nonroot, OCI label)
**Description.** Move/rewrite the manager Dockerfile to `components/reporter-manager/Dockerfile`. Keep
build context repo-root. Replace the SELECTIVE COPY pattern (`COPY pkg/ pkg/` + `COPY components/manager/`)
with `COPY . .` — mirroring ledger's Dockerfile. RATIONALE (state in the Dockerfile comment or PR): reporter
imports ZERO midaz packages, so Go package-level dead-code elimination keeps uncopied midaz packages out of
the build graph; `COPY . .` is the only FUTURE-PROOF option (the selective-COPY alternative would also need
`go.mod`/`go.sum` from root AND would silently break the moment any future reporter code imports a midaz
`pkg`). Build target becomes `./components/reporter-manager/cmd/app/main.go`. Harmonize the final image from
`alpine:3.23`+hand-rolled-nonroot to `gcr.io/distroless/static-debian12:nonroot` (manager is REST-only, no
Chromium — dossier 08 §3.4). Preserve EXPOSE 4005. Distroless has no shell, so DROP the `wget /health`
HEALTHCHECK entirely and rely on the orchestrator probe — this MIRRORS ledger's distroless Dockerfile,
which has no HEALTHCHECK (do NOT introduce an embedded probe binary; ledger does not). **Rewrite the OCI
source label**: `LABEL org.opencontainers.image.source="https://github.com/LerianStudio/midaz"` (the origin
points at `LerianStudio/reporter` today — a stale pointer to a soon-to-be-archived repo).
**Files (NEW/moved).** `components/reporter-manager/Dockerfile`.
**Depends on.** P6-T06.
**Acceptance.** `docker build -f components/reporter-manager/Dockerfile -t midaz-reporter-manager:test .` (context repo-root) succeeds; resulting image runs and serves `/health` on 4005 via orchestrator probe; image is distroless-nonroot with no HEALTHCHECK directive; `grep "image.source" components/reporter-manager/Dockerfile` shows `LerianStudio/midaz`.
**Tests.** `docker build` exit 0; `docker run` + orchestrator-probe-equivalent `curl localhost:4005/health` 200; `grep -c "LerianStudio/reporter" components/reporter-manager/Dockerfile` → 0.
**Effort.** M (0.5d).
**Risk refs.** R12 (image naming for downstream helm).

---

### P6-T10 — Rewrite worker Dockerfile (repo-root context, stays fat alpine+Chromium, OCI label)
**Description.** Move/rewrite the worker Dockerfile to `components/reporter-worker/Dockerfile`. Same COPY
strategy as T09 (`COPY . .`, repo-root context, build target `./components/reporter-worker/cmd/app/main.go`).
Worker MUST stay fat alpine + Chromium — preserve ALL of: `apk add chromium nss freetype harfbuzz
ttf-freefont ttf-dejavu ttf-liberation font-noto font-noto-cjk font-noto-emoji font-noto-extra`, `ENV
CHROME_BIN=/usr/bin/chromium-browser`, `CHROME_PATH`, `CHROMEDP_SKIP_CHROMEDP_DOWNLOAD=true`,
`CHROMEDP_USE_SYSTEM_CHROME=true`, `HOME=/app`, the non-root appuser with `/app/.chromium`, EXPOSE 4006,
the `wget /health` HEALTHCHECK (alpine keeps a shell). Do NOT attempt distroless (R20 — Chromium needs the
alpine userland). **Rewrite the OCI source label**: `LABEL
org.opencontainers.image.source="https://github.com/LerianStudio/midaz"` (origin points at
`LerianStudio/reporter` today).
**Files (NEW/moved).** `components/reporter-worker/Dockerfile`.
**Depends on.** P6-T06.
**Acceptance.** `docker build -f components/reporter-worker/Dockerfile -t midaz-reporter-worker:test .` succeeds; image launches headless Chromium and renders a test PDF; image remains alpine+Chromium (NOT distroless); `grep "image.source" components/reporter-worker/Dockerfile` shows `LerianStudio/midaz`.
**Tests.** `docker build` exit 0; container starts, `/health` 200 on 4006; a smoke render produces a non-empty PDF; `grep -c "LerianStudio/reporter" components/reporter-worker/Dockerfile` → 0.
**Effort.** M (0.5d).
**Risk refs.** R20 (worker fat-image stays alpine).

---

### P6-T11 — Fold reporter compose into midaz topology; REPOINT services onto infra-network; reconcile shared infra; hand SeaweedFS/KEDA to Phase 8
**Description.** Reconcile reporter's per-component `docker-compose.yml` into midaz's topology. GROUND TRUTH
correction: reporter's composes attach each SERVICE to `reporter-infra-network` + `reporter-{manager,
worker}-network`; `infra-network` is declared `external: true` in the top-level `networks:` block but is
NOT in any service's `networks:` list — a DANGLING DECLARATION, not a working attachment. "Keep the gift"
is a no-op. This task must:
- **(a) REPOINT each service's `networks:` block** from `reporter-infra-network`/`reporter-{manager,worker}-network`
  to midaz's shared `infra-network`. Remove the now-unused `reporter-infra-network`/`reporter-manager-network`/
  `reporter-worker-network` declarations (or justify retaining any).
- **(b) Reconcile ALL bundled backing services to midaz's shared infra.** Reporter bundles
  `reporter-postgres`, `reporter-mongodb`, `reporter-valkey`, `reporter-rabbitmq:4.0` in its
  `components/infra/docker-compose.yml` (NOT carried — Phase 8 owns the infra compose). The reporter
  composes/env must point at the SHARED midaz services: `midaz-mongo` (rs0), `midaz-valkey`,
  `midaz-rabbitmq:4.1.3` (drop reporter's 4.0 override; midaz wins). Where reporter env names a
  `reporter-mongo`/`reporter-valkey` host, repoint to the midaz shared host. (Reporter's Postgres is used
  only by the fetcher as an EXTERNAL datasource target, not as reporter's own store — confirm and leave
  fetcher connection config untouched, R21.)
- **(c) SeaweedFS/KEDA handoff.** Net-new infra (SeaweedFS `chrislusf/seaweedfs:4.05` S3, KEDA
  `ghcr.io/kedacore/keda:2.16.0`) is owned by Phase 8 — Phase 6 must NOT add the SERVICE definitions to
  midaz's infra compose. The seaweedfs CONFIG files were carried in T02 to `components/infra/seaweedfs/`.
  Leave the worker compose referencing a SeaweedFS service with a clear Phase-8 TODO marker. T17 brings up
  a local-only SeaweedFS from the carried config for verification.
- **(d) Port-collision check.** Verify 4005/4006 do not collide with the LIVE component ports at execution
  time. Enumerate from the actual running topology — `components/{crm,infra,ledger}` today (crm:4003 may
  still be present; tracer:4020 only if P5 landed). Do NOT hardcode a `tracer`/`crm` assumption; read the
  live composes.

**Files (NEW).** `components/reporter-manager/docker-compose.yml`, `components/reporter-worker/docker-compose.yml`;
`components/infra/seaweedfs/{s3.json,init-bucket.sh}` (carried in T02, referenced here as the source for the
worker's local SeaweedFS marker).
**Depends on.** P6-T06.
**Acceptance.** Each reporter service's `networks:` block lists `infra-network` (asserted against the SERVICE
block, not the top-level declaration); `reporter-infra-network`/`reporter-{manager,worker}-network` removed
or justified; reporter composes/env point at shared `midaz-mongo`/`midaz-valkey`/`midaz-rabbitmq:4.1.3`,
not reporter's bundled services; SeaweedFS/KEDA service definitions are explicitly deferred to Phase 8 with
a marker, not silently dropped; `components/infra/seaweedfs/{s3.json,init-bucket.sh}` present; no port
collision against the LIVE component set.
**Tests.** `docker compose -f components/reporter-manager/docker-compose.yml config` validates; a script that
parses each service's `networks:` block confirms `infra-network` is attached to the SERVICE (not merely
declared); `grep -L "reporter-infra-network" components/reporter-*/docker-compose.yml` (no service still on the old net); a port-collision check enumerating ports from the live `components/*/docker-compose.yml` passes.
**Effort.** M (0.5-1d).
**Risk refs.** R21 (fetcher reachability over infra-network preserved), R20.

---

### P6-T12 — Fold reporter Makefile into midaz component-delegation pattern (live component list)
**Description.** Reporter's root Makefile + `make.sh` + `mk/` are NOT carried as-is. Create
`components/reporter-manager/Makefile` and `components/reporter-worker/Makefile` from midaz's
component-Makefile template (keyed off `SERVICE_NAME` + `MIDAZ_ROOT`, `include $(MIDAZ_ROOT)/mk/*.mk`).
ADD `reporter-manager` and `reporter-worker` to midaz's root `Makefile` `COMPONENTS` list by reading the
LIVE value and appending — today `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)`, so the result is
`COMPONENTS := $(INFRA_DIR) $(CRM_DIR) $(LEDGER_DIR?) $(REPORTER_MANAGER_DIR) $(REPORTER_WORKER_DIR)` as it
exists at execution (do NOT hardcode `ledger tracer reporter-manager reporter-worker`; tracer/crm presence
depends on P5/P3 landing). Migrate reporter's genuinely-useful Make targets (`generate-docs`,
`generate-mocks`, multi-tenant test target) into the component Makefiles. The root-level Makefile/mk
consolidation itself is owned by Phase 8 — Phase 6 only wires the two reporter components INTO whatever
component-delegation pattern exists, and flags any root-Makefile change needed.
**Files (NEW).** `components/reporter-manager/Makefile`, `components/reporter-worker/Makefile`; edit `/Users/fredamaral/repos/lerianstudio/midaz/Makefile` `COMPONENTS` list (append, do not replace).
**Depends on.** P6-T06.
**Acceptance.** `make reporter-manager COMMAND=build` and `make reporter-worker COMMAND=build` succeed from repo root; root `COMPONENTS` list includes both reporter components AND retains whatever was already there; no reporter root Makefile/make.sh carried over.
**Tests.** `make reporter-manager COMMAND=build`; `make reporter-worker COMMAND=build`; both exit 0; `grep "COMPONENTS" Makefile` shows the appended reporter dirs without dropping the pre-existing entries.
**Effort.** M (0.5d).
**Risk refs.** R25.

---

### P6-T13 — Extend midaz build.yml + CI filter_paths for the two reporter components (RELATIVE to live components)
**Description.** Edit midaz's `.github/workflows/build.yml` to ADD `components/reporter-manager` and
`components/reporter-worker` to `filter_paths` — UNIONING onto whatever is already there. GROUND TRUTH:
the live `filter_paths` is `components/crm` + `components/ledger` (NO tracer; crm present). The expected
fan-out is therefore `<current components> + reporter-manager + reporter-worker` = EXACTLY +2 images
(`midaz-reporter-manager`, `midaz-reporter-worker`). Do NOT hardcode a `midaz-tracer` image or a
`ledger tracer` literal — tracer becomes a component only if/when P5 lands, which P6 does NOT depend on.
The acceptance gate asserts the post-change set equals the pre-change set PLUS exactly the two reporter
images. The shared workflow at `path_level: '2'` auto-emits one image per level-2 dir (verified: dossier 08
§4.3). `components/reporter/pkg` and `components/reporter/tests` are NOT in `filter_paths` (no `cmd/`, no
Dockerfile) but `components/reporter/pkg` IS added to `shared_paths` so a pkg change rebuilds dependents.
Set/keep `app_name_prefix: "midaz"` → images become `midaz-reporter-manager` / `midaz-reporter-worker`
(renamed from reporter's `reporter-*` prefix). ADD to `helm_values_key_mappings`:
`{"midaz-reporter-manager":"manager","midaz-reporter-worker":"worker"}` (union onto the existing
`{"midaz-crm":"crm","midaz-ledger":"ledger"}`); `helm_chart: "midaz"`; enable BOTH DockerHub+ghcr (reporter
was ghcr-only — harmonize to most-permissive). Mirror the union into `go-combined-analysis.yml` and
`pr-security-scan.yml` filter_paths; drop `go_private_modules` (reporter never needed it; confirm with clean
`go mod download`). Add `reporter` to `pr-validation.yml` `pr_title_scopes`. The shared-workflow VERSION
pin is owned by **P8-T18** (CI harmonization) — Phase 6 uses whatever version is current and flags any
coverage-gate behavior skew between midaz's pinned shared-workflow version and reporter's.
**Files.** `/Users/fredamaral/repos/lerianstudio/midaz/.github/workflows/build.yml`, `go-combined-analysis.yml`, `pr-security-scan.yml`, `pr-validation.yml`.
**Depends on.** P6-T09, P6-T10.
**Acceptance.** `filter_paths` includes both reporter components in addition to the pre-existing entries; `path_level: '2'` yields the pre-existing image set PLUS exactly two reporter images on a tag touching both; image names are `midaz-reporter-{manager,worker}`; `helm_values_key_mappings` retains crm/ledger AND adds the two reporter keys; both registries enabled; `go_private_modules` removed and `go mod download` clean.
**Tests.** A dry-run/`act` or a real beta tag push fanning out to `<current component images> + midaz-reporter-manager + midaz-reporter-worker` (assert the DELTA is exactly +2, do NOT assert an absolute list containing `midaz-tracer`); `go mod download` with no netrc/token succeeds.
**Effort.** M (0.5-1d incl. a real fan-out validation).
**Risk refs.** R12 (image rename lockstep with helm/gitops — coordinated in T19).

---

### P6-T14 — Delete reporter origin CI/release/dependabot duplicates
**Description.** First-class deletion task. The reporter `.github/` was never carried into midaz (T02);
this task asserts that and removes any reporter-specific workflow/config that DID leak in, and confirms
midaz does NOT gain a duplicate `release.yml`/`.releaserc.yml`/`.releaserc.hotfix.yml`/`dependabot.yml`/
`labeler.yml`/`CODEOWNERS` from reporter. Reporter loses its independent semantic-release pipeline and
rides midaz's single repo-wide version. Reporter's `.releaserc.hotfix.yml` and
`@saithodev/semantic-release-backmerge` are adopted at the MIDAZ root by Phase 8, not duplicated per
component. Archive reporter's `CHANGELOG.md` under `docs/monorepo/legacy-changelogs/reporter-CHANGELOG.md`
(do NOT concatenate into midaz's changelog).
**Files.** Delete any leaked `components/reporter*/.github/**`, `components/reporter*/.releaserc*.yml`; NEW `docs/monorepo/legacy-changelogs/reporter-CHANGELOG.md`.
**Depends on.** P6-T13.
**Acceptance.** No reporter-origin workflow/releaserc files under `components/reporter*`; midaz has exactly one `release.yml`/`.releaserc.yml`; reporter CHANGELOG archived not merged.
**Tests.** `find components/reporter* -name '.releaserc*' -o -path '*.github*'` → empty; `ls docs/monorepo/legacy-changelogs/reporter-CHANGELOG.md` exists.
**Effort.** S (1-2h).
**Risk refs.** R24 (single repo-wide release model).

---

### P6-T15 — Coverage-gate backfill audit for moved reporter code
**Description.** The midaz monorepo CI enforces an 85% per-directory coverage hard-fail gate
(`coverage_threshold: 85`, `fail_on_coverage_threshold: true` in `go-combined-analysis.yml`). Reporter's
own threshold (its `.ignorecoverunit` exclusions) may differ. Run unit tests for both components under
midaz's coverage tooling, measure per-directory coverage against 85%, and produce a backfill list for any
directory that drops below the gate. Port reporter's `.ignorecoverunit` exclusion patterns (rewriting the
paths to the new layout — done in T08) so legitimately-excluded files (generated docs, mocks, itestkit,
the `components/reporter/tests/` suites) are not counted. Backfill only the directories that fail; do NOT
write tests speculatively. Flag any coverage-gate behavior skew arising from the shared-workflow version
mismatch between midaz's pin and reporter's prior pin (the version pin itself is P8-T18's call).
**Files.** `components/reporter-manager/**`, `components/reporter-worker/**`, `.ignorecoverunit`, NEW test files only where a real gap fails the gate.
**Depends on.** P6-T06.
**Acceptance.** Every reporter component directory either passes 85% or is on the documented backfill list with a test plan; no false failures from un-ported exclusion patterns; `components/reporter/tests/` and `components/reporter/pkg/itestkit/**` correctly excluded.
**Tests.** `make reporter-manager COMMAND=test-unit` + coverage report ≥85% per dir (or documented exclusions); same for worker.
**Effort.** M-L (1-3d depending on real gaps; reporter is heavily tested already — likely M).
**Risk refs.** R11 (coverage gate on moved code).

---

### P6-T16 — Integration test: fetcher external-DB reachability preserved (R21)
**Description.** Reporter's worker fetcher connects at runtime to EXTERNAL customer DBs (incl.
`midaz_onboarding`/`midaz_transaction`) configured via `/v1/management/connections`. Co-location must NOT
change network reachability or credential handling. Use the CARRIED, RENAMED suite at
`components/reporter/tests/e2e/infra_datasources_test.go` (moved in T02, prefix-rewritten in T03 — NOT
authored from scratch) and adapt it to run under midaz's tooling: it spins testcontainers MySQL + Postgres,
configures a datasource connection, runs the fetcher from the worker on `infra-network`, and asserts the
extraction succeeds with the SAME credential/connection-config shape as origin. Preserve the file's build
tags through the move. Confirm the fetcher's connection-string construction (in
`components/reporter/pkg/datasource/types.go`, `components/reporter/pkg/fetcher/types.go`) is byte-identical
to origin post-rename. This task depends on T11 because the worker must be attached to the SHARED
`infra-network` (repointed there in T11) for the reachability assertion to mean anything.
**Files.** `components/reporter/tests/e2e/infra_datasources_test.go` (carried + rewritten; run under midaz tooling), `components/reporter/pkg/{datasource,fetcher}/types.go` (read/assert unchanged).
**Depends on.** P6-T06, P6-T11.
**Acceptance.** The carried `infra_datasources_test.go` passes under midaz tooling against testcontainers MySQL + Postgres; the worker reaches the datasource over the SHARED `infra-network`; connection-config and credential handling unchanged from origin.
**Tests.** `go test` (with the suite's build tags) on `components/reporter/tests/e2e/infra_datasources_test.go` passes against testcontainers MySQL+Postgres; `git diff` on `pkg/{datasource,fetcher}/types.go` shows only the import-prefix rewrite, no logic change.
**Effort.** M (0.5-1d).
**Risk refs.** R21 (fetcher external-DB reachability/credentials).

---

### P6-T17 — End-to-end PDF pipeline verification (manager → RabbitMQ → worker → SeaweedFS)
**Description.** Stand up the full local stack (`make up` extended with the two reporter components +
shared infra + the env wiring from T21; SeaweedFS service authoring is Phase 8 but the worker needs it —
boot a local-only `chrislusf/seaweedfs:4.05` from the carried `components/infra/seaweedfs/{s3.json,
init-bucket.sh}` for this verification, flagging the Phase-8 dependency). Drive the end-to-end pipeline:
POST a report request to manager:4005 → manager produces to the `reporter.generate-report.exchange` →
routing key `reporter.generate-report.key` → queue `reporter.generate-report.queue` → worker consumes →
fetches a datasource → renders a pongo2 template via headless Chromium → writes a PDF to SeaweedFS → updates
Mongo status → notifies completion. Use the CARRIED, RENAMED suite at
`components/reporter/tests/e2e/template_report_validation_test.go` (moved T02, rewritten T03) as the
verification mechanism — NOT a from-scratch rewrite. Assert a non-empty PDF lands in SeaweedFS and the
report doc reaches a terminal success status. This is the definitive "clean entry" verification and the
move-verified gate that P9's `PHASE-reporter-move` resolves to.
**Files.** local compose stack; `components/reporter/tests/e2e/template_report_validation_test.go` (carried + rewritten); local-only SeaweedFS from `components/infra/seaweedfs/`.
**Depends on.** P6-T09, P6-T10, P6-T11, P6-T21.
**Acceptance.** A report request submitted to manager produces a rendered PDF in SeaweedFS and a success status in Mongo; manager `/health`+`/readyz` green; worker consumes `reporter.generate-report.queue` and reports healthy on 4006.
**Tests.** The carried `template_report_validation_test.go` passes end-to-end against the local stack (real exchange/queue/key names: `reporter.generate-report.{exchange,queue,key}`); manual `curl` POST → poll status → fetch PDF.
**Effort.** L (1-2d, full-stack bring-up + Chromium render).
**Risk refs.** R20, R21, R14.

---

### P6-T18 — Update STRUCTURE.md and monorepo docs for the two reporter components (live topology)
**Description.** STRUCTURE.md is stale (R25). Update it (and `AGENTS.md`/`llms-full.txt` component lists if
they enumerate components) to ADD the two reporter components and the shared reporter trees, reflecting the
LIVE topology at execution — do NOT assert a final fixed list that names `tracer` if P5 has not landed or
omits `crm` if P3 has not landed. Concretely: the component list must include `components/reporter-manager`,
`components/reporter-worker`, `components/reporter/pkg`, `components/reporter/tests`, plus whatever was
already there (`crm`/`ledger`/`infra`, and `tracer` only if present). Document that reporter is two deploy
units (:4005 manager, :4006 worker), that reporter's shared library lives at `components/reporter/pkg` and
its shared suites at `components/reporter/tests`, that SeaweedFS config is staged at
`components/infra/seaweedfs/` and SeaweedFS+KEDA services are reporter-only net-new infra owned by Phase 8.
Documentation debt only — no code. (P9's final doc sweep re-verifies the complete topology after ALL four
moves land; P6 only adds the reporter facts.)
**Files.** `STRUCTURE.md`, `AGENTS.md` / `llms-full.txt` (component sections if present).
**Depends on.** P6-T13.
**Acceptance.** STRUCTURE.md lists the two reporter components + `components/reporter/{pkg,tests}` accurately alongside the pre-existing components; no stale reporter-as-separate-repo references; no hardcoded assertion of a `tracer` component that may not exist yet.
**Tests.** Manual doc review; `grep -n "reporter" STRUCTURE.md` shows the two-component layout plus the shared pkg/tests trees.
**Effort.** S (1-2h).
**Risk refs.** R25.

---

### P6-T19 — Out-of-repo Helm/gitops/APIDog lockstep coordination
**Description.** Coordination task with cross-team blast radius (R12). The image rename
`reporter-{manager,worker}` → `midaz-reporter-{manager,worker}` and the move to `helm_chart: "midaz"` will
break ArgoCD sync unless the external Helm chart `midaz`, the `midaz-firmino-gitops` repo
`yaml_key_mappings`, and the APIDog e2e definitions extend in lockstep. Produce the exact set of
helm-values keys and gitops yaml-key-mappings the consolidated `build.yml` will emit (from T13), confirm
ownership, and sequence the chart/gitops PRs to land WITH (not after) the build.yml change. A co-located
component whose images build but whose helm/gitops mappings are absent builds-but-never-deploys. Reporter's
old `reporter` chart and `reporter-{manager,worker}` mappings are removed in the same lockstep.

**Owner-unavailable / chart-rejected fallback (CGap2).** If the Helm/gitops/APIDog owner does NOT sign off
in lockstep, OR the chart PR is rejected: (a) do NOT merge T13's `app_name_prefix`/`helm_chart` rename to
`midaz` into the default branch — keep it on the phase branch; (b) the images may still build under the
RENAMED prefix only behind a feature/preview tag, never promoted to the gitops-tracked tag; (c) the move
PR (T02–T18) can still land because it is deploy-neutral until the prefix flips; (d) record the blocker as
a residual open item and re-attempt lockstep before T20 (origin archive) — T20 is HARD-gated on T19, so an
unresolved lockstep BLOCKS archiving the origin, preserving rollback. This is the likeliest real-world
stall; treat it as an expected branch, not an exception.
**Files.** (out-of-repo) `midaz` Helm chart values, `midaz-firmino-gitops` mappings, APIDog e2e — coordinated, not edited here; produce a coordination checklist artifact.
**Depends on.** P6-T13.
**Acceptance.** Helm `midaz` chart has `manager`/`worker` value keys mapped to `midaz-reporter-{manager,worker}` images; gitops `yaml_key_mappings` updated; old `reporter` chart entries removed; sequencing confirmed with the deploy owner — OR the owner-unavailable fallback is invoked and recorded, with the prefix flip held off the default branch.
**Tests.** ArgoCD dry-run/diff against the updated chart shows the two reporter components resolve; a beta-tag fan-out (T13) plus updated gitops produces a deployable manifest. If fallback invoked: confirm the default branch retains the old prefix and no broken gitops manifest is produced.
**Effort.** M (0.5-1d, gated on cross-team availability).
**Risk refs.** R12 (helm/gitops lockstep).

---

### P6-T20 — Mark reporter origin repo read-only archive
**Description.** Per PD-3 (fresh import; origin repos become read-only archives), after the move is
verified end-to-end (T17) and deploy lockstep is set OR fallback-resolved (T19): coordinate archiving
`github.com/LerianStudio/reporter` (branch protection → read-only / GitHub "Archive repository"), with a
final README note pointing to `midaz/components/reporter-{manager,worker}`. No further commits to the
origin. This makes the migration irreversible-by-default and prevents drift between origin and monorepo.
This is the second half of P9's `PHASE-reporter-move` resolution (origin archived). Do NOT archive if T19's
lockstep is unresolved (the fallback branch holds T20 open until deploy is coherent).
**Files.** (out-of-repo) reporter repo settings + a final README pointer commit.
**Depends on.** P6-T17, P6-T19.
**Acceptance.** `github.com/LerianStudio/reporter` is archived/read-only; its README points to the monorepo location; no new commits possible.
**Tests.** GitHub repo shows "archived" badge; a push to origin is rejected.
**Effort.** S (1h, mostly coordination).
**Risk refs.** R-history (PD-3 fresh-import archive discipline).

---

### P6-T21 — Wire reporter env-var surface into midaz `make set-env`
**Description.** Reporter's two components carry a substantial env surface in their `.env.example` files
that is NOT reflected in midaz's `make set-env` env-generation flow today. Without this, local `make up`
(and T17's full-stack bring-up) cannot start the reporter components with valid config. Reconcile the
reporter env vars into midaz's env-generation: worker needs `OBJECT_STORAGE_*` (SeaweedFS S3 endpoint/region/
access-key/secret/bucket/path-style/ssl), `PDF_POOL_WORKERS`/`PDF_TIMEOUT_SECONDS`, `FETCHER_ENABLED`,
`MULTI_TENANT_ENABLED`, `RABBITMQ_EXCHANGE`/`RABBITMQ_GENERATE_REPORT_{QUEUE,KEY}`/`RABBITMQ_DLQ_QUEUE`, and
shared `MONGO_*`/`RABBITMQ_*`; manager additionally needs `SERVER_PORT`/`SERVER_ADDRESS`, `REDIS_*`,
`FETCHER_URL`, `SWAGGER_*`, `PLUGIN_AUTH_*`, `APP_ENC_KEY`. Point the shared connection vars (MONGO/RABBITMQ/
REDIS) at the midaz shared infra hosts (consistent with T11's network/service repointing). Component-local
`.env.example` files were carried in T02; this task ensures `make set-env` generates working `.env` files
for both reporter components from those templates, matching midaz's env-generation pattern. The root
Makefile/mk set-env consolidation is Phase 8's; Phase 6 only wires the two reporter components into
whatever `set-env` pattern exists and flags root-level changes.
**Files.** `components/reporter-manager/.env.example`, `components/reporter-worker/.env.example` (carried), midaz `make set-env` wiring (component env-generation), flag any root-Makefile change.
**Depends on.** P6-T11.
**Acceptance.** `make set-env` produces valid `.env` files for both reporter components from their `.env.example` templates; shared-infra connection vars point at midaz's shared services (not reporter's bundled ones); the generated env is sufficient for T17's local bring-up.
**Tests.** `make set-env` then `test -f components/reporter-manager/.env && test -f components/reporter-worker/.env`; `grep -E "OBJECT_STORAGE_BUCKET|RABBITMQ_GENERATE_REPORT_QUEUE" components/reporter-worker/.env` resolves; the T17 stack starts both components without missing-env failures.
**Effort.** M (0.5d).
**Risk refs.** R21, R20.

---

## Exit criteria

1. `go build ./components/reporter-manager/cmd/app ./components/reporter-worker/cmd/app ./components/reporter/pkg/...` all succeed under the single root `go.mod` (no `replace`, no `go.work`, no nested module).
2. Zero `github.com/LerianStudio/reporter` references in any `.go` file under `components/reporter*` (all five prefixes rewritten, including `.../reporter/tests`); zero `lib-commons/v5/commons/{log,opentelemetry,zap}` imports (P2b precondition holds); zero `mongo-driver/v2` imports.
3. `docs/codereview/ast-before-*` did NOT enter the monorepo (git-based move).
4. Two Docker images build from repo-root context: `midaz-reporter-manager` (distroless-nonroot, :4005, no HEALTHCHECK) and `midaz-reporter-worker` (fat alpine+Chromium, :4006); both Dockerfile OCI `image.source` labels point at `LerianStudio/midaz`, zero `LerianStudio/reporter` references remain in either Dockerfile.
5. `build.yml` fan-out emits the pre-existing image set PLUS exactly two reporter images at `path_level: 2` (delta = +2, NOT an absolute list assuming `midaz-tracer`); CI filter_paths/security/analysis unioned; `helm_values_key_mappings` retains crm/ledger and adds the two reporter keys; `go_private_modules`/github_token machinery confirmed unneeded by a clean `go mod download`.
6. End-to-end PDF pipeline (manager → `reporter.generate-report.{exchange,queue,key}` → worker → SeaweedFS → Mongo status) renders a non-empty PDF, verified by the carried `components/reporter/tests/e2e/template_report_validation_test.go`.
7. Fetcher reaches external customer DBs (MySQL+Postgres incl. midaz_onboarding/transaction) over the SHARED `infra-network` with unchanged credential handling (R21), verified by the carried `components/reporter/tests/e2e/infra_datasources_test.go`.
8. 85% coverage gate passes (or documented backfill plan) for both components; `components/reporter/{tests,pkg/itestkit}` correctly excluded.
9. Reporter composes attach each SERVICE to midaz's shared `infra-network` and shared backing services (Mongo/Valkey/RabbitMQ:4.1.3); `make set-env` generates working reporter env files.
10. Helm `midaz` chart + gitops + APIDog updated in lockstep (or the owner-unavailable fallback recorded and the prefix flip held off the default branch); reporter origin repo archived read-only (only after lockstep is coherent).
11. STRUCTURE.md reflects the two reporter components + `components/reporter/{pkg,tests}` alongside the live component set.

## Open items / flags

- **P6 is independent of P5 (tracer) and P3 (crm).** Every component-list / fan-out / port-collision /
  STRUCTURE.md task reads the LIVE state and appends the two reporter components; nothing here assumes
  `components/tracer` exists or that `components/crm` is gone. If P6 lands before P5, there is simply no
  `midaz-tracer` image in the fan-out — that is correct, not a failure.
- **SeaweedFS + KEDA ownership boundary.** The worker hard-depends on SeaweedFS at runtime, but the net-new
  infra SERVICE definitions are locked to Phase 8. Phase 6 carries the seaweedfs CONFIG (`s3.json`,
  `init-bucket.sh`) into `components/infra/seaweedfs/` (T02, additive, non-clobbering) and uses a local-only
  SeaweedFS for T17 verification, with explicit Phase-8 TODO markers in the worker compose. If Phase 8
  slips, the worker has no production object store. Flag the cross-phase coupling; do NOT add SeaweedFS/KEDA
  service definitions to midaz infra compose in Phase 6.
- **Reporter's bundled non-RabbitMQ infra** (`reporter-postgres`/`reporter-mongodb`/`reporter-valkey` in
  reporter's `components/infra/docker-compose.yml`, NOT carried) must be reconciled to midaz's shared
  services in T11/T21, not just RabbitMQ. Confirm reporter's own Postgres is only a fetcher datasource
  target (R21), not reporter's own store.
- **lib-commons GA tag is Phase-1's call.** Proxy shows `v5.2.1` (5.2 GA) plus `v5.3.x`/`v5.4.x` GA. Phase 6
  consumes "the Phase-1-pinned GA" (P1-T06) as a variable (T01 records it). The live midaz `go.mod` pins a
  BETA (`v5.2.0-beta.12`) today, so T01's gate is currently UNMET — P6 cannot start until P1-T06 lands the
  GA pin. If P2b migrated reporter to a DIFFERENT lib-commons tag than P1 pinned for midaz, T06 will surface
  the skew — resolve by re-pinning, never by `replace`.
- **lib-auth v2.7→v2.8 middleware drift** (8 import sites, not 5) may need real call-site fixes in T06; if
  the signature change is non-trivial it is still a clean fix, not a shim.
- **Shared-workflow version pin** is Phase 8's (P8-T18) decision; midaz and reporter were on different
  pinned shared-workflow versions, and a coverage-gate behavior skew between them could surface in T15 —
  flagged, not owned here.
- **Swagger host annotations** stay `localhost:4005`/`:4006` (ports unchanged), so T08 is module-ref/title
  only — confirm no port-derived strings changed. Historical GitHub doc URLs (`/releases`, `/compare`,
  `/discussions`) are intentionally NOT rewritten by the T03 import-rewrite (they are not module imports).
- **Phase 8 owns** the root Makefile/mk consolidation, the single `.golangci.yml` floor, the shared-workflow
  version pin (P8-T18), `.releaserc` adoption (hotfix + backmerge), and the unified infra compose superset
  (incl. authoring the SeaweedFS/KEDA SERVICE definitions). Phase 6 wires the two reporter components INTO
  those structures and flags root-level changes; it does not own them.


---

<a id="phase-7"></a>

# Phase 7 — go.mod Unification & Final Tidy (23 tasks)

_Verbatim from `docs/monorepo/plan/P7.md`._


**Phase ID:** P7
**Objective:** With all four moves landed in the single root module (`github.com/LerianStudio/midaz/v3`),
unify the dependency graph into one coherent `go.mod`/`go.sum`: merge the incoming `require` blocks,
drop the reporter `toolchain` directive, drop fees' stale `lib-observability` beta, collapse duplicate
indirects, MVS-resolve all same-major third-party skew upward, reconcile the pre-existing `validator/v9`
second-major case, and `go mod tidy`. Then prove the whole tree is green
(`make lint && test-unit && test-integration && sec`), stand up godog in CI (R15), re-prove the PD-5
fee-on-revert / pending-cancel REFUND third rail in the unified module (P7-T18), and confirm no transitive
private Lerian module remains (clean `go mod download`) so the `github_token` machinery can be deleted in Phase 8.

**Topology (locked, PD-1):** single root `go.mod`. NO `go.work`, NO `replace`, NO temporary fences.
Any step that reaches for a compat layer is a plan defect — find the clean way or escalate in open_items.

**Phase numbering (LOCKED — PD-3 / DAG-1):** the four moves use this scheme everywhere in this file:
- **P3 = crm embed** (already in-module; no dep event — only contributes the CRM shim/dead-code deletions owned in P3).
- **P4 = plugin-fees** service-collapse into ledger (fee engine + fee-on-revert behavior built and first-proven here).
- **P5 = tracer** co-location (v4→v5 tracer migration done in-repo first; ships godog BDD; audit hash-chain).
- **P6 = reporter-manager + reporter-worker** co-location (ships the `toolchain` directive, chromedp/cdproto,
  dual mongo-driver v1+v2, CEL/Cron/SQL-driver fan-out).

Execution order is P3 → P4 → P5 → P6 → P7. All four moves GATE P7; P7 GATES P8. (Mapping matches PLAN.md lines 79-82.)

**Gating:** This phase runs LAST. It is GATED on all moves having landed in-module:
- P3 (crm embed — already in-module, no dep event; shim/dead-code deletions complete per PD-2).
- P4 (plugin-fees service-collapse into ledger; observability + lib-commons migrated in-repo first, then moved).
- P5 (tracer co-location, v4→v5 + lib-observability done in-repo first, then moved; godog BDD comes from here).
- P6 (reporter-manager + reporter-worker co-location; observability migrated in-repo first; toolchain directive originates here).

Per PD-6, each incoming repo's deps (observability + lib-commons) were migrated IN-PLACE and validated
against that repo's own CI BEFORE the move; observability migration and co-location never shared a commit.
Phase 7 is therefore the convergence of already-compatible import graphs, not a migration.

---

## Ground truth — FACTS verified against the CURRENT single-component tree (2026-06-03)

These were checked against the live repo at plan time and are reproducible TODAY:

- Root `go.mod`: `module github.com/LerianStudio/midaz/v3`, `go 1.26.3`, NO `toolchain`, NO `replace`, NO `go.work`.
- Current Lerian pins: `lib-auth/v2 v2.8.0`, `lib-commons/v5 v5.2.0-beta.12`, `lib-observability v1.0.1`, `lib-streaming v1.4.0`.
- `lib-commons/v5` GA on proxy: **v5.2.0 and v5.2.1 both exist** (also v5.3.x, v5.4.1). v5.2.1 is the latest v5.2.x GA → PD-4 target.
- `lib-observability` latest stable in 1.0 line: **v1.0.1** (1.1.0 only betas) → keep v1.0.1.
- Third-party pins already at MVS-target in midaz: otel 1.44.0, redis 9.20.0, fasthttp 1.71.0, testcontainers 0.42.0,
  rabbitmq 1.11.0, grpc 1.81.1, fiber 2.52.13, pgx 5.9.2, mongo-driver 1.17.9, migrate 4.19.1, uuid 1.6.0, decimal 1.4.0.
  validator/v10 currently 10.30.2 (indirect) → bump to 10.30.3.
- **Pre-existing `validator/v9` second major:** the current `go.mod` ALREADY carries `github.com/go-playground/validator v9.31.0+incompatible`
  AND `gopkg.in/go-playground/validator.v9 v9.31.0` (both indirect) ALONGSIDE `validator/v10 v10.30.2`. This is a
  same-conceptual-package, different-major-path coexistence that PRE-DATES every move. T07 must account for it (P7-T07a).
- Root `mk/`: only `coverage-unit.mk` + `tests.mk` today. `mk/tests.mk` exposes `test-unit` (≈:105), `test-integration` (≈:224),
  `wait-for-services` (≈:66). NO `test-bdd` target today; NO godog/cucumber anywhere in midaz.
- Root Makefile `make lint` (≈:213-266) does **NOT** run a single root `golangci-lint run ./...`. It DELEGATES:
  (1) loops `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` calling each component's own `$(MAKE) lint` (≈:220);
  (2) special-cases `$(LEDGER_DIR)` with its own delegated `$(MAKE) lint` (≈:228);
  (3) runs `golangci-lint run --build-tags=integration ./...` directly in `$(TESTS_DIR)` (≈:243);
  (4) runs `golangci-lint run ./...` directly in `$(PKG_DIR)` (≈:261).
  `GOLANGCI_LINT_VERSION := v2.4.0`. There are NO per-component `.golangci.yml` files (verified: only root `.golangci.yml` exists;
  ledger/crm have none). golangci-lint resolves `.golangci.yml` by walking up from its working dir.
- `.github/workflows/`: build.yml/go-combined-analysis.yml/pr-security-scan.yml/pr-validation.yml/release.yml/
  gptchangelog.yml/release-notification.yml. build.yml on shared-workflow `v1.27.5`, filter_paths still `components/crm` + `components/ledger`.
- midaz workflows carry NO `go_private_modules` input today (dossier-confirmed). No `GOPRIVATE`/`github_token`/`.secrets` machinery present today.
- **Real transaction call sites (verified, used by P7-T18):**
  - Revert / create path: `components/ledger/internal/adapters/http/in/transaction_create.go` — `executeCreateTransaction` (func at ≈:969),
    `createRevertTransaction` (func at ≈:964), and the single `validate, _ := mtransaction.ValidateSendSourceAndDistribute(...)` assignment at ≈:1045.
  - Pending-commit/cancel path: `components/ledger/internal/adapters/http/in/transaction_state_handlers.go` —
    `CancelTransaction` (≈:107), `RevertTransaction` (≈:166), `commitOrCancelTransaction` (≈:366), and its
    `validate, err := mtransaction.ValidateSendSourceAndDistribute(...)` assignment at ≈:433.
  - `ValidateSendSourceAndDistribute` lives in `pkg/mtransaction/validations.go` (≈:545), NOT the command layer.
  - Downstream consumers in `transaction_create.go` all read the ONE `validate` var: `GetBalances` (≈:1086),
    `SendTransactionToRedisQueue` (≈:1076), `buildBalanceOperations` (≈:1113), `enrichOverdraftOperations` (≈:1128),
    `ValidateAccountingRules` (≈:1140), `ProcessBalanceOperations` (≈:1151), `BuildOperations` (≈:1210), `WriteTransaction` (≈:1251).

## Ground truth — ASSUMPTIONS about the POST-MOVE unified graph (P7 must RE-VERIFY at execution)

These are NOT checkable today because the moved components are not yet in-tree (only `crm/infra/ledger` exist now).
They are forward-looking and gated on P4/P5/P6 landing. P7 owns re-verifying each at execution time:

- mongo-driver **v2.6.0** is NOT in the current `go.mod`; the dual mongo-driver v1+v2 coexistence (R14) materializes ONLY after
  the reporter (P6) move. Its BSON/codec runtime safety is UNVERIFIED today and must be proven in P7-T11.
- `lib-license-go/v2 v2.3.4` is absent from today's `go.mod`/`go.sum`; it entered only via the fees standalone
  binary, which P4 tears down (P4-T04 drops the direct require, license middleware deleted in P4-T19), so it must
  remain ABSENT after the fees collapse — P7 verifies its absence, not its presence.
- godog/cucumber subtree resolution under the unified graph is unverifiable until tracer (P5) lands.
- Third-party skew beyond `validator/v10` may surface from the merged reporter/tracer/fees pins (P7-T05 re-verifies at execution).
- Fees engine internals (`applyFeeCorrection`, `asset_precision.go::getAssetPrecision`, `CalculateFee`, synthetic
  `GetRouteFrom`/`GetRouteTo`) and fee-on-revert behavior arrive with P4; P7-T18 RE-VERIFIES that behavior in the unified
  module but does NOT implement it.

---

## Task DAG (intra-phase)

```
P7-T01 (verify GA) ─┐
                     ├─> P7-T03 (bump lib-commons GA) ─┐
P7-T02 (drop toolchain, single go directive) ──────────┤
                                                        ├─> P7-T06 (tidy + collapse indirects) ─> P7-T07 (verify single-version graph)
P7-T04 (drop fees stale lib-observability beta) ────────┤                                              │
P7-T05 (MVS-resolve third-party skew upward) ───────────┤                                              ├─> P7-T07a (reconcile pre-existing validator/v9 second major)
P7-T07a ────────────────────────────────────────────────┘ (T07a feeds T06 graph + is asserted by T07)
                                                                                                       ├─> P7-T08 (go build ./...) ─> P7-T09 (lint topology + green) ─> P7-T10 (test-unit) ─> P7-T11 (test-integration) ─> P7-T12 (sec)
P7-T09a (audit moved-component Makefile targets) ──────> P7-T09                                         │
P7-T13 (godog deps into go.mod) ─> P7-T14 (godog runner target) ─> P7-T15 (godog CI job, R15) ─────────┤ (T15 depends on T08 too)
P7-T13a (DECIDE godog CI workflow: shared vs bespoke) ─> P7-T15                                         │
P7-T07 ─> P7-T16 (clean go mod download / no private module) ─> P7-T17 (Phase-8 readiness handoff note)
P7-T11 + P7-T15 ─> P7-T18 (fee-on-revert + pending-cancel REFUND balance proof gate, third rail PD-5)
P7-T18 ─> P7-T18a (static guard: no downstream consumer reads a pre-fee `validate`)
ALL green (T12,T15,T16,T18,T18a) ─> P7-T19 (out-of-repo lockstep coordination check)
```

> **Gate relationship note (DAG / SP1):** Per the locked execution order (P3 → P4 → P5 → P6 → P7), the P4 and P3
> teardowns complete in their OWN phases, BEFORE P7 exists. P7-T18 is the FINAL unified RE-PROOF of the PD-5 third
> rail that P4 already built and first-proved (P4-T16) — it is a backstop that BLOCKS phase completion on failure and
> loops back to P4, NOT a precondition of P4/P3 teardown. Do not invert this into a P3/P4 → P7-T18 dependency; that
> would create a cycle (P7 is gated on the moves landing). The teardown abort/rollback safety lives in P3/P4 (mirroring
> P5-T16): standalone services stay intact until P4-T16's in-phase balance proof is green; P7-T18 is the convergence re-proof.

---

## P7-T01 — Verify lib-commons v5.2.x GA exists on the proxy and pick the pin
**Effort:** S — 1-2h
**Depends on:** (none)
**Files:** `docs/monorepo/plan/P7.md` (record finding)
**Description:** PD-4 mandates a planner/engineer task that VERIFIES a v5.2.x GA of `lib-commons/v5`
actually exists on the module proxy before anyone rewrites code against it; if no GA, pick the latest
stable tag and record it. Run `go list -m -versions github.com/LerianStudio/lib-commons/v5` and confirm a
non-prerelease `v5.2.x` tag. Verified at plan time: **v5.2.0 and v5.2.1 are GA** (v5.3.x/v5.4.1 also exist).
Pin decision: **`v5.2.1`** — latest within the v5.2.x GA line (smallest semantic jump off `v5.2.0-beta.12`,
no minor/feature drift from v5.3+). Also confirm `lib-observability` stays at `v1.0.1` (no 1.0.2; 1.1.x is beta-only) —
keep v1.0.1 per PD-4. Record the exact resolved version strings in this file.
**Acceptance criteria:**
- `go list -m -versions github.com/LerianStudio/lib-commons/v5` output shows a GA `v5.2.x` tag (non-prerelease).
- Chosen pin recorded as `v5.2.1` (or the actual latest v5.2.x GA at execution time if newer patch released).
- `lib-observability` confirmed staying at `v1.0.1` with rationale (no stable >1.0.1).
**Tests:**
- `go list -m -versions github.com/LerianStudio/lib-commons/v5` (real command; assert GA tag present).
- `go list -m -versions github.com/LerianStudio/lib-observability` (assert v1.0.1 is the latest non-beta).
**Risk refs:** R3

## P7-T02 — Drop the reporter `toolchain` directive; enforce single `go 1.26.3`
**Effort:** S — <1h
**Depends on:** (none — independent of version pins)
**Files:** `go.mod`
**Description:** The unified root `go.mod` already declares `go 1.26.3` and has NO `toolchain` line (verified).
Reporter's incoming module (P6) declared `go 1.26` + `toolchain go1.26.2`; on co-location its module dissolves, so
the only action is to ENSURE no `toolchain` directive leaked into the root `go.mod` during the reporter (P6) move
and that the single `go 1.26.3` directive governs all code. If a `toolchain` line is present, delete it.
This is a guard/cleanup task, not a rewrite — patch-level Go differences (1.26.0→1.26.3) carry zero language breakage.
**Acceptance criteria:**
- Root `go.mod` contains exactly one `go 1.26.3` directive and zero `toolchain` directives.
- `grep -c '^toolchain' go.mod` returns 0.
**Tests:**
- `grep -E '^go |^toolchain' go.mod` (assert single `go 1.26.3`, no toolchain).
- `go build ./components/reporter-manager/... ./components/reporter-worker/...` compiles under the unified Go directive.
**Risk refs:** R3

## P7-T03 — Bump root module off the lib-commons beta to the v5.2.x GA pin
**Effort:** S — 1-2h
**Depends on:** P7-T01
**Files:** `go.mod`, `go.sum`
**Description:** Replace `github.com/LerianStudio/lib-commons/v5 v5.2.0-beta.12` with the GA pin from P7-T01
(`v5.2.1`). Run `go get github.com/LerianStudio/lib-commons/v5@v5.2.1` then `go mod tidy`. This is the stable
target every incoming repo's in-place migration (P4/P5/P6) was already rewritten TO; converging the root pin
to GA closes the last beta. Keep `lib-observability v1.0.1` unchanged. lib-observability v1.0.1 declares NO
lib-commons require (verified in the module cache: its go.mod targets go 1.25.10 and has zero lib-commons line —
a lower-toolchain dependency, which is fine under the root's go 1.26.3), so the lib-commons GA bump cannot regress observability.
**Acceptance criteria:**
- `go.mod` pins `github.com/LerianStudio/lib-commons/v5 v5.2.1` (no `-beta`).
- `grep beta go.mod` returns no `lib-commons` line.
- `lib-observability v1.0.1` retained.
- `go build ./...` succeeds against the GA pin.
**Tests:**
- `go list -m github.com/LerianStudio/lib-commons/v5` returns `v5.2.1`.
- `go build ./...` (full-tree compile against GA).
**Risk refs:** R3, R4

## P7-T04 — Drop fees' stale `lib-observability v1.1.0-beta.5` require; converge to v1.0.1
**Effort:** S — <1h
**Depends on:** (none)
**Files:** `go.mod`
**Description:** plugin-fees declared `lib-observability v1.1.0-beta.5` in its standalone `go.mod` but never
imported it in code (0 source refs — confirmed in dossier 07). On the fees embed (**P4**), its `require` block
merged into the root module. Ensure NO `lib-observability v1.1.0-beta.x` require survived the merge; the root
must resolve `lib-observability` to exactly `v1.0.1`. If a beta require leaked in, remove it and re-tidy. Do not
let a beta leak into the monorepo graph.
**Acceptance criteria:**
- `go.mod` has exactly one `lib-observability` require at `v1.0.1`; no `v1.1.0-beta` line anywhere.
- `grep 'lib-observability' go.mod go.sum | grep -c beta` returns 0.
**Tests:**
- `go list -m github.com/LerianStudio/lib-observability` returns `v1.0.1`.
- `grep -n 'lib-observability' go.mod` (assert single v1.0.1, no beta).
**Risk refs:** R4

## P7-T05 — MVS-resolve all same-major third-party skew upward
**Effort:** M — 2-4h
**Depends on:** (none — operates on the merged require graph)
**Files:** `go.mod`, `go.sum`
**Description:** After the incoming `require` blocks merged, several third-party deps have minor/patch skew that
MVS resolves to the highest. Explicitly `go get` each to the unified target so the resolution is intentional and
recorded, not accidental: otel `1.44.0`, redis/go-redis/v9 `9.20.0`, testcontainers-go `0.42.0`, fasthttp
`1.71.0`, go-playground/validator/v10 `10.30.3`, grpc `1.81.1`, rabbitmq/amqp091-go `1.11.0`. (Already aligned in
root: fiber 2.52.13, pgx 5.9.2, mongo-driver 1.17.9, migrate 4.19.1, uuid 1.6.0, decimal 1.4.0 — verify, no action.)
The only value needing a real bump in midaz's CURRENT graph is `validator/v10 10.30.2 → 10.30.3` (currently indirect;
tidy will reclassify). None of these removed a package a lower version imports, so MVS-upward is safe with zero code
changes. **At execution, RE-VERIFY against the actual merged reporter/tracer/fees pins** — the unified graph may surface
skew not visible in the current single-component tree (e.g. mongo-driver v2's transitive set). Leave the dual mongo-driver
majors (`v1.17.9` + `v2.6.0`) as path-distinct coexistence — collapsing them is out of scope (separate optional cleanup);
the `validator/v9` second major is handled separately in P7-T07a, not here.
**Acceptance criteria:**
- `go.mod` pins match the §7 list of dossier 07: otel 1.44.0, redis 9.20.0, testcontainers 0.42.0, fasthttp 1.71.0,
  validator/v10 10.30.3, grpc 1.81.1, amqp091 1.11.0.
- No same-major dep is pinned below its highest incoming version (re-checked against the merged graph at execution).
- `go build ./...` succeeds with no code changes attributable to these bumps.
**Tests:**
- `go list -m go.opentelemetry.io/otel github.com/redis/go-redis/v9 github.com/testcontainers/testcontainers-go github.com/valyala/fasthttp github.com/go-playground/validator/v10 google.golang.org/grpc github.com/rabbitmq/amqp091-go` (assert target versions).
- `go build ./...` (full-tree compile).
**Risk refs:** R3, R4

## P7-T06 — `go mod tidy`: collapse duplicate indirects, settle the unified graph
**Effort:** M — 2-4h
**Depends on:** P7-T02, P7-T03, P7-T04, P7-T05, P7-T07a
**Files:** `go.mod`, `go.sum`
**Description:** Run `go mod tidy` (root `make tidy` wraps `go mod tidy`) on the unified module. This drops the
duplicate/stale indirects that the merge dragged in: fees' (P4) transitive `lib-commons/v2 v2.9.1` (should fall out
once nothing references it), tracer's (P5) transitive `lib-commons/v5 v5.3.0` indirect (subsumed by the direct GA pin),
and any duplicate indirect lines from the three merged require blocks. `lib-license-go/v2 v2.3.4` must be
ABSENT — it entered only via the fees standalone binary that P4 tears down (P4-T04 drops the direct require,
license middleware deleted in P4-T19); ledger already enforces license, so the unified module carries no
`lib-license-go/v2`. Confirm the net-new direct deps that legitimately enter are present and correctly classified:
`cel-go`+`cel.dev/expr`+`antlr` (tracer CEL/P5 — note `antlr4-go/antlr/v4` already in root), `chromedp`+`cdproto`
(reporter-worker/P6), `aws-sdk-go-v2` stack, `go-mssqldb`, `go-ora/v2`, `go-sql-driver/mysql`, `pongo2/v6`, `resty/v2`,
`cloud.google.com/go/*` (reporter/P6), `miniredis/v2` (tracer test/P5). Tidy must leave `go.mod`/`go.sum` byte-stable on
a second run. P7-T07a's validator/v9 disposition (allowlist or removal) must already be reflected before this tidy settles.
**Acceptance criteria:**
- `go mod tidy` produces no diff on a second consecutive run (idempotent).
- `grep 'lib-commons/v2' go.mod` returns nothing (the fees-transitive v2 dropped out).
- No duplicate require lines for any module path.
- `grep 'lib-license-go' go.mod` returns nothing — the dep was torn down with the fees standalone binary (P4-T04/P4-T19), absent after the collapse.
- `git diff --stat go.mod go.sum` shows the expected collapse, no unexplained additions.
**Tests:**
- `make tidy && git diff --exit-code go.mod go.sum` after a second `go mod tidy` (assert idempotent).
- `go mod verify` (assert go.sum integrity).
**Risk refs:** R3, R4

## P7-T07 — Verify a single-version graph for every shared module path (no hidden duplicates)
**Effort:** S — 1-2h
**Depends on:** P7-T06
**Files:** `go.mod`, `go.sum`
**Description:** Prove the unification actually happened: assert that `go list -m all` resolves exactly ONE version
per same-major module path, and that the only legal multi-major coexistences are the DOCUMENTED path-distinct ones
(`lib-commons/v5` only — no `/v4`; `mongo-driver` v1 + `/v2`; and the pre-existing `validator/v9` + `validator/v10`
case whose disposition P7-T07a decides). Specifically assert NO `lib-commons/v4` remains anywhere in the graph
(tracer's/P5's v4 must be fully gone — its presence would mean the move shipped a shim, a PD-1 defect).
Run `go mod graph` and grep for `lib-commons/v4` and any `=>` replace arrows. A second `/v5` version (even if
transitively dragged in) is a DEFECT, not a "documented exception" — it loops back to the source move (P5 for tracer's
v5.3.0 indirect) for elimination, never merely a note. The graph must carry exactly one resolved `lib-commons/v5`.
**Acceptance criteria:**
- `go list -m all | grep lib-commons` shows only `/v5 v5.2.1` — NO `/v4`, NO second `/v5` version. A second `/v5`
  is a defect that BLOCKS this task and loops to the source move; it is NEVER accepted as "documented".
- `go mod graph | grep -c '=>'` returns 0 (no replace directives anywhere in the resolved graph).
- `go list -m all | grep 'lib-observability'` shows exactly `v1.0.1`.
- The ONLY paths with >1 resolved major are the documented set: `mongo-driver` v1+v2, and `validator` v9+v10 per P7-T07a's disposition.
**Tests:**
- `! go list -m all | grep -q 'lib-commons/v4'` (assert absent; fails the task if v4 present).
- `test "$(go mod graph | grep -c ' => ')" -eq 0` (assert no replace edges).
- `test "$(go list -m all | grep -c 'lib-commons/v5')" -eq 1` (assert exactly one /v5 version resolved).
- `go list -m all | sort | awk '{print $1}' | uniq -d` returns no same-PATH duplicates.
**Risk refs:** R3

## P7-T07a — Reconcile the pre-existing `validator/v9` second-major case (allowlist or remove)
**Effort:** S — 1-2h
**Depends on:** P7-T05
**Files:** `go.mod`, `go.sum`, `docs/monorepo/plan/P7.md` (record disposition)
**Description:** The CURRENT `go.mod` (pre-dating every move) already carries TWO majors of go-playground/validator:
`github.com/go-playground/validator v9.31.0+incompatible` and `gopkg.in/go-playground/validator.v9 v9.31.0` (both
indirect) ALONGSIDE `github.com/go-playground/validator/v10 v10.30.2`. T07's single-version assertion would have flagged
this as a violation of the documented multi-major allowlist. Resolve it explicitly: (1) run `go mod why` on each v9 path
to find the importing module(s); (2) if the v9 importer is a real transitive dependency with no v10-bearing replacement,
ADD `validator/v9` to T07's documented multi-major allowlist with the `go mod why` evidence recorded here; (3) if v9 is
an orphaned indirect that tidy can drop (no live importer after the GA bumps), REMOVE it via tidy. Do NOT add a `replace`
to force a single major — that is a PD-1 shim. The disposition (allowlist vs removal) is recorded in this file with the
`go mod why` output before P7-T06 settles the final graph.
**Acceptance criteria:**
- `go mod why github.com/go-playground/validator` and `go mod why gopkg.in/go-playground/validator.v9` outputs recorded in this file.
- Disposition decided and recorded: either (a) validator/v9 added to the documented multi-major allowlist with the importing-module evidence, or (b) removed by tidy because it has no live importer.
- If removed: `! grep -q 'validator.v9\|validator v9' go.mod` after tidy.
- If allowlisted: T07's allowlist assertion (P7-T07) includes `validator` v9+v10 as a documented path-distinct pair.
- NO `replace` directive introduced for any validator path.
**Tests:**
- `go mod why github.com/go-playground/validator` (record output).
- `go mod why gopkg.in/go-playground/validator.v9` (record output).
- `! go mod graph | grep -q 'go-playground/validator.*=>'` (assert no replace edge for validator).
**Risk refs:** R3

## P7-T08 — Full-tree `go build ./...` and `go vet ./...` green
**Effort:** M — 2-4h
**Depends on:** P7-T06
**Files:** (no source edits expected; this is the compile gate)
**Description:** Compile the entire unified module: `go build ./...` then `go vet ./...`. Per-binary dead-code
elimination keeps each artifact clean (ledger won't link chromedp), but `go build ./...` exercises every package
including reporter-worker's (P6) chromedp/cdproto and tracer's (P5) CEL subtree. Any failure here is a real unification
defect (e.g. a residual `commons/log` import, a tenant-manager API drift, a telemetry-middleware path that didn't move —
R13). This task FINDS such defects; fixing them belongs to the move phase that introduced them (loop back to P4/P5/P6),
but Phase 7 owns the gate that the whole tree compiles as one module.
**Acceptance criteria:**
- `go build ./...` exits 0 across all packages including `components/reporter-worker/...`, `components/reporter-manager/...`, `components/tracer/...`, `components/ledger/...` (fees+crm embedded).
- `go vet ./...` exits 0 (or only pre-existing, documented suppressions).
**Tests:**
- `go build ./...` (full-tree compile).
- `go vet ./...` (full-tree vet).
**Risk refs:** R3, R4, R5, R13

## P7-T09 — Own the `make lint` topology change AND make the unified tree lint-green
**Effort:** L — 1-2 days
**Depends on:** P7-T08, P7-T09a
**Files:** root `Makefile` (lint target ≈:213-266), `.golangci.yml`, line-scoped `//nolint` blocks as needed
**Description:** The current `make lint` does NOT run a single root `golangci-lint run ./...`. It DELEGATES per component:
loops `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` calling each component's own `$(MAKE) lint` (≈:220), special-cases
`$(LEDGER_DIR)` (≈:228), then runs `golangci-lint run` DIRECTLY in `$(TESTS_DIR)` (≈:243, with `--build-tags=integration`)
and `$(PKG_DIR)` (≈:261). There are NO per-component `.golangci.yml` files — only the root one. Consequence: tracer
(P5) and reporter-manager/reporter-worker (P6) would be SILENTLY SKIPPED by the lint loop (they are neither in COMPONENTS
nor special-cased), making the green-lint gate theater. **This task OWNS the topology fix in P7 (it is NOT deferred to P8).**
Choose ONE and state it in the implementation:
  (a) **Single-root model:** replace the per-component delegation with a single `golangci-lint run ./...` at the repo root
      over the unified module under the root `.golangci.yml`, retaining the separate `$(TESTS_DIR)` integration-tag run; OR
  (b) **Normalized-delegation model:** keep delegation but replace `COMPONENTS` with a list covering
      `ledger tracer reporter-manager reporter-worker` (crm is folded into ledger, dropped from the list), and ensure each
      moved component still has a working `lint` target (verified in P7-T09a).
Adopt fees' (P4) strict config as the single root `.golangci.yml` floor (dossier 08). Under model (a) the floor is
discoverable everywhere by construction; under model (b) confirm each delegated `cd` lands where the root config is
discoverable. The existing direct `$(TESTS_DIR)` and `$(PKG_DIR)` lint runs MUST be preserved in whichever model.
Then drive the cleanup pass so lint is green. Do NOT introduce blanket `//nolint` to paper over real findings; fix or
line-scope precisely with a reason. (NOTE the FromTo.Route `//nolint:staticcheck` lines are a SEPARATE, accepted exception
documented in P9-T-shim-exceptions — do not remove them here.)
**Acceptance criteria:**
- The chosen lint topology (model a or b) is implemented in the root `Makefile` and stated explicitly; tracer + reporter-manager + reporter-worker are actually linted (not skipped). crm is not separately linted (folded into ledger).
- The separate `$(TESTS_DIR)` (integration tag) and `$(PKG_DIR)` lint runs are preserved.
- `make lint` exits 0 across the full unified tree.
- No blanket file-level `//nolint` added solely to silence the unified config; suppressions are line-scoped with reason.
**Tests:**
- `make lint` (real command; assert exit 0).
- A grep/inspection proving the moved components are reached by the lint surface (e.g. introduce a deliberate lint violation in a tracer file and confirm `make lint` turns red, then revert).
**Risk refs:** R11

## P7-T09a — Audit and reconcile the moved components' own Makefile targets
**Effort:** S — 1-2h
**Depends on:** P7-T08
**Files:** `components/tracer/Makefile`, `components/reporter-manager/Makefile`, `components/reporter-worker/Makefile`, root `Makefile`
**Description:** tracer (P5) and reporter (P6) shipped their OWN component-level Makefiles; ledger/crm carry 10-16KB
Makefiles today. After the moves, these component Makefiles can collide with or duplicate the unified Make surface:
each may independently reinstall `golangci-lint@v2.4.0`, declare its own `lint`/`test`/`build` targets, or carry orphaned
delegations to a now-dissolved module. Audit the moved components' Makefile targets for: (1) collisions with the unified
root targets; (2) orphaned delegations (e.g. references to a standalone `go.mod` that no longer exists); (3) the
`lint` target that P7-T09 model (b) depends on (if model (b) is chosen). Reconcile: either keep a minimal per-component
`lint`/`test-bdd` target that the root delegates to, or remove the component Makefile entirely if the root single-model
makes it dead. Record which targets survive and why.
**Acceptance criteria:**
- Each moved component's Makefile is audited; collisions and orphaned delegations are listed and resolved (kept-with-reason or removed).
- No two Makefiles install golangci-lint at conflicting versions; the version is `v2.4.0` everywhere it appears.
- If P7-T09 chooses model (b), each of tracer/reporter-manager/reporter-worker has a working `lint` target reachable by the root delegation.
**Tests:**
- `grep -rn 'golangci-lint' components/*/Makefile` (assert single consistent version `v2.4.0`).
- `grep -rn 'lint:\|test-bdd:\|build:' components/tracer/Makefile components/reporter-manager/Makefile components/reporter-worker/Makefile` (inventory targets; assert no orphaned standalone-module delegation).
**Risk refs:** R11

## P7-T10 — Full-tree `make test-unit` green
**Effort:** M — 4-8h
**Depends on:** P7-T08
**Files:** root Makefile / `mk/tests.mk` (ensure `test-unit` covers the unified package set), test files needing fixture repair
**Description:** Run `make test-unit` (via `mk/tests.mk` `test-unit` target) over the unified module. The incoming
repos' unit tests (tracer/P5, reporter/P6, fees/P4) now run under midaz's go directive and version pins. Expect
fixture/import drift from the observability rename (already done in-repo per PD-6, so should be minimal) and the
third-party bumps. Ensure the streaming `JSONShape` unit tests and any version-locked tests still pass. The 85% coverage
hard-fail gate (R11) applies — backfill where folded code dips below floor (open-ended; flag if a component's folded code
lands materially below floor so the backfill is budgeted, not silently absorbed). Per CLAUDE.md: no `time.Now()` in tests.
**Acceptance criteria:**
- `make test-unit` exits 0 for the full unified package set.
- Coverage gate (85%) not regressed below floor on any component (or documented waiver with a named backfill task).
- `pkg/streaming/events` JSONShape locks pass unchanged.
**Tests:**
- `make test-unit` (real command; assert exit 0).
**Risk refs:** R11, R14

## P7-T11 — Full-tree `make test-integration` green (testcontainers, real deps)
**Effort:** L — 1-2 days
**Depends on:** P7-T08, P7-T10
**Files:** `mk/tests.mk` (`test-integration` target), integration test files needing real-dep wiring
**Description:** Run `make test-integration` (via `mk/tests.mk` `test-integration` target) over the unified module.
This pulls every testcontainer image across the merged tree (Mongo, Postgres, Redis/Valkey, RabbitMQ, plus reporter's
(P6) mssql/mysql fan-out and tracer's (P5) miniredis). Validate: ledger+fees+crm integration against real Postgres/Mongo;
reporter's dual mongo-driver (v1+v2) collapse surfaces no BSON/codec runtime issue (R14); tracer's audit hash-chain
`VerifyHashChain` integration intact against shared Postgres (R19 — relocation only, no renumber). This is the
heaviest green gate. Repository/adapter coverage uses real dependencies per CLAUDE.md (no mocked DB in integration).
**NOTE:** the mongo-driver v1+v2 coexistence is UNVERIFIABLE until P6 lands (v2 not in-tree today) — this task is where it
gets PROVEN at execution, not assumed; a codec/BSON panic here loops back to P6.
**Acceptance criteria:**
- `make test-integration` exits 0 for the full unified package set.
- Reporter mongo-driver v1+v2 coexistence VERIFIED AT EXECUTION against a real Mongo (no codec panic) — flagged as the heaviest unverified runtime claim; failure loops to P6.
- Tracer audit hash-chain integration passes against the shared Postgres (no migration renumber).
**Tests:**
- `make test-integration` (real command; testcontainers; assert exit 0).
**Risk refs:** R11, R14, R16, R19

## P7-T12 — Full-tree `make sec` green (gosec + govulncheck over the unified surface)
**Effort:** M — 4-8h
**Depends on:** P7-T08
**Files:** (no source edits expected; vuln-suppression config only if a documented CVE waiver is needed)
**Description:** Run `make sec` (`sec-gosec` + `sec-govulncheck` over `./components/... ./pkg/...`) on the unified
module. The merged graph enlarges the vuln-scan surface: chromedp (P6), full aws-sdk-go-v2 (P6), Oracle/MSSQL/MySQL
drivers (P6), CEL (P5), etc. all now scan under one `make sec`. Triage any new findings: real CVEs get a dependency bump
(loop into the relevant pin) or a documented, time-boxed waiver; gosec findings in folded code get fixed or precisely
justified. Do not blanket-suppress.
**Acceptance criteria:**
- `make sec` exits 0 (or only documented, justified waivers remain).
- No HIGH/CRITICAL govulncheck finding in a reachable code path without an owned remediation/waiver.
**Tests:**
- `make sec` (real command; gosec + govulncheck; assert exit 0).
**Risk refs:** R11

## P7-T13 — Add godog + cucumber dependency subtree to the unified `go.mod`
**Effort:** S — 1-2h
**Depends on:** P7-T06
**Files:** `go.mod`, `go.sum`
**Description:** tracer (P5) ships godog BDD e2e tests (R15) — a test mode midaz CI has never run. On co-location the
godog dep tree (`github.com/cucumber/godog` + transitive `cucumber/*`, `gherkin`, etc.) must resolve in the unified
module. Confirm `go test` of tracer's BDD package compiles and that `cucumber/godog` is a recorded require (direct,
since it's used in test code). This task ONLY lands the dependency; the runner target is T14, the CI workflow DECISION
is T13a, and the CI job is T15.
**Acceptance criteria:**
- `github.com/cucumber/godog` present in `go.mod` at the version tracer used (or higher per MVS), recorded after tidy.
- `go test -run xxx ./components/tracer/<bdd-package>/...` compiles (no missing godog symbols).
**Tests:**
- `go build ./components/tracer/...` includes the godog test package (via `go test -c`).
- `go list -m github.com/cucumber/godog` returns a resolved version.
**Risk refs:** R15

## P7-T13a — DECIDE the godog CI delivery model (shared-workflow vs bespoke)
**Effort:** S — 1-2h
**Depends on:** P7-T13
**Files:** `docs/monorepo/plan/P7.md` (record decision), `.github/workflows/` (inspect existing shared-workflow surface)
**Description:** godog is net-new midaz CI. Before T15 stands up the job, ONE owning task must DECIDE whether the BDD job
runs via the existing `LerianStudio` shared-workflow surface (build.yml is on shared-workflow `v1.27.5`) or as a bespoke
midaz-local workflow. This decision was repeatedly deferred in earlier drafts (FG3); pin it here. Evaluate: does the
shared workflow expose a BDD/feature-test entrypoint or a generic "run make target" hook that `test-bdd` (T14) can ride?
If yes and it path-filters cleanly to `components/tracer`, choose shared. If the shared workflow cannot invoke an arbitrary
make target without an upstream change that out-scopes P7, choose a bespoke midaz-local workflow file for the BDD job and
record why. The decision binds T15's implementation. Coordinate the shared-vs-bespoke choice with the Phase-8 CI
harmonization owner (P8-T18) so it does not get re-litigated downstream. **Scope boundary (vs P5-T12a-decide):**
this task owns the UNIFIED-MODULE delivery decision — how godog ultimately runs in the consolidated monorepo CI;
P5-T12a-decide's earlier call is scoped to the tracer-move-time stand-up and does NOT pre-bind the unified-module
delivery model decided here.
**Acceptance criteria:**
- A recorded decision: "godog BDD CI runs via {shared-workflow vX | bespoke midaz-local workflow}" with the rationale and the entrypoint it uses.
- If shared: the shared-workflow input/hook the BDD job rides is named. If bespoke: the new workflow filename is named and justified (shared workflow could not invoke the target without an out-of-scope upstream change).
- The decision is acknowledged by the P8-T18 (CI harmonization) owner so it is not re-decided in P8.
**Tests:**
- `grep -n 'godog\|test-bdd\|shared-workflow' docs/monorepo/plan/P7.md` confirms the decision is recorded.
**Risk refs:** R15

## P7-T14 — Wire a `godog` runner target into the Makefile/mk surface
**Effort:** M — 2-4h
**Depends on:** P7-T13
**Files:** `mk/tests.mk` (NEW `test-bdd`/`godog` target) or `components/tracer/Makefile`, `Makefile` (root delegation)
**Description:** tracer (P5) ran godog via its own Makefile target; on co-location that runner must exist in the unified
Make surface so CI and local dev can invoke it. Add a `test-bdd` (or `godog`) target to `mk/tests.mk` (consistent with
`test-unit`/`test-integration` already there) that runs tracer's `.feature` suites against the godog runner, plus a
root delegation so `make test-bdd` or `make tracer COMMAND=test-bdd` works. The BDD e2e needs the tracer service +
shared Postgres up (it's e2e), so the target documents/depends-on the infra prerequisite (mirror `test-integration`'s
`wait-for-services`). If tracer's pinned godog MVS-bumps a cucumber subtree under the unified graph and the bump changes
the step-definition API, absorb the small rewrite here (OI-4).
**Acceptance criteria:**
- A `test-bdd` (or `godog`) Make target exists in `mk/tests.mk` and runs tracer's feature suites.
- Root `make test-bdd` (or `make tracer COMMAND=test-bdd`) delegates correctly.
- Target documents its infra dependency (tracer + shared Postgres reachable).
**Tests:**
- `make test-bdd` against a local stack (or the godog suite in dry-run mode) exits 0.
- `grep -n 'godog\|test-bdd' mk/tests.mk` confirms the target landed.
**Risk refs:** R15

## P7-T15 — Stand up godog BDD as a CI job (R15)
**Effort:** M — 4-8h
**Depends on:** P7-T08, P7-T14, P7-T13a
**Files:** `.github/workflows/` (NEW or extended workflow invoking the godog target, per T13a's decision), `mk/tests.mk`
**Description:** midaz CI has no godog mode. Per T13a's decision (shared-workflow vs bespoke), add a CI job that runs the
godog BDD suite (the `test-bdd` target from T14) on PRs touching `components/tracer` (path-filtered, consistent with the
shared-workflow path model). The job spins up the required infra (tracer + shared Postgres), runs the feature suites, and
fails the build on a red scenario. This is discrete net-new CI plumbing flagged by R15. Keep it path-scoped so it does
not run on unrelated component changes.
**Acceptance criteria:**
- A CI job (in the form decided by T13a) runs the godog suite on `components/tracer` changes and reports pass/fail.
- A deliberately-broken `.feature` scenario turns the job red (proven once, then reverted).
- The job is path-filtered to tracer (does not run on ledger-only PRs).
**Tests:**
- CI run on a tracer-touching PR shows the godog job green.
- A throwaway red-scenario commit shows the job red (verification artifact, reverted).
**Risk refs:** R15

## P7-T16 — Verify clean `go mod download` with NO transitive private Lerian module
**Effort:** S — 1-2h
**Depends on:** P7-T07
**Files:** (verification only; record finding in this plan file)
**Description:** This is the precondition gate for Phase 8 deleting the `github_token` BuildKit secret + `.secrets/`
+ `go_private_modules` machinery. Prove the unified module resolves with ZERO private Lerian modules: in a clean
environment (no `~/.netrc`, no `GOPRIVATE`/`GONOSUMDB`/`GOINSECURE` for `github.com/LerianStudio/*`, default public
GOPROXY), run `go mod download` and `go mod verify`. The only formerly-private import was fees' (P4) `midaz/v3` (now the
module itself); tracer/fees' other Lerian deps (lib-commons, lib-auth, lib-observability, lib-license-go) are public
and midaz/reporter already resolve them without a token. Assert `GOFLAGS`/env carries no `GOPRIVATE=...LerianStudio`.
(NOTE: env-var names — use real Go vars only: `GOPRIVATE`, `GONOSUMDB`, `GOPROXY`, `GONOSUMCHECK` is NOT a real Go env var
and must not appear in the commands.)
**Acceptance criteria:**
- `env -u GOPRIVATE -u GONOSUMDB GOPROXY=https://proxy.golang.org,direct go mod download` (clean env, no netrc) exits 0.
- `go mod verify` exits 0.
- No module path under `github.com/LerianStudio/*` requires authenticated fetch (all resolve via public proxy).
- Finding recorded: "github_token machinery safe to delete in Phase 8" with the exact command output referenced.
**Tests:**
- `env -u GOPRIVATE -u GONOSUMDB GOPROXY=https://proxy.golang.org,direct go mod download` in a scratch module cache (real command; assert exit 0).
- `go mod verify` (assert "all modules verified").
**Risk refs:** R3

## P7-T17 — Record Phase-8 readiness handoff (github_token machinery removable; P7 gates P8)
**Effort:** S — <1h
**Depends on:** P7-T16
**Files:** `docs/monorepo/plan/P7.md` (append handoff section)
**Description:** Append a short, explicit handoff to this plan file stating Phase 7's exit state for Phase 8: the
unified module compiles/lints/tests/scans green; the dependency graph is single-version per shared path with no
`replace`/`go.work`; `go mod download` is clean with no private module → Phase 8 may delete the `github_token`
BuildKit secret, `.secrets/`, `~/.netrc` dance, and `go_private_modules` workflow inputs from the incoming
Dockerfiles/workflows. State explicitly that **P7 GATES P8**: P8 may not begin until the full P7 exit-criteria set
(below) is green. Reference the verification commands from T16. This is a coordination artifact so Phase 8
doesn't re-derive the precondition.
**Acceptance criteria:**
- Plan file contains a "Phase 8 readiness" section naming the exact artifacts Phase 8 may delete and the evidence (T16 command output).
- The section states "P7 gates P8" and ties P8 entry to the full P7 exit criteria.
**Tests:**
- `grep -n 'Phase 8 readiness' docs/monorepo/plan/P7.md` confirms the handoff exists.
**Risk refs:** R3

## P7-T18 — Fee-on-revert / pending-cancel REFUND balance proof gate (third rail, PD-5)
**Effort:** M — 4-8h
**Depends on:** P7-T11, P7-T15
**Files:** `components/ledger/internal/adapters/http/in/transaction_create.go` (`executeCreateTransaction` ≈:969, `createRevertTransaction` ≈:964, `validate` assignment ≈:1045), `components/ledger/internal/adapters/http/in/transaction_state_handlers.go` (`commitOrCancelTransaction` ≈:366, `RevertTransaction` ≈:166, `CancelTransaction` ≈:107, `validate` assignment ≈:433), `pkg/mtransaction/validations.go` (`ValidateSendSourceAndDistribute` ≈:545), NEW fee-revert integration tests alongside `components/ledger/internal/adapters/http/in/transaction_integration_test.go`
**Description:** PD-5 (THIRD RAIL): on transaction revert AND pending-cancel, the original fees MUST be refunded
(reverse the fee legs) and the reversal MUST balance (`sum(all legs incl. reversed fees) == 0`). Phase 7 does NOT
implement the fee engine (that's P4) — it owns the FINAL gate that the embedded behavior is correct in the unified module.

**This is a VERIFY-not-rebuild gate, anchored at the REAL handler-layer call sites (both files are in
`adapters/http/in/`, NOT `services/command/` — there is no fee/revert logic in `services/command/`):**

- **Revert-refund limb →** `transaction_create.go::executeCreateTransaction` (the revert path runs through
  `createRevertTransaction` → `executeCreateTransaction`). `TransactionRevert` (in `pkg`, transaction.go ≈:293)
  reconstructs froms/tos from `t.Operations` and sets the reverse `Send.Value = *t.Amount` (≈:386); `tran.Amount`
  derives from the MUTATED `Send.Value` (≈:1188) for DEDUCTIBLE fees. The revert balances ONLY IF
  `sum(reconstructed legs) == persisted t.Amount`. The proof MUST pin the **DEDUCTIBLE-fee** revert case specifically
  (a non-deductible-fee revert can pass while a deductible-fee revert breaks).
- **Pending-cancel-refund limb →** `transaction_state_handlers.go::commitOrCancelTransaction` (the cancel path;
  `validate` recomputed at ≈:433).
- **No double-reverse:** `TransactionRevert` is ALREADY fee-aware. The test MUST assert that the gate does NOT inject
  additional refund legs on top (that would DOUBLE-REVERSE). The proof confirms the existing fee-aware revert balances;
  it does not add a second refund mechanism.

**Single-`validate`-reassignment invariant (the mechanism PD-5 protects):** in BOTH files the post-fee invariant lives at
exactly one `validate := mtransaction.ValidateSendSourceAndDistribute(...)` assignment (transaction_create.go ≈:1045 and
transaction_state_handlers.go ≈:433). EVERY downstream consumer reads that ONE var: GetBalances (≈:1086),
SendTransactionToRedisQueue (≈:1076), buildBalanceOperations (≈:1113), enrichOverdraftOperations (≈:1128),
ValidateAccountingRules (≈:1140), ProcessBalanceOperations (≈:1151 — the Lua balance mutation that MOVES MONEY),
BuildOperations (≈:1210 — persisted Operation rows), validate.Sources/Destinations (≈:1230-1231), WriteTransaction (≈:1251).
If fee legs are injected, `validate` MUST be REASSIGNED from `ValidateSendSourceAndDistribute` on the MUTATED payload BEFORE
any of those consumers run; NO downstream consumer may read a pre-fee `validate`/Responses. P7-T18 owns the runtime balance
proof; P7-T18a owns the static guard for this no-stale-read invariant.

If P4's implementation lacks the deductible-fee revert-refund balance assertion, this gate FAILS and loops back to P4
(it surfaces in P4 review, not 4-8h into P7 — P4-T16 is the in-phase first proof; P7-T18 is the unified re-proof).
Per CLAUDE.md, double-entry correctness is non-negotiable.
**Acceptance criteria:**
- An integration test reverts a **DEDUCTIBLE-fee-bearing** transaction (via the executeCreateTransaction revert path) and asserts the fee legs are reversed AND `sum(all operation amounts incl. reversed fee legs) == 0` under exact `decimal.Equal`.
- An integration test cancels a PENDING fee-bearing transaction (via commitOrCancelTransaction) and asserts the same (refund + balance).
- The deductible-fee revert assertion confirms `sum(reconstructed legs) == persisted t.Amount` (the condition under which TransactionRevert's reverse Send.Value balances).
- No-double-reverse asserted: the gate does NOT inject extra refund legs on top of the already-fee-aware TransactionRevert.
- `ValidateSendSourceAndDistribute` is re-run on the reversal payload (no reuse of pre-fee responses).
- **Streaming wire-contract guard (CRITICAL event):** after a fee-bearing transaction create, assert the emitted `transaction_lifecycle` CloudEvents payload (built via `SendTransactionEvents` from the post-fee `tran`) carries the fee legs in `Operations[]` and the post-fee `tran.Amount` — asserted against an event-spy/fake emitter. Guards the CRITICAL streaming wire contract so a future pre-fee-snapshot refactor cannot silently ship wrong financial events. (Re-proof in the unified module of the same guard first proven in P4-T16.)
- Both tests run inside `make test-integration` and are green.
**Tests:**
- `make test-integration` includes the two revert/cancel-refund balance tests (assert green).
- Test bodies assert `sum(all operation amounts incl. reversed fee legs) == 0` (exact `decimal.Equal`) for the reversal transaction, with the deductible-fee case explicitly exercised.
- A test asserts the emitted `transaction_lifecycle` payload (from `SendTransactionEvents` on the post-fee `tran`) carries the fee legs in `Operations[]` and the post-fee `tran.Amount` against an event-spy/fake emitter.
**Risk refs:** R1, R2, R18

## P7-T18a — Static guard: NO downstream consumer reads a pre-fee `validate` (code-level invariant)
**Effort:** S — 1-2h
**Depends on:** P7-T18
**Files:** `components/ledger/internal/adapters/http/in/transaction_create.go`, `components/ledger/internal/adapters/http/in/transaction_state_handlers.go`, NEW guard (grep/AST check, e.g. a `scripts/` check or a `go vet`-style test)
**Description:** The integration test (P7-T18) proves RUNTIME balance; this task proves the COMPLEMENTARY static
invariant: in BOTH handler files, no fee mutation happens AFTER the `validate := ValidateSendSourceAndDistribute(...)`
assignment without REASSIGNING `validate` from a fresh `ValidateSendSourceAndDistribute` on the mutated payload, and no
downstream consumer reads a stale pre-fee `validate`/`Responses`. This is the #1 third-rail gap the audit flagged
(TF1): the single `validate` variable must carry the post-fee result to every consumer. Implement a deterministic
code-level guard — a grep/AST assertion in the CI/lint surface — that fails if (a) a second `ValidateSendSourceAndDistribute`
result is assigned to a NEW variable that downstream code ignores, or (b) any of the named consumers (GetBalances,
buildBalanceOperations, ValidateAccountingRules, ProcessBalanceOperations, BuildOperations, validate.Sources/Destinations,
WriteTransaction) reads a Responses value that is provably pre-fee. The cheapest robust form: assert there is exactly ONE
`validate` binding per path and all consumers reference that single binding (no shadow/second pre-fee binding leaks downstream).
**Acceptance criteria:**
- A deterministic grep/AST guard exists (and runs in CI) asserting that in `executeCreateTransaction` and `commitOrCancelTransaction` every downstream consumer reads the single `validate` binding, and that any fee mutation is followed by a `validate` reassignment from `ValidateSendSourceAndDistribute` before the first consumer.
- The guard PROVES zero downstream read of a pre-fee `Responses`/`validate` (the TF1 acceptance gate).
- The guard turns red if a contrived pre-fee read is introduced (proven once, reverted).
**Tests:**
- A grep proving ZERO downstream read of a pre-fee `Responses`/`validate` in both handler functions (assert empty match set).
- Introduce a contrived second pre-fee `validate2 := ValidateSendSourceAndDistribute(...)` read downstream and confirm the guard turns red; revert.
**Risk refs:** R1, R2, R18

## P7-T19 — Out-of-repo lockstep coordination check (Helm / gitops / APIDog)
**Effort:** M — 2-4h (coordination, cross-team)
**Depends on:** P7-T12, P7-T15, P7-T16, P7-T18, P7-T18a
**Files:** (out-of-repo; verification + sign-off recorded here)
**Description:** A co-located/collapsed component that builds but has no Helm value, gitops key, and APIDog scenario
never deploys. Before declaring Phase 7 done, confirm the out-of-repo deploy surfaces are scheduled to extend in
lockstep with Phase 8's image-topology change: the `midaz` Helm chart and `midaz-firmino-gitops`
`yaml_key_mappings` must add `tracer`/`reporter-manager`/`reporter-worker` and REMOVE `crm`/`plugin-fees` image keys;
APIDog e2e scenarios extend to the new surfaces. This is a coordination/sign-off task (the edits land in Phase 8 /
out-of-repo), but Phase 7 owns confirming ownership and sequencing so the green monorepo is actually deployable.
Flagged as cross-team blast radius (R12).

**Owner-unavailable / chart-rejected fallback (CGap2):** if an external owner (Helm/gitops/APIDog) does NOT sign off in
lockstep, or rejects the chart change, this is the likeliest real-world stall. Define the path explicitly: (1) record the
specific surface that is blocked and the owner contacted; (2) the monorepo green state (P7-T01..T18a) is NOT rolled back —
it stands as the in-repo deliverable; (3) escalate the blocked surface to the Phase-8 owner with the exact key-mapping diff
needed, so P8 can sequence the deploy-surface edit independently; (4) Phase 7 may be declared "code-complete, deploy-surface
pending-owner" with the blocked item named — it does NOT silently mark T19 done. A missing/rejected sign-off blocks the
"deployable" claim, never the "unified+green" claim.
**Acceptance criteria:**
- Written confirmation of owner + sequencing for: `midaz` Helm chart updates, `midaz-firmino-gitops` key-mapping
  updates (add tracer/reporter-*, remove crm/plugin-fees), APIDog scenario extension.
- No image-rename/delete is declared "done" without a corresponding lockstep plan recorded.
- If any owner is unavailable or rejects the change, the blocked surface is named, escalated to P8 with the needed diff, and Phase 7 is marked "code-complete, deploy-surface pending-owner" (not silently complete).
**Tests:**
- Coordination checklist recorded in this plan file with owners and any pending-owner escalations; no executable test (cross-repo, cross-team).
**Risk refs:** R12

---

## Exit Criteria (Phase 7 done) — these gate P8 entry
1. Root `go.mod` is the single unified module: `go 1.26.3`, no `toolchain`, no `replace`, no `go.work`;
   `lib-commons/v5 v5.2.1` GA, `lib-observability v1.0.1`, no beta requires; third-party skew MVS-resolved upward;
   the pre-existing `validator/v9` second major reconciled (allowlisted-with-evidence or removed; no `replace`).
2. `go mod tidy` is idempotent; `go list -m all` shows exactly one resolved version per shared major; the ONLY
   documented multi-major paths are `mongo-driver` v1+v2 and (per P7-T07a) `validator` v9+v10; NO `lib-commons/v4`
   anywhere; exactly ONE `lib-commons/v5`.
3. Full-tree green: `go build ./... && go vet ./... && make lint && make test-unit && make test-integration && make sec` all exit 0,
   with `make lint` topology fixed so tracer + reporter-manager + reporter-worker are actually linted (P7-T09/T09a).
4. godog BDD e2e is standing in CI (R15): CI delivery model decided (P7-T13a), runner target wired, path-filtered job
   runs tracer's suites, red scenario fails the build.
5. Fee-on-revert AND pending-cancel REFUND proven in integration (PD-5 third rail): fee legs reversed AND
   `sum(legs) == 0` under exact decimal equality, with the DEDUCTIBLE-fee revert case explicitly exercised and
   no-double-reverse asserted (P7-T18); PLUS the static no-stale-`validate`-read guard green (P7-T18a).
6. Clean `go mod download` in a no-credential environment — no transitive private Lerian module → `github_token`
   machinery removable in Phase 8 (handoff recorded; P7 gates P8 stated in P7-T17).
7. Out-of-repo Helm/gitops/APIDog lockstep ownership + sequencing confirmed, OR blocked surfaces named, escalated to
   P8, and Phase 7 marked "code-complete, deploy-surface pending-owner" (R12, P7-T19).

## Risks Addressed
R1, R2, R3, R4, R5, R11, R12, R13, R14, R15, R16, R18, R19

## Open Items (no shim allowed — escalate, do not paper over)
- **OI-1:** If P5 (tracer) shipped a residual `lib-commons/v4` import or a v4/v5 dual-import alias into the merged
  module, P7-T07 will catch it. That is a PD-1 shim defect — it must be fixed at the source (loop back to **P5**), NOT
  bridged with a `replace`. Phase 7 has no clean path around a v4 leak; flag and block. (Tracer is P5; the v4→v5
  migration was a HARD prerequisite gate done in-repo before the move per PD-1.)
- **OI-2:** PD-5 fee-on-revert/pending-cancel is a PRODUCT-confirmed third rail (refund). If P4's implementation does NOT
  reverse fee legs on revert/cancel, or a DEDUCTIBLE-fee revert fails `sum(legs) == 0`, P7-T18 fails and blocks phase
  completion — there is no Phase-7-local fix; it loops to **P4** (fees is P4). The fee engine and fee-on-revert behavior
  are a P4 deliverable whose balance assertion is a P4-T16 acceptance criterion; P7-T18 is the unified RE-PROOF backstop.
- **OI-3:** RESOLVED — the `make lint` topology change (single-root or normalized-delegation; drop crm special-case,
  cover tracer/reporter-*) is OWNED by P7-T09 (not deferred to P8), with the moved-component Makefile reconciliation in
  P7-T09a. The single `.golangci.yml` floor (fees' strict config) is adopted in P7-T09. Coordinate the COMPONENTS-list
  shape with the P8-T18 CI-harmonization owner, but the green-lint gate and the Makefile edit both land in P7.
- **OI-4:** godog version selection — tracer's pinned godog may pull a cucumber subtree that MVS-bumps under the
  unified graph. If the bump changes godog's step-definition API, T13/T14 absorb a small rewrite; budgeted as part of T14.
- **OI-5:** If a govulncheck HIGH/CRITICAL surfaces in a reachable path with no upstream fix (T12), the only clean
  options are a dependency bump or a documented time-boxed waiver — NOT a vendored patch/shim. Escalate the choice.
- **OI-6:** Surviving accepted exceptions (named, NOT shims) — these are pre-existing, by-design passive-compat fields
  that P9 explicitly accepts; P7 must NOT remove them and must NOT count them as defects:
  - `FromTo.Route` + `//nolint:staticcheck` (transaction_create.go ≈:635/685/768/820/948/1201; `RouteID` is canonical)
    — accepted exception, owned/named in P9.
  - The misleading alias `libCommons "github.com/LerianStudio/lib-observability"` is PRE-EXISTING and WIDESPREAD in
    ledger code (e.g. redis/rabbitmq/mongodb adapters, http/in/metadata.go, transaction_overdraft_enrichment.go), NOT
    just CRM. P9's grep-for-misleading-aliases sweep must cover the WHOLE tree, not just shims, or it survives the final
    cleanup. P7 surfaces this for P9; it is out of P7's remit to rename (no behavior change in P7).
```


---

<a id="phase-8"></a>

# Phase 8 — Build / CI / Docker / Release Harmonization (GATED LAST) (21 tasks)

_Verbatim from `docs/monorepo/plan/P8.md`._


**Phase ID:** P8
**Objective:** After the four bodies of code compile as ONE module (`github.com/LerianStudio/midaz/v3`, single root go.mod, no go.work/replace/shims — PD-1), harmonize the operational plumbing: Makefile/mk fragments, Dockerfiles, docker-compose infra superset, GitHub Actions CI, and the release/versioning pipeline. End state: 4 deploy units (ledger+fees+crm `:3002`, tracer `:4020`, reporter-manager `:4005`, reporter-worker `:4006`), one repo-wide semantic-release, one shared-workflow version, one Go version, one golangci config, zero `github_token`/private-module machinery, and the external Helm `midaz` chart + `midaz-firmino-gitops` updated in lockstep.

**Hard gate:** Every task here is downstream of module/lib-commons unification (P7) and the four moves (P3–P6). CI consolidation is the LAST step of the merge, not the first. Nothing unified builds until the code is one module. No shims, no `replace`, no temporary fences anywhere — any such construct is a plan defect.

**Locked decisions in force:** PD-1 (single root go.mod, no go.work/replace/fence). PD-2 (CRM `ErrorCodeTransformer` shim deleted + 12 dead `CRM-00xx` codes pruned in P3 — `error_transformer_test.go` is the shim test that dies with it; `backward_compat_test.go` is a legit MT test and is NOT touched). PD-3 (fresh git import, origins archived read-only, reporter `ast-before-*` excluded). PD-4 (midaz on `lib-commons/v5` GA `v5.2.0`, `lib-observability v1.0.1`). PD-7 (fees persist as new Mongo collections — no separate `plugin-fees-mongodb`). TRACER ROLE is independent of `otel-lgtm` (no telemetry coupling; tracer just points at shared Postgres).

**Upstream phase references (LOCKED numbering — `depends_on` may name them):**
- `P1` — bump midaz off `v5.2.0-beta.12` to `lib-commons/v5` GA (PD-4). `P1-T06` records the canonical downstream version pin (`v5.2.0` first GA; `v5.2.1` equally safe). This is the version CI reads.
- `P2a` — plugin-fees IN-PLACE dependency migration (lib-observability) before any move.
- `P2b` — reporter IN-PLACE dependency migration; `P2b-T14` proves clean `go mod download` (no private module).
- `P2c` — tracer IN-PLACE dependency migration (the long pole); `P2c-T22` is the tracer ready-to-move gate.
- `P3` — **crm → ledger service collapse** (incl. PD-2 `ErrorCodeTransformer` deletion + dead-code prune; `initCRM`, `crmRouteRegistrar`, `ModuleCRM`; CRM CI/Docker/Makefile removal at collapse time via P3-T16/T17/T18).
- `P4` — **plugin-fees → ledger service collapse** (the third-rail move; fee seam, `validate` reassignment, PD-5 refund-on-revert).
- `P5` — **tracer co-location** → `components/tracer`.
- `P6` — **reporter co-location** → `components/reporter-manager` + `components/reporter-worker`.
- `P7` — **go.mod unification & final tidy**. `P7-T01` re-verifies the GA pin at unification time; `P7-T13/T14/T15` add the godog dependency subtree + Makefile runner + CI job; `P7-T16` proves clean `go mod download` with NO transitive private Lerian module; `P7-T17` records the Phase-8 readiness handoff (github_token machinery removable); `P7-T18` is the UNIFIED fee-on-revert / pending-cancel REFUND balance proof (third rail, PD-5); `P7-T19` is the out-of-repo lockstep coordination check (Helm / gitops / APIDog).

**Ownership boundary with the move phases (do not double-edit):** The CRM-specific teardown (CRM Makefile/Dockerfile/compose deletion, CRM removed from CI fan-out, CRM crypto-key gen folded into ledger) happens at COLLAPSE time in P3 (P3-T16/T17/T18). P8 does NOT redo CRM-specific removal; P8 performs the FINAL cross-component normalization across all four surviving components and verifies CRM is already gone. Where a P8 task touches a target P3 already edited (e.g. the Makefile component list), P8 reconciles the end state and asserts the CRM remnant is absent — it does not reintroduce CRM to delete it again.

---

## Task DAG (intra-phase)

All intra-phase edges point from a higher task number to a strictly lower one, so the graph is acyclic by construction.

- P8-T02 → P8-T01
- P8-T03 → P8-T02
- P8-T04 → P8-T03
- P8-T05 → P8-T02
- P8-T06 → P8-T01
- P8-T07 → (move phases only)
- P8-T08 → P8-T07
- P8-T09 → (move phases only)
- P8-T10 → P8-T02, P8-T09
- P8-T11 → P8-T01, P8-T07, P8-T21
- P8-T12 → P8-T01, P8-T08
- P8-T13 → P8-T11
- P8-T14 → P8-T11, P8-T12, P8-T13
- P8-T16 → P8-T11
- P8-T17 → P8-T14, P8-T16
- P8-T18 → P8-T06, P8-T07, P8-T08, P8-T11, P8-T12, P8-T13, P8-T14, P8-T16
- P8-T19 → P8-T05, P8-T09, P8-T10
- P8-T20 → P8-T11, P8-T18, P8-T22
- P8-T21 → P8-T07 (decision task feeding T11)
- P8-T22 → (none — early coordination, must START before T18 lands a tag)

---

## Tasks

### P8-T01 — Record the single Go / golangci / lib-commons version constants for CI from the already-chosen P1/P7 GA pin (reference, not a gate)

**Description:** This is a RECORD/REFERENCE task, not a runtime gate and not a re-verification. The live GA verification already happened upstream: P1-T01 verified `github.com/LerianStudio/lib-commons/v5` GA exists on the proxy and P1-T06 recorded the canonical downstream pin; P7-T01 re-confirms it at unification time. Verified ground truth (live proxy): `v5.2.0` is the first GA of the v5.2 line and `v5.2.1` is an equally-safe GA patch — both resolve as stable non-pre-release tags. Per PD-4 the recorded pin is **`lib-commons/v5 v5.2.0`** (`v5.2.1` is an accepted equally-safe alternative). There is NO live "if no GA exists" branch — that contingency is dead and removed. This task simply transcribes the three CI-facing constants into one referenceable place so the Makefile (P8-T02) and the CI workflows (P8-T11/T12) cite a single source: (1) `lib-commons/v5` pin = `v5.2.0`; (2) single Go toolchain = `1.26.3` (midaz current — reporter's `1.26`/toolchain `1.26.2` is dropped in P7-T02); (3) single golangci-lint version = the value the unified `.golangci.yml` floor (P8-T06) is validated at, currently `v2.4.0` as in midaz's `go-combined-analysis.yml`. No code change, no backward dependency on P1.

**Files:**
- `go.mod` (read — confirm `lib-commons/v5 v5.2.0` line after P7's bump landed)
- `docs/monorepo/plan/P8.md` (record the three constants in Open Items)

**Depends on:** P1-T06, P7-T01

**Acceptance criteria:**
- The three constants are recorded in one place and referenced verbatim by P8-T02 (Makefile pins) and P8-T11/T12 (CI pins): `lib-commons/v5 v5.2.0`, Go `1.26.3`, golangci-lint `v2.4.0`.
- No "if no GA exists" branch survives anywhere in P8 — the recorded GA is `v5.2.0`.
- No P8→P1 backward dependency edge exists (P8-T01 consumes P1-T06's output; it does not gate P1).

**Tests:** `grep -n 'lib-commons/v5 v5.2.0' go.mod` confirms the merged pin; `go list -m github.com/LerianStudio/lib-commons/v5` reports `v5.2.0` (or recorded `v5.2.1`); the three constants appear identically in the Makefile and both CI workflows after T02/T11/T12.

**Effort:** S — 1-2h.

**Risk refs:** R3

---

### P8-T02 — Normalize the root Makefile component list (kill the ledger-special-cased footgun) and pin tool versions in one place

**Description:** Replace the brittle `COMPONENTS := $(INFRA_DIR) $(CRM_DIR)` (root `Makefile` L16) + the ledger/CRM hand-handling pattern with a single complete Go-component list. Define `LEDGER_DIR`, `TRACER_DIR`, `REPORTER_MANAGER_DIR`, `REPORTER_WORKER_DIR`; set the Go-component list to `ledger tracer reporter-manager reporter-worker` (dir-var equivalents). Remove `CRM_DIR` and every `crm`/`$(CRM_DIR)` reference — CRM is already collapsed into ledger by P3 (P3-T16 reworked COMPONENTS; this task reconciles the end state to the four-component list and asserts no CRM remnant). Infra stays handled separately (no Go build).

There are TWO distinct structural shapes in the live Makefile, and BOTH must be normalized — missing either reintroduces the footgun:

- **Loop-based (already iterate `$(COMPONENTS)`, some then hand-append `$(LEDGER_DIR)`):** `check_env_files` (L37 loop), `lint` (L216 loops `$(COMPONENTS)` then hand-appends a `$(LEDGER_DIR)` block at L225), `format`/`tidy` (loop), `set-env` (L388 loop), `clear-envs` (L415 loop), and the `$(COMPONENTS) $(LEDGER_DIR)` hand-append at L554. Rewrite these so they iterate the ONE normalized four-component list with NO trailing `$(LEDGER_DIR)` append and NO `$(CRM_DIR)`.
- **Hand-unrolled per-component (NOT loops today — literal `@(cd $(LEDGER_DIR) && $(MAKE) build)` then `@(cd $(CRM_DIR) && $(MAKE) build)` blocks):** `build` (L180), `up` (L436), `down` (L449), `start` (L466), `stop` (L477), `rebuild-up` (L493). These have no `$(COMPONENTS)` loop at all. CONVERT each to a single `$(foreach c,$(COMPONENTS),...)` / shell `for` loop over the four-component list.

**Ownership split with P8-T10:** the docker-lifecycle targets `up`/`down`/`start`/`stop`/`rebuild-up` are CONVERTED to the loop shape HERE (T02 owns the structural conversion: hand-unrolled → loop over the normalized list, CRM removed). P8-T10 then RE-SEQUENCES those same targets (inserts `wait-for-infra`, orders infra→backends, reverses on down) on top of the loop T02 produced. T02 = "make it a loop over the right list"; T10 = "order the loop correctly and gate on infra health". Neither hand-unrolls; the target is loop-based after T02 and stays loop-based after T10.

Pin `GOLANGCI_LINT_VERSION` (L19) to the golangci-lint version recorded in P8-T01 and add a `GO_VERSION` pin (`1.26.3`), retaining the existing "keep in sync with go-combined-analysis.yml" comment.

**Files:**
- `Makefile`

**Depends on:** P3, P8-T01

**Acceptance criteria:**
- `grep -n CRM_DIR Makefile` returns nothing; `grep -c CRM Makefile` == 0.
- No target hand-appends `$(LEDGER_DIR)` after a `$(COMPONENTS)` loop; no target hand-unrolls per-component `cd` blocks — `build`/`up`/`down`/`start`/`stop`/`rebuild-up` each iterate the single list.
- Adding a future component requires editing ONLY the component-list var, not every fan-out target.
- `make -n build` and `make -n up` enumerate exactly: ledger, tracer, reporter-manager, reporter-worker (+ infra separately), each via the loop.

**Tests:** `make -n build`, `make -n up`, `make -n set-env`, `make -n lint` each expand over the new list with no `crm` and no duplicate/trailing ledger entry; `grep -c CRM Makefile` == 0; `grep -nE '@\(cd \$\((LEDGER|CRM)_DIR\)' Makefile` returns nothing for the lifecycle targets.

**Effort:** M — 0.5-1 day.

**Risk refs:** R25

---

### P8-T03 — Promote tracer's clean mk/{docker,database,docs,quality,security}.mk fragments to root and replace midaz's inlined sec targets

**Description:** Midaz root `mk/` has only `coverage-unit.mk` + `tests.mk` and inlines security targets in the root Makefile (`sec-gosec` ~L307, `sec-govulncheck` ~L327). Promote tracer's factored fragments — `mk/docker.mk` (keyed off `DOCKER_CMD` + `SERVICE_NAME`: up/down/start/stop/logs/rebuild-up/run/ps/build-docker), `mk/database.mk`, `mk/docs.mk`, `mk/quality.mk`, `mk/security.mk` — to the monorepo root `mk/`. Replace the root Makefile's inlined `sec-gosec`/`sec-govulncheck` block with `include $(MK_DIR)/security.mk`. Each fragment must be parameterized on `SERVICE_NAME` + `MIDAZ_ROOT` so components `include` it rather than re-implementing. This is a net upgrade for midaz (it lacks database/docs/quality/security fragments today). Reconcile any tracer-specific assumptions (e.g. hardcoded ports) into variables.

**Files:**
- `mk/docker.mk` (NEW — promoted from tracer)
- `mk/database.mk` (NEW — promoted from tracer)
- `mk/docs.mk` (NEW — promoted from tracer)
- `mk/quality.mk` (NEW — promoted from tracer)
- `mk/security.mk` (NEW — promoted from tracer; replaces inlined root sec targets)
- `Makefile` (remove inlined `sec-gosec`/`sec-govulncheck`, add `include $(MK_DIR)/security.mk`)

**Depends on:** P5 (tracer code/mk physically in `components/tracer`), P8-T02

**Acceptance criteria:**
- `make sec` still runs gosec + govulncheck (now via `mk/security.mk`); `make sec SARIF=1` still produces SARIF.
- The inlined `sec-gosec`/`sec-govulncheck` recipe bodies no longer appear in the root `Makefile`.
- Each promoted fragment references `SERVICE_NAME`/`MIDAZ_ROOT`, no hardcoded `tracer` paths.

**Tests:** `make sec` exits 0 on clean code; `make -n sec` shows the recipe sourced from `mk/security.mk`; `grep -n "sec-gosec" Makefile` returns nothing.

**Effort:** M — 0.5-1 day.

**Risk refs:** R25

---

### P8-T04 — Adopt one component-Makefile template and apply it to tracer / reporter-manager / reporter-worker

**Description:** Establish a single component-Makefile template keyed off `SERVICE_NAME` + `MIDAZ_ROOT` that `include`s the root `mk/*.mk` fragments (docker, coverage-unit, security, etc.) and implements the standardized target set required by the fan-out: `build test lint format tidy sec build-docker up start down stop restart rebuild-up clean-docker logs logs-api ps run generate-docs dev-setup help`. Write `components/tracer/Makefile`, `components/reporter-manager/Makefile`, `components/reporter-worker/Makefile` from this template. Each derives `MIDAZ_ROOT ?= $(shell cd ../.. && pwd)`, sets `SERVICE_NAME`, sets `COVERAGE_PACKAGES := ./components/<name>/...`, and `include`s shared fragments — no hand-rolled docker target blocks. Add component-delegation targets to root: `make tracer COMMAND=`, `make reporter-manager COMMAND=`, `make reporter-worker COMMAND=` (extend the existing `all-components` loop's list, do not add bespoke logic per component).

**Files:**
- `components/tracer/Makefile` (NEW — from template; tracer's standalone root Makefile + make.sh deleted in P8-T05)
- `components/reporter-manager/Makefile` (NEW — from template)
- `components/reporter-worker/Makefile` (NEW — from template)
- `Makefile` (add `tracer`/`reporter-manager`/`reporter-worker` delegation targets)

**Depends on:** P5, P6, P8-T03

**Acceptance criteria:**
- Each component Makefile implements the full standardized target set and `include`s root `mk/*.mk` (no inlined docker/coverage logic).
- `make tracer COMMAND=build`, `make reporter-manager COMMAND=build`, `make reporter-worker COMMAND=build` each delegate correctly.
- `make ledger COMMAND=<x>` still works (untouched seam).

**Tests:** `make -n tracer COMMAND=up`, `make -n reporter-worker COMMAND=build` expand to a `cd components/... && $(MAKE) <target>`; `make tracer COMMAND=lint` runs golangci against `components/tracer/...`.

**Effort:** M — 1 day.

**Risk refs:** R25

---

### P8-T05 — Verify crm generate-keys already migrated into ledger set-env; delete fees standalone Makefile/mk/make.sh

**Description:** CRM's crypto-key generation (`inject_crypto_key` helper + `generate-keys` target producing `LCRYPTO_HASH_SECRET_KEY`/`LCRYPTO_ENCRYPT_SECRET_KEY` via `alpine/openssl rand -hex 32`) was migrated into ledger's setup path at COLLAPSE time by P3-T16 (which also deleted `components/crm/Makefile`). P8 does NOT redo that migration. This task (1) VERIFIES the ledger `set-env`/`dev-setup` path generates the LCRYPTO keys and that root `set-env` no longer references `$(CRM_DIR)` or `components/crm`; and (2) DELETES the fees standalone build scaffolding that arrived with the fees source repo and is dead after the P4 embed: fees' standalone `Makefile`, `mk/tests.mk`, and `make.sh`. Any fees-only test target worth keeping migrates into ledger's test surface (confirm with P4's test layout). The root `set-env` ledger-special-case env-copy block is folded into the normalized loop from P8-T02 if P3 left any remnant.

**Files:**
- `components/ledger/Makefile` (verify `generate-keys` + `inject_crypto_key` present from P3-T16; no change unless a remnant needs folding)
- `Makefile` (root `set-env`, `check_env_files`: verify no `components/crm` reference survives; fold any ledger-special-case into the normalized loop)
- fees standalone `Makefile`, `mk/tests.mk`, `make.sh` (DELETE — were in plugin-fees source repo, dead after P4 embed)

**Depends on:** P3, P4, P8-T02

**Acceptance criteria:**
- `make set-env` from a clean checkout produces `components/ledger/.env` AND generates `LCRYPTO_HASH_SECRET_KEY`/`LCRYPTO_ENCRYPT_SECRET_KEY` in it (64-hex each), with NO `components/crm` reference.
- No fees standalone Makefile/mk/make.sh remains anywhere in the tree.
- `make set-env` does not error on the missing crm dir.

**Tests:** From clean tree: `make set-env` then `grep -E '^LCRYPTO_(HASH|ENCRYPT)_SECRET_KEY=.{64}$' components/ledger/.env` matches both keys; `make set-env` re-run is idempotent (keys not regenerated); `grep -rn 'CRM_DIR\|components/crm' Makefile` returns nothing; fees standalone Makefile/mk absent.

**Effort:** S — 2-4h.

**Risk refs:** R7, R17

---

### P8-T06 — Reconcile to ONE root .golangci.yml (fees strict config as floor) and run a per-component lint-cleanup pass

**Description:** Today midaz root `.golangci.yml` is 3.0K; fees ships a 6.2K strict config (newest linter). Adopt fees' strict config as the monorepo floor at root `.golangci.yml`, governing ledger+embedded-fees+embedded-crm, tracer, and reporter. Then run a lint-cleanup pass per folded component (parallelizable, mechanical) to clear the new findings the stricter config surfaces across previously-looser code. Use path-scoped config blocks or `//nolint` ONLY where a real divergence exists (e.g. tracer generated code, CEL/antlr-generated files) — not as a blanket suppression. Delete the per-repo `.golangci.yml` copies that came in with tracer/reporter/fees. NOTE on effort: the per-component cleanup parallelizes, but reconciling ONE root config that satisfies all four (especially tracer's antlr/CEL generated trees, which need carefully-scoped exclusions) is serial and the time sink — budget for a long tail of findings.

**Files:**
- `.golangci.yml` (root — replace with fees strict config as floor)
- per-component generated-code exclusions inside `.golangci.yml` (issues.exclude-rules paths) as needed
- tracer/reporter/fees `.golangci.yml` copies (DELETE if they landed in component dirs)

**Depends on:** P3, P4, P5, P6, P8-T01

**Acceptance criteria:**
- Exactly one `.golangci.yml` at repo root; no per-component lint configs except documented path-scoped exclusions for generated code.
- `golangci-lint run ./...` (at the version recorded in P8-T01) exits 0 across the whole module.
- The chosen golangci version matches P8-T01 and the Makefile pin (P8-T02).

**Tests:** `make lint` exits 0 over all four components; `golangci-lint run --timeout=5m ./components/... ./pkg/...` clean; CI go-analysis lint stage green (validated in P8-T18).

**Effort:** L — 1-2 days (config reconciliation is serial and dominates; cleanup pass parallelizable per component).

**Risk refs:** R11

---

### P8-T07 — Switch tracer Dockerfile to repo-root build context; delete fees and crm Dockerfiles; pin the WORKDIR→migrations-COPY contract

**Description:** Midaz/reporter Dockerfiles already build with context `../../` (repo root) and `COPY . .` + build `components/<x>/cmd/app/main.go` (see `components/ledger/Dockerfile`, which uses builder `WORKDIR /ledger-app`). Tracer's Dockerfile built with context `.` (own repo root). Rewrite `components/tracer/Dockerfile` to the monorepo pattern: context `../../`, `dockerfile: ./components/tracer/Dockerfile`, build `components/tracer/cmd/app/main.go`.

**WORKDIR↔COPY contract (the silent-failure trap):** the migrations COPY in the distroless final stage is HARDCODED to the builder-stage `WORKDIR` (ledger uses `COPY --from=builder /ledger-app/components/ledger/migrations/...`). The tracer rewrite MUST keep the builder `WORKDIR` and the final-stage `COPY --from=builder <WORKDIR>/components/tracer/migrations ...` IN SYNC. Set the tracer builder `WORKDIR` explicitly (e.g. `/tracer-app`, or standardize all images to one `WORKDIR`) and make the migrations COPY source path exactly `<WORKDIR>/components/tracer/migrations`. A `--help` smoke test PASSES even when migrations are missing, so the acceptance test below asserts the migrations are actually present in the final image, not just that `--help` runs. Keep tracer's `GOMEMLIMIT` and distroless-nonroot final image.

DELETE the fees Dockerfile (fees folds into the ledger binary — no separate image) and the crm Dockerfile (`components/crm/Dockerfile`, EXPOSE 4003 — crm collapses into ledger; note P3-T17 may already have deleted it at collapse time — verify absence and delete any remnant). Reporter-manager/worker Dockerfiles already use repo-root context; verify their build target paths point at `components/reporter-manager/cmd/app/main.go` and `components/reporter-worker/cmd/app/main.go`. Worker stays fat alpine + Chromium (cannot be distroless); harmonize the non-Chromium manager image to distroless-nonroot if cheap.

**Files:**
- `components/tracer/Dockerfile` (rewrite context + COPY/build paths; pin WORKDIR→migrations COPY)
- `components/crm/Dockerfile` (DELETE / verify already deleted by P3-T17)
- fees `Dockerfile` (DELETE — was in plugin-fees source repo)
- `components/reporter-manager/Dockerfile` (verify build target + context)
- `components/reporter-worker/Dockerfile` (verify build target + context; stays fat alpine+Chromium)

**Depends on:** P3, P4, P5, P6

**Acceptance criteria:**
- `docker build -f components/tracer/Dockerfile .` (context repo root) produces a working tracer image.
- The tracer final image CONTAINS its migrations at a known path matching the builder `WORKDIR` — proven by listing the `.sql` files in the image, not by `--help`.
- No `components/crm/Dockerfile` and no fees Dockerfile remain.
- Reporter manager/worker images build from repo-root context.

**Tests:** `docker build --build-arg TARGETOS=linux --build-arg TARGETARCH=amd64 -f components/tracer/Dockerfile -t midaz-tracer:test .` succeeds; `docker run --rm midaz-tracer:test /app --help` runs; `docker run --rm --entrypoint sh midaz-tracer:test -c 'ls components/tracer/migrations/*.sql'` (or the path matching the chosen layout) lists the migration files (asserts migrations shipped); same build for reporter-manager/worker; `ls components/crm/Dockerfile` fails.

**Effort:** M — 0.5-1 day.

**Risk refs:** R16

---

### P8-T08 — Drop github_token BuildKit secret, .secrets/, go_private_modules, and ~/.netrc dance after the P7-verified clean go mod download

**Description:** P7-T16 proved there are no transitive private Lerian modules left once `midaz/v3` vanishes on merge, and P7-T17 recorded the Phase-8 readiness handoff that the github_token machinery is removable. Re-confirm here with a clean `go mod download` (empty module cache, no `~/.netrc`, no GOPRIVATE) that every dependency resolves through the public GOPROXY. THEN delete ALL github_token machinery that came in with tracer/fees: the `--mount=type=secret,id=github_token` lines + `~/.netrc` writes in any incoming Dockerfile, the `.secrets/` directory and its compose file-secret wiring, and `go_private_modules: "github.com/LerianStudio/*"` inputs in any incoming CI workflow. midaz/reporter already prove the common Lerian libs (lib-auth, lib-commons, lib-observability, lib-streaming, lib-license-go) are public. This is squarely "liso e final" — legacy auth plumbing the merge makes obsolete. The tracer Dockerfile github_token mount is removed as a HARD assertion (grep == 0), not "likely none after the rewrite".

**Files:**
- `components/tracer/Dockerfile` (HARD-assert zero `--mount=type=secret,id=github_token` / netrc lines post-T07 rewrite)
- `.secrets/` (DELETE if present — came from fees source repo)
- compose files referencing `.secrets/github_token.txt` (remove the secret block) — search the WHOLE tree, including component-local compose files
- any incoming CI workflow with `go_private_modules` (remove the input — folded into P8-T11/T12)
- `.gitignore` (drop `.secrets/` entry once dir is gone)

**Depends on:** P7, P8-T07

**Acceptance criteria:**
- `GOFLAGS=-mod=mod GOPRIVATE= GONOSUMCHECK= go mod download` with a fresh `GOMODCACHE` and NO `~/.netrc` succeeds for the whole module.
- A whole-tree grep (excluding `.git`) for `github_token`, `go_private_modules`, `.netrc`, and `id=github_token` returns nothing.
- No `.secrets/` directory; no compose secret references it anywhere in the tree.

**Tests:** `GOMODCACHE=$(mktemp -d) GOPRIVATE= go mod download ./...` exits 0 from a clean network identity; `grep -rIl --exclude-dir=.git "github_token\|go_private_modules\|\.netrc\|id=github_token" .` returns nothing (whole-tree, not just `.github/`+`components/`).

**Effort:** S — 2-4h.

**Risk refs:** R3

---

### P8-T09 — Assemble the unified docker-compose infra superset (add SeaweedFS + KEDA; drop tracer-postgres + fees-mongo; reconcile versions)

**Description:** `components/infra/docker-compose.yml` is the single source of truth (most production-like): `mongo:8` rs0 (+init), `valkey/valkey:8`, `postgres:17` primary+replica, `grafana/otel-lgtm`, `rabbitmq:4.1.3-management-alpine`, `infra-network` (external owner). Extend it: ADD SeaweedFS (S3-compatible object store, reporter-only, net-new) and KEDA (worker autoscaler, net-new) from reporter's infra. DROP tracer's standalone `tracer-postgres` (`postgres:16-alpine`) — tracer points at the shared `midaz-postgres-primary` under its own database/schema (the 16→17 migration-compat check is a DB-dossier concern flagged in Open Items; this task lands the service deletion). DROP fees' `plugin-fees-mongodb` — fees rides ledger's existing Mongo (PD-7, new collections). Reconcile version skew toward midaz (newer): RabbitMQ stays `4.1.3` (drop reporter's `4.0`), Valkey stays `8` (drop reporter's `8.0-alpine`). otel-lgtm stays as-is — tracer is INDEPENDENT of it (TRACER ROLE decision), no telemetry coupling, tracer just points at shared Postgres.

**Files:**
- `components/infra/docker-compose.yml` (add SeaweedFS + KEDA services + volumes; keep on `infra-network`)
- tracer compose `tracer-postgres` service (DELETE; repoint tracer at `midaz-postgres-primary`)
- fees compose `plugin-fees-mongodb` service (DELETE — was in plugin-fees source repo)
- `components/infra/.env`/`.env.example` (add SeaweedFS/KEDA env if needed)

**Depends on:** P4, P5, P6

**Acceptance criteria:**
- `docker compose -f components/infra/docker-compose.yml config` validates with SeaweedFS + KEDA added and no `tracer-postgres`/`plugin-fees-mongodb`.
- RabbitMQ pinned `4.1.3`, Valkey `8`, Postgres `17`, Mongo `8`, otel-lgtm present and unchanged.
- SeaweedFS + KEDA join `infra-network`.

**Tests:** `docker compose -f components/infra/docker-compose.yml config -q` exits 0; `make infra COMMAND=up` brings up SeaweedFS (S3 endpoint reachable) and KEDA; `grep -rc "tracer-postgres\|plugin-fees-mongodb" components/` == 0.

**Effort:** M — 1-1.5 days (SeaweedFS/KEDA wiring is the time sink).

**Risk refs:** R14, R20

---

### P8-T10 — Put every component compose on infra-network, adopt reporter's wait-for-infra gate, and re-sequence make up/down on the T02 loop

**Description:** Standardize per-component compose files (`components/{ledger,tracer,reporter-manager,reporter-worker}/docker-compose.yml`) to join the single external `infra-network` (owned by infra). Drop per-component private networks in the unified dev topology (verified: crm compose carried both `infra-network` external AND a private `crm-network`; recommend dropping the private nets — they add little single-host and conflict with "liso"; keep only if a component genuinely needs isolation, none observed across the moved components). Adopt reporter's `wait-for-infra` Make target (midaz lacks it — its cross-compose `depends_on` does not work across separate compose projects) into the root Makefile, and have it cover the new SeaweedFS/KEDA services too (worker depends on RabbitMQ + SeaweedFS healthy).

**Ownership split with P8-T02:** T02 already CONVERTED `up`/`down`/`start`/`stop`/`rebuild-up` from hand-unrolled per-component blocks into loops over the normalized four-component list (CRM removed). This task RE-SEQUENCES those loop targets — it does NOT re-touch the loop's component enumeration. `up` becomes: `infra up` → `wait-for-infra` → loop the four backends. `down` reverses: loop the four backends down → infra down. Do not reintroduce per-component `cd` blocks; sequence the existing loop. CRM blocks are already gone after T02 — assert their absence.

**Files:**
- `Makefile` (root `up`/`down`/`start`/`stop`/`rebuild-up`: re-sequence the T02 loop with `wait-for-infra` + infra ordering; do NOT re-add per-component blocks)
- `Makefile` (add `wait-for-infra` target, adopted from reporter, covering SeaweedFS/KEDA)
- `components/ledger/docker-compose.yml`, `components/tracer/docker-compose.yml`, `components/reporter-manager/docker-compose.yml`, `components/reporter-worker/docker-compose.yml` (join `infra-network`, drop private nets)
- `components/crm/docker-compose.yml` (verify already deleted by P3-T17; delete any remnant)
- fees `docker-compose.yml` (DELETE — was in plugin-fees source repo)

**Depends on:** P8-T02, P8-T09

**Acceptance criteria:**
- `make up` sequences infra → wait-for-infra → the four backends (looping the T02 list); never references crm.
- `wait-for-infra` blocks until Postgres/Mongo/Valkey/RabbitMQ/SeaweedFS report healthy.
- No `components/crm/docker-compose.yml`; every component compose joins `infra-network` and carries no private network.

**Tests:** `make -n up` shows `wait-for-infra` between infra and backends and loops exactly the four components (no `cd $(CRM_DIR)`, no hand-unrolled block); `make up` end-to-end brings all four services healthy on `infra-network` (validated in P8-T19); `docker compose -f components/tracer/docker-compose.yml config` shows `infra-network` external and no `tracer-network`.

**Effort:** M — 1 day.

**Risk refs:** R17, R21

---

### P8-T11 — Consolidate build.yml: union filter_paths, drop crm/fees, pin one shared-workflow version, single midaz-* prefix, fix the TWO distinct helm/gitops key-mapping schemas and migration jobs

**Description:** Rewrite `.github/workflows/build.yml` to the unified topology. `filter_paths` (L19) becomes the union: `components/ledger`, `components/tracer`, `components/reporter-manager`, `components/reporter-worker` — DROP `components/crm` and never add a fees image (collapsed). Keep `shared_paths: go.mod go.sum pkg/ Makefile`, `path_level: '2'`, single `app_name_prefix: "midaz"` (L28; renames tracer/reporter images to `midaz-*`). Pin ONE shared-workflow version across every `uses:` (currently `@v1.27.5`; six versions exist across repos — pick the newest validated, e.g. tracer's `v1.32.0`, after reading the shared-repo changelog v1.27→v1.32 for input-name changes; a renamed input silently no-ops).

**The two key-mapping schemas are DISTINCT and must NOT be conflated** (verified live in build.yml):

- `helm_values_key_mappings` (L37, today `'{"midaz-crm": "crm", "midaz-ledger": "ledger"}'`) — keys are the `midaz-*` image names, values are bare Helm value-block keys. Update to:
  `'{"midaz-ledger":"ledger","midaz-tracer":"tracer","midaz-reporter-manager":"manager","midaz-reporter-worker":"worker"}'` (remove `midaz-crm`).
- `yaml_key_mappings` (L57, today `'{"midaz-crm.tag": ".crm.image.tag", "midaz-ledger.tag": ".ledger.image.tag"}'`) — keys carry a `.tag` SUFFIX and values are full dotted YAML paths (`.image.tag`). Update to:
  `'{"midaz-ledger.tag":".ledger.image.tag","midaz-tracer.tag":".tracer.image.tag","midaz-reporter-manager.tag":".manager.image.tag","midaz-reporter-worker.tag":".worker.image.tag"}'` (remove the crm mapping; PRESERVE the `.tag` key suffix and `.image.tag` value path — do NOT collapse this into the helm schema, or the gitops-update job cannot resolve which YAML key to bump and the pipeline builds four images then silently fails to deploy).

Keep the two ledger migration S3-upload jobs (onboarding, transaction); ADD a tracer migration S3-upload job ONLY IF the tracer-migration decision (P8-T21) says tracer ships S3-uploaded migrations. Reporter/worker have no SQL migrations. Harmonize registry policy to DockerHub+ghcr for all (reporter was ghcr-only) unless a licensing reason blocks it (flag in Open Items). Keep the APIDog e2e job.

**Files:**
- `.github/workflows/build.yml`

**Depends on:** P3, P4, P5, P6, P8-T01, P8-T07, P8-T21

**Acceptance criteria:**
- `filter_paths` lists exactly the four surviving components; no `components/crm`, no fees.
- All `uses:` refs pin the same single shared-workflow version.
- `helm_values_key_mappings` covers ledger/tracer/reporter-manager(`manager`)/reporter-worker(`worker`) with bare value-block keys; `yaml_key_mappings` covers the same four with `midaz-<x>.tag` keys and `.<x>.image.tag` values — the two schemas remain DISTINCT.
- Tracer migration S3-upload job added iff P8-T21 says tracer ships S3 migrations; ledger's two unchanged.

**Tests:** `yamllint .github/workflows/build.yml`; `grep -c "crm\|fees" .github/workflows/build.yml` == 0; the `uses:` shared-workflow version is a single value (`grep -oE '@v[0-9.]+' .github/workflows/build.yml | sort -u | wc -l` == 1); `yaml_key_mappings` keys all end in `.tag` and values all start `.` and end `.image.tag`; full validation in P8-T18 (real tag → fan-out → 4 images).

**Effort:** M — 0.5-1 day.

**Risk refs:** R12, R24

---

### P8-T12 — Consolidate go-combined-analysis.yml and pr-security-scan.yml: union filter_paths, drop crm/fees, single go/golangci, drop go_private_modules, preserve godog stage

**Description:** Update `.github/workflows/go-combined-analysis.yml` (`filter_paths` currently `'["components/crm", "components/ledger"]'` L31, `go_version: "1.26.3"` L34, `golangci_lint_version: "v2.4.0"` L35, `coverage_threshold: 85` L37) and `.github/workflows/pr-security-scan.yml` (`filter_paths` L37-39) to the unified four-component list, dropping crm and never adding fees. Pin `go_version` and `golangci_lint_version` to the single values recorded in P8-T01. Pin both workflows' `uses:` to the single shared-workflow version chosen in P8-T11. DROP any `go_private_modules: "github.com/LerianStudio/*"` input that arrived with tracer/reporter/fees (midaz proves the libs are public — P8-T08). Per-component 85% coverage threshold applies per directory automatically via path-level filtering — keep `fail_on_coverage_threshold: true`. **Preserve the godog stage:** P7-T15 stood up godog as a CI job and P7-T14 wired the Makefile runner. The consolidation MUST keep the godog invocation pointing at the unified module's BDD suite — do not drop or mis-scope it during the filter_paths rewrite. (Whether godog runs via the shared-workflow's built-in test stage or a bespoke midaz step is decided in P8-T18's sub-check.)

**Files:**
- `.github/workflows/go-combined-analysis.yml`
- `.github/workflows/pr-security-scan.yml`

**Depends on:** P3, P4, P5, P6, P8-T01, P8-T08

**Acceptance criteria:**
- Both workflows' `filter_paths` list exactly ledger/tracer/reporter-manager/reporter-worker; no crm, no fees.
- `go_version` and `golangci_lint_version` match P8-T01; `uses:` refs match the single shared-workflow version.
- No `go_private_modules` input anywhere.
- The godog BDD stage survives the consolidation and still targets the unified module.

**Tests:** `yamllint` both files; `grep -rl "go_private_modules\|components/crm" .github/workflows/` returns nothing; the godog stage is present (grep for the godog/cucumber invocation); go-analysis runs green on a PR touching each component (validated in P8-T18).

**Effort:** S — 3-4h.

**Risk refs:** R11, R3

---

### P8-T13 — Consolidate pr-validation.yml scope taxonomy (+tracer/reporter/fees, -crm) and the dependabot config (remove ALL collapsed-component entries)

**Description:** Update `.github/workflows/pr-validation.yml` `pr_title_scopes` (L30, currently `crm ledger api pkg infra migrations scripts deps workflows`): ADD `tracer`, `reporter`, `fees` (fees as a scope is reasonable since fees code now lives in ledger); REMOVE `crm` (crm code is folded into ledger and no longer a component). Keep `pr_title_types` (L18) and source-branch enforcement. Pin `uses:` to the single shared-workflow version. Note: the live workflow sets `require_scope: false` (L40), so an unknown scope WARNS, it does not hard-reject — the test below asserts warn-not-reject. If hard rejection of `crm`-scoped PRs is desired, set `require_scope: true` explicitly and adjust the assertion (decision: keep `false`, warn is sufficient — a stale `crm` scope is cosmetic, not a correctness gate).

Separately, consolidate `.github/dependabot.yml` (verified live — it carries STALE entries pointing at directories that no longer exist or are collapsed): `gomod` + `docker` entries for `/components/transaction` (L5/L74) and `/components/onboarding` (L19/L88) — both folded into ledger BEFORE P8 and their dirs are gone — and `/components/crm` (L47/L116) — folded into ledger by P3. In a single-go.mod monorepo there is ONE Go module to watch → ONE `gomod` ecosystem entry rooted at repo root (`/`), plus ONE `docker` ecosystem entry per surviving Dockerfile dir (ledger, tracer, reporter-manager, reporter-worker), plus the existing `github-actions` entry at `/`. DELETE the `/components/transaction`, `/components/onboarding`, `/components/crm`, and any `/components/fees` gomod AND docker entries explicitly. Decide whether to keep tracer's `dependabot-auto-merge.yml` (tracer-only today) repo-wide — recommend keep, it applies to the whole monorepo.

**Files:**
- `.github/workflows/pr-validation.yml`
- `.github/dependabot.yml`
- `.github/workflows/dependabot-auto-merge.yml` (NEW if adopting tracer's — repo-wide)

**Depends on:** P3, P4, P5, P6, P8-T11

**Acceptance criteria:**
- `pr_title_scopes` contains tracer/reporter/fees and NOT crm.
- `dependabot.yml` has exactly ONE `gomod` entry (root `/`) + one `docker` entry per surviving Dockerfile dir (ledger, tracer, reporter-manager, reporter-worker) + the `github-actions` entry; ZERO entries for transaction/onboarding/crm/fees.
- `uses:` ref matches the single shared-workflow version.

**Tests:** `yamllint .github/workflows/pr-validation.yml .github/dependabot.yml .github/workflows/dependabot-auto-merge.yml`; `grep -cE 'transaction|onboarding|crm|fees' .github/dependabot.yml` == 0; `grep -c 'package-ecosystem:.*gomod' .github/dependabot.yml` == 1; a PR titled `feat(tracer): ...` passes validation; a PR titled `feat(crm): ...` produces a scope WARNING (not a hard reject) given `require_scope: false`.

**Effort:** S — 2-4h.

**Risk refs:** R24

---

### P8-T14 — Delete the three duplicate workflow sets that came in with tracer/reporter/fees; keep midaz-canonical workflows

**Description:** Each incoming repo carried its own near-identical `build.yml`, `pr-validation.yml`, `go-combined-analysis.yml`, `pr-security-scan.yml`, `release.yml`, `gptchangelog.yml`. The single monorepo keeps midaz's (consolidated by P8-T11/T12/T13/T16/T17) and DELETES the tracer/reporter/fees copies — they were in their source repos, but verify none leaked into the merged tree under component dirs or stray `.github/` paths. Keep midaz's `release-notification.yml` (repo-wide, product-level). Adopt tracer's `dependabot-auto-merge.yml` once (handled in P8-T13). Net result: ONE of each workflow at `.github/workflows/`.

**Files:**
- any incoming `.github/workflows/*.yml` copies under component dirs or merged paths (DELETE)
- `.github/workflows/` (verify exactly one of each: build, pr-validation, go-combined-analysis, pr-security-scan, release, gptchangelog, release-notification, dependabot-auto-merge)

**Depends on:** P3, P4, P5, P6, P8-T11, P8-T12, P8-T13

**Acceptance criteria:**
- Exactly one of each workflow type at `.github/workflows/`; no component-local `.github/workflows/`.
- No duplicate `build.yml`/`release.yml`/etc. anywhere in the tree.

**Tests:** `find . -path ./.git -prune -o -name 'build.yml' -print` returns exactly `.github/workflows/build.yml`; `ls .github/workflows/` shows the canonical single set.

**Effort:** S — 1-2h.

**Risk refs:** R24

---

### P8-T16 — Adopt ONE repo-wide .releaserc.yml: keep midaz CHANGELOG canonical, add @saithodev backmerge, add .releaserc.hotfix.yml tied to a real workflow consumer

**Description:** Keep midaz's `.releaserc.yml` as the base (verified: it already commits `CHANGELOG.md` via `@semantic-release/git`, has the conventionalcommits releaseRules, and the main/develop-beta/release-candidate-rc branch model; it has NO `@saithodev/semantic-release-backmerge` today). ADD the `@saithodev/semantic-release-backmerge` plugin (main→develop) — reporter/fees have it, midaz lacks it, and midaz already does manual `chore(changelog): backmerge` commits (see recent git log), so automate it. ADD a `.releaserc.hotfix.yml` for hotfix releases off `main` (tracer/reporter/fees have one; midaz does not — verified absent). **Tie the hotfix config to a real consumer:** confirm the shared-workflow version chosen in P8-T11 actually has a hotfix release entrypoint that consumes a separate `.releaserc.hotfix.yml`; if the release workflow has no hotfix path, the file is dead config on arrival — in that case either wire the hotfix invocation in `release.yml` or drop the file. Keep the single repo-wide version model (option 1) — one tag → `build.yml` fans out per changed component, all stamped the same semver. Delete the incoming repos' `.releaserc.yml`/`.releaserc.hotfix.yml` copies. Document the version discontinuity (tracer/reporter reset to midaz's v3.x line) in the CHANGELOG.

**Files:**
- `.releaserc.yml` (add backmerge plugin to the plugins list)
- `.releaserc.hotfix.yml` (NEW — hotfix flow off main; only if a workflow consumer exists)
- `.github/workflows/release.yml` (verify/wire the hotfix entrypoint that consumes `.releaserc.hotfix.yml`)
- incoming `.releaserc.yml`/`.releaserc.hotfix.yml` copies (DELETE if they landed in tree)

**Depends on:** P8-T11

**Acceptance criteria:**
- One `.releaserc.yml` at root with the backmerge plugin present; one `.releaserc.hotfix.yml` IFF a workflow consumes it.
- Single repo-wide semantic-release model preserved (no per-component tagFormat).
- No incoming `.releaserc*` copies remain.
- The hotfix config has a real workflow consumer (not dead config).

**Tests:** `npx semantic-release --dry-run --no-ci` (or the shared-workflow's release in dry-run) parses both configs without plugin-resolution errors; `grep -c "saithodev" .releaserc.yml` == 1; the release workflow references `.releaserc.hotfix.yml` on the hotfix path (or the file is absent).

**Effort:** S — 0.5 day.

**Risk refs:** R24

---

### P8-T17 — Archive tracer/reporter/fees CHANGELOGs; keep gptchangelog single-app mode; rewrite STRUCTURE.md to the live topology

**Description:** Do NOT concatenate changelogs. Keep midaz's `CHANGELOG.md` (770K) as the canonical going-forward monorepo changelog. Move the three incoming changelogs (tracer 3.8K, reporter 4.8K, fees 130K) to `docs/monorepo/legacy-changelogs/{tracer,reporter,fees}.md` for provenance. Keep midaz's `gptchangelog.yml` in single-app mode (`# No filter_paths = single app mode` comment, correct for one repo-wide version); DELETE the incoming gptchangelog workflows. Rewrite the stale `STRUCTURE.md` (verified — still lists onboarding (L40), transaction (L65), crm (L117) as components) to reflect the live `components/{infra,ledger,tracer,reporter-manager,reporter-worker}` + `pkg/` topology. (Wider stale-doc/agent-doc rewrites — AGENTS.md, CLAUDE.md, llms-full.txt — are owned by P9's doc sweep; this task owns STRUCTURE.md only.)

**Files:**
- `docs/monorepo/legacy-changelogs/tracer.md` (NEW — archived)
- `docs/monorepo/legacy-changelogs/reporter.md` (NEW — archived)
- `docs/monorepo/legacy-changelogs/fees.md` (NEW — archived)
- `CHANGELOG.md` (add a one-line version-discontinuity note; do NOT merge incoming history)
- `STRUCTURE.md` (rewrite to live topology)
- incoming `gptchangelog.yml` copies (DELETE)

**Depends on:** P8-T14, P8-T16

**Acceptance criteria:**
- Three legacy changelogs archived under `docs/monorepo/legacy-changelogs/`; midaz `CHANGELOG.md` not bloated by concatenation.
- One `gptchangelog.yml` in single-app mode.
- `STRUCTURE.md` lists the five live components + pkg, no onboarding/transaction/crm-as-component.

**Tests:** `ls docs/monorepo/legacy-changelogs/` shows the three files; `grep -cE 'onboarding|transaction' STRUCTURE.md` reflects accurate (collapsed) reality; `grep -c "filter_paths" .github/workflows/gptchangelog.yml` == 0.

**Effort:** S — 0.5 day.

**Risk refs:** R24, R25

---

### P8-T18 — End-to-end CI validation: cut a real tag, confirm fan-out builds exactly 4 images, all PR-gates green (incl. godog + PD-2 sentinel invariant)

**Description:** With all workflows consolidated, validate the real pipeline on a throwaway branch/tag. (1) Open a PR touching each component in turn and confirm `go-combined-analysis` + `pr-security-scan` + `pr-validation` go green with the single Go/golangci/shared-workflow versions and the 85% coverage gate per component. (2) Cut a real semantic-release-driven tag and confirm `build.yml` fan-out builds and pushes EXACTLY four images (`midaz-ledger`, `midaz-tracer`, `midaz-reporter-manager`, `midaz-reporter-worker`) to DockerHub+ghcr, and ZERO `midaz-crm`/`plugin-fees` images. (3) Confirm only changed components rebuild (touch only `components/tracer`, confirm only tracer image rebuilds; touch `pkg/`, confirm all four rebuild via `shared_paths`). (4) Confirm migration S3-upload jobs fire for ledger (and tracer iff P8-T21 says so). (5) **godog sub-check:** confirm the consolidated `go-combined-analysis.yml`/`build.yml` still RUN the godog BDD stage for the unified module and it passes — the consolidation in P8-T11/T12 must not have dropped or mis-scoped the net-new godog mode P7-T15 stood up. Record the workflow-ownership decision: godog runs via the shared-workflow's built-in test stage vs. a bespoke midaz step (FG3 resolution — decide and document here; default to whichever P7-T15 wired, do not leave it deferred). (6) **PD-2 invariant:** assert no dead `CRM-00xx` sentinel survives in the SHARED `pkg/constant/errors.go` after P3's prune — `grep -c 'CRM-00' pkg/constant/errors.go` matches only the codes still referenced by live CRM-in-ledger handlers (the 12 dead 1:1 codes are gone). This is the last build-level gate that catches a missed upstream prune. This task is the gate that proves the whole phase.

**Files:**
- (validation only — no file changes; may produce a CI run log + the godog-ownership decision captured in plan notes)

**Depends on:** P8-T06, P8-T07, P8-T08, P8-T11, P8-T12, P8-T13, P8-T14, P8-T16

**Acceptance criteria:**
- Real tag → exactly 4 images built+pushed; no crm/fees image produced.
- Changed-detection works: single-component change rebuilds only that image; `pkg/`/`go.mod` change rebuilds all four.
- All PR gates green at the pinned versions; 85% coverage gate satisfied per component.
- The godog BDD stage runs and passes under the consolidated workflows; the shared-vs-bespoke godog ownership is decided and recorded.
- No dead `CRM-00xx` sentinel survives in `pkg/constant/errors.go` (PD-2 prune confirmed at the build level).

**Tests:** Real `git tag` push triggering `build.yml`; inspect the Actions run: `has_builds == true`, four image jobs, registry shows the four `midaz-*` tags; a `pkg/`-only PR rebuilds all four; the godog job is present and green in the run; `grep -c 'CRM-00' pkg/constant/errors.go` equals the count of still-live CRM codes (12 dead ones absent).

**Effort:** L — 1-2 days (validation + iterating on real CI failures).

**Risk refs:** R11, R12, R24

---

### P8-T19 — Local dev validation: clean make set-env → up → all 4 services healthy, env merge has no silent missing vars

**Description:** Validate the local dev loop end to end from a clean checkout. (1) `make set-env` produces every component `.env` including ledger's merged fees+crm env (LCRYPTO keys generated). (2) `make up` brings infra (incl. SeaweedFS/KEDA) up, `wait-for-infra` gates, then the four backends start healthy. (3) Confirm no silent missing env var: 3-way diff the fees + crm `.env.example` surfaces against ledger's merged `.env.example` to catch namespace collisions (`SERVER_PORT`, `MONGO_*`, `DB_*` — ledger's win; fees/crm-specific knobs prefixed/namespaced per the config phase). (4) `make down` reverses cleanly. This catches the R17 class of "passes CI build, fails at runtime on missing env" before any deploy.

**Files:**
- `components/ledger/.env.example` (verify merged fees+crm vars present, no collisions)
- (validation — confirms P8-T05/T09/T10 wiring)

**Depends on:** P8-T05, P8-T09, P8-T10

**Acceptance criteria:**
- Clean `make set-env && make up` brings all four services to healthy with no missing-env runtime failure.
- Ledger `.env.example` contains all required fees + crm vars with no key collision overwriting ledger's own.
- `make down` stops all and tears down infra last.

**Tests:** From clean tree: `make set-env && make up`, then hit `:3002/health` (ledger+fees+crm), `:4020/readyz` (tracer), `:4005/health` (reporter-manager), `:4006` (reporter-worker health) — all OK; `make down` exits clean; diff fees/crm `.env.example` vs ledger merged shows every var accounted for.

**Effort:** M — 1 day.

**Risk refs:** R17, R7

---

### P8-T20 — Out-of-repo lockstep: update external Helm "midaz" chart + midaz-firmino-gitops; remove crm/fees image entries; align APIDog e2e

**Description:** The image rename to `midaz-*` and the deletion of crm/fees images break ArgoCD sync unless the external repos update in lockstep (R12, cross-team blast radius — these repos are NOT in midaz; the in-repo coordination check happened in P7-T19). Coordinate: (1) Helm chart `midaz` adds `tracer`, `reporter-manager` (value key `manager`), `reporter-worker` (value key `worker`) value blocks and REMOVES the `crm` and `plugin-fees` image entries — matching `helm_values_key_mappings` from P8-T11 (bare value keys: `ledger`/`tracer`/`manager`/`worker`). (2) `midaz-firmino-gitops` updates the `yaml_key_mappings` TARGETS — the dotted value paths `.ledger.image.tag`, `.tracer.image.tag`, `.manager.image.tag`, `.worker.image.tag` — and removes crm/fees keys; the KEY side stays `midaz-<x>.tag` (the `.tag`-suffixed schema, NOT the helm bare-key schema). (3) APIDog e2e scenarios extend to cover the new API-bearing components (tracer, reporter-manager) and drop crm-standalone scenarios. Confirm ownership and sequencing BEFORE the image-rename/delete tag in P8-T18 lands — a co-located component without Helm/gitops entries builds but never deploys. P8-T22 owns the owner-availability fallback if a chart owner does not respond in lockstep; this task does the actual coordinated edits once owners are confirmed.

**Files:**
- (external) Helm chart `midaz` values + templates
- (external) `LerianStudio/midaz-firmino-gitops` `yaml_key_mappings` targets
- (external) APIDog test scenarios

**Depends on:** P8-T11, P8-T18, P8-T22

**Acceptance criteria:**
- Helm `midaz` chart has value keys for ledger/tracer/manager/worker and NO crm/plugin-fees image keys; matches build.yml `helm_values_key_mappings`.
- `midaz-firmino-gitops` updates target `.ledger.image.tag`/`.tracer.image.tag`/`.manager.image.tag`/`.worker.image.tag` with `midaz-<x>.tag` keys; crm/fees keys removed.
- A real fan-out tag triggers gitops update + ArgoCD sync with no orphaned/missing key errors; APIDog e2e passes for the new components.

**Tests:** Post-tag, `update_gitops` job succeeds and ArgoCD syncs the four apps with no "key not found" / "image not in chart" errors; APIDog e2e job green; manual confirm the chart has no crm/fees image stanzas.

**Effort:** L — 1-2 days engineering, but WALL-CLOCK-blocked on cross-team confirmation — calendar time, not engineering time, is the real cost (see P8-T22 fallback).

**Risk refs:** R12

---

### P8-T21 — DECIDE whether tracer ships S3-uploaded migrations (feeds build.yml migration jobs and the Dockerfile migrations COPY)

**Description:** Tracer has a `migrations/` dir, but the deploy model determines whether those migrations are (a) baked into the image and run by the binary at startup (no S3-upload job needed), or (b) uploaded to S3 like ledger's onboarding/transaction migrations and applied by a separate job. This decision is currently an unresolved Open Item that BLOCKS two tasks from being written correctly: P8-T11 (whether to add a tracer migration S3-upload job) and P8-T07 (whether the tracer image must SHIP its migrations via the WORKDIR→COPY contract). Resolve it explicitly against the deploy model: inspect how tracer applied migrations standalone (binary self-migrate vs. external job), confirm with the deploy/DB owner, and record the decision. If tracer self-migrates from the baked image, P8-T11 adds NO tracer S3 job and P8-T07's migrations-present assertion is the relevant gate. If tracer uses S3-upload, P8-T11 adds the job mirroring ledger's two and the Dockerfile may not need migrations baked.

**Files:**
- `components/tracer/` (read — inspect migration application path: binary self-migrate vs. external job)
- `docs/monorepo/plan/P8.md` (record the decision in Open Items)

**Depends on:** P5 (tracer co-located; migrations physically in `components/tracer/migrations`)

**Acceptance criteria:**
- A recorded decision: tracer migrations are EITHER baked-and-self-applied (no S3 job; image must ship them) OR S3-uploaded (job added in P8-T11).
- P8-T11 and P8-T07 both cite this decision; neither is written on an unresolved assumption.

**Tests:** The decision is recorded in Open Items and referenced by P8-T07 (migrations-present assertion applicability) and P8-T11 (S3-upload job presence); no "confirm whether tracer migrations need S3" remains unresolved at P8-T11/T07 execution time.

**Effort:** S — 2-3h.

**Risk refs:** R16

---

### P8-T22 — Define and execute the external-owner fallback when a Helm/gitops/APIDog owner does NOT confirm lockstep

**Description:** P8-T20 assumes the external Helm `midaz` chart, `midaz-firmino-gitops`, and APIDog owners update in lockstep with the image rename/delete. The likeliest real-world stall is an owner being unavailable or rejecting the chart change on the timeline of the P8-T18 tag. Define the explicit fallback BEFORE the rename/delete tag lands so a missing sign-off does not silently produce "builds four images, deploys none" (R12). Fallback options, in preference order: (1) **Gate the rename tag** — do not cut the production fan-out tag until all three owners confirm; cut throwaway/test-registry tags for P8-T18 validation in the meantime (recommended — keeps the image names off the live chart until the chart is ready). (2) **Stage the chart additions first** — land the new `tracer`/`manager`/`worker` value blocks in the chart as additive (no removals) ahead of the tag, then remove crm/fees keys only after the first successful four-image sync. (3) **Temporary registry isolation** — push the renamed images to a non-prod registry namespace until the chart catches up, never to the namespace ArgoCD watches. This task assigns an owner, sets the confirmation deadline relative to P8-T18, and records which fallback is armed. It must START early (no in-repo dependency) and must be resolved before P8-T20 executes the coordinated edits.

**Files:**
- `docs/monorepo/plan/P8.md` (record the armed fallback + owner + deadline in Open Items)

**Depends on:** (none — early coordination; must resolve before P8-T18's production tag and P8-T20's edits)

**Acceptance criteria:**
- A named fallback is armed (gate-the-tag / stage-additive / registry-isolation) with an owner and a confirmation deadline tied to P8-T18.
- No path exists where the production rename/delete tag lands with an unconfirmed external owner and silently fails to deploy.
- P8-T20 cites this fallback as its precondition.

**Tests:** Open Items records the armed fallback, the owner, and the deadline; P8-T18's production (non-throwaway) tag is explicitly gated on the chosen fallback's confirmation; a dry-run/walkthrough of the "owner unavailable" case shows no orphaned-image deploy.

**Effort:** S — 0.5 day (coordination, not code).

**Risk refs:** R12

---

## Exit Criteria

1. Single normalized root Makefile component list (`ledger tracer reporter-manager reporter-worker`); no ledger special-casing, no crm references, no hand-unrolled lifecycle targets; tool versions pinned in one place from the P8-T01 record.
2. Root `mk/{docker,database,docs,quality,security}.mk` fragments promoted; every component Makefile is the shared template; `make sec` runs via `mk/security.mk`.
3. `make set-env` generates ledger's LCRYPTO keys (crm key-gen migrated in P3, verified here); crm/fees standalone Makefiles/mk deleted.
4. One root `.golangci.yml` (fees strict floor); `golangci-lint run ./...` clean module-wide.
5. Tracer Dockerfile on repo-root context with the WORKDIR→migrations COPY contract pinned and migrations proven present in the image; crm + fees Dockerfiles deleted; worker stays fat alpine.
6. No `github_token`/`go_private_modules`/`.secrets/`/`.netrc` anywhere (whole-tree grep == 0); clean `go mod download` from a fresh module cache.
7. Unified infra compose: SeaweedFS + KEDA added, tracer-postgres + fees-mongo dropped, versions reconciled to midaz; otel-lgtm unchanged (tracer independent of it).
8. All component composes on `infra-network`, no private nets; `wait-for-infra` gate; `make up/down` sequences the four components over the T02 loop.
9. CI consolidated: `build.yml`/`go-combined-analysis.yml`/`pr-security-scan.yml`/`pr-validation.yml` union four-component filter_paths, drop crm/fees, single shared-workflow + go + golangci version, single `midaz-*` prefix, the two distinct helm/gitops key-mapping schemas correct, godog stage preserved; duplicates deleted; dependabot has zero collapsed-component entries.
10. One `.releaserc.yml` (+ `.releaserc.hotfix.yml` tied to a real consumer, backmerge); midaz CHANGELOG canonical, others archived; gptchangelog single-app; STRUCTURE.md rewritten.
11. Validated: real tag → fan-out → exactly 4 `midaz-*` images, changed-detection works, godog green, PD-2 sentinel invariant holds, PR gates green; clean local `make set-env && make up` → 4 healthy services.
12. External Helm `midaz` + `midaz-firmino-gitops` + APIDog e2e updated in lockstep (with an armed fallback for owner-unavailability); ArgoCD syncs the four apps; no crm/fees image keys.
13. Tracer-migration deploy model decided (baked-self-apply vs. S3-upload), feeding the Dockerfile and build.yml correctly.

## Risks Addressed

R3 (lib-commons v4/v5 — version recorded from P1/P7 + clean download proves no private-module bridge), R7 (LCRYPTO carry into ledger set-env, verified), R11 (single golangci floor + 85% coverage gate per component), R12 (Helm/gitops lockstep + owner-unavailability fallback), R14 (mongo collapse validated via compose), R16 (Dockerfile context/migration WORKDIR→COPY contract + migrations-present assertion + tracer-migration deploy decision), R17 (env-merge no silent missing var), R20 (worker fat alpine accepted), R21 (fetcher network reachability preserved), R24 (single release/scope taxonomy + dependabot cleanup), R25 (Makefile footgun across both structural shapes + STRUCTURE.md staleness).

## Open Items

- **PD-4 GA tag (P8-T01):** RECORDED — `lib-commons/v5 v5.2.0` is the first GA (verified live on the proxy; `v5.2.1` is an equally-safe alternative). The "if no GA exists" branch is dead and removed. P8-T01 records: pin `v5.2.0`, Go `1.26.3`, golangci-lint `v2.4.0`.
- **Shared-workflow version (P8-T11):** pick the single version (recommend tracer's `v1.32.0` or newest validated at merge time) AFTER reading the shared-repo changelog v1.27→v1.32 for input-name changes — a renamed input silently no-ops.
- **Tracer migration deploy model (P8-T21):** DECIDE baked-self-apply vs. S3-upload before P8-T11 (S3 job?) and P8-T07 (migrations-present assertion) are coded. Owner: deploy/DB. Blocking input to T07/T11.
- **Registry policy (P8-T11):** confirm reporter can move from ghcr-only to DockerHub+ghcr (no licensing block) before harmonizing.
- **Postgres 16→17 compat (P8-T09):** tracer drops its own `postgres:16` and points at shared `postgres:17` — DB-dossier owns the migration-compat check; flagged here as the compose service deletion's precondition. Flag: this can stall late once the shared instance is targeted; an early executable compat probe is desirable if feasible.
- **External-repo ownership + fallback (P8-T20 / P8-T22):** Helm `midaz` chart + `midaz-firmino-gitops` + APIDog scenarios are owned outside midaz; confirm owners and sequencing BEFORE the rename/delete tag lands, OR arm the P8-T22 fallback (gate-the-tag / stage-additive / registry-isolation), or co-located components build but never deploy.
- **godog CI ownership (P8-T18):** decide and record whether the consolidated workflows run godog via the shared-workflow's built-in test stage or a bespoke midaz step (default: whatever P7-T15 wired). Do not leave deferred.


---

<a id="phase-9"></a>

# Phase 9 — Harmonization cleanup & docs ("liso e final") (15 tasks)

_Verbatim from `docs/monorepo/plan/P9.md`._


**Phase ID:** P9
**Objective:** Final sweep to honor *liso e final*. After all four moves have landed (crm collapse, fees embed, tracer + reporter co-location) and the unified dependency/build/test gate is green (P7), Phase 9 (a) deletes every surviving mid-migration shim/dead-code — first and foremost the CRM `ErrorCodeTransformer` and the 12 dead `CRM-00xx` codes per PD-2, plus the orphan `ErrMissingHeadersInRequest` (CRM-0018, zero references) and the full standalone-service residue under `components/crm/`; (b) mechanically renames the 143 tree-wide misleading `libCommons "…/lib-observability"` aliases (and the `libCommonsOtel`/`commons` variants) so no import name lies about its target; (c) reconciles and documents the auth/RBAC namespace divergence (R9); (d) documents the CRM `X-Organization-Id` vs ledger path-scoping API inconsistency (R22); (e) rewrites all stale agent/structure docs (STRUCTURE.md, AGENTS.md, CLAUDE.md, llms-full.txt, llms.txt) for the new 4-deploy-unit / `components/{ledger,tracer,reporter-manager,reporter-worker,infra}` topology; (f) greps the whole tree for leftover `replace` directives, TODO-compat markers, dual-imports, lib-commons-v4, and orphaned code; and (g) confirms the four origin repos are archived read-only and HARD-asserts the PD-5 double-entry fee-reversal balance proof runs green in the consolidated suite.

**Hard prerequisite framing:** Phase 9 is the LAST phase. It is a documentation + deletion + verification sweep over the *consolidated* tree, so its tasks depend on completion of the earlier phases. Cross-phase prerequisites are bound to concrete tail task ids (verified against the sibling phase files), NOT left as descriptive placeholders:

| Prerequisite (descriptive)   | Bound concrete tail task        | Source file / title                                                            |
| ---------------------------- | ------------------------------- | ------------------------------------------------------------------------------ |
| CRM collapse done            | **P3-T21**                      | terminal task of P3 (crm collapse)                                             |
| Fees embed done              | **P4-T19**                      | "Teardown: delete fees main/bootstrap/Dockerfile/compose/CI/standalone middleware" |
| Tracer move done             | **P5-T15**                      | "Full in-module verification gate (lint/unit/integration green; godog tracked)" |
| Reporter move done           | **P6-T17** + **P6-T20**         | E2E PDF pipeline verification + reporter origin read-only archive              |
| CI/Docker harmonization done | **P8-T18**                      | "End-to-end CI validation: cut a real tag, confirm fan-out builds exactly 4 images" |
| Unified dep/build/test gate  | **P7-T10**, **P7-T11**, **P7-T18** | full-tree `make test-unit` / `make test-integration` (via `mk/tests.mk`) / unified third-rail proof |
| lib-commons GA bump          | **P7-T03**                      | "Bump root module off the lib-commons beta to the v5.2.x GA pin"               |

These are HARD edges, not orchestrator-resolved hints. Within Phase 9 the DAG is explicit (below).

**Ground-truth verified during planning (all file paths/anchors are REAL unless marked NEW; counts re-verified against the live tree on the planning date):**
- Root module is single `go.mod` (`github.com/LerianStudio/midaz/v3`, `go 1.26.3`); zero `replace`, zero `go.work` — confirmed.
- **lib-commons is pinned at `github.com/LerianStudio/lib-commons/v5 v5.2.0-beta.12` — a BETA, NOT the GA that PD-4 hard-requires.** lib-observability is `v1.0.1` (matches PD-4). The GA bump is owned by **P7-T03**; P9-T01 reads the live version from `go.mod` and BLOCKS the inventory freeze if it is still on a `-beta`/`-rc` suffix. P9 docs MUST record the actual pinned version verbatim, never the aspirational "GA" string.
- CRM error-shim surface = `components/crm/internal/adapters/http/in/error_transformer.go` (middleware), `error_mapping.go` (the 12-code map), `error_transformer_test.go` (shim tests), and the `f.Use(ErrorCodeTransformer())` line at `components/crm/internal/adapters/http/in/routes.go:38`. **PD-2 named `backward_compat_test.go` as a shim file; that is WRONG** — `components/crm/internal/bootstrap/backward_compat_test.go` is a legitimate multi-tenant single-tenant-mode test with zero error-transformer coupling. The shim test is `error_transformer_test.go`. (See open_items.)
- The 12 dead transform-target codes (from `error_mapping.go`): `CRM-0001, CRM-0002, CRM-0003, CRM-0004, CRM-0005, CRM-0007, CRM-0009, CRM-0011, CRM-0012, CRM-0014, CRM-0015, CRM-0016`. Of these, **11 are a clean 1:1 transform of a generic midaz code; CRM-0004 is the EXCEPTION** — `error_mapping.go:27` maps `constant.ErrInvalidRequestBody.Error()` (**0094**) → `ErrInvalidFieldTypeInRequest.Error()` (CRM-0004), i.e. CRM-0004's source is **0094**, not a 0004-shaped code. All 12 sentinels are referenced ONLY in `pkg/constant/errors.go`, `error_mapping.go`, and `error_transformer_test.go` — all three deleted/pruned in this phase. Safe to prune.
- **`ErrMissingHeadersInRequest` (CRM-0018) is dead code**: `grep -rn 'ErrMissingHeadersInRequest' --include='*.go' .` returns exactly 1 hit — its own definition at `pkg/constant/errors.go:240`. Zero usages. Pruned in this phase by the same evidence standard as the 12 PD-2 codes (P9-T03).
- The other 16 `CRM-00xx` codes are live domain sentinels and STAY. Numbering gaps after pruning are accepted (cheaper than churn, keeps blame stable).
- RBAC namespaces in code: ledger `midazName="midaz"` + `routingName="routing"` (`components/ledger/internal/adapters/http/in/routes.go:26-27`); crm `ApplicationName="plugin-crm"` (`components/crm/internal/adapters/http/in/routes.go:21`); fees `plugin-fees` (enters via fees embed). PD/R9 = preserve per-domain, document, defer unification.
- CRM `X-Organization-Id` header scoping: `components/crm/internal/adapters/http/in/holder.go` (`c.Get("X-Organization-Id")` at L54/101/154/222/289) + `alias.go`. Ledger uses path-based org hierarchy. R22 = document, do not rework.
- **FromTo.Route passive-compat is an EXPLICIT ACCEPTED EXCEPTION, not a shim.** `buildCompanionFromTo` (`components/ledger/internal/adapters/http/in/transaction_overdraft_enrichment.go:644-650`) propagates BOTH `FromTo.Route` and `FromTo.RouteID` by design (RouteID canonical, Route legacy passive-read). `send_transaction_events.go:293` carries `Route: tran.Route, //nolint:staticcheck // legacy field kept for backward compatibility; RouteID is canonical`. T13's sweep MUST NOT flag or delete these — they are owned by the transaction/fees surface as deliberate dual-field passive-compat.
- Stale docs: `STRUCTURE.md` (shows onboarding/transaction/crm as components, `pkg/transaction`); `AGENTS.md` (Go 1.25+, components list missing tracer/reporter/fees); `CLAUDE.md` (`Go: 1.25+` L9, only ledger+crm components L11-12); `llms-full.txt` (Go 1.25, "lib-commons v4 tenant-manager", `pkg/transaction`, CRM-as-plugin); `llms.txt` (same staleness). `go.mod` is `go 1.26.3` — the canonical version string.
- `docs/PROJECT_RULES.md` is DO-NOT-OVERWRITE (39KB). No Phase 9 task touches it except read-only consistency checks.
- **Misleading-alias smell is FAR larger than first estimated and tree-wide, not CRM-scoped.** Verified counts: `libCommons "github.com/LerianStudio/lib-observability"` appears in **143** `.go` files (ledger, crm, AND `pkg/`); 27 of those bind `libCommons`→lib-observability in the SAME import block as a real `lib-commons/v5` import (`tmcore`/`libLog`/etc.) — actively confusing. Two more variants point at lib-observability: `libObservability` (45 files, NOT misleading — it is honest) and `commons` (1 file, misleading). The misleading set to rename = `libCommons` (143) + `commons` (1) = 144 occurrences. This is a pure mechanical rename with zero behavioral risk and IS fixed in this phase (P9-T13a), not deferred.
- The standalone CRM service footprint at `components/crm/` (verified `ls`) is: `cmd/`, `Dockerfile`, `docker-compose.yml`, `Makefile`, `.env`, `.env.example`, `.swaggo`, `api/`, `scripts/`, `artifacts/`, `reports/`, plus `internal/` (the package tree that survives). Everything except `internal/` (and whatever ledger references after the collapse) is standalone-service residue to delete (P9-T13).
- Root `Makefile` has NO `test-unit`/`test-integration` targets TODAY (only `check-tests`, `cover`, `sec`, and `ledger COMMAND=…` / `all-components COMMAND=…` delegation; component test target is `make ledger COMMAND=test`). **P7-T10/T11 introduce `mk/tests.mk` with `test-unit`/`test-integration` aggregator targets**, so by P9 (which runs after P7) those targets exist. P9-T12 binds to them via the P7-T10/T11 prerequisite and guards their presence before invoking.
- Origin remote: `github.com/lerianstudio/midaz`. tracer/reporter/crm/plugin-fees origins are separate repos to be archived read-only (out-of-repo coordination).

---

## Task DAG (intra-phase)

```
P9-T01 (freeze final topology inventory; GA-pin pre-gate) ──┬─> P9-T07 (STRUCTURE.md)
                                                            ├─> P9-T08 (AGENTS.md)
                                                            ├─> P9-T09 (CLAUDE.md)
                                                            ├─> P9-T10 (llms-full.txt)
                                                            └─> P9-T11 (llms.txt)

P9-T02 (delete CRM ErrorCodeTransformer shim) ─> P9-T03 (prune 12 dead CRM-00xx + CRM-0018) ─> P9-T04 (CRM error-contract verification test)
P9-T05 (RBAC namespace reconcile + doc) ───────────────────────────────────────────────────> P9-T12 (final gate)
P9-T06 (X-Organization-Id inconsistency doc, R22)
P9-T13  (tree-wide leftover-shim/dead-code/orphan-service grep + delete sweep) ─────────────> P9-T12
P9-T13a (rename 144 misleading libCommons/commons->lib-observability aliases) ──────────────> P9-T12
P9-T14 (origin repos archived read-only — out-of-repo)
P9-T12 (final unified gate: lint + unit + integration + build + PD-5 balance proof) — depends on T02..T11, T13, T13a
```

---

## Tasks

### P9-T01 — Freeze the final component/topology inventory + assert lib-commons GA pin (single source of truth for all doc rewrites)
- **Description:** Produce one authoritative, machine-checked inventory of the post-consolidation end-state to drive every doc rewrite in this phase, eliminating doc-drift between STRUCTURE/AGENTS/CLAUDE/llms. Run `go.mod` version reads (`go` directive + the **literal `lib-commons/v5` version string read verbatim from `go.mod`** — do NOT hardcode "GA"), `ls components/`, port grep, and image-name grep to capture the FROZEN truth: 5 component dirs (`components/{ledger,tracer,reporter-manager,reporter-worker,infra}`), 4 deploy units (ledger+fees+crm `:3002`, tracer `:4020`, reporter-manager `:4005`, reporter-worker `:4006`), Go `1.26.3`, the actual `lib-commons/v5` pin, `lib-observability v1.0.1`. CRM is now a package tree inside `components/ledger` (no longer a top-level component). **Pre-gate (hard):** before freezing, assert the lib-commons pin is a GA release — `grep -E 'lib-commons/v5 v5\.2\.[0-9]+$' go.mod` must match (no `-beta`/`-rc` suffix). If it still shows `v5.2.0-beta.12` (the current state), P9-T01 BLOCKS and the phase cannot close until P7-T03 (the GA bump) has landed. Emit the inventory as a short fenced block reused verbatim by T07–T11, recording the version string exactly as read. This is a coordination/no-code task; its output is the canonical table.
- **Files:** `docs/monorepo/plan/P9-inventory.md` (NEW)
- **Depends on:** P3-T21, P4-T19, P5-T15, P6-T17, P6-T20, P8-T18, P7-T03
- **Acceptance criteria:**
  - Inventory lists exactly 5 component dirs and matches `ls -d components/*/` output.
  - Ports table matches the host-port mappings in each component `docker-compose.yml`.
  - Go version string equals `go.mod`'s `go` directive (`1.26.3`).
  - **The lib-commons version string in the inventory is read verbatim from `go.mod` and is a GA pin (`v5.2.x`, no `-beta`/`-rc`); a beta pin fails the pre-gate.**
  - No reference to `components/crm` or `components/onboarding`/`components/transaction` as deploy units.
- **Tests:** `grep -Eq 'lib-commons/v5 v5\.2\.[0-9]+$' go.mod` (GA pre-gate, no beta/rc) — fails the task if beta; `bash -c 'diff <(ls -d components/*/ | sed "s#components/##;s#/##") <(grep -oE "ledger|tracer|reporter-manager|reporter-worker|infra" docs/monorepo/plan/P9-inventory.md | sort -u)'` returns no unexpected dirs; `grep -q "1.26.3" docs/monorepo/plan/P9-inventory.md`; `bash -c 'V=$(grep -oE "lib-commons/v5 v[0-9.]+([-a-z0-9.]*)" go.mod | awk "{print \$2}"); grep -qF "$V" docs/monorepo/plan/P9-inventory.md'` (doc records the live pin verbatim).
- **Effort:** S + 2-3 hours
- **Risk refs:** R25

### P9-T02 — Delete the CRM global ErrorCodeTransformer shim (PD-2)
- **Description:** Remove the CRM error-code backward-compat shim entirely. Delete `components/crm/internal/adapters/http/in/error_transformer.go`, `components/crm/internal/adapters/http/in/error_mapping.go`, and `components/crm/internal/adapters/http/in/error_transformer_test.go`. Remove the registration line `f.Use(ErrorCodeTransformer()) // Transform generic error codes to CRM-specific codes` at `components/crm/internal/adapters/http/in/routes.go:38`. After this, CRM 4xx/5xx responses carry canonical midaz codes (e.g. `0046`, `0009`, `0047`, `0094`) — no `CRM-00xx` rewrite. NO scoped-transformer fallback; the shim dies completely. Confirm no other file imports/calls `TransformErrorCode`/`CRMErrorMapping`/`ErrorCodeTransformer` (grep showed only routes.go + the deleted test). Do NOT touch `components/crm/internal/bootstrap/backward_compat_test.go` (legit MT test — see open_items).
- **Files:** `components/crm/internal/adapters/http/in/error_transformer.go` (DELETE), `components/crm/internal/adapters/http/in/error_mapping.go` (DELETE), `components/crm/internal/adapters/http/in/error_transformer_test.go` (DELETE), `components/crm/internal/adapters/http/in/routes.go` (edit: remove L38)
- **Depends on:** P3-T21
- **Acceptance criteria:**
  - The three files no longer exist.
  - `grep -rn "ErrorCodeTransformer\|TransformErrorCode\|CRMErrorMapping" components/` returns nothing (excluding generated `reports/*.out`).
  - `routes.go` compiles with the `ErrorCodeTransformer()` middleware removed and no other middleware reorder.
  - The CRM route group's surviving middleware order (recover → telemetry → cors → logging → tenant) is unchanged apart from the removed line.
  - `backward_compat_test.go` is untouched and still present.
- **Tests:** `go build ./components/crm/...` succeeds; `go vet ./components/crm/internal/adapters/http/in/...`; existing CRM handler tests in `components/crm/internal/adapters/http/in/*_test.go` (minus the deleted shim test) pass.
- **Effort:** S + 1-2 hours
- **Risk refs:** R6

### P9-T03 — Prune the 12 dead CRM-00xx transform sentinels + the orphan CRM-0018 from pkg/constant/errors.go (PD-2)
- **Description:** Remove the 12 dead transform-target sentinels that existed only to feed the deleted `CRMErrorMapping`: `ErrInvalidMetadataNestingCRM` (CRM-0001), `ErrMetadataKeyLengthExceededCRM` (CRM-0002), `ErrMissingFieldsInRequestCRM` (CRM-0003), `ErrInvalidFieldTypeInRequest` (CRM-0004, fed FROM `ErrInvalidRequestBody`/0094 — the one non-1:1 mapping), `ErrInvalidPathParameterCRM` (CRM-0005), `ErrUnexpectedFieldsInTheRequestCRM` (CRM-0007), `ErrPaginationLimitExceededCRM` (CRM-0009), `ErrInvalidSortOrderCRM` (CRM-0011), `ErrMetadataValueLengthExceededCRM` (CRM-0012), `ErrInternalServerCRM` (CRM-0014), `ErrBadRequestCRM` (CRM-0015), `ErrInvalidQueryParameterCRM` (CRM-0016). **ALSO prune the orphan sentinel `ErrMissingHeadersInRequest` (CRM-0018)** — verified dead (1 hit tree-wide, its own definition at `pkg/constant/errors.go:240`, zero usages). It is dead code by the exact evidence standard used for the 12 PD-2 codes; in the dead-code-elimination phase it does not survive. All in `pkg/constant/errors.go` (~L223-244). Do NOT touch the 16 surviving live CRM domain sentinels (`ErrHolderNotFound` CRM-0006, `ErrAliasNotFound` CRM-0008, `ErrDocumentAssociationError` CRM-0010, `ErrAccountAlreadyAssociated` CRM-0013, `ErrHolderHasAliases` CRM-0017, `ErrMetadataQueryInvalidFormat` CRM-0019, `ErrMetadataQueryInvalidKey` CRM-0020, `ErrMetadataQueryContainsOperator` CRM-0021, `ErrInvalidHeaderValue` CRM-0022, `ErrAliasClosingDateBeforeCreation` CRM-0023, `ErrRelatedPartyNotFound` CRM-0024, `ErrInvalidRelatedPartyRole` CRM-0025, `ErrRelatedPartyDocumentRequired` CRM-0026, `ErrRelatedPartyNameRequired` CRM-0027, `ErrRelatedPartyStartDateRequired` CRM-0028, `ErrRelatedPartyEndDateInvalid` CRM-0029). Do NOT renumber survivors; numbering gaps are accepted. Depends on T02 because the only non-errors.go references to the 12 live in the now-deleted `error_mapping.go`/`error_transformer_test.go`.
- **Files:** `pkg/constant/errors.go` (edit)
- **Depends on:** P9-T02
- **Acceptance criteria:**
  - The 12 named transform sentinels AND `ErrMissingHeadersInRequest` (CRM-0018) no longer exist in `pkg/constant/errors.go`.
  - `grep -rn "ErrInvalidMetadataNestingCRM\|ErrMetadataKeyLengthExceededCRM\|ErrMissingFieldsInRequestCRM\|ErrInvalidFieldTypeInRequest\|ErrInvalidPathParameterCRM\|ErrUnexpectedFieldsInTheRequestCRM\|ErrPaginationLimitExceededCRM\|ErrInvalidSortOrderCRM\|ErrMetadataValueLengthExceededCRM\|ErrInternalServerCRM\|ErrBadRequestCRM\|ErrInvalidQueryParameterCRM\|ErrMissingHeadersInRequest" --include="*.go" .` returns nothing.
  - The 16 surviving CRM sentinels remain and still compile.
  - Whole module builds: `go build ./...`.
- **Tests:** `go build ./...`; `go test ./pkg/constant/...`; any error-uniqueness/lint guard (`make lint`) passes with no duplicate/orphan sentinel warnings.
- **Effort:** S + 1 hour
- **Risk refs:** R6

### P9-T04 — Add a CRM canonical-error-contract regression test (lock the no-shim wire contract)
- **Description:** Add a focused handler test proving CRM error responses now emit canonical midaz codes (not `CRM-00xx`) for the formerly-transformed paths, so the deletion cannot silently regress. Hit CRM handler error paths that previously mapped and assert the JSON `code` field equals the **exact** canonical midaz sentinel each path genuinely throws (pinned, no hedge). Verified inversions against `error_mapping.go`: missing-fields → was `CRM-0003`, now `0009`; bad-request → was `CRM-0015`, now `0047`; internal → was `CRM-0014`, now `0046`; invalid-request-body → was `CRM-0004`, now **`0094`** (the non-1:1 path). For each chosen handler, confirm the genuinely-thrown canonical code against the live handler at test-authoring time and pin it exactly — do NOT assert a set-of-acceptable-codes fallback. Use the existing CRM handler test harness/table style (mirror the structure that was in the deleted `error_transformer_test.go` but invert the expectation). Separately assert the survivors are unaffected: at least one live CRM domain sentinel (e.g. CRM-0006 holder-not-found) is still emitted unchanged on its path. This replaces shim-behavior tests with end-state-contract tests.
- **Files:** `components/crm/internal/adapters/http/in/error_contract_test.go` (NEW)
- **Depends on:** P9-T02, P9-T03
- **Acceptance criteria:**
  - Test pins at least 4 formerly-mapped error paths to their EXACT canonical midaz codes (`0009`, `0046`, `0047`, `0094`) — one assertion per path, no "or" fallback set.
  - Test asserts no response `code` field matches the regex `^CRM-00(01|02|03|04|05|07|09|11|12|14|15|16|18)$`.
  - Test asserts at least one surviving live CRM domain sentinel (e.g. `CRM-0006`) is still emitted unchanged on its path.
  - Test is deterministic (no `time.Now()`, fixed inputs).
- **Tests:** `go test ./components/crm/internal/adapters/http/in/ -run ErrorContract -v` passes.
- **Effort:** S + 2-3 hours
- **Risk refs:** R6

### P9-T05 — Reconcile and document the auth/RBAC namespace divergence (R9)
- **Description:** The merged binary authorizes under four namespaces: `midaz` and `routing` (ledger, `routes.go:26-27`), `plugin-crm` (crm, `routes.go:21`), `plugin-fees` (fees embed). Per the locked decision, PRESERVE per-domain namespaces initially (route merge ≠ authz merge; tenant-manager RBAC policies key on these literal strings — a silent rename breaks authorization). Two deliverables: (1) Verify in code that each domain's routes still call `auth.Authorize(<namespace>, resource, action)` with its ORIGINAL namespace string after the collapses — confirm CRM still uses `"plugin-crm"`, fees still uses `"plugin-fees"`, ledger still uses `"midaz"`/`"routing"`. (2) Write a dedicated, authoritative section in a new doc `docs/auth/RBAC-NAMESPACES.md` enumerating the four namespaces, the resources under each (ledger: settings/accounts/etc.; routing: operation-routes/transaction-routes; plugin-crm: holders/aliases; plugin-fees: fees/estimates/packages/billing-*), the tenant-manager policy-key coupling, and an explicit "deferred unification" note (a coordinated policy migration is a separate funded effort — out of Phase 9 scope). The resource enumeration for plugin-fees depends on the actual post-embed route file (confirm at exec time per open_item #6); the path risk lives in the DOC CONTENT, not the path-agnostic grep test. NO code rename of namespaces in this phase.
- **Files:** `docs/auth/RBAC-NAMESPACES.md` (NEW); read-only verification across `components/ledger/internal/adapters/http/in/routes.go`, `components/crm/internal/adapters/http/in/routes.go`, fees route registrar (post-embed location, e.g. `components/ledger/internal/adapters/http/in/fees_routes.go` — confirm actual path at exec time)
- **Depends on:** P3-T21, P4-T19
- **Acceptance criteria:**
  - `docs/auth/RBAC-NAMESPACES.md` enumerates all four namespaces with their resource sets and the policy-key coupling warning.
  - Grep confirms CRM routes still use `"plugin-crm"` and fees routes still use `"plugin-fees"` (no accidental rename to `midaz`).
  - Doc states the deferred-unification posture explicitly and references R9.
- **Tests:** `grep -q '"plugin-crm"' components/crm/internal/adapters/http/in/routes.go`; `grep -rq '"plugin-fees"' components/ledger/internal/`; doc-lint/markdown-lint passes on the new file.
- **Effort:** M + 3-5 hours
- **Risk refs:** R9

### P9-T06 — Document the CRM X-Organization-Id vs ledger path-scoping API inconsistency (R22)
- **Description:** Post-collapse, CRM endpoints scope by an `X-Organization-Id` HTTP header (`c.Get("X-Organization-Id")` in `components/crm/internal/adapters/http/in/holder.go` L54/101/154/222/289 and `alias.go`), while ledger scopes by path-based org hierarchy (`/v1/organizations/:organization_id/...`). This inconsistency PERSISTS in the unified API surface by decision (do not rework now) — it is a legitimately-accepted exception (PD says document, do not rework), not a shim. Deliverable: document it clearly in the unified API docs — add a "Scoping conventions" subsection to `docs/api/SCOPING.md` (NEW) that states: ledger/routing/fees use path-based org/ledger scoping; CRM uses the `X-Organization-Id` header (legacy from the standalone CRM service); both are intentional for now; a future harmonization (header→path or path→header) is out of scope and tracked as R22. Cross-link from the component docs updated in T07-T11. NO endpoint/handler changes.
- **Files:** `docs/api/SCOPING.md` (NEW)
- **Depends on:** P3-T21
- **Acceptance criteria:**
  - `docs/api/SCOPING.md` documents both scoping mechanisms with concrete examples (a CRM `X-Organization-Id` request and a ledger path-scoped request).
  - Doc explicitly marks the inconsistency as known/deferred and references R22.
  - No handler code changed (grep confirms `c.Get("X-Organization-Id")` sites in CRM are untouched).
- **Tests:** `grep -c "X-Organization-Id" docs/api/SCOPING.md` >= 1; `grep -q "R22" docs/api/SCOPING.md`; markdown-lint passes.
- **Effort:** S + 2-3 hours
- **Risk refs:** R22

### P9-T07 — Rewrite STRUCTURE.md for the new component set (R25)
- **Description:** `STRUCTURE.md` is badly stale — it shows `components/{crm,onboarding,transaction}` as separate components, lists `pkg/transaction` (renamed to `pkg/mtransaction`), and documents an `onboarding` directory layout that no longer exists. Rewrite it from the T01 frozen inventory: top-level `components/{ledger,tracer,reporter-manager,reporter-worker,infra}` + `pkg/`. Reflect that CRM and fees are now package trees INSIDE `components/ledger` (no own component dirs). Update the `pkg/` table to current reality (`pkg/{mmodel,constant,streaming,gold,mtransaction,net,utils,mbootstrap,mongo,pagination,repository,shell}`). Remove the bogus `./pkg` "Common Utilities" list that mislabels lib-commons/lib-observability symbols (libLog/libZap/libHTTP) as pkg subdirs. Keep it accurate to the directory tree, not aspirational. The post-consolidation files this verifies against (tracer/reporter/fees) only exist after P3-P6 land; budget read-only verification time at exec.
- **Files:** `STRUCTURE.md` (rewrite)
- **Depends on:** P9-T01
- **Acceptance criteria:**
  - `STRUCTURE.md` contains no reference to `components/onboarding`, `components/transaction`, or `components/crm` as top-level components.
  - The component tree matches `ls components/`.
  - `pkg/transaction` replaced by `pkg/mtransaction`; pkg table matches `ls pkg/`.
  - tracer, reporter-manager, reporter-worker appear with their roles/ports.
- **Tests:** `bash -c '! grep -qE "components/(onboarding|transaction)" STRUCTURE.md'`; `grep -q "components/tracer" STRUCTURE.md && grep -q "components/reporter-manager" STRUCTURE.md`; `bash -c '! grep -q "pkg/transaction\b" STRUCTURE.md || grep -q "pkg/mtransaction" STRUCTURE.md'`.
- **Effort:** M + 4-5 hours (incl. post-consolidation read-only verification)
- **Risk refs:** R25

### P9-T08 — Update AGENTS.md for the new component set and toolchain
- **Description:** Fix the stale facts in `AGENTS.md`: change `Language | Go 1.25+` to `Go 1.26.3` (matching `go.mod`); update the `Components` row from `Ledger (:3002), CRM (:4003), Infra` to the 4 deploy units (`Ledger+CRM+Fees (:3002), Tracer (:4020), Reporter-Manager (:4005), Reporter-Worker (:4006), Infra`); update `make up` comment from "infra → ledger → CRM" to the real ordering (infra → ledger → tracer → reporter-manager → reporter-worker). Add brief "where the new domains live" pointers: CRM at `components/ledger/...` (folded), fees at the tx-create seam, tracer/reporter as own components. Keep the conventions section intact (it is current). Do not touch `docs/PROJECT_RULES.md` references.
- **Files:** `AGENTS.md` (edit)
- **Depends on:** P9-T01
- **Acceptance criteria:**
  - `AGENTS.md` Go version reads `1.26.3` (or `1.26+`), not `1.25+`.
  - Component/port table lists tracer/reporter-manager/reporter-worker and no longer shows CRM as a separate `:4003` deploy unit.
  - `make up` ordering note reflects the consolidated startup sequence.
- **Tests:** `grep -q "1.26" AGENTS.md && ! grep -q "Go 1.25" AGENTS.md`; `grep -q "4020" AGENTS.md && grep -q "4005" AGENTS.md`; `! grep -q "4003" AGENTS.md`.
- **Effort:** S + 1-2 hours
- **Risk refs:** R25

### P9-T09 — Update CLAUDE.md project/component facts for the new topology
- **Description:** Update the `## Project` section of `CLAUDE.md`: line 9 `Go: 1.25+.` → `Go: 1.26.3+.`; expand the component list (currently `Main component: components/ledger` L11 + `CRM component: components/crm` L12) to reflect that CRM and fees now live inside `components/ledger`, and that `tracer`, `reporter-manager`, `reporter-worker` are co-located components. Add a one-line note on the fee seam (executeCreateTransaction post-ApplyDefaultBalanceKeys, pre-idempotency) and the CRM-folded-into-ledger fact so future agents do not look for `components/crm`. Do NOT alter the coding-rules / streaming / multi-tenancy sections except where a component path is now wrong. CLAUDE.md is agent-facing operating truth; keep it terse and correct.
- **Files:** `CLAUDE.md` (edit)
- **Depends on:** P9-T01
- **Acceptance criteria:**
  - `CLAUDE.md` Go line reads `1.26.3+` not `1.25+`.
  - Component section names tracer, reporter-manager, reporter-worker and states CRM+fees are folded into ledger.
  - No stale reference to `components/crm` as a standalone component (except where intentionally describing the historical fold).
- **Tests:** `grep -q "1.26.3" CLAUDE.md && ! grep -q "Go: 1.25" CLAUDE.md`; `grep -q "tracer" CLAUDE.md && grep -q "reporter-manager" CLAUDE.md`.
- **Effort:** S + 1-2 hours
- **Risk refs:** R25

### P9-T10 — Rewrite llms-full.txt for the consolidated monorepo
- **Description:** `llms-full.txt` (36KB) is heavily stale: header says "Go 1.25 monorepo", "unified Ledger HTTP API (onboarding + transaction) and a CRM plugin", "multi-tenant isolation via lib-commons v4 tenant-manager", lists `pkg/transaction`, and omits tracer/reporter/fees entirely. Rewrite the affected sections from the T01 frozen inventory: (1) header → Go 1.26.3, the **actual lib-commons/v5 pin recorded by T01 (read verbatim from `go.mod`, GA per the T01 pre-gate)** + lib-observability v1.0.1, 4 deploy units; (2) component map → `components/{ledger(+crm+fees),tracer,reporter-manager,reporter-worker,infra}`; (3) ports → 3002/4020/4005/4006; (4) `pkg/transaction` → `pkg/mtransaction`; (5) add CRM endpoints (holders/aliases under `X-Organization-Id`), fees-on-transaction behavior, tracer endpoints, reporter manager/worker roles; (6) add the new env-var surfaces (CRM_*, fee/billing, tracer, reporter, SeaweedFS/KEDA) at a reference level. Preserve the accurate transaction/error-code/model sections. This is the deepest doc rewrite; budget post-consolidation read-only verification of the fees route path, tracer ports, and reporter roles. Do NOT write the literal "GA" string in place of the version number — use the pinned version T01 recorded.
- **Files:** `llms-full.txt` (rewrite affected sections)
- **Depends on:** P9-T01
- **Acceptance criteria:**
  - Header reads Go 1.26.3 and the actual `lib-commons/v5` version string from `go.mod` (no "lib-commons v4 tenant-manager", no `-beta`).
  - Component/port map lists all 4 deploy units; no `:4003` CRM standalone.
  - `pkg/transaction` references replaced with `pkg/mtransaction`.
  - tracer, reporter-manager, reporter-worker, and fees-on-transaction appear in the doc.
- **Tests:** `! grep -q "Go 1.25" llms-full.txt && grep -q "1.26.3" llms-full.txt`; `! grep -q "lib-commons v4" llms-full.txt`; `! grep -q "beta" llms-full.txt`; `grep -q "4020" llms-full.txt && grep -q "reporter-worker" llms-full.txt`; `! grep -qE "\bpkg/transaction\b" llms-full.txt`.
- **Effort:** L + 1-1.5 days
- **Risk refs:** R25

### P9-T11 — Rewrite llms.txt (concise overview) for the consolidated monorepo
- **Description:** `llms.txt` (5.2KB, llmstxt.org spec) mirrors `llms-full.txt`'s staleness (onboarding+transaction framing, CRM-as-plugin, `pkg/transaction`, no tracer/reporter/fees). Update it to the concise consolidated overview: 4 deploy units, Go 1.26.3, the folded CRM+fees-in-ledger fact, tracer/reporter components, and corrected `pkg/` paths. Keep it short and spec-compliant (it is the index, `llms-full.txt` is the body). Cross-link to the new `docs/auth/RBAC-NAMESPACES.md` and `docs/api/SCOPING.md`.
- **Files:** `llms.txt` (rewrite)
- **Depends on:** P9-T01, P9-T10
- **Acceptance criteria:**
  - `llms.txt` lists the consolidated component set and Go 1.26.3.
  - No "CRM plugin" standalone framing; CRM shown as folded into ledger.
  - Links to RBAC-NAMESPACES.md and SCOPING.md present.
  - Conforms to llmstxt.org structure (H1 + blockquote summary + sectioned links).
- **Tests:** `grep -q "1.26.3" llms.txt && grep -q "tracer" llms.txt`; `grep -q "RBAC-NAMESPACES" llms.txt && grep -q "SCOPING" llms.txt`.
- **Effort:** S + 2-3 hours
- **Risk refs:** R25

### P9-T12 — Final unified gate: lint + unit + integration + per-component build + PD-5 balance proof (HARD)
- **Description:** After all deletions, renames, and doc rewrites land, run the full quality gate over the consolidated single module to prove "liso e final" actually compiles, lints, and tests green with zero shims. Run `make lint` (golangci v2.x floor), `make test-unit`, `make test-integration` (the `mk/tests.mk` aggregator targets introduced by P7-T10/T11 — guard their presence first: `grep -q '^test-unit:' mk/tests.mk` and `grep -q '^test-integration:' mk/tests.mk`, else bind to the real component target `make ledger COMMAND=test` / `make all-components COMMAND=test` per open_item), and `go build ./...` for every component binary. **PD-5 double-entry balance proof is a HARD requirement of this gate, not a deferral.** By P9 the fees code is merged into `components/ledger` (P4-T19 is a hard prerequisite of T01/T05/T13), so the fee-reversal/pending-cancel balance test(s) ARE reachable from the consolidated suite — there is no legitimate "unreachable" branch. The unified third-rail proof is owned by **P7-T18**; P9-T12 re-runs it against the final tree and HARD-asserts it green: (a) revert reversal sums to zero incl. DEDUCTIBLE-fee revert (`sum(legs)==0` under exact `decimal.Equal`); (b) pending-cancel fee-refund sums to zero. Add a pre-check that greps the merged fees/transaction test code for the balance-assertion test by name and FAILS the gate if the test is absent — absence is a gate failure, not a deferral. Confirm the 85% coverage hard gate holds on the changed CRM error surface. This is the phase-closing verification.
- **Files:** (no source files; CI/gate execution) — touches none, validates all
- **Depends on:** P9-T02, P9-T03, P9-T04, P9-T05, P9-T06, P9-T07, P9-T08, P9-T09, P9-T10, P9-T11, P9-T13, P9-T13a, P7-T10, P7-T11, P7-T18
- **Acceptance criteria:**
  - `go build ./...` succeeds for the whole module.
  - `make lint` passes with zero new findings.
  - `make test-unit` passes (via `mk/tests.mk`); CRM error-contract test (T04) green.
  - `make test-integration` passes (via `mk/tests.mk`).
  - **PD-5 balance proof runs in `components/ledger` and passes: the DEDUCTIBLE-fee revert reversal sums to zero AND the pending-cancel fee-refund reversal sums to zero (`sum==0`, exact `decimal.Equal`). The named balance-assertion test EXISTS in the merged tree — its absence is a gate FAILURE, not a deferral.**
  - Coverage gate (85%) holds on touched packages.
- **Tests:** `bash -c 'grep -q "^test-unit:" mk/tests.mk && grep -q "^test-integration:" mk/tests.mk'` (aggregator targets present, else fall back to `make ledger COMMAND=test`); `go build ./...`; `make lint`; `make test-unit`; `make test-integration`; `bash -c 'grep -rqniE "revert.*balance|fee.*revers|pending.?cancel.*refund" components/ledger/internal/services/command/*_test.go'` (PD-5 balance test present); `go test ./components/ledger/internal/services/command/... -run 'Revert|Reversal|PendingCancel|FeeRefund' -v` passes with the deductible-fee case asserting sum==0.
- **Effort:** M + 4-6 hours (mostly wall-clock for integration)
- **Risk refs:** R6, R11, R18, PD-5

### P9-T13 — Tree-wide leftover-shim / dead-code / dual-import / orphan-service grep & delete sweep
- **Description:** Grep the ENTIRE consolidated tree for any mid-migration crutch that survived earlier phases and is forbidden by the "no shims" end-state, then DELETE it in-task (orphan-service residue, dead scaffolding) or escalate (genuine judgment calls). Targets:
  1. `replace ` directives in any `go.mod` (must be ZERO — currently zero, re-verify post-merge).
  2. any `go.work`/`go.work.sum` file (must be absent).
  3. lib-commons v4 imports (`lib-commons/v4`, `lib-commons/commons/{log,opentelemetry,zap}`) — must be ZERO after tracer migration (PD-1 v4→v5 gate; currently zero, re-verify).
  4. dual-import of lib-commons v4+v5 in one file.
  5. TODO/FIXME markers mentioning compat/migration/"remove after"/"temporary"/"shim"/"fence" in non-test Go under `components/`/`pkg/`. Use word-boundary/context guards so a legitimate domain use of the word "temporary" (e.g. a temp-table comment, a field literally named) does not false-positive.
  6. **Full standalone-service residue for collapsed domains — DELETE, do not merely grep.** For CRM (verified full footprint at `components/crm/`): `cmd/`, `Dockerfile`, `docker-compose.yml`, `Makefile`, `.env`, `.env.example`, `.swaggo`, `api/`, `scripts/`, `artifacts/`, `reports/`. After the collapse, `components/crm/` must contain ONLY the surviving package tree (`internal/` and whatever ledger references) — no service entrypoint or service-config files. Apply the same deletion standard to any surviving plugin-fees `main.go`/`Dockerfile`/`docker-compose.yml`/standalone `Makefile`/`.releaserc` and any tracer/reporter standalone `.releaserc`. This is the safety net if the upstream collapse phases left residue.
  7. duplicate error sentinels outside `pkg/constant/errors.go`.

  **EXPLICIT ACCEPTED EXCEPTIONS (do NOT flag, do NOT delete):**
  - `FromTo.Route` passive-compat dual-field: `buildCompanionFromTo` (`transaction_overdraft_enrichment.go:644-650`) propagates BOTH `FromTo.Route` and `FromTo.RouteID` by design; `send_transaction_events.go:293` carries `Route: tran.Route //nolint:staticcheck`. RouteID is canonical, Route is a deliberate legacy passive-read. Owned by the transaction/fees surface, NOT a shim.
  - the `//nolint:staticcheck` legacy `Code` field on operation routes (`create_operation_route.go:40`, `update_operation_route.go:35`) — persisted legacy field, intentional.
  - R22 `X-Organization-Id` header scoping — documented accepted exception (T06), not a shim.

  The misleading `libCommons`/`commons`→lib-observability aliases are handled by the dedicated rename task **P9-T13a** (not flagged here as a deferral). Produce a findings list; clean the blockers/orphans in-task, record the rest.
- **Files:** sweep across whole tree; deletions of orphan-service files (`components/crm/{cmd,Dockerfile,docker-compose.yml,Makefile,.env,.env.example,.swaggo,api,scripts,artifacts,reports}` if the collapse did not remove them; equivalent plugin-fees/tracer/reporter standalone residue)
- **Depends on:** P3-T21, P4-T19, P5-T15, P6-T17, P6-T20, P9-T02, P9-T03
- **Acceptance criteria:**
  - `grep -rn "^replace " --include=go.mod .` returns nothing.
  - `find . -name go.work -o -name go.work.sum` returns nothing.
  - `grep -rn "lib-commons/v4\|lib-commons/commons/" --include="*.go" .` returns nothing.
  - No `TODO`/`FIXME` mentioning shim/compat/fence/"remove after"/temporary in non-test Go source under `components/`/`pkg/` (word-boundary guarded; accepted exceptions excluded).
  - **`components/crm/` after collapse contains ONLY the surviving package tree — no `cmd/`, `Dockerfile`, `docker-compose.yml`, standalone `Makefile`, `.env`, `.env.example`, `.swaggo`, `api/`, `scripts/`, `artifacts/`, `reports/`.** Same for plugin-fees/tracer/reporter standalone residue.
  - No duplicate `errors.New("<code>")` sentinels outside `pkg/constant/errors.go`.
  - Accepted exceptions (FromTo.Route, legacy Code field, R22) are present and untouched.
  - Findings list recorded.
- **Tests:** `bash -c 'set -e; ! grep -rqn "^replace " --include=go.mod .; ! find . -name go.work | grep -q .; ! grep -rqn "lib-commons/v4" --include="*.go" .'`; `bash -c 'set -e; for f in cmd Dockerfile docker-compose.yml Makefile .env .env.example .swaggo api scripts artifacts reports; do ! test -e "components/crm/$f"; done'`; `make lint` (sentinel-uniqueness/dead-code rules).
- **Effort:** M + 4-6 hours
- **Risk refs:** R3, R4, R5

### P9-T13a — Rename the 144 misleading lib-observability import aliases (libCommons/commons -> libObs)
- **Description:** Mechanically rename every import alias that lies about its target: `libCommons "github.com/LerianStudio/lib-observability"` (143 files, incl. `pkg/`, ledger, crm) and `commons "github.com/LerianStudio/lib-observability"` (1 file) → a truthful alias `libObs` (and, for any `libCommonsOtel`/`libCommonsLog`→lib-observability variants discovered at exec time, `libObsOtel`/`libObsLog`). 27 of the 143 files bind `libCommons`→lib-observability in the SAME import block as a real `lib-commons/v5` import (`tmcore`/`libLog`/etc.) — these dual-presence files are the worst offenders and a hard acceptance check. LEAVE the already-honest `libObservability "…/lib-observability"` (45 files) alone — it does not lie. This is a pure mechanical rename with ZERO behavioral risk: rename the alias at the import site AND every use within the file, then `goimports`/`gofmt`, then `go build ./...`. In a phase explicitly chartered to leave the tree clean, an import named `libCommons` pointing at lib-observability is a surviving workaround, not acceptable debt — so it is FIXED here, not deferred to an open_item.
- **Files:** all 144 `.go` files importing lib-observability under a misleading alias (tree-wide: `components/ledger`, `components/crm`, `pkg/`); rename alias + uses in each
- **Depends on:** P3-T21, P4-T19, P5-T15, P6-T17
- **Acceptance criteria:**
  - `grep -rln 'libCommons "github.com/LerianStudio/lib-observability"' --include="*.go" .` returns ZERO files.
  - `grep -rln 'commons "github.com/LerianStudio/lib-observability"' --include="*.go" .` returns ZERO files.
  - **No file binds the name `libCommons` (or `commons`) to `lib-observability`** — including the 27 dual-presence files.
  - Honest `libObservability "…/lib-observability"` imports are unchanged (still ~45, not renamed).
  - `gofmt -l` reports no formatting drift on touched files; `go build ./...` green.
- **Tests:** `bash -c '! grep -rqln "libCommons \"github.com/LerianStudio/lib-observability\"" --include="*.go" . && ! grep -rqln "commons \"github.com/LerianStudio/lib-observability\"" --include="*.go" .'`; `go build ./...`; `bash -c '[ -z "$(gofmt -l $(git diff --name-only -- "*.go"))" ]'`.
- **Effort:** M + 3-4 hours (mechanical, broad blast radius across 144 files)
- **Risk refs:** R3, R25

### P9-T14 — Confirm origin repos archived read-only (out-of-repo coordination, PD-3)
- **Description:** Per PD-3 (fresh import; origin repos become read-only archives AFTER the consolidated build is deploying in all environments), verify the four origin repositories (`LerianStudio/tracer`, `LerianStudio/reporter`, `LerianStudio/plugin-fees`, and standalone `LerianStudio/crm` if it was ever a separate repo) are set to archived/read-only on GitHub. Because PD-3 mandates deploy-first ordering, archival typically happens during a rollback grace window; therefore the PRIMARY acceptance is **owner sign-off recorded** (archival scheduled/confirmed by the owner), and `isArchived: true` is the confirming signal once the grace window closes — NOT a hard precondition that fails the task during the window. Use `gh repo view <repo> --json isArchived` to capture state. Add a note to `docs/monorepo/ARCHIVE-STATE.md` recording which origins were archived (or scheduled), their final commit SHAs, the archive/sign-off date, and the grace-window decision, so provenance is captured (the git log of the fresh-import commits already carries the "import <repo>" markers). **CGap2 fallback (owner-unavailable / chart-rejected path):** if an origin owner is unavailable or refuses to flip `isArchived` within the agreed window, this does NOT block P9 closure — record the blocker, the named owner, and a follow-up ticket in `ARCHIVE-STATE.md`, and proceed; archival is a coordination commitment, not a code gate. The same escalation path applies to any external sign-off stall (Helm/gitops/APIDog) surfaced in CI phases: document the owner, the rejection/absence, and the follow-up, and do not hard-block the harmonization on an absent external party.
- **Files:** `docs/monorepo/ARCHIVE-STATE.md` (NEW)
- **Depends on:** P9-T12, P8-T18
- **Acceptance criteria:**
  - Owner sign-off recorded for each consolidated origin (archival done or scheduled within the agreed grace window).
  - `gh repo view <origin> --json isArchived` state captured per origin (`true` once the window closes; `false` + scheduled date during the window is acceptable).
  - `docs/monorepo/ARCHIVE-STATE.md` records each origin repo, final SHA, archive/sign-off date, and the grace-window decision.
  - Owner-unavailable / refusal cases recorded with named owner + follow-up ticket; they do not block phase closure.
- **Tests:** `gh repo view LerianStudio/tracer --json isArchived -q .isArchived` captured per origin; `grep -q "sign-off" docs/monorepo/ARCHIVE-STATE.md`; `grep -qE "final SHA|final commit" docs/monorepo/ARCHIVE-STATE.md`.
- **Effort:** S + 1-2 hours (mostly coordination/wall-clock)
- **Risk refs:** R12

---

## Exit criteria (phase done when ALL hold)
1. CRM `ErrorCodeTransformer` shim (3 files + routes.go registration) fully deleted; the 12 dead `CRM-00xx` transform sentinels AND the orphan `ErrMissingHeadersInRequest` (CRM-0018) pruned; CRM error responses emit canonical midaz codes pinned exactly (incl. `0094` for the CRM-0004 path), locked by a regression test (T02-T04).
2. RBAC namespace divergence reconciled (verified preserved per-domain) and documented in `docs/auth/RBAC-NAMESPACES.md`; R22 `X-Organization-Id` vs path-scoping inconsistency documented in `docs/api/SCOPING.md` (T05-T06).
3. STRUCTURE.md, AGENTS.md, CLAUDE.md, llms-full.txt, llms.txt all rewritten to the frozen 4-deploy-unit / `components/{ledger,tracer,reporter-manager,reporter-worker,infra}` topology, Go 1.26.3, and the live GA `lib-commons/v5` pin recorded by T01 (T07-T11).
4. Tree-wide sweep proves ZERO `replace`/`go.work`/lib-commons-v4/dual-import/duplicate-sentinel and ZERO standalone-service residue for collapsed domains (CRM/fees/tracer/reporter); accepted exceptions (FromTo.Route, legacy Code field, R22) explicitly preserved (T13).
5. ALL 144 misleading `libCommons`/`commons`→lib-observability import aliases renamed to truthful names; no import name lies about its target (T13a).
6. Full unified gate green: `go build ./...`, `make lint`, `make test-unit`, `make test-integration`, AND the PD-5 double-entry balance proof (deductible-fee revert sum==0 + pending-cancel fee-refund sum==0) running in the consolidated suite (T12).
7. Origin repos confirmed archived read-only (or scheduled with owner sign-off + grace window) with provenance recorded; owner-unavailable path defined (T14).
8. lib-commons is on a GA `v5.2.x` pin (no `-beta`/`-rc`); P9-T01 pre-gate enforced.
9. `docs/PROJECT_RULES.md` untouched (never overwritten).

## Risks addressed
- **R6** — CRM ErrorCodeTransformer shim deleted (T02), dead codes + CRM-0018 pruned (T03), canonical-contract pinned (T04).
- **R9** — Auth/RBAC namespaces preserved per-domain + documented; unification explicitly deferred (T05).
- **R22** — CRM `X-Organization-Id` vs ledger path-scoping inconsistency documented as known/deferred accepted exception (T06).
- **R25** — All stale docs (STRUCTURE/AGENTS/CLAUDE/llms-full/llms) rewritten to live truth, GA pin recorded verbatim (T01, T07-T11); misleading import aliases renamed (T13a).
- **R3/R4/R5** — Leftover lib-commons-v4/replace/dual-import/orphan-service crutches grepped out and deleted tree-wide; misleading aliases renamed (T13, T13a).
- **R11** — 85% coverage gate re-validated after deletions (T12).
- **R12** — Helm/gitops handled in CI phase; origin-archive coordination closed out with owner-unavailable fallback (T14).
- **R18 / PD-5** — Double-entry fee-reversal balance proof (deductible revert + pending-cancel refund, sum==0) HARD-asserted in the consolidated suite at the final gate (T12).
- **PD-1** — lib-commons v4→v5 / tracer-v5 gate residue re-verified zero (T13); GA pin (no beta) enforced as a T01 pre-gate bound to P7-T03.

## Open items (flagged, not silently resolved)
1. **PD-2 file-name discrepancy:** PD-2 lists `backward_compat_test.go` as a CRM shim file to delete. Ground truth: `components/crm/internal/bootstrap/backward_compat_test.go` is a legitimate multi-tenant single-tenant-mode test with ZERO error-transformer coupling. The actual shim test is `error_transformer_test.go`. P9-T02 deletes the correct file and intentionally does NOT touch `backward_compat_test.go`. Confirmed accepted per LOCKED DECISION PD-2.
2. **CRM-0004 is the non-1:1 mapping:** `error_mapping.go:27` maps `ErrInvalidRequestBody` (0094) → CRM-0004, so the formerly-CRM-0004 path now emits canonical `0094`, not a 0004-shaped code. T04 pins this exact code; the "12 dead 1:1 codes" framing is corrected to "11 1:1 + CRM-0004 (from 0094)."
3. **fees route file path:** the exact post-embed location of the fees route registrar (assumed `components/ledger/internal/adapters/http/in/fees_routes.go`) must be confirmed at execution time against what P4 produced. T05's grep (`grep -rq '"plugin-fees"'`) is path-agnostic; the path risk lives in the RBAC-NAMESPACES.md content, not the grep.
4. **`mk/tests.mk` aggregator targets:** P9-T12 relies on `make test-unit`/`make test-integration` introduced by P7-T10/T11 (`mk/tests.mk`). Root Makefile has NO such targets today (only `check-tests`/`cover`/`ledger COMMAND=test`). T12 guards their presence and falls back to `make ledger COMMAND=test` / `make all-components COMMAND=test` if P7's targets are not yet wired — confirm at exec time.
5. **Origin-archive timing (T14):** archiving origins read-only happens only AFTER the consolidated build is deploying in all environments (rollback grace window). Primary acceptance is owner sign-off; `isArchived: true` confirms once the window closes. Owner-unavailable / chart-rejected fallback is defined (record + follow-up ticket, do not block phase closure).
6. **godog CI workflow ownership (FG3):** whether tracer's godog BDD suite runs via a shared midaz CI workflow or a bespoke one is decided/owned upstream (P5-T12a / P7-T13..T15), NOT in P9. P9-T12's gate does not re-decide it; it only requires the unified `make test-unit`/`make test-integration` and PD-5 proof to be green. Flagged so P9 is not assumed to own the godog harness decision.
7. **PG16→17 / logical-replication tracer-migration compat (FG2):** the pg-version + 34-migration + audit-hash-chain compat against the shared instance is a late-stall risk owned upstream (P5-T10), gated by sign-off. Out of P9 scope; flagged as a potential late stall that could delay the P3-T21/P4-T19/P5-T15/P6 prerequisites this phase depends on.


---

## 6. Milestones (M0 .. M9)

| Milestone | Met when | Gating tasks |
| --- | --- | --- |
| **M0 — Pre-flight clear** | GA tag verified on proxy; baseline regression snapshot captured; tracer v4→v5 + reporter/fees pin reconciliation dry-run green; CRM shim/dead-code surface locked file:line; external owners (Helm/gitops/APIDog) named with a no-sign-off fallback; CRM `LCRYPTO_*` carry-over contract documented. | P0-T01..T17 (esp. P0-T17 tracer dry-run, P0-T16 shim surface, P0-T15 owner fallback) |
| **M1 — Stable dep target** | midaz on `lib-commons/v5 v5.2.0` GA + `lib-observability v1.0.1`, full-repo green (build/vet/lint/unit/sec/streaming-JSONShape), canonical frozen pin recorded as the single downstream source of truth. | P1-T06 (GATE) |
| **M2 — Incoming repos dep-clean** | fees / reporter / tracer each compile against the midaz target lib stack IN their own repos, validated against their own CI; fees-engine correctness spike frozen; tracer READY-TO-MOVE. | P2a-T15 + P2a-T17; P2b-T13 ∧ P2b-T14; **P2c-T22** |
| **M3 — crm collapsed** | ledger binary serves the 11 holder/alias routes on `:3002`; shim + 12 dead codes gone; `WithRecover` hoisted; crm-api Mongo wired; in-module e2e + full quality gate green; standalone teardown gated on green. | P3-T20, P3-T21 (+ teardown P3-T13/T17/T18 after green) |
| **M4 — fees embedded** | fee legs applied via the single `validate` reassignment; revert/pending-cancel refund balances to zero (DEDUCTIBLE case proven); 11 Mongo indexes; structural seam gates green; teardown gated on P7-T18 + P4-T21. | P4-T16, P4-T19a (+ P4-T19b after P7-T18 + lockstep) |
| **M5 — tracer co-located** | `components/tracer` compiles + tests green in the single root go.mod; pg16→17 + logical-replication compat cleared; audit-hash-chain byte-identical; CI fans out `midaz-tracer`. | P5-T15 (+ origin archive P5-T13b after green) |
| **M6 — reporter co-located** | two reporter components build from repo-root; end-to-end PDF pipeline renders; fetcher reachability preserved; coverage gate met. | P6-T17 (+ origin archive P6-T20 after lockstep) |
| **M7 — unified module** | one coherent `go.mod`/`go.sum`: single resolved `/v5`, no `/v4`, no `replace`/`go.work`; whole tree green (build/lint/unit/integration/sec); godog in CI; **PD-5 fee-on-revert re-proved unified**; clean `go mod download`. | P7-T18, P7-T16, P7-T17 |
| **M8 — CI/build/release harmonized** | normalized Makefile component list; one `.golangci.yml`; unified compose superset (SeaweedFS+KEDA in, tracer-postgres+fees-mongo out); real tag → exactly 4 `midaz-*` images; external Helm/gitops/APIDog updated in lockstep (or fallback armed); single semantic-release. | P8-T18, P8-T20 (+ P8-T22 fallback) |
| **M9 — liso e final** | all surviving shims/dead-code/orphan-service residue deleted; 144 misleading aliases renamed; docs (STRUCTURE/AGENTS/CLAUDE/llms*) rewritten; final unified gate incl. PD-5 balance proof green; origins archived read-only. | P9-T12, P9-T14 |

---

## 7. Residual Risk Register (R1 .. R25)

Synthesized from the risk references across all twelve phase files. Each risk lists where it is addressed and
its residual exposure after the plan executes.

| ID | Risk | Primarily addressed in | Residual exposure |
| --- | --- | --- | --- |
| **R1** | Fee leg-sum balancing — `sum(fee legs) == fee total` must hold exactly under the validator's `decimal.Equal`. | P2a-T17 (conservation invariant), P4-T11, P4-T16, P4-T24 | Held by `applyFeeCorrection` residual-to-max reconciliation, independent of the ISO-4217 table; proven in-fees-repo first, re-proven in-tree. |
| **R2** | The fee-augmented transaction must balance under the SECOND `ValidateSendSourceAndDistribute`; pre-fee `validate` must not leak downstream. | P4-T12, P4-T25, P4-T27 (structural gate), P7-T18a (static guard) | Single `=` reassignment + AST/grep gates catch a future fork/reorder at test time. |
| **R3** | lib-commons v4↔v5 cannot coexist; no `replace`/shim bridge; moving-beta target. | P0-T17, P1 (whole), P2a/P2b/P2c, P5-T06, P7-T07 | One resolved `/v5`, zero `/v4` asserted; a v4 leak loops to P5, never bridged. |
| **R4** | Observability split — `commons/{log,opentelemetry,zap}` + root `NewTrackingFromContext`/`ctxutil` fail to compile in the unified module. | P1-T02 (import-target audit), P2a-T04..T10, P2b-T04..T08b, P2c-T11..T18, P7-T08 | The genuine compile-breaker (reporter `ctxutil`, tracer config shim) is migrated in-repo first. |
| **R5** | Fee-engine semantic conflicts at embed (route shape, amount mutation, `pkg/transaction`→`pkg/mtransaction`). | P2a-T17 (route-shape RESOLVED: label strings not UUIDs), P4-T01/T02/T06 | Synthetic route values flow through `RouteID` dual-write; `name→ID` step built if non-UUID. |
| **R6** | CRM `ErrorCodeTransformer` mounted globally would rewrite ledger's own codes; deleting it is a wire-contract change. | P0-T13/T16, P3-T03/T04/T07, P9-T02/T03/T04 | No external CRM-00xx consumer (P0-T13); canonical-contract regression test locks the end state. |
| **R7** | CRM PII crypto keys (`LCRYPTO_*`) must carry with EXACT values or holder/alias docs become undecryptable. | P0-T14, P3-T06/T08/T15/T18, P8-T19 | Keys sourced from secret store (not a tracked file); round-trip proven in P3-T20. |
| **R8** | `ModuleCRM`/`ModuleFees` provisioning + `WithMB` + per-tenant DB or MT requests fail silently at DB resolution. | P3-T05/T08/T09/T11, P4-T05/T22 | MT-path provisioning is cross-team (tenant-manager); confirm provisioning identity survives collapse. |
| **R9** | Route merge ≠ authz merge — preserve `plugin-crm`/`plugin-fees`/`midaz`/`routing` namespaces verbatim. | P3-T07 (`ApplicationName` const), P4-T10/T22, P9-T05 | Namespaces preserved + documented in `RBAC-NAMESPACES.md`; unification explicitly deferred. |
| **R10** | tracer tenant-manager v4→v5 API drift (highest uncertainty). | P2c-T01 (API diff), P2c-T06, P2c-T10 (MT chaos tests) | `/bootstrap/` is coverage-excluded; MT chaos + godog are the safety net, not unit coverage. |
| **R11** | 85% coverage hard-fail gate turns CI red on under-covered moved code. | P0-T12 (baseline), P2a-T13, P2b-T12, P5-T12c, P6-T15, P7-T10, P8-T06 | Per-component backfill budgeted; gate must evaluate component-scoped (not whole-module-blended) coverage. |
| **R12** | Cross-team blast radius — external Helm `midaz` chart + `midaz-firmino-gitops` + APIDog must update in lockstep or builds-never-deploys / ArgoCD sync breaks. | P0-T06/T07/T15, P3-T18, P4-T21, P5-T14, P6-T19, P7-T19, P8-T20/T22, P9-T14 | The likeliest real-world stall; owner-unavailable fallback armed at every move + P8-T22; HARD-BLOCKING for Helm/gitops, downgrade for APIDog. |
| **R13** | Telemetry middleware split (`commons/net/http` bundled → `lib-observability/middleware`) won't compile on a naive path swap. | P2a-T08, P2b-T07, P2c-T18, P5-T15 (span-emission), P7-T08 | Treated as a real code move with confirmed target API; net/http HTTP helpers stay. |
| **R14** | Dual mongo-driver (v1 + v2) BSON/codec runtime drift. | P2b-T10, P6-T05, P7-T11 | v2 is test-only (one itestkit file); collapsed to v1.17.9; coexistence proven at P7-T11 (the heaviest unverified-until-merge claim). |
| **R15** | godog/cucumber is a test mode midaz CI has never run. | P2c-T20/T24, P5-T12a-decide/T12a, P7-T13a/T14/T15, P8-T12/T18 | shared-vs-bespoke CI delivery DECIDED before implementation; may ship as same-week fast-follow, must be green before deployable. |
| **R16** | Migration/startup path — Dockerfile WORKDIR→migrations COPY contract, pg16→17 compat, DB provisioning. | P5-T01a (early probe)/T08/T10/T10a, P8-T07/T09/T21, P3-T16, P4-T05 | pg16→17 + logical-replication is a possible late stall (FG2); early executable probe in P5-T01a de-risks it. |
| **R17** | Env-var merge collisions — a silent missing/colliding var passes CI build but fails at runtime. | P3-T15/T16, P4-T20, P5-T11, P6-T21, P8-T05/T10/T19 | 3-way `.env.example` diffs; ledger's `SERVER_PORT`/`MONGO_*` win; CRM/fees knobs namespaced. |
| **R18** | Fee-on-revert / pending-cancel refund must balance (sum == 0) — PD-5 third rail. | P4-T14/T16, P7-T18, P9-T12 | VERIFY-not-rebuild; DEDUCTIBLE case + no-double-reverse + applyFees-skipped-on-isRevert all asserted. |
| **R19** | Audit-hash-chain (tracer migrations 000001/000002/000017 + `VerifyHashChain`) SOX/GLBA integrity. | P2c-T21, P5-T01a/T03 (byte-identical SHA + CI guard), P7-T11 | Pure relocation; SHA-verified byte-identical; replication behavior probed on primary + replica. |
| **R20** | reporter-worker fat Chromium image cannot go distroless. | P0-T08, P2b-T13, P6-T10/T17, P8-T07/T09 | Worker stays fat alpine + Chromium (accepted); manager goes distroless-nonroot. |
| **R21** | reporter fetcher external-DB reachability + credential handling on the shared network. | P0-T08, P2b-T13, P6-T11/T16/T21, P8-T10 | Worker repointed to shared `infra-network`; connection-config byte-identical post-rename; e2e proven. |
| **R22** | CRM `X-Organization-Id` header scoping vs ledger path-based scoping API inconsistency. | P3-T20 (documented), P9-T06 (`docs/api/SCOPING.md`) | Accepted exception; documented, not reworked; harmonization out of scope. |
| **R23** | Relicensing moved fee source to EL2.0; `make sec` advisories on the enlarged graph. | P1-T04 (sec residual), P4-T18, P7-T12 | EL2.0 headers applied to moved fee files; new govulncheck findings get a bump or time-boxed waiver, never a vendored patch. |
| **R24** | Single repo-wide semantic-release/version model; conventional-commit scope taxonomy; dependabot. | P1-T06, P5-T13a, P6-T14, P7 (clean tidy), P8-T11/T13/T16/T17 | One `.releaserc.yml` (+ hotfix tied to a real consumer + backmerge); dependabot has zero collapsed-component entries. |
| **R25** | Stale docs / Makefile footgun / misleading `libCommons`→lib-observability aliases survive "liso e final". | P0-T11, P3-T14, P5-T11, P6-T18, P8-T02/T17, P9-T07..T11/T13a | 144 misleading aliases renamed tree-wide (P9-T13a); STRUCTURE/AGENTS/CLAUDE/llms* rewritten; Makefile normalized across both structural shapes. |

---

## 8. Out-of-repo coordination checklist

Surfaces outside the midaz repo that MUST move in lockstep or the green monorepo builds-but-never-deploys.
Each carries a named-owner + timebox + blocks-vs-proceeds rule (defined in P0-T15; re-checked at every move).

| Surface | What changes | Lockstep with | Owner gate | Fallback |
| --- | --- | --- | --- | --- |
| **Helm chart `midaz`** | ADD `tracer`, `reporter-manager`(→`manager`), `reporter-worker`(→`worker`) value keys; REMOVE `crm` + `plugin-fees` image keys. `helm_values_key_mappings` = bare value keys. | `build.yml` filter_paths/prefix change | **HARD-BLOCKING** — move phase cannot start without written sign-off (P0-T06/T15). | P8-T22: gate-the-tag / stage-additive / registry-isolation. |
| **`midaz-firmino-gitops`** | `yaml_key_mappings` ADD `.tracer.image.tag`, `.manager.image.tag`, `.worker.image.tag`; REMOVE `.crm.image.tag` + fees tag. KEYS keep `midaz-<x>.tag` suffix; values are dotted `.image.tag` paths (DISTINCT schema from Helm — do NOT conflate). | same merge window as build.yml | **HARD-BLOCKING** (P0-T07/T15) — yaml-key delete without gitops update breaks ArgoCD sync. | held until lockstep; standalone stays deployable. |
| **APIDog e2e** | ADD scenarios for tracer (`:4020`) + reporter-manager (`:4005`); keep CRM/fees scenarios pointed at ledger `:3002` post-collapse. | post-deploy validation | **NON-BLOCKING / PROCEED** — downgrade to a tracked follow-up on non-sign-off (P0-T15). | e2e job marked non-required; restore coverage post-move. |
| **Origin repos** (`tracer`, `reporter`, `plugin-fees`, standalone `crm` if separate) | Set archived/read-only AFTER consolidated build deploys in all envs; disable ALL origin workflows; README → monorepo pointer. | each repo's move phase (tracer→P5, reporter→P6, fees→P4, crm→P3) | owner sign-off PRIMARY; `isArchived: true` confirms once the rollback grace window closes (P9-T14). | owner-unavailable → record blocker + follow-up ticket, do NOT block phase closure; origin stays writable as rollback target until green. |
| **CRM secret store** | `LCRYPTO_HASH_SECRET_KEY` / `LCRYPTO_ENCRYPT_SECRET_KEY` carried with EXACT production values into the unified ledger config (CRM-namespaced, sourced from secret store, never a tracked file). | P3 crm collapse | secret owner identified (P0-T14); no key material in any committed file. | block carry-over until value rotated/removed if ever found tracked. |
| **`PLUGIN_AUTH_ADDRESS` → `PLUGIN_AUTH_HOST`** | breaking operator-facing env-var rename when CRM reconciles to ledger's auth key. | P3 ops handoff (P3-T18) | flagged to ops alongside `LCRYPTO_*` carry-over. | — (config rename, no code risk). |
| **Shared-workflows version** | pin ONE version across all `uses:` (six versions exist across repos; recommend tracer's `v1.32.0` or newest validated, after reading the v1.27→v1.32 changelog — a renamed input silently no-ops). | P8 CI consolidation (P8-T11/T18) | P8-T18 owner. | — (single-version pin decision). |
| **Registry policy** | harmonize reporter ghcr-only → DockerHub+ghcr for all images. | P8-T11 | confirm no licensing block. | flag in Open Items if blocked. |

---

## 9. Notes on this synthesis

- **Verbatim integrity:** §5 reproduces each phase's task bodies by concatenating the canonical phase files
  (header line stripped, our own phase header supplied). No task was summarized away; all 239 tasks are
  present. If a phase file is later revised, re-synthesize §5 from it.
- **DAG validation result:** every concrete `depends_on` id resolves to a defined task. The prior
  placeholder-label mismatch in P4-T01/T02/T11 `depends_on` is now RESOLVED — those edges reference the
  fees-spike by its concrete id **P2a-T17** (P2a's own DAG states "P2a-T17 … GATES Phase 4 (P4-T01/P4-T02
  depend on it)"). `dag_issues` is now empty. `P1-T-final`/`P2c-T-final` appear only in explanatory prose
  showing their resolution to P1-T06/P2c-T22 (not live edges). The `P8-T15` gap is a numbering gap, not a
  dangling reference.
