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
| 2 | Specs carry clean schema names (annotated set), consistent Title-Case tags with group descriptions, lowercase router methods, and no wrong/malformed runtime-visible text | 2.1, 2.2, 2.3 | Detailed |
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

## Phase 2 — Mechanical wire sweeps (low blast radius)

**Milestone:** The three regenerated specs carry clean schema names for the annotated set (the zero-annotation set stays for Phase 4/5), a single Title-Case tag taxonomy with `@tag` group descriptions, lowercase `@Router` methods, and no factually-wrong or malformed runtime-visible text. `make check-docs` stays green; all binaries build.

**Scope correction from Phase-2 elaboration (against the committed tree):**
- The dotted-name set splits in two. **In scope here:** structs that ALREADY carry swag annotations and only lack `// @name`. **Deferred to Phase 5 (full annotation, not just a name):** the entire tracer `pkg/model.*` set (→5.1, 21 names), the `feeshared_model.*` billing family (→5.2, 11 names: `AccountTarget`, `BillingCalculate{Request,Response,Summary}`, `BillingCalculationResult`, `BillingPackage`, `BillingPackageUpdate`, `DiscountTier`, `EventFilter`, `PricingTier`), and the reporter `internal_manager_adapters_http_in.*` unexported types (→5.3: `errorMetrics`, `metricsResponse`, `notificationItem`, `notificationResponse`). 2.1 must SKIP these and record them as deferred — adding a bare `@name` to a zero-annotation struct would collide with the Phase 5 work.
- **L13 and L5 are NOT mechanical** and are scoped down (see Epic 2.3). L13's `TRC-` literal appears in 33 files (tests, `rule_repository.go`, `limit_validation.go`, …) — distinguishing a stale comment from a live error string needs per-site reading, so Phase 2 touches ONLY confirmed stale comments in the two audit-named annotation files and flags the rest. L5's `app.request.alias_id` span-attribute rename is an observability surface and is EXCLUDED from Phase 2 (separate, acknowledged change).

### Epic 2.1: `@name` sweep to clean leaked schema names (annotated set only)

**Goal:** Every wire-facing struct that already carries swag annotations but lacks `// @name` gets a clean name, so its spec definition stops emitting a package-dotted/`github_com_...` key.
**Scope:** `pkg/mmodel/*`, `pkg/net/http`, `components/ledger/internal/adapters/mongodb/fees/pack`, `pkg/reporter/*` and reporter `template_builder`/`pongo`/`datasource`/`report`/`deadline`/`template` packages, tracer `api` package.
**Dependencies:** Phase 1
**Done when:** the dotted-name count in each `components/<c>/api/swagger.json` drops to exactly the deferred set named above (ledger: only the 11 feeshared billing names remain; tracer: only the 21 pkg/model names remain; reporter: only the 4 unexported names remain); `make check-docs` green; builds pass.
**Status:** Pending

#### Task 2.1.1: Name the ledger + shared `mmodel` annotated structs

- [ ] Done

**Context:** `components/ledger/api/swagger.json` carries 18 dotted definition names. Of these, the annotated-but-unnamed set is: `mmodel.Balance`, `mmodel.BalanceSettings`, `mmodel.LedgerSettings`, `mmodel.TracerSettings`, `mmodel.AccountingValidation`, `mmodel.Date` (all in `pkg/mmodel/*`), `github_com_..._pkg_net_http.Pagination` (`pkg/net/http`), and `..._mongodb_fees_pack.Package` (`components/ledger/internal/adapters/mongodb/fees/pack`). Reference: `mmodel.Balance` (`pkg/mmodel/balance.go:19-23`) has full `swagger:model`+field annotations but is missing `// @name Balance`, while sibling `Account` (`pkg/mmodel/account.go:277`) emits cleanly because it has the directive.

