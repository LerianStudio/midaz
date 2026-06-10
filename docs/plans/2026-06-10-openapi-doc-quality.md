# OpenAPI Documentation Quality Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Resolve all 52 findings from the 2026-06-10 OpenAPI audit (`docs/openapi/AUDIT-2026-06-10.md`) so that the ledger, tracer, and reporter specs are reproducible, consistent, and at uniform quality parity.

**Architecture:** Swaggo/swag annotations in Go source generate OpenAPI 2.0 specs per component (`components/<c>/api/*`), published into a shared Postman hub (`postman/specs/<c>/*`) by `postman/generator/generate-docs.sh`. Fixes are almost entirely annotation/comment edits plus one generator bug, two new CI guardrails, and (Phase 5) a handful of new wire DTOs. The work is ordered so the foundation — a generator that reproduces the committed specs — lands first; everything after is verified by regenerating and diffing.

**Tech Stack:** Go 1.26 (single root `go.mod`, module `github.com/LerianStudio/midaz/v4`), swaggo/swag `v1.16.6`, openapi-generator-cli `v7.10.0` (Docker), Fiber v2, `jq` for spec assertions, GitHub Actions (`pr-validation.yml`).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | `make generate-docs` runs clean on a fresh tree, reproduces all three specs, general-info at parity, two CI guardrails green | 1.1, 1.2, 1.3 | Detailed |
| 2 | Specs carry no leaked dotted schema names, consistent Title-Case-plural tags with group descriptions, lowercase router methods, and no wrong/stale runtime-visible text | 2.1, 2.2, 2.3 | Epic-level |
| 3 | Ledger spec documents authentication correctly: authenticated operations require `BearerAuth`, no dangling scheme, no duplicated `Authorization` param | 3.1 | Epic-level |
| 4 | Every endpoint across all three components declares the full, correct error-code set with descriptive response strings; folded-in CRM+fees surface matches the native bar | 4.1, 4.2, 4.3 | Epic-level |
| 5 | Every wire-facing schema is fully described (no raw domain entities, no persistence structs, no unexported types, no opaque maps on the public surface) | 5.1, 5.2, 5.3, 5.4 | Epic-level |

**Finding-to-epic coverage map** (all 52):
- **Phase 1:** H5, H6 → 1.1 · M4, M5, M7, L11, L14, L15, L16 → 1.2 · L17 (+ H6 CI half) → 1.3
- **Phase 2:** M13, L12 → 2.1 · M6, M8, L1 → 2.2 · M2, M11, L2, L3, L5, L10, L13 → 2.3
- **Phase 3:** C1 → 3.1
- **Phase 4:** M1, L4 → 4.1 · H3, H4, M14, M9 → 4.2 · M3 → 4.3
- **Phase 5:** H1 → 5.1 · H2, L7, L8 → 5.2 · M10, M12, L9 → 5.3 · L6 → 5.4

---

## Phase 1 — Reproducible pipeline + general-info parity

**Milestone:** A clean `git clone` → `make generate-docs` reproduces the committed `components/<c>/api/*` and `postman/specs/<c>/*` for all three components with no diff; the three general-info headers agree on every shared field; CI fails if either invariant breaks.

### Epic 1.1: Repair the doc-generation pipeline

**Goal:** `make generate-docs` succeeds end-to-end against the real `components/reporter` binary, and the published hub no longer carries the stale `reporter-manager` directory.
**Scope:** `postman/generator/generate-docs.sh`, `postman/specs/`, `postman/README.md`, `postman/generator/sync-postman.sh` (read-only check).
**Dependencies:** none
**Done when:** `make generate-docs` exits 0 on a fresh tree; `postman/specs/reporter/` exists and matches `components/reporter/api/`; `postman/specs/reporter-manager/` is gone; `git diff components/reporter/api` is empty after regeneration.
**Status:** Pending

#### Task 1.1.1: Repoint the generator to `reporter` and retire the stale specs directory

- [ ] Done

**Context:** `postman/generator/generate-docs.sh:20` declares `COMPONENTS=("ledger" "tracer" "reporter-manager")`. `components/reporter-manager` is a Dockerfile-only CI anchor with no `cmd/app/main.go`, so the `swag init -g cmd/app/main.go` call (`generate-docs.sh:97`) cannot run against it — `make generate-docs` fails at the reporter step on a clean checkout (H5). The real consolidated binary is `components/reporter` (commit `bfa9b4b69`). `publish_specs` (`generate-docs.sh:145-160`) uses the component name verbatim as the destination dir, so the published hub copy currently lives at the stale `postman/specs/reporter-manager/` and has already drifted from source — `postman/specs/reporter-manager/swagger.json` lacks the `Partial` status enum present in live `components/reporter/api/swagger.json` (H6).

