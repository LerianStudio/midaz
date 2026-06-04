# F0 — Consumer / Coordination Inventory

> **Status:** Gate-3 artifact for v4 phase F0 (`docs/v4/plan/F0.md` tasks F0-T07..F0-T14, tracing macro plan `docs/v4/PLAN.md` §4 Scope (a)+(e) and §13). This is the single committed record of every in-repo consumer surface and every out-of-repo coordination item the v4 work touches. F0 **records**; downstream phases (F2/F3/F5/F6) **act**. Each row carries file:line evidence at the gate-zero epoch SHA; each out-of-repo item carries a named owner.
> - **Date:** 2026-06-04
> - **Branch:** `feat/monorepo-consolidation`
> - **Gate-zero epoch SHA:** `46706c9eaff0a85fab80f693b1d4f82de56e239b`

This artifact closes v4 F0 **Gate 3** (consumer/coordination inventory committed, all Scope (a)+(e) rows with file:line evidence, named owners, SDK row marked coordination with the correct negative grep), **Gate 1** (baseline record below), and **Gate 5** (branch + epoch section below). The companion `docs/v4/plan/F0-EXECUTION-NOTE.md` carries the decisions/SHA log and drift resolutions.

---

## Scope (a) — In-repo consumer surfaces

### 1. Authz namespace surface (X1 — D-10 migration completeness)

The unified ledger binary keys RBAC authorization on **four distinct `ApplicationName` namespace strings**, not just `plugin-crm`. Each is the first argument to `auth.Authorize(<namespace>, <resource>, <action>)`. The tenant-manager / auth-server RBAC policies key on these strings, so any rename orphans the corresponding policy grants (R1/R51). D-10 flips **only** `plugin-crm` (F2); the other three are recorded here as untouched-by-D-10 but external-policy-keyed where noted, so the migration surface is not under-counted.

| Namespace string | Constant (file:line) | Registration sites (file:line) | Resource literals gated | External-policy-keyed? | What v4 does to it |
|------------------|----------------------|--------------------------------|--------------------------|------------------------|--------------------|
| `plugin-crm` | `ApplicationName` — `components/crm/adapters/http/in/routes.go:21` | `routes.go:86-90` (`holders`), `routes.go:93-98` (`aliases`) — 11 routes total | `holders` (post/get/patch/delete), `aliases` (post/get/patch/delete, incl. related-parties delete) | **Yes** — tenant-manager RBAC policies key on `"plugin-crm"` | **D-10 renames this (F2).** This is the namespace-migration surface. Renaming the literal without coordinating the external policy update orphans all CRM grants (R1, Critical). |
| `plugin-fees` | `feesApplicationName` — `components/ledger/internal/adapters/http/in/fees_routes.go:19` | `fees_routes.go:40-44` (`packages`), `:47` (`estimates`), `:50-54` (`billing-packages`), `:57` (`billing-calculate`) | `packages` (post/get/patch/delete), `estimates` (post), `billing-packages` (post/get/patch/delete), `billing-calculate` (post) | **Yes** — explicit keying-contract warning at `fees_routes.go:16-18`: preserved verbatim from the standalone plugin-fees service; "tenant-manager RBAC policies key on this string, so it MUST NOT be renamed (R9)" | **Untouched by D-10.** Recorded as a do-not-rename invariant; the in-code comment already pins R9. |
| `midaz` | `midazName` — `components/ledger/internal/adapters/http/in/routes.go:26` | direct: `routes.go:75/82/89` (`settings`); via wrapper `protectedMidaz` (`routes.go:226-228`) across the core ledger CRUD surface | `organizations`, `ledgers`, `assets`, `asset-rates`, `portfolios`, `segments`, `accounts`, `balances`, `transactions`, `operations`, `settings` | Yes — auth-server policies key on `"midaz"` for the core ledger surface | **Untouched by D-10.** Recorded as external-policy-keyed; not in the F2 rename scope. |
| `routing` | `routingName` — `components/ledger/internal/adapters/http/in/routes.go:27` | via wrapper `protectedRouting` (`routes.go:230-232`) | `account-types`, `operation-routes`, `transaction-routes` | Yes — auth-server policies key on `"routing"` for the routing surface | **Untouched by D-10.** Recorded as external-policy-keyed; not in the F2 rename scope. |