**Implementation vision:** For each dotted name above, open its struct, CONFIRM it carries swag annotations (`swagger:model` and/or field-level `example`/`description` tags — they all do, that's why they have rich definitions), then add a `// @name <CleanName>` line as the last line of the struct's doc comment (matching the `account.go:277` placement). Clean names: `Balance`, `BalanceSettings`, `LedgerSettings`, `TracerSettings`, `AccountingValidation`, `Date`, `Pagination`, `Package`. **Do NOT touch** the 11 `feeshared_model.*` billing structs — verify each is zero-annotation (only json/bson tags) and leave it for Phase 5.2. If any supposed target turns out to be zero-annotation, skip it and report it as deferred instead.

**Files:**
- Modify: `pkg/mmodel/balance.go`, `pkg/mmodel/ledger.go` (or wherever `LedgerSettings`/`TracerSettings`/`BalanceSettings`/`AccountingValidation`/`Date` are defined — grep `type BalanceSettings`, etc.)
- Modify: `pkg/net/http/*.go` (the `Pagination` struct)
- Modify: `components/ledger/internal/adapters/mongodb/fees/pack/*.go` (the `Package` struct)

**Verification:** `cd components/ledger && swag init -g cmd/app/main.go -o api --parseDependency --parseInternal --outputTypes go,json,yaml` then `jq -r '.definitions|keys[]' api/swagger.json | grep -E '\.|github_com'` lists ONLY the 11 `feeshared_model.*` billing names (the deferred set) — `mmodel.*`, `Pagination`, and `Package` are gone, replaced by clean keys.

**Done when:** the 8 named structs emit clean definition keys; the 11 billing names remain (deferred); ledger builds.

#### Task 2.1.2: Name the reporter + tracer-`api` annotated structs

- [ ] Done

**Context:** `reporter/api/swagger.json` has 23 dotted names; the annotated-but-unnamed set is `report.Report`, `deadline.Deadline`, `..._mongodb_template.Template`, `datasource.ValidationWarning`, `model.FilterCondition`, the `pongo.*` family (5), and the `template_builder.*` family (8). The 4 `internal_manager_adapters_http_in.*` unexported types are DEFERRED to Phase 5.3 (they need exporting, not just naming). `tracer/api/swagger.json` has 40 dotted names, of which only the `api.*` wrappers (`api.ErrorResponse`, `api.ReadyzCheck`, `api.ReadyzResponse`, `api.VersionResponse`) are annotated-but-unnamed; the 21 `pkg/model.*` names and `pkg.HTTPError` patterns are handled elsewhere (`pkg/model` → Phase 5.1).

**Implementation vision:** Add `// @name <CleanName>` to each annotated reporter struct (`Report`, `Deadline`, `Template`, `ValidationWarning`, `FilterCondition`, the `pongo` block/filter response types, the `template_builder` request/response types) and to the four tracer `api` wrapper structs (`ErrorResponse`, `ReadyzCheck`, `ReadyzResponse`, `VersionResponse`). For each, first confirm swag annotations exist; if a struct is zero-annotation, skip and report deferred. Do NOT export or rename the reporter `internal_manager_adapters_http_in.*` types — that is Phase 5.3.

**Files:**
- Modify: reporter structs under `pkg/reporter/mongodb/{report,deadline,template}`, `pkg/reporter/model` (`FilterCondition`, `ValidationWarning` — grep to confirm packages), and the `pongo`/`template_builder` packages (grep `type GenerateCodeResponse`, etc.)
- Modify: tracer `components/tracer/api/*.go` (the `api.*` wrapper structs)

**Verification:** regenerate both specs; `jq -r '.definitions|keys[]' reporter/api/swagger.json | grep -E '\.|github_com'` lists only the 4 `internal_manager_adapters_http_in.*` names; tracer's `api.*` wrappers emit clean. Both components build.

**Done when:** reporter + tracer-`api` annotated structs emit clean keys; the deferred sets remain; builds pass.

### Epic 2.2: Normalize tag taxonomy, add `@tag` groups, lowercase router methods

**Goal:** One consistent Title-Case tag taxonomy across all three components, `@tag.name`/`@tag.description` group blocks in every general-info header, and lowercase `@Router` HTTP methods.
**Scope:** handler files in `components/{ledger,tracer,reporter}/internal/.../http/in/`, the three `cmd/app/main.go`.
**Dependencies:** Phase 1
**Done when:** the tag renames below are applied; every component's `main.go` carries `@tag.name`+`@tag.description` for each of its tags; zero capitalized `@Router` methods remain; `make check-docs` green.
**Status:** Pending

#### Task 2.2.1: Apply the locked tag renames + lowercase `@Router` methods

- [ ] Done

**Context:** Full `@Tags` inventory (verified): ledger has `Operation Route` (5), `Transaction Route` (5), `BillingPackages` (5), `BillingCalculate` (1) breaking the Title-Case-with-spaces convention its siblings (`Account Types`, `Asset Rates`, `Metadata Indexes`) follow; tracer's 7 tags are ALL lowercase (`audit`, `health`, `info`, `limits`, `reservations`, `rules`, `validations`); reporter has `Data source` (2) breaking its own Title-Case (`Data Sources` would match `Templates`/`Reports`). Separately, 13 `@Router` lines use a capitalized HTTP method (`[Put]`, `[Get]`, `[Delete]`, `[Post]`), all in ledger handlers (e.g. `assetrate.go:42 [Put]`, `balance.go:49,121,192,243`).

**Implementation vision — locked rename map** (apply by editing `@Tags` lines in the handler files; swag tag = the literal string):
- ledger: `Operation Route` → `Operation Routes`; `Transaction Route` → `Transaction Routes`; `BillingPackages` → `Billing Packages`; `BillingCalculate` → `Billing Calculate`.
- tracer: `audit` → `Audit`; `health` → `Health`; `info` → `Info`; `limits` → `Limits`; `reservations` → `Reservations`; `rules` → `Rules`; `validations` → `Validations`.
- reporter: `Data source` → `Data Sources`.
- Leave already-conforming tags untouched (`Accounts`, `Templates`, `Reports`, `Deadlines`, `Metrics`, `Template Builder`, `Account Types`, etc.).
- `@Router` methods: lowercase the bracketed method on all 13 capitalized lines (`[Put]`→`[put]`, `[Get]`→`[get]`, `[Delete]`→`[delete]`, `[Post]`→`[post]`). swag is case-insensitive so the spec is unchanged, but normalize for consistency + a future grep guard.

**Files:** ledger/tracer/reporter handler files under `internal/.../http/in/` (grep `@Tags` and `@Router` to locate; the renames are find-replace on exact tag strings).

**Verification:** `for c in ledger tracer reporter; do grep -rh '@Tags' components/$c --include='*.go' | sort -u; done` shows only Title-Case tags, no lowercase, no singular `Route`, no CamelCase billing; `grep -rn '@Router' components --include='*.go' | grep -E '\[(Get|Post|Put|Patch|Delete)\]'` returns nothing; regenerate, `make check-docs` green.

**Done when:** tag taxonomy is uniform Title-Case and router methods are lowercase across all three.

#### Task 2.2.2: Add `@tag.name`/`@tag.description` group blocks to each general-info header

- [ ] Done

**Context:** No component declares `@tag.*` group metadata (M8), so Swagger UI shows bare tag folders with no description. Tracer is worst — its `Reservations` group is a non-obvious two-phase hold/confirm/release lifecycle. The tag set per component is now the normalized set from Task 2.2.1.

**Implementation vision:** In each `components/<c>/cmd/app/main.go` general-info block, after the `@securityDefinitions`/description lines, add one `@tag.name <Tag>` + `@tag.description <one-line>` pair per tag the component uses (matching the tab-aligned comment style). Descriptions are short and behavioral. For tracer's `Reservations`, the description must name the two-phase lifecycle ("Two-phase balance reservations: hold, then confirm or release."). For ledger, describe each native + folded-in group; for reporter, each of its groups. **Dependency:** Task 2.2.1 must land first so the `@tag.name` values match the renamed tags exactly (a `@tag.name` that doesn't match any operation tag is dead metadata).