**Implementation vision:** Change `COMPONENTS` element `"reporter-manager"` → `"reporter"` (single edit at `generate-docs.sh:20`). Grep `postman/README.md` and `postman/generator/*.sh` for any other `reporter-manager` literal and update to `reporter` (the README documents the dead name). Run `make generate-docs` once; it will create `postman/specs/reporter/` from fresh swag output. Then `git rm -r postman/specs/reporter-manager` to delete the stale, drifted copy. Do NOT hand-edit any generated artifact — let swag produce them. If regeneration changes `components/reporter/api/*` (it shouldn't if committed artifacts are current), inspect the diff: a non-empty diff means the committed specs were stale, and the regenerated version is the new truth — commit it.

**Files:**
- Modify: `postman/generator/generate-docs.sh:20`
- Modify: `postman/README.md` (any `reporter-manager` references)
- Delete: `postman/specs/reporter-manager/` (swagger.json, swagger.yaml, openapi.yaml)
- Create (via generator): `postman/specs/reporter/{swagger.json,swagger.yaml,openapi.yaml}`

**Verification:** `go install github.com/swaggo/swag/cmd/swag@v1.16.6 && make generate-docs` exits 0; `ls postman/specs/reporter/` lists three files; `ls postman/specs/reporter-manager 2>&1` reports no such directory; `git diff --stat components/reporter/api` is empty (or, if not, the diff is reviewed and is the corrected truth); `grep -rn reporter-manager postman/` returns only the two Dockerfile-anchor mentions outside `postman/` (none inside `postman/`).

**Done when:** the generator targets `reporter`, the hub has a single up-to-date `postman/specs/reporter/`, and a clean regeneration is byte-reproducible.

---

### Epic 1.2: Bring the three general-info headers to parity

**Goal:** The `info` block of all three generated specs agrees on contact, license, termsOfService, schemes, version format, title branding, and security-description phrasing; tracer's description names its bounded contexts; reporter's description states the REST surface is api/all-mode only.
**Scope:** `components/ledger/cmd/app/main.go`, `components/tracer/cmd/app/main.go`, `components/reporter/cmd/app/main.go`.
**Dependencies:** none (parallelizable with 1.1; both feed the 1.3 assertion)
**Done when:** the three regenerated `swagger.json` files carry identical `info.contact`, `info.license`, `info.termsOfService`, and `info.schemes`; `info.version` is `4.0.0` everywhere; every `info.title` starts with `Midaz `; the two Bearer services share one security-scheme description string.

**Decisions locked for this epic** (applied uniformly):
- `@version` → `4.0.0` (drop ledger's `v` prefix — M7).
- `@title` → `Midaz Ledger API` / `Midaz Tracer API` / `Midaz Reporter API` (add prefix to tracer+reporter — L15).
- `@termsOfService` → replace the swag scaffold `http://swagger.io/terms/` with `https://www.elastic.co/licensing/elastic-license` in all three (L16). _(If Lerian has a dedicated terms URL, substitute it during execution.)_
- `@schemes` → `http https` in all three (ledger gains `https`, reporter gains the line — M5).
- `@contact.name`/`@contact.url` → `Discord community` / `https://discord.gg/DnhqKwkGv3` in all three (add to tracer+reporter — M4).
- `@license.name`/`@license.url` → `Elastic License 2.0` / `https://www.elastic.co/licensing/elastic-license` in all three (add to tracer+reporter — M4).
- Bearer security-scheme `@description` (ledger + reporter only) → canonical string: `Bearer token authentication. Format: 'Bearer {access_token}'. Only required when auth plugin is enabled.` (reporter adopts ledger's wording — L14). **Tracer's `ApiKeyAuth`/`X-API-Key` block is correct and stays untouched** (audit non-finding).

#### Task 1.2.1: Normalize the ledger general-info header

- [ ] Done

**Context:** `components/ledger/cmd/app/main.go:19-33` is the most complete header but carries two defects: `@version v4.0.0` (`:20`, the `v` prefix belongs on git tags, not `info.version` — M7) and the swag scaffold `@termsOfService http://swagger.io/terms/` (`:22` — L16). It already has contact, license, and the canonical Bearer description.

**Implementation vision:** Two edits only — `:20` `v4.0.0` → `4.0.0`; `:22` URL → `https://www.elastic.co/licensing/elastic-license`. Add `https` to `@schemes` (`:29` `http` → `http https`). Leave everything else (`@title`, contact, license, security block) as-is.

**Files:**
- Modify: `components/ledger/cmd/app/main.go:20,22,29`

**Verification:** `cd components/ledger && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml` succeeds; `jq '.info.version, .info.termsOfService, .schemes' components/ledger/api/swagger.json` shows `"4.0.0"`, the Elastic URL, and `["http","https"]`.

**Done when:** ledger's `info.version` is `4.0.0`, termsOfService points at the real license, and schemes is `http https`.

#### Task 1.2.2: Bring the tracer header to parity and enrich its description

- [ ] Done

**Context:** `components/tracer/cmd/app/main.go:25-35` lacks `@contact` and `@license` entirely (generated spec emits `contact:{}`, `license:null` on a source-available product — M4), uses a 7-word `@description` (`:27` "Transaction validation service with rules and limits" — M4), no `Midaz ` title prefix (`:25` — L15), and the swag scaffold termsOfService (`:28` — L16). Its `ApiKeyAuth`/`X-API-Key` security block (`:32-35`) is correct — do not touch.

**Implementation vision:** Edit `@title` (`:25`) → `Midaz Tracer API`. Replace `@termsOfService` (`:28`) URL with the Elastic license URL. Insert `@contact.name`/`@contact.url` and `@license.name`/`@license.url` lines (mirror ledger `main.go:23-26` exactly) in the conventional position (after `@termsOfService`, before `@host`). Expand `@description` (`:27`) to a single line naming the bounded contexts. Use this exact description (justified — exact artifact, contract-shaping copy): `Midaz Tracer API — pre-flight transaction validation. Provides CEL-based rule evaluation, spending limits, two-phase reservations (hold / confirm / release), validation decisions, and a hash-chained audit trail.` Leave `@schemes` (`:30`) as `http https` (already correct). Leave the security block untouched.

**Files:**
- Modify: `components/tracer/cmd/app/main.go:25,27,28` and insert contact/license lines after `:28`

**Verification:** `cd components/tracer && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml` succeeds; `jq '.info.title, .info.contact, .info.license, .info.termsOfService' components/tracer/api/swagger.json` shows `Midaz Tracer API`, the Discord contact, Elastic License 2.0, and the Elastic URL.

**Done when:** tracer's `info` block matches ledger's on contact/license/termsOfService and carries the enriched, prefixed title and description.

#### Task 1.2.3: Bring the reporter header to parity, align Bearer description, and note worker-mode

- [ ] Done

**Context:** `components/reporter/cmd/app/main.go:20-29` lacks `@contact`, `@license`, and `@schemes` (→ `schemes:null` — M4, M5), no `Midaz ` prefix (`:20` — L15), swag scaffold termsOfService (`:23` — L16), and a Bearer security-description that diverges in wording from ledger's (`:29` — L14). The header explains RUN_MODE at the binary level but doesn't warn that REST endpoints serve only in api/all mode — a consumer pointing at the worker port (:4006) gets connection failures with no spec-level signal (L11).

**Implementation vision:** `@title` (`:20`) → `Midaz Reporter API`. Replace termsOfService (`:23`) URL. Insert contact/license lines (mirror ledger) and a `@schemes http https` line in the conventional position. Append one clause to `@description` (`:22`): `All REST endpoints documented here serve only when RUN_MODE=api or all (port :4005); the worker (port :4006) exposes health/readyz only.` (L11). Change the Bearer `@description` (`:29`) to the canonical string from the epic decisions (matching ledger verbatim — L14).

**Files:**
- Modify: `components/reporter/cmd/app/main.go:20,22,23,29` and insert contact/license/schemes lines

**Verification:** `cd components/reporter && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml` succeeds; `jq '.info.title, .info.contact, .info.license, .info.termsOfService, .schemes' components/reporter/api/swagger.json` shows the prefixed title, Discord contact, Elastic License 2.0, Elastic URL, and `["http","https"]`; `jq -r '.info.description' components/reporter/api/swagger.json` contains the worker-mode clause.

**Done when:** reporter's `info` block matches the other two on all shared fields, states the worker-mode constraint, and shares ledger's Bearer description string.

---

### Epic 1.3: Lock parity and drift with CI guardrails

**Goal:** CI fails when (a) the committed specs are not reproducible from source, or (b) the three specs diverge on the shared general-info fields — closing the root cause behind H6 and L17.
**Scope:** new `make` target (in `mk/docs.mk`), a small assertion script under `postman/generator/`, `.github/workflows/pr-validation.yml`, `postman/README.md` (parity checklist).
**Dependencies:** 1.1 and 1.2 (the invariants must already hold before CI enforces them)
**Done when:** a new `make check-docs` target passes locally on the parity-fixed tree and fails if a header field is desynced or a spec is hand-edited; the check runs in `pr-validation.yml`.

#### Task 1.3.1: Add a `check-docs` target asserting reproducibility and general-info parity

- [ ] Done

**Context:** No CI workflow currently runs swag or `generate-docs` (`.github/workflows/` has build, pr-validation, security, release — none reference swag). So both guardrails are net-new. `make generate-docs` (`Makefile:621` → `generate-docs.sh`) is the reproduction command; the three committed `components/<c>/api/swagger.json` are the parity subjects.

**Implementation vision:** Add a script `postman/generator/check-docs.sh` that does two things and exits non-zero on either failure. (1) **Drift check:** run `generate-docs.sh` into a throwaway state and assert `git diff --exit-code -- components/*/api postman/specs` is clean — i.e. regeneration reproduces committed artifacts. (2) **Parity check:** with `jq`, extract `.info.contact`, `.info.license`, `.info.termsOfService`, and `.schemes` from the three `components/<c>/api/swagger.json` and assert all three are byte-identical for each field; assert `.info.version` equals `4.0.0` in all three and each `.info.title` matches `^Midaz `. Emit a clear diff on mismatch (which component, which field). Add a `check-docs` phony target in `mk/docs.mk` that invokes it, and surface it in `Makefile` help (`Makefile:132` block). Keep the drift check guarded so it skips gracefully if `swag`/Docker are unavailable locally but always runs in CI (parameterize with an env flag, e.g. `CHECK_DOCS_REGEN=1`).

**Files:**
- Create: `postman/generator/check-docs.sh`
- Modify: `mk/docs.mk` (add `check-docs` target)
- Modify: `Makefile:132` help block (document `make check-docs`)
- Modify: `postman/README.md` (add a "general-info parity fields" checklist documenting the four synced fields + version/title rules)

**Verification:** `make check-docs` exits 0 on the current tree; manually desyncing one field (e.g. revert reporter's `@license` and regenerate) makes `make check-docs` exit non-zero with a message naming `reporter` and `info.license`; restoring it returns to green.

**Done when:** `make check-docs` enforces both reproducibility and field parity locally.

#### Task 1.3.2: Wire `check-docs` into PR validation

- [ ] Done

**Context:** `.github/workflows/pr-validation.yml` is the PR gate. The drift half of the check needs `swag` (`v1.16.6`) and Docker (for the openapi-yaml step via `openapitools/openapi-generator-cli:v7.10.0`, `generate-docs.sh:124-129`); GitHub-hosted runners provide Docker.

**Implementation vision:** Add a job (or step in an existing job) to `pr-validation.yml` that installs Go + `swag@v1.16.6`, then runs `CHECK_DOCS_REGEN=1 make check-docs`. Match the existing job's setup conventions (Go version pin, caching) already present in the workflow — read the file and mirror its `actions/setup-go` block rather than inventing one. Keep it a required check.

**Files:**
- Modify: `.github/workflows/pr-validation.yml`

**Verification:** the workflow YAML is valid (`yamllint` or a dry parse); the new step appears in the PR checks on the next push; intentionally desyncing a header in a scratch commit turns the check red.

**Done when:** PRs that break spec reproducibility or header parity fail CI.

---

## Phase 2 — Mechanical wire sweeps (no decisions, low blast radius)

**Milestone:** The three regenerated specs carry clean schema names (no `github_com_LerianStudio_...` / package-dotted definitions), a single Title-Case-plural tag taxonomy with `@tag` group descriptions, lowercase `@Router` methods, and no factually-wrong or stale runtime-visible text.

### Epic 2.1: `@name` sweep to eliminate leaked schema names

**Goal:** Every wire-facing struct that already carries `swagger:model`/field annotations but lacks `@name` gets a clean `// @name <Type>`, removing the dotted names from all three specs. (Structs with *zero* annotations are out of scope here — they are annotated wholesale in Phase 5.)
**Scope:** `pkg/mmodel/*` (esp. `balance.go`, the `*Settings` family), `components/ledger/pkg/feeshared/model/*` (the already-annotated `fees.go` set), `pkg/reporter/model/*` and `pkg/reporter/mongodb/*` (`report.Report`, `template.Template`, `deadline.Deadline`).
**Dependencies:** Phase 1 (regeneration must be clean to verify the name reduction)
**Done when:** `grep -c 'github_com_LerianStudio\|"[a-z_]*\.[A-Z]' ` over each `components/<c>/api/swagger.json` `definitions` block drops to zero for the annotated set; `make check-docs` stays green. M13, L12.
**Status:** Pending

### Epic 2.2: Normalize tag taxonomy and router-method case

**Goal:** All `@Tags` are Title-Case-plural and consistent across components; `@tag.name`/`@tag.description` group blocks exist in every component's general-info; all `@Router` HTTP methods are lowercase.
**Scope:** all handler files in `components/{ledger,tracer,reporter}/internal/.../http/in/`, plus the three `cmd/app/main.go` for the `@tag` group blocks.
**Dependencies:** Phase 1
**Done when:** tracer's 7 lowercase tags, reporter's `Data source`, ledger's singular route tags (`Operation Route`/`Transaction Route`), and the CamelCase fee tags (`BillingPackages`/`BillingCalculate`) are all Title-Case-plural; `@tag.*` descriptions render in all three Swagger UIs (tracer's two-phase reservations lifecycle is described); no capitalized `[Get]`/`[Put]`/`[Post]` remain in `@Router`. M6, M8, L1.
**Status:** Pending

### Epic 2.3: Fix wrong, stale, and malformed runtime-visible text

**Goal:** Every factually-incorrect or stale doc string that misleads a spec reader is corrected.
**Scope:** ledger `transaction_state_handlers.go`, `balance.go`, `operation.go`; reporter `mongodb/report/report.go`, `data-source.go`, `data-source-information.go`; ledger `mongodb/fees/pack/package.go`; reporter `pkg/reporter/model/data-source-information.go`; tracer `transaction_validation_handler.go`, `rule_validation.go`; ledger `instrument.go` (comment/log side of the alias→instrument drift).
**Dependencies:** Phase 1
**Done when:**
- Commit/Cancel `@Failure 400` strings no longer say "reverted" (M2);
- reporter status example is `Processing` not `processing` (M11);
- malformed `example\t"..."` query-param tokens render real examples or fold into descriptions (L2);
- single-quote array-literal examples use swag's array form and the `ac0002` typo is fixed (L3);
- reporter "plugin" wording → "reporter" (L10);
- stale `TRC-####` references in tracer comments updated to the numeric registry (L13);
- ledger alias→instrument comment/log drift reconciled (L5 — **decision gate:** the `app.request.alias_id` span attribute is an observability surface; renaming it is logged as a separate, explicitly-acknowledged change, not folded in silently).
M2, M11, L2, L3, L5, L10, L13.
**Status:** Pending

---

## Phase 3 — Correct the ledger authentication contract (decision gate)

**Milestone:** The ledger spec documents how to authenticate: authenticated operations require `BearerAuth`, Swagger UI's Authorize button applies to real operations, and generated SDKs attach the token. No dangling security definition, no duplicated per-endpoint `Authorization` param.

### Epic 3.1: Apply `@Security BearerAuth` across authenticated ledger operations

**Goal:** Resolve C1 — the declared `BearerAuth` scheme (`ledger/cmd/app/main.go:30-33`) is referenced by every authenticated operation, and the ad-hoc optional `@Param Authorization header string false` lines are removed.
**Scope:** every annotated handler in `components/ledger/internal/adapters/http/in/` (native + folded-in CRM/fees), `components/ledger/cmd/app/main.go`.
**Dependencies:** Phase 1 (header normalized). Touches the same handler files as Phase 4 — sequence 3 before 4 so the auth annotation lands before the error-code lift.
**Done when:** authenticated operations carry `@Security BearerAuth`; the `Authorization` `@Param` is gone from those operations; truly-public endpoints (health/version, if any) carry none; the regenerated spec shows `security` on operations and no orphan param; `make check-docs` green.
**Status:** Pending

**⛔ DECISION GATE — resolve before elaborating tasks:** the audit recommends model (a): auth is real (`ProtectedRouteChain`), so `@Security BearerAuth` is correct, and the optional `@Param Authorization` is removed. Model (b) — keep the param approach and instead *delete* the unused `securityDefinition` — is only correct if auth is meant to read as fully optional in the contract. **Recommended: (a).** Confirm with the owner before writing tasks, because the choice flips whether the security block stays and the params go, or vice-versa. This also sets the pattern tracer/reporter already follow.

---

## Phase 4 — Annotation completeness lift

**Milestone:** Every endpoint across all three components declares the correct, complete error-code set with descriptive `@Success`/`@Failure` strings; the folded-in CRM+fees surface is indistinguishable in quality from native onboarding/transaction.

### Epic 4.1: Lift the ledger native thin tier to the reference bar

**Goal:** The thin-tier native handlers reach the `organization.go` quality bar.
**Scope:** `components/ledger/internal/adapters/http/in/{transaction.go,transaction_state_handlers.go,balance.go,operation.go,assetrate.go}`.
**Dependencies:** Phase 3 (same files)
**Done when:** the four transaction-create modes carry mode-distinct `@Description` (what inflow/outflow/annotation actually mean, not the shared "Create a Transaction with the input payload"); `@Success` lines carry description strings; path params say "in UUID format"; transaction-create declares `404` and uses a qualified response type. M1, L4.
**Status:** Pending

### Epic 4.2: Lift the folded-in CRM + fees surface to the native bar

**Goal:** All 25 folded-in endpoints carry response-description strings and the full authenticated error-code vocabulary; the related-party sub-resource is coherent; list params are behavioral.
**Scope:** `components/ledger/internal/adapters/http/in/{holder.go,instrument.go,*composition*,fees_package_handler.go,billing_package_handler.go,*billing_calculate*}`.
**Dependencies:** Phase 3 (same files), Epic 4.1 (establishes the lifted pattern)
**Done when:** every `@Success`/`@Failure` on the folded-in surface has a description string (H3); CRM endpoints declare `401/403` and `409` on uniqueness-bearing creates, `422` where business validation fires (H4); related-party create/list is either annotated (if HTTP-exposed) or DELETE carries an `@Description` explaining the asymmetry (M9); terse one-word list params become behavioral with enum hints (M14). H3, H4, M9, M14.
**Status:** Pending

### Epic 4.3: Add business-error and not-found codes to tracer and reporter writes

**Goal:** Write endpoints routing through `ValidateBusinessError` document `422`; tracer Confirm/Release document `404`.
**Scope:** tracer `validation_handler.go`, `rule_handler.go`, `reservation_handler.go`; reporter `report.go`, `template.go`, `deadline.go`, `download-report.go`.
**Dependencies:** Phase 1
**Done when:** every endpoint whose handler calls `ValidateBusinessError` declares `@Failure 422`; tracer Confirm/Release single-id endpoints declare `@Failure 404`; parse-failure 400 and business 422 are no longer conflated. M3.
**Status:** Pending

---

## Phase 5 — Schema annotation deepening

**Milestone:** No raw domain entity, persistence struct, unexported type, or opaque map appears unannotated on any public surface; a reader of any of the three specs gets fully-described models with realistic examples.

### Epic 5.1: Annotate the tracer schema layer

**Goal:** Tracer's wire schemas are fully described — either via response DTOs in `http/in` or via `@name`+field annotations on the domain entities.
**Scope:** `components/tracer/pkg/model/*` (`rule.go`, `limit.go`, scope/audit/validation types), possibly new DTOs in `components/tracer/internal/adapters/http/in/`.
**Dependencies:** Phase 2 (tag/name conventions established), Phase 4.3 (same component, error codes first)
**Done when:** `Rule`, `Limit`, `Scope`, `AuditEvent`, `TransactionValidation`, `ValidationResponse` carry `@name`, field `description`/`example`, and enum hints; `Rule.Expression` has a realistic CEL example; the `Decision`/`RuleStatus`/`LimitStatus`/`LimitType` enums are documented; no raw domain entity is returned untyped. **Decision gate at elaboration:** response-DTO indirection vs. annotating the domain entity in place — choose per the project's leakage tolerance. H1.
**Status:** Pending

### Epic 5.2: Annotate the fee billing schema family and fix fee schema leaks

**Goal:** The billing structs reach the `fees.go` bar; the fee-estimate type leak and missing enum/examples are resolved.
**Scope:** `components/ledger/pkg/feeshared/model/{billing_package.go,fees.go}`, `components/ledger/internal/adapters/mongodb/fees/pack/package.go`.
**Dependencies:** Phase 2
**Done when:** `PricingTier`, `DiscountTier`, `AccountTarget`, `EventFilter`, `BillingPackage`, `BillingPackageUpdate` carry `swagger:model`+`@name`+`@Description`+field examples (H2); bson-on-wire is resolved (thin DTO if the struct is dual-purpose); `Calculation.Type` carries `enums:"percentage,flat"`+example (L7); `FeeEstimate`/`FeeCalculate` mirror a wire-specific transaction projection instead of embedding `transaction.Transaction`, with realistic UUID-v7 examples (L8). H2, L7, L8.
**Status:** Pending

### Epic 5.3: Fix reporter schema discoverability and naming

**Goal:** Reporter's richest request body is discoverable, response models are exported and described, and pagination is typed.
**Scope:** `pkg/reporter/model/{report.go,pagination.go}`, reporter `internal/.../http/in/{metrics.go,notification.go}` and list handlers.
**Dependencies:** Phase 2, Phase 4.3
**Done when:** `CreateReportInput.Filters` carries a realistic `datasource→table→field→{op:[vals]}` example + `@Description`, with operator semantics promoted from Go comments into swag (M10); `notificationResponse`/`metricsResponse`/`errorMetrics`/`notificationItem` are exported (or moved to `pkg/reporter/model`) with `@name`+`@Description`+examples (M12); list handlers apply the `model.Pagination{items=[]X}` generic override so `items` is typed not `{}` (L9). M10, M12, L9.
**Status:** Pending

### Epic 5.4: Deepen the CRM mmodel schemas to the Account standard

**Goal:** Holder/Instrument mmodel schemas match the `Account`/`Balance` richness.
**Scope:** `pkg/mmodel/{holder.go,instrument.go}`.
**Dependencies:** Phase 2 (these already get `@name` in 2.1; this epic adds field-level depth)
**Done when:** Holder/Instrument carry `@example` bodies, maxLength/format hints, and per-field descriptions matching the Account standard; `LegalPerson`'s `@Description` no longer mislabels a shared request/response type as "response payload data". L6.
**Status:** Pending

---

## Self-Review

- **Spec coverage:** all 52 findings mapped to an epic in the coverage map at the top; verified each of C1, H1–H6, M1–M14, L1–L17 appears exactly once. The four audit non-findings (tracer ApiKeyAuth, reporter stub dirs, tracer artifact layout, 1-route tags) are explicitly *not* turned into work — tracer's security block is called out as untouched in 1.2.2.
- **Vagueness scan:** Phase 1 tasks name exact files, line numbers, jq assertions, and locked string values; no "appropriate"/"TBD". Later phases carry deferrals deliberately (rolling wave) but each epic's Done-when names concrete observable criteria, not "handle edge cases".
- **Contract consistency:** the canonical Bearer description string, the four parity fields, version `4.0.0`, and the `Midaz ` title prefix are defined once in Epic 1.2's decision block and referenced by 1.2.1–1.2.3 and enforced by 1.3.1. The `check-docs` target named in 1.3.1 is the same one wired in 1.3.2.
- **Phase boundaries:** every phase ends with a regenerable, compiling, verifiable state — all edits are annotations/comments/scripts except Phase 5's new DTOs, which still compile and regenerate. Go build is never left broken mid-phase.
- **Verification plausibility:** commands use real targets (`make generate-docs`, `make check-docs`, `swag init ...` with the repo's exact flags from `generate-docs.sh:97`) and real paths.
- **Open decisions surfaced, not buried:** C1 model (a vs b) is a hard gate on Phase 3; the `alias_id` span-attribute rename (L5) and the tracer DTO-vs-in-place choice (H1) are flagged as gates at their epics rather than silently assumed.