**DRIFT NOTE (HEAD vs macro plan §4):** The macro plan §4 cites the `midaz`/`routing` namespaces as raw string literals at `routes.go:25-28`. At HEAD they are **named constants** — `midazName = "midaz"` (`routes.go:26`) and `routingName = "routing"` (`routes.go:27`) — consumed at registration sites through the `protectedMidaz` (`:226`) and `protectedRouting` (`:230`) wrappers and a few direct `auth.Authorize(midazName, ...)` calls (`:75/82/89`). A grep for bare `"midaz"`/`"routing"` string literals at route-registration call sites will NOT match; the inventory cites the constant names. (`plugin-crm` and `plugin-fees` are also named constants — `ApplicationName`, `feesApplicationName` — consistent with the plan.)

**Why this matters for X1 (consumed by F2/F6 release gate):** the D-10 namespace flip touches exactly one of these four strings (`plugin-crm`). The other three are external-policy-keyed and must be left verbatim. The complete four-namespace surface is the input the X1 coordination owner (Fred + plugin-auth) uses to confirm no in-repo namespace string is renamed without a matching external RBAC-policy update.

---

### 2. Tracer `POST /v1/validations` — the D-6 migration surface (EXISTING, not net-new)

**Risk: R2 (Critical). Owning phases: F0 (record) → F3 (convert). Coordination: out-of-repo callers of `POST /v1/validations` (owner: tracer-API consumers, see Scope (e)).**

The tracer validate endpoint **already exists** and **already commits usage counters synchronously and atomically**. D-6 (two-phase reserve→confirm/release with TTL) is therefore a **redesign of the inline commit**, not a greenfield build. A D-6 implementer who designs as if there were "no endpoint today" misses the existing idempotency contract and the `usage_counter_repository` migration constraints — the highest-cost framing error in F0, hence this row.