**Files:** Modify: `components/ledger/cmd/app/main.go`, `components/tracer/cmd/app/main.go`, `components/reporter/cmd/app/main.go`.

**Verification:** regenerate; `jq '.tags' components/<c>/api/swagger.json` lists every operation tag with a non-empty description; `make check-docs` green. (Parity check is unaffected — `@tag` blocks are per-component content, not parity fields.)

**Done when:** all three Swagger UIs render grouped, described tag navigation; every `@tag.name` matches a live tag.

### Epic 2.3: Fix wrong and malformed runtime-visible text (safe subset)

**Goal:** Every factually-incorrect or malformed doc string that misleads a spec reader is corrected — limited to genuinely safe, doc-only edits.
**Scope:** ledger `transaction_state_handlers.go`, the 8 files with malformed `example` tokens, `mongodb/fees/pack/package.go`, `feeshared/model/update_package_input.go`; reporter `mongodb/report/report.go`, `data-source.go`, `data-source-information.go`, `pkg/reporter/model/data-source-information.go`; tracer `transaction_validation_handler.go`, `rule_validation.go` (comment sites only).
**Dependencies:** Phase 1
**Done when:** all sub-items below are corrected and regeneration is clean.
**Status:** Pending

#### Task 2.3.1: Correct wrong/malformed examples and stale wording (safe set)

