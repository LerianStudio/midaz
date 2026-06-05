# F5 — Execution note (contract harmonization: one v4.0.0 spec, served == mounted)

> Created at F5-T15 (skeleton, with the §6 external-coordination register populated up front per
> `docs/v4/plan/F5.md` §5) and completed DURING F5 execution by the orchestrator's assembler. Records the
> commit ledger, the green baseline at the F5 tip, the spike saga (the structural risk this phase exists to
> retire), the decisions register, the gate-closure walk, the Gate-5 newman smoke record, the drift log, and
> the inheritance flags handed to F6. Mirrors `docs/v4/plan/F4-EXECUTION-NOTE.md` and
> `docs/v4/plan/F3-EXECUTION-NOTE.md`.
>
> - **Date:** 2026-06-05
> - **Branch:** `feat/monorepo-consolidation`
> - **F5 base SHA:** `a957c8d18` (F4 execution-note commit; F4 tip + post-F4 docs)
> - **F5 tip SHA:** `564e2761f`
> - **Module at HEAD:** `github.com/LerianStudio/midaz/v3` (the `/v3 → /v4` bump is F6-T18's, D-11; see §7)
> - **Phase risk class:** STRUCTURAL in one place — DC-1 (the swag fold). Everything else (version train,
>   postman, llms, the diff gate) is mechanical regeneration. The whole phase was gated FIRST on the F5-T01
>   spike proving the fold path; once the spike passed, the remaining work was generator runs plus one
>   anti-drift test. F5 owns no Go source rename — it runs the generator over already-final F1/F2/F3/F4 source.

---

## 1. Commit ledger (chronological, on top of the F5 base `a957c8d18`)

F5 landed in three workflow waves. **W1 made TWO commits** — the gate split the relocation/version work from
the tracer/reporter/postman normalization — followed by one post-Gate-5 source fix. Dependency-first
ordering: the spike must pass and the fold must land (W1.a) before the version train and any spec
regeneration; the regenerated ledger spec (W2) must exist before the contract gate and the llms sync (W3);
and the Gate-5 newman smoke surfaced the one defect that became the post-gate fix (`564e2761f`).

| SHA | Wave | Tasks closed | What it closed |
|-----|------|--------------|----------------|
| `5dad5ddc1` | W1.a — fold + pipeline + ledger version | F5-T01, T02, T03, T04 | **DC-1 relocation.** Spike-proven first (T01), then the FULL CRM HTTP layer (`holder.go`, `holder_accounts.go`, `instrument.go` + tests + the route registrar) moved out of `components/crm/adapters/http/in/` into `components/ledger/internal/adapters/http/in/` so the ledger `swag init --parseInternal` discovers every holder/instrument path — mirroring the proven fees fold (T02, R24). Bootstrap import-path fix-ups re-point `crmhttp → httpin`; route paths and the F2-flipped `midaz` authz namespace stay **byte-identical**. The old standalone CRM infra (observability/swagger/registrar test) was DROPPED, not moved (see §3). `generate-docs.sh` `COMPONENTS=("ledger" "crm")` → `("ledger")` so the target stops exiting non-zero on the deleted `components/crm/cmd/app/main.go` (R12, T03). Ledger `main.go` general-info: `@version v3.7.0 → v4.0.0`, `@description` rewritten to enumerate the folded domains (onboarding + transaction + holders + instruments + fees + composition) (T04). |
| `fa1d318e7` | W1.b — version train + tracer/reporter + postman ports + test URLs | F5-T05, T09, T11, T12 | **DC-2 single version source (T05/R44):** ledger `.env.example` `VERSION → v4.0.0`, `SWAGGER_VERSION=${VERSION}` unchanged, collapsing the baked-vs-runtime version fork to one v4 train; `.env` is gitignored, the tracked `.env.example` carries the value. **Tracer + reporter-manager normalized to the `4.0.0` train (T11/Q12/Q13):** general-info `@title`/`@version` in each `cmd/app/main.go` (`Tracer API` / `Reporter API`), `reporter-manager` `.env.example` `SWAGGER_TITLE='Reporter API'` + `VERSION=v4.0.0`, each component's `api/` set regenerated — they stay independent specs/ports/deploy units (no `:3002` mega-spec). **Postman env (T09):** split-service ports retired (`onboardingPort=3000`/`transactionPort=3001` removed), `*Url` vars point straight at `:3002`; `postman/backups` stays gitignored. **`tests/helpers/env.go` (T12):** already read `:3002` at HEAD — pre-satisfied by F0-T02, verified no-op. |
| `e62362e31` | W2 — folded ledger spec + ledger-only postman + orphan removal | F5-T06, T07, T08, T10 | Four-file ledger set (`docs.go`/`swagger.json`/`swagger.yaml`/`openapi.yaml`) regenerated with holders/instruments/composition at `v4.0.0` (T06). `sync-postman.sh` collapsed to ledger-only (`CRM_API` leg dropped, T08). `components/crm/api/` deleted in full — closes the F2 grep-zero carve-out R10 (T07). Postman collection + environment regenerated through the repaired pipeline (T10). |
| `6d5d7ce1f` | W3 — contract gate + llms + coordination register | F5-T13, T14, T15 | **DC-3 served==mounted set-equality test (`TestContractSpecMatchesRoutes`, T14):** builds the unified ledger Fiber route surface, normalizes Fiber `:param` / OpenAPI `{param}` to a positional structure key, drops the LOCKED public-infra exclusions (`/health` `/version` `/readyz` `/swagger` `/swagger/*`) plus Fiber's auto HEAD twins, and asserts set-equality against the committed `swagger.json` paths in BOTH directions — the durable anti-drift mechanism. **`llms.txt` + `llms-full.txt` synced (T13):** instruments noun, composition endpoint (`GET /v1/holders/:id/accounts`), removed `/v1/aliases` and `plugin-crm` references, v4.0.0 language. **§6 external-coordination register (T15):** X1/X4/X5 listed, owned, explicitly NOT EXECUTED — Gate 8 discharge only. |
| `564e2761f` | post-Gate-5 fix — generator emits `:3002` | (T09 source repair) | **The Gate-5 newman smoke caught the T10 regen clobbering T09's env fix:** F5-T09 edited the artifact, but the GENERATOR (`convert-openapi.js`) still emitted the split-service ports, so regen reintroduced `onboardingPort=3000`/`transactionPort=3001`. Fixed at the SOURCE — both ports now emit `3002` in `convert-openapi.js` — and the committed environment aligned with what a regen produces. Var names kept (the collection references `onboardingUrl`/`transactionUrl` 296×). **= F5 baseline-capture tip.** |

Wave boundaries follow the `F5.md` §3 DAG and the hard ordering constraint (R11/Gate 2): the spike runs FIRST
and no downstream task starts until it proves the fold. W1.a folds + repairs the pipeline + bumps the ledger
version; W1.b normalizes the version train, tracer/reporter, postman ports, and test URLs; W2 regenerates the
ledger spec/postman and deletes the orphaned CRM spec set; W3 lands the contract gate, syncs llms, and files
the coordination register. The post-gate `564e2761f` is the F5 analogue of F3's out-of-wave defect commits —
here the defect was a generator/artifact desync that only the end-to-end newman smoke could surface.

---

## 2. Baseline (captured at the F5 tip `564e2761f`)

| Command | Exit | Result |
|---------|------|--------|
| `make test-unit` | 0 | 16,138 tests, 6 skipped |
| `make test-integration` | 0 | 1,005 tests, 80 skipped (**`RETRY_ON_FAIL=1` declared**, 1 non-F5 flake absorbed) |
| `make test-property` | 0 | 70 tests, 7 skipped |
| `make test-reporter-chaos` | 0 | 39 tests, 39 skipped (`CHAOS=1` opt-in by design) |
| `make ci` | 0 | single exit code; all four legs reproduced green at the tip |

`make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor; `make ci` is the
single-verdict superset. **Unit count fell vs the F4 tip (16,154 → 16,138, a delta of −16): the dead
standalone CRM router tests dropped with the unreachable code.** The DC-1 relocation moved the live CRM HTTP
layer into the ledger tree and DROPPED the standalone CRM router surface (`NewRouter`/`ReadyzHandler`/CRM
`swagger.go`/`observability.go`) as unreachable-and-colliding rather than moving it (see §3); the unit tests
that exercised that now-deleted surface went with it. This is a deletion of dead-code coverage, **not** a
regression — the live holder/instrument handler tests relocated intact alongside their handlers and still run.
Integration (1,005) and property (70) are flat against the F4 tip: F5 regenerates artifacts and adds one
contract gate; it ships no new integration-driven behavior. The 6 unit skips / 7 property skips are
pre-existing/benign and unchanged from F4.

### Environment disclosure (recorded honestly)

The integration leg saw **one flake** during capture, absorbed by the declared `RETRY_ON_FAIL=1`. Same family
the F0–F4 notes flagged: the docker.sock inspect-deadline on macOS Docker Desktop under sustained sequential
testcontainers load — the daemon inspect API wedges at a random matrix position while containers start fine;
zero assertion failures, and it is **not** in an F5-touched package (F5 added no integration test). The
declared retry discharged it within the same run. Linux CI runners remain the authoritative environment; the
declared-retry green + zero assertion failures is the binding signal.

---

## 3. Spike saga (the structural risk F5 exists to retire — R11 / DC-1)

F5's one real risk was mechanical and binary: **does relocating CRM `@Router` handlers under the ledger
`http/in` tree make `swag init` discover them?** The whole phase's "single spec" premise (D-11) collapses if
the answer is no. F5-T01 ran the spike FIRST, before any version/postman/llms work was spent.

1. **The fold is proven.** The spike relocated the instrument handler into `components/ledger/internal/adapters/http/in/`,
   ran `swag init -g cmd/app/main.go -o api --parseDependency --parseInternal`, and confirmed the
   holder/instrument paths appeared in the generated `swagger.json` — **4 instrument paths surfaced from a
   starting 0**. The mechanism is `--parseInternal` traversal of the ledger tree, NOT `--parseDependency`
   walking the imported `components/crm` subtree (the pre-fold run already imported `crmhttp` and produced
   zero CRM paths — direct evidence the import graph alone does not drive discovery). The regression guards
   held: `/v1/aliases` stayed at 0 (the F2 rename already removed the alias surface; the guard is a
   set-membership check, not the load-bearing assertion — that's the positive fold-completeness direction).

2. **The `pkg.HTTPError` gotcha.** swag does not skip an unresolved annotation type — it **aborts the whole
   run**. The relocated handlers' `@Failure` annotations reference `pkg.HTTPError`; once the handlers lived in
   the ledger tree, the ledger `swag init` needed the root `pkg` import resolvable for those annotation types
   or the generation failed outright. Fixed by ensuring the relocated handlers carry the root `pkg` import so
   swag resolves `pkg.HTTPError`. Recorded because the failure mode is non-obvious: a single unresolved type
   takes the entire spec down, not just the one path.

3. **The relocation reality (Go internal rule).** The handler structs could not be split from their registrar
   and infra across the `internal/` boundary, so the **WHOLE CRM HTTP layer moved** into the ledger `http/in`
   tree, mirroring fees. But the standalone CRM router surface — `NewRouter`, `ReadyzHandler`, the CRM
   `swagger.go`, the CRM `observability.go` — was **DROPPED as unreachable-and-colliding rather than moved.**
   There is no standalone CRM service (per CLAUDE.md: CRM is a package tree imported by the ledger binary), so
   that router surface had no live caller and would have collided with the ledger's own router/readyz/swagger/
   observability wiring. `routes_test.go` was trimmed to the surviving `ApplicationName` assertion (the rest
   tested the dropped router). This drop is the source of the −16 unit-count delta in §2.

The spike's verdict — fold proven, gotcha understood, relocation scope settled — is what unblocked W1.a
through W3. No re-plan was triggered; the phase ran as planned.

---

## 4. Decisions register (`F5.md` §5 mandated list + execution findings)

| Decision | Value | Source / rationale |
|----------|-------|--------------------|
| **`@Router` path typo fix** | `{holder_id}` → `{id}` on the relocated handler annotation | Execution finding (W1.a, `5dad5ddc1`). The served route declares `:id`; the inherited CRM annotation read `{holder_id}`, which would have failed the DC-3 served==mounted gate. Fixed to `{id}` to match the served route AND the sibling composition/account docs. This is the only annotation TEXT change F5 made — the F2 rename owned the noun, F5 only relocates; this typo was a pre-existing mismatch caught by the gate. |
| **Version string form** | `4.0.0` (unprefixed) for swag-baked specs; `v4.0.0` (prefixed) for env knobs | HEAD convention. swag general-info `@version` bakes `4.0.0` into the spec (OpenAPI `info.version` is conventionally unprefixed); `VERSION`/`SWAGGER_VERSION` env values carry the `v`-prefixed `v4.0.0` product-train form. Both are the v4 train — the prefix difference is the spec-vs-env convention, not a fork (T04/T05). |
| **VERSION train source** | `reporter-manager .env.example` is the **durable** train source; `.env` is gitignored | T05/T11 (`fa1d318e7`). `.env` carries the live value but is gitignored, so the tracked `.env.example` is the committed source of the `v4.0.0` train for both ledger and reporter-manager. F6's release-version normalization consumes these `.env.example` values. |
| **reporter-manager openapi.yaml regen** | regenerated via the `mk/docs.mk` docker step (NOT a bespoke per-component target) | T11 (`fa1d318e7`). reporter-manager's component Makefile lacks a swag-to-openapi target, so its `openapi.yaml` was regenerated through the shared `mk/docs.mk` docker step. Recorded so a later phase does not look for (and fail to find) a reporter-manager-local generate target. |
| **T09 env-var handling** | KEEP `onboardingUrl`/`transactionUrl` (referenced 296×); re-point their VALUES to `:3002` | T09 (`fa1d318e7`, source fix `564e2761f`). Pragmatic: the collection references these var names 296 times; renaming would churn the whole collection for no contract benefit. The var NAMES stay; only their port VALUES move to `:3002`. The split-service `onboardingPort`/`transactionPort` numeric vars were removed outright (no longer meaningful in a single-binary deploy). |
| **T12 status** | pre-satisfied by F0-T02 — verified no-op | T12 (`fa1d318e7`). `tests/helpers/env.go` already read `:3002` at HEAD (F0-T02 landed earlier on this branch). The F5-T12 task asserts the grep and skips the edit; a pre-satisfied state is not a failure (per F5.md §2 T12 Notes). |
| **Stray uncommitted `api/` regen** | DISCARDED at start, regenerated from clean HEAD | Execution finding (start of W1). An uncommitted `api/` regen of unknown provenance was present in the working tree at phase start. Discarded rather than trusted — every committed spec artifact in F5 was regenerated from a clean HEAD by the in-phase pipeline, so provenance is the F5 generator runs, not an inherited working-tree mutation. |

---

## 5. Gate-closure walk (`F5.md` §3, all 8 gates)

Every exit gate mapped to its closing task(s)/commit and where the proof lives. A gate without a located proof
is a defect.

| Gate (§3) | Closing task(s) / commit | Where the proof lives |
|-----------|--------------------------|-----------------------|
| **1 — Served swagger == mounted routes (diff check clean)** | T14 against the T06 spec (`6d5d7ce1f`, spec from `e62362e31`) | `TestContractSpecMatchesRoutes` in the unit floor — builds the unified ledger Fiber route surface, normalizes `:param`/`{param}`, drops the LOCKED public-infra exclusions (`/health` `/version` `/readyz` `/swagger` `/swagger/*`) + Fiber auto-HEAD twins, asserts set-equality against the committed `swagger.json` paths in BOTH directions. Served and mounted can no longer silently diverge. |
| **2 — CRM/instruments folded into the ledger spec (JSON + YAML)** | T01 (proves mechanism), T02 (folds, `5dad5ddc1`), T06 (regenerates, `e62362e31`) | The spike surfaced 4 instrument paths from 0 (§3); T02 relocated the full layer; the regenerated `components/ledger/api/swagger.json` + `swagger.yaml` carry the holder/instrument/composition surface. `/v1/aliases`==0 holds as a regression guard. |
| **3 — Pipeline runs end-to-end (`make generate-docs` exits 0)** | T03 (COMPONENTS fix, `5dad5ddc1`), T06 (ledger regen, `e62362e31`), T10 (postman leg, `e62362e31`) | `generate-docs.sh` `COMPONENTS=("ledger")` no longer references the deleted `components/crm/cmd/app/main.go`; the target runs ledger swag + postman conversion to completion, `tmp/` cleaned. |
| **4 — Zero stale ports/hosts/versions across touched files** | T04, T05 (ledger version, `5dad5ddc1`/`fa1d318e7`), T06 (regen), T07 (`:4003` deletion, `e62362e31`), T09 (postman ports, `fa1d318e7`+`564e2761f`), T11 (tracer/reporter, `fa1d318e7`), T12 (test URLs, `fa1d318e7`) | No surviving `:4003`/`v3.7.0`/`v3.8.0`/`:3000`/`:3001` in touched files; tracer + reporter-manager at `4.0.0`; the postman env at `:3002` (after the `564e2761f` generator fix — see Gate 5 catch). |
| **5 — Postman collection green against `make up`** | T08, T09, T10 (`e62362e31`/`fa1d318e7`); defect fix `564e2761f` | The newman smoke record below. PASS — all folded domains green on `:3002`, zero route-not-found. |
| **6 — api/ artifacts regenerated for tracer + reporter-manager (`4.0.0` + Q12 titles)** | T11 (`fa1d318e7`) | tracer + reporter-manager `api/` sets regenerated, full set, `4.0.0`, titles `Tracer API` / `Reporter API`; independent specs/ports/deploy units (Q13) — no `:3002` mega-spec. |
| **7 — llms sync (instruments noun, composition, removed `/aliases`+`plugin-crm`, v4.0.0)** | T13 (`6d5d7ce1f`) | `llms.txt` + `llms-full.txt`: instruments noun, `GET /v1/holders/:id/accounts` composition endpoint, no surviving `/v1/aliases` or `plugin-crm` references, v4.0.0 language. |
| **8 — External coordination listed (NOT executed)** | T15 (`6d5d7ce1f`) | §6 below — X1 (auth-server RBAC), X4 (APIDog), X5 (docs.lerian.studio) listed, owned, explicitly NOT EXECUTED. |

---

## 5a. Gate-5 newman smoke record

**Verdict: PASS.** End-to-end against a running stack (`make up`), `:3002`.

- **Workflow folder:** 57/57 requests, 165/165 assertions.
- **Targeted folded-domain subset:** 9/9 requests, 12/12 assertions.
- **Composition + fees:** probed PAST the mount to handler-level validation errors (the routes are reachable
  and validating, not 404ing) — the load-bearing signal is **zero route-not-found** across every folded
  domain.

**THE CATCH (the defect that became `564e2761f`).** The T10 regen clobbered T09's environment fix: the
generator (`convert-openapi.js`) still emitted `onboardingPort=3000`/`transactionPort=3001`, so regenerating
the environment reintroduced the split-service ports that T09 had hand-edited out of the artifact. The smoke
caught it. **Fixed at the SOURCE** — both ports now emit `3002` in `convert-openapi.js` — and the committed
environment was aligned with what a regen produces, so the fix survives the next regeneration.

**Caveats (recorded honestly):**
- **CRM/fees collection folders are not self-chaining** — their generated test scripts are thin, so those
  folders do not auto-thread IDs the way the workflow folder does. Recorded as a **docs-pipeline improvement
  candidate** (enrich the generated test scripts), not an F5 blocker.
- **No `make newman` target exists** — the smoke was run via `npx newman@6`. A `make newman` target is a
  candidate for the docs pipeline.
- **Stack brought up via `docker compose`** directly per the rtk-hook caveat (the hook rewrites `make up`).

---

## 6. External coordination (§13 register reflection) — LISTED, NOT EXECUTED

F5 reflects the new contract surface in specs/docs. It **does not** execute any out-of-repo coordination.
The three items below mirror `docs/v4/PLAN.md` §13 (rows X1/X4/X5) and the Scope (e) register in
`docs/v4/F0-consumer-coordination-inventory.md`. Each is the discharge of F5 **Gate 8**: enumerated, owned,
and explicitly marked NOT EXECUTED. The authoritative upstream register is F0's inventory; this section is a
pointer to it, not a substitute.

### X1 — auth-server / tenant-manager RBAC policy migration

- **STATUS: NOT EXECUTED by F5.** RELEASE/DEPLOY gate, **not** a merge gate.
- **Owner:** **Fred + plugin-auth team.**
- **What F5 does:** F5-T13 regenerates `llms*.txt` and the specs to reflect the NEW namespace the F2 flip
  settled (D-10). F5 does **not** touch RBAC policies and does **not** perform the migration.
- **What must happen out-of-repo (Q3-RESOLVED, Fred 2026-06-04):** F2 performs the FULL in-code hard cut —
  the old `/v1/holders/:holder_id/aliases*` routes are removed AND the `plugin-crm` authz namespace is
  flipped in code. That flip orphans every tenant's `plugin-crm:*` grant in the external auth-server
  (`docs/auth/RBAC-NAMESPACES.md:8-12`). Fred works with the plugin-auth team at v4 finalization to migrate
  every grant to the new namespace.
- **Gate posture:** the in-code flip merges in F2; **NO auth-enabled environment deploys v4 until X1
  confirms.** Local/dev stacks with auth disabled are unaffected. This is the single most dangerous
  coordination item in v4.
- **R-class:** R1 (Critical) / R9-class namespace-keying. The complete migration surface is the
  four-namespace inventory in F0 Scope (a) §1 (`plugin-crm` is the only one flipped; `plugin-fees`/`midaz`/
  `routing` stay verbatim).
- **Upstream rows:** `docs/v4/PLAN.md` §13 X1 (`:882`); `docs/v4/F0-consumer-coordination-inventory.md`
  Scope (e) X1.

### X4 — APIDog e2e scenario refresh

- **STATUS: NOT EXECUTED by F5.**
- **Owner:** **QA / API owners.**
- **What must happen out-of-repo:** update APIDog scenarios for the renamed `/instruments` routes, the
  removed `/aliases` surface, the composition endpoint, and the tracer reservation API; repoint
  `MIDAZ_APIDOG_TEST_SCENARIO_ID` at the v4 scenarios.
- **Trigger / gate:** F2/F5 trigger (renamed routes + harmonized contract); F6 gates at release. Consumes
  the contract divergence recorded in F0 Scope (a) §4.
- **Upstream rows:** `docs/v4/PLAN.md` §13 X4 (`:885`); `docs/v4/F0-consumer-coordination-inventory.md`
  Scope (e) X4.

### X5 — docs.lerian.studio publication

- **STATUS: NOT EXECUTED by F5.** No in-repo publication step exists; F5 lists it, does not execute it.
- **Owner:** **Docs owners.**
- **What must happen out-of-repo:** publish the v4 API docs and the D-10 migration guide / release notes to
  docs.lerian.studio.
- **Trigger:** F5 triggers (contract harmonization). Consumes the contract divergence recorded in F0
  Scope (a) §4.
- **Upstream rows:** `docs/v4/PLAN.md` §13 X5 (`:886`); `docs/v4/F0-consumer-coordination-inventory.md`
  Scope (e) X5.

> **Scope fence.** F5 owns the in-repo contract surface only (specs, postman, llms, the diff gate). The
> external Helm chart / `midaz-firmino-gitops` lockstep (X2/X3) and the external Go-importer migration (X6)
> are F6-triggered and are NOT F5's coordination items — they are listed in F0 Scope (e) for completeness but
> are out of F5's Gate 8.

---

## 7. Inheritance flags for F6

1. **The `VERSION=v4.0.0` train values are in place — F6 consumes them.** Ledger `.env.example` and
   reporter-manager `.env.example` both carry `VERSION=v4.0.0` (the durable, gitignored-`.env`-backed source);
   tracer bakes `4.0.0` into its spec. F6's release-version normalization
   (`v3.7.0`/`0.1.0`/`1.2.0`/`v1.0.0` → `4.0.0`) reads from these. F5 set the train; F6 ships on it.

2. **The D-11 module bump (`/v3 → /v4`) is F6-T18's, not F5's.** At the F5 tip the module is still
   `github.com/LerianStudio/midaz/v3`. The bump sweeps imports + OTEL service-version values + lands the
   **release-trigger `BREAKING CHANGE` commit**. F6-T18 runs `semantic-release` **dry-run** against the latest
   tag `v3.8.0-beta.9` to confirm the BREAKING CHANGE footer drives the major. F5 deliberately did not touch
   the module path — doing so before F6 would churn every import for no F5 contract benefit and pre-fire the
   release trigger.

3. **RUN_PATTERN / legacy-tracer-suite harness decision is STILL OPEN for F6 Gates 8/13.** F5 did not resolve
   how the legacy tracer suite plugs into the unified `RUN_PATTERN ^TestIntegration` harness. F6 must close
   this before its integration gates.

4. **Remaining recorded debts (carried forward, none F5-blocking):**
   - 31 `//go:build unit` zombies (the tag is no longer the unit-test selector; the files still carry it).
   - 25 untagged, mock-based, integration-NAMED tests (named `*_integration_test.go` but neither
     `//go:build integration` nor real-dependency-backed — they run in the unit leg).
   - `gocyclo` outliers.
   - `bin/` not gitignored.
   - down-migration linter gap.

   These are pre-existing/inherited; F5 added none of them and resolved none (out of scope). F6 or a
   dedicated cleanup phase owns them.

---

## 8. Drift log (HEAD-vs-spec, recorded at execution)

| Drift | Resolution |
|-------|------------|
| **F0-T02 / F5-T12 overlap** | `tests/helpers/env.go` already read `:3002` at HEAD (F0-T02 landed earlier on this branch). F5-T12 verified the grep and skipped the edit — a pre-satisfied no-op, not a failure (F5.md §2 T12 Notes). |
| **T09 fix vs T10 regen desync** | The generator (`convert-openapi.js`) out-of-sync with the hand-edited artifact: T09 fixed the artifact, T10's regen reintroduced `:3000`/`:3001`. Caught by the Gate-5 newman smoke; fixed at the source (`564e2761f`). HEAD-wins discipline: the generator is now the truth, the artifact matches a clean regen. |
| **`@Router {holder_id}` typo** | The inherited CRM annotation read `{holder_id}` against a served `:id` route — would have failed the DC-3 gate. Fixed to `{id}` to match the served route + sibling docs (W1.a). |
| **Stray uncommitted `api/` regen at phase start** | Unknown provenance; DISCARDED and regenerated from clean HEAD by the in-phase pipeline. |
