# F5 Execution Note — Contract Harmonization

> **Status:** SKELETON. This note is created at F5-T15 and completed DURING F5 execution by the
> orchestrator's assembler. It records decisions and commit SHAs as tasks land (per `docs/v4/plan/F5.md` §5,
> mirroring `docs/monorepo/plan/P2a-EXECUTION-NOTE.md`). The external-coordination section below (§13
> register reflection) is populated NOW because F5-T15 / Gate 8 require it; everything else is a placeholder
> filled at execution time.
> - **Branch:** `feat/monorepo-consolidation`
> - **Phase:** F5 (contract harmonization) — task breakdown at `docs/v4/plan/F5.md`, macro plan
>   `docs/v4/PLAN.md` §9.
> - **Upstream register:** `docs/v4/F0-consumer-coordination-inventory.md` (the single committed record of
>   every in-repo consumer surface and every out-of-repo coordination item; Scope (e) is the external
>   register this section reflects).

---

## 1. Task ledger (filled during execution)

> Placeholder. As each F5 task lands, record: decision made, the regenerated/relocated artifact, and the
> commit SHA. Mirror the P2a note's per-task structure.

| Task | Outcome | Commit SHA |
|------|---------|------------|
| F5-T01 — swag-fold spike (pass / re-plan) | _pending_ | _pending_ |
| F5-T02 — CRM `@Router` handlers relocated under ledger `http/in` | _pending_ | _pending_ |
| F5-T03 — `generate-docs.sh` `COMPONENTS=("ledger")` | _pending_ | _pending_ |
| F5-T04 — ledger swag general-info `v4.0.0` + description | _pending_ | _pending_ |
| F5-T05 — `VERSION=v4.0.0` single source (DC-2) | _pending_ | _pending_ |
| F5-T06 — ledger `api/` artifact set regenerated | _pending_ | _pending_ |
| F5-T07 — `components/crm/api/` deleted | _pending_ | _pending_ |
| F5-T08 — `CRM_API` leg dropped from `sync-postman.sh` | _pending_ | _pending_ |
| F5-T09 — stale split-service postman env vars retired + backups pruned | _pending_ | _pending_ |
| F5-T10 — postman collection + environment regenerated; `v4.0.0` | _pending_ | _pending_ |
| F5-T11 — tracer + reporter-manager specs normalized to `4.0.0` train | _pending_ | _pending_ |
| F5-T12 — `tests/helpers/env.go` defaults → `:3002` | _pending_ | _pending_ |
| F5-T13 — `llms.txt` + `llms-full.txt` synced to v4 contract surface | _pending_ | _pending_ |
| F5-T14 — spec-vs-routes diff gate (`TestContractSpecMatchesRoutes`, DC-3) | _pending_ | _pending_ |
| F5-T15 — this external-coordination note | _this note_ | _pending_ |

---

## 2. External coordination (§13 register reflection) — LISTED, NOT EXECUTED

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

## 3. Drift log (filled during execution)

> Placeholder. Record any HEAD-vs-spec drift encountered while executing F5 tasks (e.g. the F0-T02/F5-T12
> overlap, the R31 phantom-holder-YAML residual, the F2-settled namespace string). HEAD wins over spec;
> drift is recorded here and returned with the deliverable.