- [ ] Done

**Context & exact targets:**
- **M2:** `transaction_state_handlers.go:41` (Commit) and `:97` (Cancel) both carry `@Failure 400 ... "Invalid request or transaction cannot be reverted"` — copied from Revert at `:153`. (Line `:226` is a Revert span message, correct, leave it.)
- **M11:** `components/reporter/internal/.../mongodb/report/report.go:26` tags `example:"processing"` but the persisted constant is `Processing` (capitalized) — `GetAllReports` `@Param` enum at `report.go:235` uses the capitalized form.
- **L2 (15 sites):** malformed `example\t"value"` query-param tokens (swag needs `example(value)` syntax; the current form renders `example=None`). Exact lines: `operation_route.go:353-354`, `assetrate.go:138,140-141`, `operation.go:36-37`, `transaction_query_handlers.go:26-27`, `transaction_route.go:275-276`, `balance.go:40-41,111-112`. Fix to `example(...)` form OR fold the value into the param description (as `balance.go:513` already does for a date hint) — pick whichever swag renders cleanly; verify the rendered example is non-null.
- **L3 (3 sites):** single-quote array-literal examples: `mongodb/fees/pack/package.go:85` and `feeshared/model/update_package_input.go:32` both `example:"['acc001', 'ac0002']"` (single quotes + `ac0002` typo → fix to `acc002`); `pkg/reporter/model/data-source-information.go:37` `example:"['id', 'name', 'parent_account_id']"`. Use swag's valid array-example form for `[]string`.
- **L10:** reporter "plugin" wording in `data-source.go:42` and `data-source-information.go:7,11,18` → "reporter".
- **L13 (narrowed):** ONLY inspect `transaction_validation_handler.go` and `rule_validation.go` for `TRC-` references that are stale COMMENTS (not live error strings, not test assertions). Update only confirmed stale comments to the numeric registry. REPORT every other `TRC-` site found (tests, `rule_repository.go`, `limit_validation.go`, `audit_event_validation.go`) as out-of-scope-needs-review — do NOT edit them.

**Implementation vision:** Pure text edits. For L2, prefer `example(2021-01-01)` form; if a param already conveys the format in its description, dropping the broken token is acceptable. For L3, emit a valid swag array example (double-quoted JSON-style or swag's accepted form) and fix the `ac0002`→`acc002` typo. Each edit must leave the file compiling (they're struct tags / comment annotations).

**EXCLUDED from this task (acknowledged, not silently dropped):**
- **L5** — the `app.request.alias_id` span-attribute rename and "Failed to create alias" log strings in `instrument.go`. This is an observability surface; renaming spans/log keys can break dashboards/alerts. Flagged for a separate, explicitly-approved change.
- The broad `TRC-` usage outside the two named files (tests + repository code).

**Files:** the files listed in the targets above.

**Verification:** regenerate all three specs; `make check-docs` green; spot-check via jq that a previously-broken example now renders (e.g. `jq '.paths."/v1/...".get.parameters[]|select(.name=="start_date").example'` is non-null); `grep -rn "example	\"" components pkg --include='*.go'` returns nothing; `grep -rn "\['" components pkg --include='*.go'` (in example tags) returns nothing; `grep -rn "plugin" components/reporter/.../data-source*.go` returns nothing; builds pass.

**Done when:** the safe set is corrected, the excluded items are reported, and regeneration is clean.

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