| Migration-surface element | HEAD evidence (file:line) | What it is today | D-6 conversion (F3) |
|---|---|---|---|
| The endpoint | `components/tracer/internal/adapters/http/in/routes.go:329` — `api.Post("/validations", guard.With("validations", "post", cfg.APIKeyOnlyValidation), validationHandler.Validate)` | Counter-mutating POST; under the `validations` authz resource. (Read-only siblings: `GET /validations` at `:319`, `GET /validations/:id` at `:320`.) | Same endpoint becomes **reserve** (with TTL); not a new route. |
| The synchronous atomic count-commit contract | `components/tracer/internal/services/validation_service.go:152-166` (contract comment; load-bearing lines `:160-163`), enforced in-transaction at `:264-289` (BeginTx + deferred Rollback) and committed at `:358-359` (`tx.Commit()`) | "If ALLOW: COMMIT saves counters, validation record, and audit event atomically; if limit exceeded or REVIEW: `tx.Rollback()` atomically undoes counter increments." Single-tx, no compensating rollbacks. | Convert the single inline commit into **reserve(TTL) → confirm (post-commit) / release (on abort)**, preserving counter-correctness across the TTL window. |
| The idempotency contract (must be preserved) | `components/tracer/internal/adapters/http/in/validation_handler_idempotency_test.go` — `TestValidationHandler_Validate_ReturnsCorrectStatusCodes` (`:38`), `TestValidationHandler_Validate_IdempotencyHeader` (`:153`) | DD-3 (Stripe-model) duplicate-request handling already locked by test. | Reserve/confirm transition MUST NOT break this; the reserve phase inherits the idempotency key contract. |
| The counter repository | `components/tracer/internal/adapters/postgres/usage_counter_repository.go` — atomic upsert+increment CTE with `INSERT ... ON CONFLICT DO UPDATE` + WHERE guard at `:36-53`; `UpsertAndIncrementAtomic` accepts an **external `db` connection** at `:318` (so the increment is composable inside the caller's tx); `IncrementAtomic` at `:247` | Single atomic increment guarded by `current_usage + amount <= maxAmount`. | Modified WHERE guard `current_usage + reserved_usage + amount <= maxAmount`; a `reserved_usage` bucket + `usage_reservations` table; a sub-minute reservation reaper. |

**Negative fact (verified):** the ledger does **not** import or call the tracer today — `grep -rn "components/tracer" components/ledger --include="*.go"` → **0 hits**. D-5 (F3) wires the call at the transaction-creation seam; D-6 redesigns the counter semantics on the tracer side.

**Net-new delta (the only genuinely greenfield pieces):** confirm/release flow + the TTL reservation lifecycle (`reserved_usage` bucket, `usage_reservations` table, reservation reaper). Everything else listed above is **converted, not built**.

**TTL-adjacent primitives already present (narrows the delta):** `UpsertAndIncrementAtomic` already takes an `expiresAt` parameter (`usage_counter_repository.go:318`), and `DeleteExpiredCounters(ctx, now)` already exists at `:663`. Expiry plumbing is not greenfield; the net-new work is the reservation *bucket/table/reaper*, not TTL columns or expiry sweeps from scratch.

**DRIFT NOTE (HEAD vs macro plan):** the macro plan cites the ALLOW/COMMIT atomic contract comment at `validation_service.go:158-166`. At HEAD the contract comment block starts at `:152` with the load-bearing ALLOW→COMMIT / non-allow→Rollback lines at `:160-163`; the `:166` `Validate` func signature is exact. Only the comment-block start drifted (`:158`→`:152`); the cited `:166` anchor still resolves.

---

### 3. midaz Go-module importers + SDK/console negative grep (R29, R5)

> Scope (a) consumer row + Scope (e) coordination row (X6 below). F0 records; the FIX/coordination is owned downstream (F6/D-11 module bump, X6 release-coordination register).

#### Module-path bump blast radius

| Item | Evidence (HEAD) | What D-11 does | Verifiability |
|------|-----------------|----------------|---------------|
| midaz Go module path | `go.mod:1` — `module github.com/LerianStudio/midaz/v3` | D-11 bumps `/v3 → /v4`. A hard breaking change for any external Go importer of `pkg/*` (and for the `mmodel.Alias → mmodel.Instrument` identifier rename under D-1). | Single, exact touchpoint in-repo. |
| SDK / console / partner Go importers | None in tracked source (see negative-grep below). | Imports `github.com/LerianStudio/midaz/v3/pkg/... → /v4` plus the `mmodel.Alias → mmodel.Instrument` rename. | **Unverifiable in-repo** → coordination task X6. |

#### Negative-grep evidence — the authoritative form

The absence of any in-repo SDK/console importer is established by a **tracked-only** scan:

```
git grep -niE "midaz-sdk|sdk-golang|sdk-typescript|midaz-console"
```

This is the **authoritative negative**: it reproduces **0 actual source consumers**. The only tracked files that match the token strings are the v4 plan documents themselves — `docs/v4/PLAN.md` and `docs/v4/plan/F0.md` (and now this inventory) — which literally contain `midaz-sdk` / `sdk-golang` / `midaz-console` as plan-text describing this very inventory. Those are self-references, not source references.

**DRIFT (recorded):** Do NOT cite the `--exclude-dir=.git` form as the negative. At HEAD:

```
grep -rniE "midaz-sdk|sdk-golang|sdk-typescript|midaz-console" . --exclude-dir=.git | sed 's/:.*//' | sort -u
# → ./docs/v4/PLAN.md
# → ./docs/v4/plan/F0.md   (NON-ZERO)
```

The `--exclude-dir=.git` form returns **non-zero** solely because of the v4 plan-text self-references above — there is no real source consumer in either file. As more F-phase plan files land that name the SDK token strings, this count grows; the exact number is unstable and **not load-bearing**. The macro plan's earlier claim that the `.git`-excluded form returns 0 is stale (it predates the plan docs that self-reference the tokens). What is load-bearing: **none** of the surviving hits is a real consumer. The artifact and Gate 3 cite the `git grep` (tracked-only) form.

The `/v4` module bump and the `v4.0.0` release tag are decoupled (semantic-release derives version from the git tag, not `go.mod`) — recorded separately in the module-bump/release-tag section below (§ Module/tag decoupling).

---

### 4. Contract-surface divergence + version fork (R13, R44, records R12)

> **Owning phase:** F0 records; **F5** harmonizes the contract surface and fixes `generate-docs` (R12); **F5/D-11 owner** reconciles the version source.

| # | Divergence | HEAD evidence (file:line) | Why it masks the real API | v4 disposition |
|---|-----------|---------------------------|---------------------------|----------------|
| 1 | **Ledger spec carries fees but zero holders** | `components/ledger/api/swagger.json` — `grep -c fee` → **50**, `grep -c holder` → **0** | The unified binary serves holders/aliases (CRM, `plugin-crm`) on `:3002`, but the generated ledger OpenAPI documents fees and **no** holder surface. A consumer reading the ledger spec sees an API that does not match the running binary. | **F5** regenerates the unified spec to include the holder/alias surface served by the ledger binary. |
| 1 | **Holders documented only in a stale standalone CRM spec** | `components/crm/api/swagger.json:12` — `"localhost:4003"` (dead port; CRM has no standalone service — folded into ledger on `:3002`) | The only place holders are documented is a CRM spec pinned to a host that no longer exists. The split-service contract survives in the spec after the service was folded into ledger. | **F5** retires/merges the standalone CRM spec into the unified ledger contract. |
| 1 | **Postman merge masks the gap** | `scripts/postman-coll-generation/sync-postman.sh:92` (`convert_component "crm"`), `:110` (`for component in ledger crm`), `merge_all_collections()` at `:104-105`, invoked at `:185` | The postman sync converts **both** ledger and crm specs and slurp-merges them into one collection. The merged Postman artifact therefore *appears* complete (it has both fees and holders), hiding the fact that the canonical ledger OpenAPI spec is missing holders. The divergence is invisible at the Postman layer. | **F5** drives Postman generation from the single harmonized ledger spec; the dual-spec merge is removed once the standalone CRM spec is retired. |
| 1 | **`generate-docs` crm leg is broken** | `scripts/generate-docs.sh:16` — `COMPONENTS=("ledger" "crm")`; `:93` — `swag init -g cmd/app/main.go ...`. `components/crm/cmd/app/main.go` **does not exist** (`ls` → "No such file or directory"). | The doc generator still iterates a `crm` component and runs `swag init -g cmd/app/main.go` against it, but CRM is now a package tree with no `cmd/app/main.go`. The crm leg fails — the CRM spec cannot be regenerated, so the stale `localhost:4003` spec is never refreshed. This is **R12**'s HEAD evidence. | **F5** removes the dead `crm` leg from `COMPONENTS` (CRM is generated as part of the unified ledger spec). |
| 2 | **Version source fork: baked vs runtime** | Baked: `components/ledger/cmd/app/main.go:20` — `// @version v3.7.0` (embedded in the generated OpenAPI). Runtime: `components/ledger/.env:10` — `VERSION=v3.8.0`. | Two version strings disagree. The OpenAPI/swagger version annotation says `v3.7.0`; the running binary reports `v3.8.0`. There is no single source of truth for the API version. | **F5 / D-11 owner** reconciles to one `v4.0.0` source so the baked annotation and runtime version cannot drift again. |

**DRIFT NOTE (HEAD vs macro plan §4):** the spec cites `sync-postman.sh:89-97` as the CRM-spec merge leg; at HEAD `:89-97` is the parallel conversion+wait block (`convert_component "ledger"` at `:89`, `convert_component "crm"` at `:92`, `wait` at `:97`), NOT the merge. The actual merge is `merge_all_collections()` at `:104-105` (loops `for component in ledger crm` at `:110`, jq slurp at `:137`), invoked at `:185`. The masking mechanism (both specs slurp-merged into one collection) is unchanged; only the line citation is corrected.

**Cross-references.** Out-of-repo consumers of these contract artifacts are coordination items in the Scope-(e) register: **X4** (APIDog import, owner: QA/API) and **X5** (docs portal, owner: Docs). F0 only records the divergence; **F5** owns the fix and **F6/D-11** owns the `v4.0.0` version-source reconciliation.

---

## OTEL_LIBRARY_NAME telemetry-attribution strings (D-11 sweep targets)

**Risk:** R27 (High). **Scope:** (e). **Owner of fix:** D-11 owner (F6). **F0 role:** record only.

These are **config strings, not Go imports** — a Go-module import sweep (or a `git grep` over `*.go`) will not surface them. Six carry the `/v3` module path; a `/v3`→`/v4` find-and-replace would catch those six. **Tracer's two values contain no `/v3` substring at all** (`github.com/LerianStudio/tracer`, a wrong, pre-consolidation value), so any sweep keyed on the `/v3` token silently skips them. All eight must be enumerated and swept explicitly — the tracer pair is the trap.

| # | Component | File | Line | Value at HEAD | Sweep class |
|---|-----------|------|------|---------------|-------------|
| 1 | ledger | `components/ledger/.env` | 207 | `github.com/LerianStudio/midaz/v3/components/ledger` | `/v3` → bump to `/v4` |
| 2 | ledger | `components/ledger/.env.example` | 207 | `github.com/LerianStudio/midaz/v3/components/ledger` | `/v3` → bump to `/v4` |
| 3 | reporter-manager | `components/reporter-manager/.env` | 98 | `github.com/LerianStudio/midaz/v3/components/reporter-manager` | `/v3` → bump to `/v4` |
| 4 | reporter-manager | `components/reporter-manager/.env.example` | 98 | `github.com/LerianStudio/midaz/v3/components/reporter-manager` | `/v3` → bump to `/v4` |
| 5 | reporter-worker | `components/reporter-worker/.env` | 80 | `github.com/LerianStudio/midaz/v3/components/reporter-worker` | `/v3` → bump to `/v4` |
| 6 | reporter-worker | `components/reporter-worker/.env.example` | 80 | `github.com/LerianStudio/midaz/v3/components/reporter-worker` | `/v3` → bump to `/v4` |
| 7 | **tracer** | `components/tracer/.env` | 108 | `github.com/LerianStudio/tracer` | **WRONG VALUE — no `/v3` to catch; a `/v3` sweep MISSES this** |
| 8 | **tracer** | `components/tracer/.env.example` | 108 | `github.com/LerianStudio/tracer` | **WRONG VALUE — no `/v3` to catch; a `/v3` sweep MISSES this** |

**D-11 sweep obligation.** Rows 1–6: rewrite `midaz/v3/...` → `midaz/v4/...`. Rows 7–8: tracer's value is independently wrong (`github.com/LerianStudio/tracer` is a pre-consolidation artifact predating the monorepo move) — the D-11 owner must set it to the correct consolidated module path, NOT merely bump a version digit, because there is no `/v3` digit present. Both `.env` and `.env.example` must move in lockstep per component.

**Verification (HEAD, reproduced):**
```
$ grep -rn "OTEL_LIBRARY_NAME" components/*/.env components/*/.env.example
components/ledger/.env:207:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/ledger
components/reporter-manager/.env:98:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/reporter-manager
components/reporter-worker/.env:80:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/reporter-worker
components/tracer/.env:108:OTEL_LIBRARY_NAME=github.com/LerianStudio/tracer
components/ledger/.env.example:207:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/ledger
components/reporter-manager/.env.example:98:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/reporter-manager
components/reporter-worker/.env.example:80:OTEL_LIBRARY_NAME=github.com/LerianStudio/midaz/v3/components/reporter-worker
components/tracer/.env.example:108:OTEL_LIBRARY_NAME=github.com/LerianStudio/tracer
# 8 lines — six /v3 values + tracer's two wrong values, all confirmed.
```

**DRIFT NOTE:** F0.md internally says "all eight values verified" (`:311`) but elsewhere "Gate 4 of F6 checks all seven OTEL values" (`:322`). The authoritative count of distinct file:line touchpoints is **eight** (3 components × 2 files = 6 `/v3` + tracer × 2 = 2). The "seven" conflates the F6 gate's count and is not load-bearing here.

---

## Module / tag decoupling (D-11, R5 Critical)

> **Posture:** F0 RECORDS only. The fix/enforcement is F6-owned (F6 Gate 10; the prerelease dry-run is F6-T18). This section exists so the decoupling does not surface at release time — i.e. so a `/v4` module never ships under a `3.x` tag.

**The decoupling.** The Go module *path* (`go.mod`) and the *release version/tag* are two independent sources of truth:

- The module path bump (`/v3` → `/v4`) is a hand edit to `go.mod:1`.
- The release version is computed by **semantic-release** from the **latest reachable git tag + conventional-commit types**. semantic-release does NOT read `go.mod`. A `/v4` module path and a `3.x` git tag are perfectly consistent from the tooling's point of view — nothing couples them.

| Touchpoint | File:line (HEAD) | Value / fact | v4 obligation |
|---|---|---|---|
| Module path (the thing that bumps) | `go.mod:1` | `module github.com/LerianStudio/midaz/v3` | D-11 edits `/v3`→`/v4`. Hand edit; invisible to semantic-release. |
| Tag format | `.releaserc.yml` (no `tagFormat` key) | `tagFormat` ABSENT → semantic-release default `v${version}` | None in F0. The default already produces `vX.Y.Z` tags; no config change needed for the `v4.0.0` tag shape. |
| Release-rules → bump level | `.releaserc.yml:8-28` | `feat`/`perf`/`build`/`refactor` → **minor**; `chore`/`ci`/`test`/`fix`/`docs` → **patch**; only `breaking: true` → **major** | A `/v4` migration commit typed `feat`/`refactor` ships as a **minor** bump on the current 3.x line **unless** it carries a `BREAKING CHANGE`. |
| BREAKING-CHANGE note keywords | `.releaserc.yml:4-7` | `parserOpts.noteKeywords: ["BREAKING CHANGE", "BREAKING CHANGES"]` | The major bump is triggered by a `BREAKING CHANGE:` / `BREAKING CHANGES:` footer **or** a `type!:` (`!`) marker on the release-trigger commit. |
| Branch / prerelease channels | `.releaserc.yml:54-59` | `main` (stable); `develop` → prerelease `beta`; `release-candidate` → prerelease `rc` | The current line is a `beta` prerelease (see latest tag below); F6-T18 handles the prerelease case via a semantic-release dry-run. |

**Required mitigation (carried to F6 Gate 10).** The commit that triggers the `v4.0.0` release MUST carry a `BREAKING CHANGE:` footer **or** the `!` bump marker (e.g. `feat!:`). Without it, a `feat`/`refactor`-typed `/v4` migration commit resolves to a **minor** release and the `/v4` module would ship under a `3.x` (next-minor) tag — exactly the failure this row exists to prevent. F6 must also verify the latest reachable tag is still on the `3.x` line before the bump (so the jump to `4.0.0` is a deliberate major, not an accident of an already-`4.x` lineage).

**Latest tag recorded for F6-T18.**

- `git describe --tags --abbrev=0` (latest tag **reachable from HEAD**, ancestry-walk) → **`v3.8.0-beta.8`** (commit `766b555d`, an ancestor of the consolidation HEAD).
- Highest tag in the repo by version sort → `v3.8.0-beta.9` (commit `1d19f9ff`). **NOT an ancestor of the consolidation HEAD** — it lives on the `develop`/`main` lineage, off `feat/monorepo-consolidation`. So it is not what semantic-release sees as "last release" when run from this branch tip.
- Both candidates are `-beta` **prereleases**, so F6-T18's semantic-release dry-run (which must account for a prerelease as the base, not a stable tag) is unaffected by which one is taken as the floor.

**DRIFT NOTE (vs spec/prompt).** The F0-T12 prompt expected `git describe --tags --abbrev=0` = `v3.8.0-beta.9`. At the F0 tip it returns **`v3.8.0-beta.8`**, because `git describe` walks ancestry and `v3.8.0-beta.9` is off-branch (not an ancestor of HEAD). The expectation conflated "highest tag in the repo" (`-beta.9`, by `git tag --sort=-v:refname`) with "latest tag reachable from HEAD" (`-beta.8`, what `git describe` reports). The decoupling conclusion is unchanged — both are 3.x prereleases — but the recorded latest-reachable tag for F6-T18 is `v3.8.0-beta.8`.

---

## Branch & epoch (Gate 5 / Scope (d))

**Branch decision (Q1-RESOLVED).** All v4 work continues on **`feat/monorepo-consolidation`**. There is **no `feat/v4` branch** — verified at the F0 tip (`git for-each-ref refs/heads/ | grep -i v4` → none). The monorepo-consolidation history (P0–P9) and the v4 history share this single branch.

**Gate-zero epoch SHA (the v4 epoch marker).**

```
46706c9eaff0a85fab80f693b1d4f82de56e239b
```

This is the F0 tip after the F0-T02..T05 fixes landed and the green baseline was captured (F0-T06). It is the **v4 epoch marker** — the base every later bisect/diff range uses.

**Consequence.** Because consolidation and v4 share `feat/monorepo-consolidation`, there is no clean branch boundary separating "consolidation work" from "v4 work" — the **epoch SHA is the v4 diff base**. Every later phase computes "what did v4 change?" as the diff from `46706c9eaff0a85fab80f693b1d4f82de56e239b` to its own tip; a phase that needs to exclude consolidation churn must range from this SHA, not from `main` or a non-existent `feat/v4` fork point.

---

## Baseline record (Gate 1)

Captured at the gate-zero epoch SHA `46706c9eaff0a85fab80f693b1d4f82de56e239b` (FINAL, after the F0-T02..T05 fixes), with a dedicated `GOCACHE=/tmp/midaz-gocache-f0` (an external cache purge had aborted the first v5 run; the dedicated cache makes the chain immune to external purges). `make ci` is the single-verdict superset reproducing all four legs; `make test-unit` + `make test-integration` are the macro-Gate-1 mandatory floor.

| Command | Exit | Result |
|---------|------|--------|
| `make test-unit` | 0 | **15,877 tests**, 6 skipped, 104.9s |
| `make test-integration` | 0 | **978 tests**, 80 skipped, 1082.8s (33 packages via tag-driven discovery) |
| `make test-property` | 0 | **70 tests**, 7 skipped |
| `make test-reporter-chaos` | 0 | **39 tests**, 39 skipped (CHAOS=1 opt-in gating by design — compile+invocation verified, run-bodies opt-in) |
| `make ci` | 0 | single exit code; all four legs reproduced (15,877 / 978 / 70 / 39) — the Gate-1 reproducibility second run |

**`make ci` matrix composition (settled at F0-T05).** unit (untagged `-race`) → `test-integration` (`-tags=integration -p 1`, discovery now reaches `./tests`) → `test-property` → `test-reporter-chaos` (compile + invocation, run-bodies gated `CHAOS=1`) → `test-bdd` (`-tags e2e`, live tracer — opt-in). The env-gated `test-chaos-system` (live docker-compose stack) is an **opt-in** leg, not default CI.

**Reproducibility (Gate 1).** The `make ci` run reproduced the four leg counts exactly (15,877 / 978 / 70 / 39) — this is the recorded second run a later phase diffs against to answer "did I break the floor?".

**Environment notes.** (1) Baseline chains use a dedicated `GOCACHE` for external-cache-purge immunity. (2) Known Docker-inspect flake class under sustained load: organization `CountIsolation` failed once in a baseline draft run and passed in isolation — **environment, not defect**. A single such flake on re-run is not a baseline regression.

---

## Harness debt (deferred to F5)

These are real but out-of-F0-scope harness defects, recorded so F5 picks them up explicitly rather than rediscovering them. F0's fixes (F0-T02..T05) deliberately did NOT touch these, because the Q16-conform fix for (b) changes the baseline counts and is therefore F5 scope.

| # | Debt | Evidence | Why it is debt | F5 disposition |
|---|------|----------|----------------|----------------|
| (a) | **25 untagged `*_integration_test.go` files** in `components/{ledger,tracer}` | mock-based (sqlmock/miniredis), no `//go:build integration` line | They run inside the **unit floor**, not the integration target — the *name* says integration, the *tag* (absence) says unit. The name lies; the tag is truth. | F5 decides whether to tag them (moves them to the integration target, requires real deps) or rename them; either way the baseline counts shift. |
| (b) | **31 files tagged `//go:build unit`** | grep `^//go:build unit` | **Invisible to every target** — `test-unit` runs the *untagged* packages, so a `unit`-tagged file is run by nothing. Zombie tests. | Q16-conform fix is **REMOVING the `unit` tag** (so they rejoin the untagged floor) — this changes the baseline count, hence F5, not F0. |
| (c) | **Internal tags `itestkit` (7 files) / `testhooks` (2 files)** | grep `^//go:build itestkit` / `testhooks` | Exist as build-tag selectors but are **not bound to any make target** — reachable only by an explicit ad-hoc `-tags` invocation. | F5 either binds them to a target or documents them as intentional internal-only selectors. |

**Note on the reporter chaos TestMain (incidental, not a tagging defect).** `tests/reporter/chaos` compiles cleanly under `-tags=chaos` and now has an invoking target (`test-reporter-chaos`), but its `TestMain` references a stale pre-consolidation build path (`components/manager/cmd/app`; now `reporter-manager`), so a full `CHAOS=1` *run* would fail at infra setup. This is a reporter-suite harness bug for F5/F6, separate from the build-tag normalization F0 delivered.

---

## Design-fact refinement (flagged for F3)

The PLAN.md key fact "**balances commit synchronously at HTTP time**" needs refinement before F3 relies on it:

- The **Redis hot state** is the synchronous authority — it commits at HTTP time, in both sync and async transaction modes.
- The **PostgreSQL cold row** persists **asynchronously** via `BalanceSyncWorker` (driven off a Redis sorted set), in **both** sync and async modes.
- The F3 anchor (`transaction_create.go:1228`, post-`ProcessBalanceOperations`) **remains valid**.
- **Test consequence:** any F3 task that asserts **PG** balance state *immediately* after the HTTP response will fail — the cold row has not been written yet. Tests must **drain the balance-sync schedule first**. A helper `drainBalanceSync` now exists in the `http/in` suite for exactly this.

---

## Scope (e) — External coordination register

Mirrors `docs/v4/PLAN.md` §13. Everything out-of-repo that a v4 phase triggers; the phase column is the one that *triggers* the coordination. F0 registers all of these with owners; F5/F6 sequence the lockstep before the auth-enabled cut / production tag.

| # | External artifact | Owner | Triggering phase | What must happen out-of-repo |
|---|-------------------|-------|------------------|------------------------------|
| X1 | auth-server / tenant-manager RBAC policies keyed on `plugin-crm` (and the parallel `plugin-fees`/`midaz`/`routing` namespaces, see Scope (a) §1) | **Fred + plugin-auth team** | F2 triggers (in-code flip); F6 gates release | Migrate every tenant's `plugin-crm:*` grant to the new namespace at v4 finalization (Q3 — Fred-owned). The in-code flip merges in F2; NO auth-enabled environment deploys v4 until this migration confirms. The single most dangerous coordination item — a **release** gate, not a merge gate (`RBAC-NAMESPACES.md:8-12`). |
| X2 | External Helm chart `midaz` | Ops | F6 triggers (release fan-out) | Add tracer/reporter-manager/reporter-worker value blocks; drop crm/plugin-fees keys (`build.yml:44`). Chart must accept the 4-image fan-out before any real v4 tag. Carries the R6 production-fan-out deploy gate. |
| X3 | gitops repo `LerianStudio/midaz-firmino-gitops` | Ops | F6 triggers (release fan-out) | Update `yaml_key_mappings` to the distinct `.tag`-suffixed schema (`build.yml:68`) for the 4 images; armed fallback gates the production tag. Carries the R6 production-fan-out deploy gate. |
| X4 | APIDog e2e scenarios | QA / API owners | F2/F5 trigger (renamed routes); F6 gate | Update scenarios for the renamed `/instruments` routes, the removed `/aliases` surface, the composition endpoint, and the tracer reservation API; `MIDAZ_APIDOG_TEST_SCENARIO_ID` points at v4 scenarios. Consumes the contract divergence in Scope (a) §4. |
| X5 | Docs portal (docs.lerian.studio) | Docs owners | F5 triggers (contract harmonization) | Publish the v4 API docs and the D-10 migration guide / release notes. No in-repo publication step exists; F5 lists it, does not execute it. Consumes the contract divergence in Scope (a) §4. |
| X6 | External Go importers — `midaz-sdk-golang`, `midaz-console`, partner code | SDK / console owners | F6 triggers (module bump) | Migrate imports `github.com/LerianStudio/midaz/v3/pkg/... → /v4` and the `mmodel.Alias → mmodel.Instrument` identifier rename. Unverifiable in-repo (see Scope (a) §3 negative grep); F6 ships the Go-importer migration table and treats external repos as their owners' problem (Q14). |

Additional coordination item recorded outside §13: **out-of-repo callers of tracer `POST /v1/validations`** (owner: tracer-API consumers) — surfaced by Scope (a) §2; if any external system calls the validate endpoint, F3's reserve/confirm/release redesign must coordinate the API change with them.
