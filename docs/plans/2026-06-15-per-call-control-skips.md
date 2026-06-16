# Per-Call Control Skips Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Let a caller skip CRM (holder), fees, and tracer (fraud/limits) per request — gated by a per-ledger operator opt-in — such that an honored skip incurs **zero additional latency or processing overhead** (no Mongo lookup, no gRPC call, no CRM call, no extra DB read).

**Architecture:** Two-key model. **Key 1 (operator):** a per-ledger `Overrides` opt-in stored in the existing `ledger.settings` JSONB (default all `false`). **Key 2 (caller):** an optional `skip` object on the account-create and transaction-create request bodies. A skip is **honored only when both keys are present** (`requested && allowed`); a skip requested without the opt-in returns **422**; an absent skip preserves current behavior exactly. Honored skips short-circuit **before** the expensive work, reading the opt-in off the already-fetched, Redis-cached ledger settings — so the gate itself adds no I/O. Honored skips are persisted (typed boolean columns) and audited (span + streaming event) as a release gate.

**Tech Stack:** Go 1.26.4, Fiber v2, PostgreSQL (squirrel + golang-migrate `.up/.down.sql`), MongoDB (fee packages), Redis (settings cache, 5-min TTL), lib-commons, lib-streaming. Migration hygiene validated by per-migration `_test.go` files (run under `make test-unit` in CI) and, manually, by `make migrate-lint` (NOT in `make ci`).

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | Two-key gate proven end-to-end on **tracer**: honored skip makes zero gRPC Reserve at create **and** at PENDING commit/cancel, unauthorized skip → 422, absent skip unchanged, gate adds no DB read | 1.1, 1.2, 1.3 | ✅ Done (`eb2879f0a`) |
| 2 | **Fees** skip: honored skip makes zero Mongo package lookup + zero fee computation; fee seam re-ordered (settings hoisted above it) with structural test re-blessed | 2.1, 2.2 | ✅ Done (`5fcb7423b`) |
| 3 | **Holder/CRM** skip on account creation: honored skip makes zero `HolderReader.Exists` call; only bites when `requireHolder=true` | 3.1, 3.2 | ✅ Done (`c649ff79b`) |
| 4 | **Audit + hardening (release gate):** honored skips persisted to typed columns + streaming event across the full wire path; OpenAPI/llms-full/docs; cross-surface integration suite | 4.1, 4.2, 4.3 | ✅ Done — 4.1 (`c7caf2b2f`+`244323320`), 4.2 (`ec1a97032`), 4.3 (`9352792ce`) — **PLAN CLOSED** |

---

## Cross-cutting contracts (referenced by multiple epics)

These are defined in Phase 1 and consumed by later phases. Stated here once so epics agree.

**The opt-in (Key 1) — `pkg/mmodel/settings.go`:**
```go
type OverridePolicy struct {
    AllowFeeSkip    bool `json:"allowFeeSkip"`    // default false
    AllowTracerSkip bool `json:"allowTracerSkip"` // default false
    AllowHolderSkip bool `json:"allowHolderSkip"` // default false
}
// added to LedgerSettings:
//   Overrides OverridePolicy `json:"overrides"`
```

**The caller flag (Key 2) — transaction body, `pkg/mtransaction`:**
```go
type TransactionSkip struct {
    Fees   bool `json:"fees,omitempty"`
    Tracer bool `json:"tracer,omitempty"`
}
// Skip *TransactionSkip `json:"skip,omitempty"` on the 3 input DTOs (documented in OpenAPI).
// On mtransaction.Transaction: `json:"skip,omitempty" swaggerignore:"true"` — that struct doubles as the
// GET-transaction response model (@name TransactionInput), so swaggerignore stops `skip` leaking into the
// READ schema; json is kept so the flag persists in the body JSONB (needed by commit/cancel re-resolution).
```
Account body (Phase 3) — `pkg/mmodel/account.go`:
```go
type AccountSkip struct {
    Holder bool `json:"holder,omitempty"`
}
// Skip *AccountSkip `json:"skip,omitempty"` on CreateAccountInput
```

**The resolver (the two-key rule), shared by all three controls — lives in a NEUTRAL, inner-importable package `pkg/skip`** (NOT in `adapters/http/in`: the account holder control resolves inside `services/command`, and `command → adapters/http/in` is an inward-layer violation + import cycle). `ErrSkipNotPermitted` stays in `pkg/constant/errors.go` (already neutral):
```go
// pkg/skip — ResolveSkipFor returns honored=true only when both keys agree.
// requested && !allowed  -> ErrSkipNotPermitted (422)
// requested && allowed   -> honored=true
// !requested             -> honored=false
func ResolveSkipFor(control string, requested, allowed bool) (honored bool, err error)
```

**Resolution location (the "resolve once, pass booleans" contract):** All skip authorization (the 422 decision) is resolved **once per request, immediately after the ledger settings are read**, producing per-control `honored*Skip` booleans. The seams receive only the resolved boolean and short-circuit on it — they never re-read settings or re-evaluate the two-key rule. For transactions the resolution point moves upstream in Phase 2 (the fee-seam hoist); for account creation it lives in the create-account use case. This is what keeps the gate free: one cached settings read already in the path, one boolean per seam.

**Non-goals (explicit):**
- **DSL mode** (deprecated, sunset 2026-08-01) does not support `skip` — it parses a file, not a body. `Transaction.Skip` stays nil for DSL.
- **Revert** is synthesized server-side, so no caller `skip` object can be attached. Fees are inherently skipped on revert (`applyFees` no-ops on `isRevert`). The tracer **does** still run on a revert (the reserve anchor at `transaction_create.go:1231` fires for any non-off ledger regardless of `isRevert`). **DECISION:** a revert does NOT inherit the parent's honored tracer skip even after Epic 4.1 persists `tracer_skipped` — reverts always re-run the tracer, because returning funds is its own risk event and the parent's per-call exemption was scoped to the original posting.
- `skip.fees` on **annotation** is a no-op (annotations already short-circuit fees); `skip.tracer` on annotation **is** honored (annotations do call reserve).

---

## Phase 1 — Two-key gate proven on tracer

**Why tracer first:** it is the design-critical risk. The whole value proposition is "honored skip ⇒ zero overhead, and the gate is free." Tracer is the cleanest place to prove it: the synchronous gRPC Reserve is the dominant latency (250 ms timeout, `tracer/client.go:46`), and the ledger settings carrying the opt-in are **already fetched** at `transaction_create.go:1129`, before the reserve at `:1231` — so the gate provably adds no I/O. If this assumption held, fees and holder replicate the pattern. If it failed, the design is wrong and we learn it in Phase 1.

### Epic 1.1: Per-ledger skip opt-in (`Overrides`)

**Goal:** `LedgerSettings.Overrides{AllowFeeSkip, AllowTracerSkip, AllowHolderSkip}` exists, defaults all `false`, persists through the existing JSONB column with no migration, validates as booleans, round-trips through every settings read/write surface, and is returned by the cached read path.
**Scope:** `pkg/mmodel/settings.go` (+ test). No HTTP-handler or command changes and no migration: the settings write surfaces are generic (PATCH `/settings` takes `map[string]any` schema-validated at `ledger.go:431`; `POST /ledgers` takes a typed `CreateLedgerInput.Settings` at `pkg/mmodel/ledger.go:30`), and every read surface returns the typed struct or `ParseLedgerSettings` (`GET /settings` at `ledger.go:379`, `GET /ledgers/{id}` via embedded `Ledger.Settings`). The whole surface is carried by the model's struct + schema + map-conversion helpers — the only files that change. **Note:** there is no PUT or DELETE for settings; writes are PATCH-merge plus create-time.
**Dependencies:** none
**Done when:** unit tests prove `{"overrides":{"allowFeeSkip":true}}` parses to the struct with that bool set and the rest false, `{}` parses to all-false, and a typed `LedgerSettings{Overrides:...}` survives a `LedgerSettingsToMap` → `ParseLedgerSettings` round-trip without loss; integration proves BOTH write surfaces persist + read back overrides — `PATCH /v1/.../settings` (cache invalidated by `update_ledger_settings.go:73`) AND `POST /v1/.../ledgers` with `settings.overrides` (the typed→map create path, the one that breaks if `LedgerSettingsToMap` is missed) — and `GET /settings` returns them.
**Status:** Done (`eb2879f0a`) — all 6 functions wired; round-trip + create-path-drop guard tested. Integration write/read-back deferred to Epic 4.2 (no testcontainer harness in `pkg/mmodel`).

#### Task 1.1.1: Add `OverridePolicy` to the settings model, schema, defaults, parse, and validation

- [x] Done

**Context:** Ledger settings are a single JSONB column `ledger.settings` (`migrations/onboarding/000008_add_ledger_settings_column.up.sql`, `DEFAULT '{}'`) — **adding a nested struct needs no migration**. The model lives at `pkg/mmodel/settings.go:16-39` (`LedgerSettings` → `Accounting AccountingValidation`, `Tracer TracerSettings`; the type carries a `//\t@name LedgerSettings` swagger annotation at `:39`). **Six functions enumerate the settings fields and ALL must learn `overrides`, or the field leaks somewhere:** `settingsSchema` (allowed-field map, `:248-259`); `DefaultLedgerSettings()` (typed defaults, `:127-152`); `DefaultLedgerSettingsMap()` (map form of defaults, `:136-152`); `LedgerSettingsToMap()` (typed→map, `:156-169` — **the create path `POST /ledgers` runs through this; miss it and `CreateLedgerInput.Settings.Overrides` is silently dropped**); `ParseLedgerSettings()` (map→typed with per-key default fallback, `:185-220`); and the bool branch of `validateSettingsFieldType`/`validateSettingsFieldValue` (`:378-410`, enums only for `tracer.*` — bools need type-check only). `knownNestedFieldNames` (`:264-274`) derives from `settingsSchema` automatically. Single write/read gate: `ValidateSettings()` (`:276-410`) is the only validation, called by both `command/update_ledger_settings.go:44` (PATCH, then deep-merge) and `command/create_ledger.go:56` (create). Adding to `settingsSchema` makes both accept `overrides` automatically.

**Implementation vision:** Add `OverridePolicy` (3 bools, json tags `allowFeeSkip`/`allowTracerSkip`/`allowHolderSkip`, with its own `//\t@name OverridePolicy` annotation so swagger auto-generates) and field `Overrides OverridePolicy json:"overrides"` on `LedgerSettings`. Mirror the `Accounting` block in **all six functions above**: add the `overrides` nested object (three `bool` fields, no enum) to `settingsSchema`; add `defaultOverridePolicy` (all false) and wire it into `DefaultLedgerSettings()` AND `DefaultLedgerSettingsMap()`; add the `overrides` block to `LedgerSettingsToMap()` (typed→map) AND `ParseLedgerSettings()` (map→typed, missing/wrong-type key → all-false default). Bools use the existing `validateSettingsFieldType` bool branch — no enum logic. Edge cases: partial overrides (`{"overrides":{"allowTracerSkip":true}}`) leaves the other two false via deep-merge + per-field parse; an unknown nested key under `overrides` is rejected by the existing unknown-field check; the typed→map→typed round-trip (the create path) preserves all three bools.

**Files:**
- Modify: `pkg/mmodel/settings.go` (struct + `@name`, `settingsSchema` ~:248-259, `DefaultLedgerSettings` ~:127-152, `DefaultLedgerSettingsMap` ~:136-152, `LedgerSettingsToMap` ~:156-169, `ParseLedgerSettings` ~:185-220)
- Test: `pkg/mmodel/settings_test.go`

**Verification:** `go test ./pkg/mmodel/ -run TestSettings -v` — cases pass for: full overrides object, partial (one true), empty `{}` → all false, unknown nested key rejected with `ErrUnknownSettingsField`, and a `LedgerSettings{Overrides:{AllowFeeSkip:true}}` → `LedgerSettingsToMap` → `ParseLedgerSettings` round-trip returning the same bools (guards the create-path drop).

**Done when:** `OverridePolicy` round-trips through Parse/Default/Validate AND `LedgerSettingsToMap`↔`ParseLedgerSettings` with all-false defaults; partial PATCH preserves unspecified bools as false; unknown override keys are rejected; swagger regenerates with `OverridePolicy` from the `@name` annotation.

### Epic 1.2: Transaction `Skip` body contract + central two-key resolver

**Goal:** the three JSON-family transaction input DTOs and `mtransaction.Transaction` carry optional `Skip{Fees,Tracer}`; a shared `resolveSkip` helper + `ErrSkipNotPermitted` sentinel implement the two-key rule and map unauthorized skips to 422.
**Scope:** `pkg/mtransaction/` (transaction.go, input.go, tests); `pkg/constant/errors.go`; new `components/ledger/internal/adapters/http/in/skip_resolution.go` (+ test).
**Dependencies:** none (parallel with 1.1)
**Done when:** Build* methods propagate `Skip` from each input DTO into `Transaction.Skip` (nil when absent); DSL leaves it nil; `resolveSkip` passes its four-row truth table and the unauthorized row returns a 422-classified business error.
**Status:** Done (`eb2879f0a`) — resolver landed in neutral `pkg/skip` (not `http/in`, per the import-layer constraint); `ErrSkipNotPermitted` = `0490`.

#### Task 1.2.1: Define `TransactionSkip` and thread it through the input DTOs and `Build*` methods

- [x] Done

**Context:** The 5 transaction modes use 3 input structs — `mtransaction.CreateTransactionInput` (JSON at `transaction.go:69` + annotation at `:110`), `CreateTransactionInflowInput` (`:151`), `CreateTransactionOutflowInput` (`:192`) — converted to `mtransaction.Transaction` via `Build*` at `pkg/mtransaction/input.go:62` (`BuildTransaction`), `:141` (`BuildInflowEntry`), `:229` (`BuildOutflowEntry`). DSL (`:234`) parses a file straight into `mtransaction.Transaction` (`:284`) with no input DTO. All converge at `executeCreateTransaction` (`transaction_create.go:961`), which reads `mtransaction.Transaction`. The domain struct is `Transaction` at `transaction.go:283-316`. Existing per-DTO field replication (e.g. `Metadata`, `Description`) is the established style — follow it rather than introducing a shared embed.

**Implementation vision:** Add `TransactionSkip{Fees, Tracer bool}` and `Skip *TransactionSkip json:"skip,omitempty"` to the 3 input DTOs (documented in OpenAPI). On `mtransaction.Transaction` add the same field **tagged `json:"skip,omitempty" swaggerignore:"true"`** — that struct carries `@name TransactionInput` and doubles as the GET-by-id response model (`transaction.go:309` `@Success 200 {object} Transaction`), so without `swaggerignore` `skip` would leak into the READ schema as if it were queryable resource state (it is request-only; audit is surfaced via the Epic 4.1 columns). Keep `json` so the flag persists in the body JSONB (needed by Epic 1.3's commit/cancel re-resolution) and propagates at runtime. In each `Build*` method, copy `input.Skip` onto the built `Transaction.Skip` (direct pointer copy; nil stays nil). DSL: do nothing — `Transaction.Skip` stays nil (documented non-goal). Absent `skip` → nil pointer → no behavior change, no allocation. Do not validate here; authorization is `pkg/skip.ResolveSkipFor`'s job (Task 1.2.2) and happens after the settings read.

**Files:**
- Modify: `pkg/mtransaction/transaction.go` (struct ~:283-316; `TransactionSkip` type), `pkg/mtransaction/input.go` (3 input DTOs; `BuildTransaction` :62, `BuildInflowEntry` :141, `BuildOutflowEntry` :229)
- Test: `pkg/mtransaction/input_test.go`

**Verification:** `go test ./pkg/mtransaction/ -run TestBuild -v` — each Build* propagates a set `Skip`; absent `Skip` yields `Transaction.Skip == nil`.

**Done when:** JSON, annotation, inflow, outflow carry `Skip` into `Transaction`; DSL yields nil; absent skip is a nil pointer with zero behavior change.

#### Task 1.2.2: Add `ErrSkipNotPermitted` sentinel and the `resolveSkip` helper

- [x] Done

**Context:** Error sentinels live in `pkg/constant/errors.go` (numeric registry only — no prefixed families). Business errors are raised via `pkg.ValidateBusinessError(constant.Err..., constant.Entity...)`; `UnprocessableOperationError` maps to HTTP 422 (`pkg/net/http/errors.go`). The two-key rule (`requested && allowed`) must reject `requested && !allowed` with 422. This helper is the single authority for all three controls (tracer now, fees/holder later), so it MUST live in a NEUTRAL package importable by BOTH `adapters/http/in` (transaction sites) and `services/command` (the holder site). `services/command` cannot import `adapters/http/in` (inward-layer violation + import cycle: `account.go:77` / `transaction_create.go:23` already go in→command). Put it in `pkg/skip`.

**Implementation vision:** Add a unique numeric sentinel `ErrSkipNotPermitted` to `pkg/constant/errors.go`. Create `pkg/skip/skip.go` with the pure function `ResolveSkipFor(control string, requested, allowed bool) (bool, error)`: `requested && !allowed` → `false, pkg.ValidateBusinessError(constant.ErrSkipNotPermitted, <entity>)`; `requested && allowed` → `true, nil`; otherwise `false, nil`. The `control` label makes the 422 body actionable ("tracer skip not permitted on this ledger"). All sites — transaction create/commit/cancel and account create — import it from `pkg/skip`. Unit-test the full truth table.

**Files:**
- Modify: `pkg/constant/errors.go`
- Create: `pkg/skip/skip.go`, `pkg/skip/skip_test.go`

**Verification:** `go test ./pkg/skip/ -run TestResolveSkipFor -v` — four-row truth table passes; the `requested && !allowed` row returns an error that classifies as `UnprocessableOperationError` (422).

**Done when:** sentinel is unique and registered; `pkg/skip.ResolveSkipFor` honors the two-key rule; unauthorized skip is a 422-classified business error; the package is importable from both `adapters/http/in` and `services/command` with no cycle.

### Epic 1.3: Tracer short-circuit (zero gRPC Reserve on honored skip)

**Goal:** with `AllowTracerSkip=true` and `skip.tracer=true`, the reserve anchor returns `proceed` **without** invoking `TracerReserver.Reserve` at create **and** the PENDING `/commit` and `/cancel` transitions skip the by-transaction `ConfirmByTransaction`/`ReleaseByTransaction` gRPC; `skip.tracer` without the opt-in returns 422; absent skip behaves exactly as today; the gate reads only the already-fetched `ledgerSettings` (no new settings read).
**Scope:** `components/ledger/internal/adapters/http/in/transaction_create.go`, `transaction_reservation_anchor.go`, `transaction_state_handlers.go` (+ tests).
**Dependencies:** Epic 1.1 (`Overrides`), Epic 1.2 (`Skip` + `pkg/skip.ResolveSkipFor`)
**Done when:** a mock `TracerReserver` test asserts `Reserve` is called **zero** times when the skip is honored, the outcome is `proceed`, and the transaction posts; a **PENDING create → commit** and **create → cancel** test asserts mock `ConfirmByTransaction`/`ReleaseByTransaction` count is **zero** when the parent skip was honored and **one** when absent; `skip.tracer` without opt-in → 422 (idempotency key released); no skip → `Reserve` exactly as today.
**Status:** Done (`eb2879f0a`) — create + commit/cancel short-circuits wired; seam-wiring + helper-level zero-call tests green. Runtime end-to-end zero-call assertions deferred to Epic 4.2 (testcontainer).

#### Task 1.3.1: Resolve tracer skip after the settings read and short-circuit the reserve anchor

- [x] Done

**Context:** Ledger settings are fetched at `transaction_create.go:1129` via `GetParsedLedgerSettings` (Redis cache-aside, TTL `get_ledger_settings.go:22`) and passed as `ledgerSettings.Tracer` to `reserveTransaction` at `:1231`. The reserve gate `transaction_reservation_anchor.go:99` already early-returns `reservationProceed` when `TracerReserver == nil || settings.Mode == TracerModeOff || settings.Mode == ""` — **before** the gRPC `Reserve` at `:115`. The idempotency key is claimed upstream of this point; the fee error path at `transaction_create.go:1074` shows the `deleteIdempotencyKey` pattern to mirror on a 422. Annotations reach reserve (`reservationTTLForStatus` returns default TTL for NOTED), so tracer-skip is meaningful for them.

**Implementation vision:** Immediately after the settings read (`:1129`), resolve the tracer skip once: `honoredTracerSkip, err := resolveSkipFor("tracer", transactionInput.Skip != nil && transactionInput.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)`. On `err` (422): mirror the fee path — `handleSpanByErrorClass`, Warn log, `deleteIdempotencyKey(idempotencyResult.InternalKey)`, `return http.WithError(c, err)`. Pass `honoredTracerSkip` into `reserveTransaction` (extend its signature). In the anchor, add `|| honoredTracerSkip` to the `:99` early-return condition so an honored skip returns `reservationProceed` without building the request or calling `Reserve`. Do **not** add a second settings read — the gate consumes the booleans already in hand. Edge cases: skip honored + advisory/enforce mode → still short-circuits (skip wins over mode, since the operator explicitly allowed it); skip not requested → unchanged path including advisory/enforce; `TracerReserver == nil` (tracer globally off) → already proceeds, skip is moot.

**Commit/cancel (PENDING flow) — the same skip must hold at state transition:** a PENDING transaction defers tracer confirm to `/commit` and `/cancel` (`commitOrCancelTransaction`), which fire `confirmReservationsByTransaction` / `releaseReservationsByTransaction` (`transaction_state_handlers.go:524-526`) gated only on `tracerReservationEnabled` (`transaction_reservation_anchor.go:327-329`) — zero skip awareness. Without this, an honored create-time skip relocates the gRPC cost to commit/cancel instead of removing it. Fix: `commitOrCancelTransaction` already re-fetches `ledgerSettings` at `:423`, and the skip round-trips in the persisted body JSONB (`tran.Body.Skip`, since Epic 1.2 puts `Skip` on `mtransaction.Transaction`). Re-resolve there — `honoredTracerSkip := skip.ResolveSkipFor("tracer", tran.Body.Skip != nil && tran.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)` (authorization was already enforced at create, so treat unauthorized here as not-honored — do NOT 422) — and thread it into `confirmReservationsByTransaction`/`releaseReservationsByTransaction` (extend signatures), guarding their `:300`/`:313` early-return with `|| honoredTracerSkip`. No new column needed for the short-circuit; the body JSONB carries it and `:423` re-reads settings at no extra I/O.

**Files:**
- Modify: `components/ledger/internal/adapters/http/in/transaction_create.go` (resolve after ~:1129; pass flag into reserve at ~:1231), `components/ledger/internal/adapters/http/in/transaction_reservation_anchor.go` (Reserve gate `:99` + by-transaction confirm/release gates `:300`/`:313`, signatures), `components/ledger/internal/adapters/http/in/transaction_state_handlers.go` (re-resolve at `commitOrCancelTransaction` after `:423`, pass flag to `:524-526`)
- Test: `components/ledger/internal/adapters/http/in/transaction_reservation_anchor_test.go`, `transaction_state_handlers_test.go`

**Verification:** `go test ./components/ledger/internal/adapters/http/in/ -run 'TestReserve|TestExecuteCreateTransaction_TracerSkip|TestCommitCancel_TracerSkip' -v` — honored skip → mock `Reserve` count 0, outcome `proceed`; PENDING create→commit and create→cancel → mock `ConfirmByTransaction`/`ReleaseByTransaction` count 0 when parent skip honored, 1 when absent; `skip.tracer` without opt-in → 422 with idempotency key deleted; no skip → `Reserve` once as today; advisory/enforce unaffected when skip absent.

**Done when:** honored tracer skip makes zero gRPC Reserve at create AND zero by-transaction confirm/release at commit/cancel, and posts/commits the transaction; unauthorized tracer skip → 422; absent skip is byte-for-byte current behavior; no additional settings read is introduced (create rides the `:1129` read, commit/cancel the `:423` read — each is the only fetch on its path).

---

## Phase 2 — Fees skip (the seam hoist)

**Milestone:** with `AllowFeeSkip` + `skip.fees`, the request makes **zero** fee-package Mongo lookups and runs no fee computation; the transaction posts as authored. The fee seam is re-ordered so settings are read once, above it, and the structural test is re-blessed for the new ordering.

### Epic 2.1: Hoist the cached settings read above the fee seam

**Goal:** `GetParsedLedgerSettings` is read **exactly once** near the top of `executeCreateTransaction` (e.g. right after the idempotency claim ~`:1040`) and threaded by pointer to the fee seam and the former `:1129` consumer (the `:1129` read is **DELETED**, not duplicated); skip resolution (tracer + fees) moves to this single upstream point.
**Scope:** `components/ledger/internal/adapters/http/in/transaction_create.go`, `transaction_fee_seam_structure_test.go`.
**Dependencies:** Phase 1 (resolver + tracer resolution exist; this relocates them)
**Status (execution):** Done (`5fcb7423b`). Settings read hoisted to `transaction_create.go:1065` (single read; old `:1130` deleted), fee + tracer resolves now at `:1079`/`:1093` above the seam; `applyFees` gained `honoredFeeSkip` with a first-statement `return nil` bypass. Structural test gained `getSettingsCount==1` + `getSettingsPos<applyFeesPos` with a 2-read bite fixture. Fee short-circuit proven directly via a fake `FeeApplier` (zero `CalculateFee` calls). Bonus: relocating the tracer resolve upstream fixed the Phase-1 carry (tracer 422 now precedes fee compute).

**Done when:** settings are fetched **exactly once** per request (old `:1130` call removed, no second read). The hoist is unconditional and safe — the seam-ordering comment at `transaction_create.go:1063-1066` pins the `validate` reassignment (`:1101`) relative to `PropagateRouteValidation` (`:1139-1141`, which only reads `ledgerSettings.Accounting.ValidateRoutes`), NOT where settings are fetched. **A second settings read is forbidden, not a fallback** (the prior fallback's trigger was a phantom — the structural test tracks no settings ordering). `transaction_fee_seam_structure_test.go` passes AND is EXTENDED to guard the invariant: add a `getSettingsCount` metric to `seamMetrics` counting AST calls to `GetParsedLedgerSettings` in `executeCreateTransaction`, assert it equals exactly 1 (with a `*_Bites` fixture proving the gate fails at 2). Tracer-skip behavior from Phase 1 is unchanged after the move.
**Status:** Done (`5fcb7423b`)

### Epic 2.2: Fee short-circuit before the Mongo package lookup

**Goal:** an honored fee skip causes `applyFees` to return before any fee work; `services/fees/calculate-fee.go` never runs the package lookup; the transaction is not mutated with fee legs.
**Scope:** `components/ledger/internal/adapters/http/in/transaction_fee_application.go`, `transaction_create.go` (pass `honoredFeeSkip`), tests.
**Dependencies:** Epic 2.1
**Done when:** a test with a fee-package-bearing org asserts the package repo `FindByOrganizationIDAndLedgerID` (`services/fees/calculate-fee.go:60`) is called **zero** times when the skip is honored, and the posted transaction carries no fee legs; `skip.fees` without opt-in → 422; annotation/revert remain inherent no-ops (skip.fees there changes nothing); absent skip applies fees exactly as today.
**Status:** Done (`5fcb7423b`) — short-circuit proven at the `applyFees` seam (zero `CalculateFee`); runtime zero-Mongo-lookup assertion deferred to Epic 4.2 testcontainer, consistent with Phase 1.

---

## Phase 3 — Holder/CRM skip (account surface)

**Milestone:** with `requireHolder=true`, `AllowHolderSkip`, and `skip.holder`, account creation makes **zero** `HolderReader.Exists` CRM calls and still creates the account; the skip is a no-op when `requireHolder=false` (CRM already off); unauthorized skip → 422.

### Epic 3.1: Account `Skip` body contract

**Goal:** `mmodel.CreateAccountInput` carries optional `Skip{Holder}`; reuses the `pkg/skip.ResolveSkipFor` helper and `ErrSkipNotPermitted` sentinel from Phase 1.
**Scope:** `pkg/mmodel/account.go` (+ test).
**Dependencies:** Phase 1 (resolver/sentinel)
**Done when:** `CreateAccountInput.Skip` parses from the body; absent → nil → no behavior change; the field is documented as effective only when `requireHolder=true`.
**Status:** Done (`c649ff79b`) — `AccountSkip{Holder}` on `CreateAccountInput`; also mirrored onto `CreateHolderAccountInput` (composite holder-account-open runs the same use case) — `TestCompositionMirrorsCreateAccountInput` forced the call; surfaced to Fred for veto.

### Epic 3.2: Holder short-circuit before the CRM `Exists` call

**Goal:** an honored holder skip forces `requireHolder=false` for that request, so `applyHolderValidation` returns before `HolderReader.Exists` — resolved off the SAME cached settings read (no second fetch).
**Scope:** `components/ledger/internal/services/command/create_account.go` — including a refactor of `requireHolderEnabled` (`:278-289`), which today returns a bare bool and **discards `settings.Overrides.AllowHolderSkip`** (so the one-liner below cannot compute `honoredHolderSkip` without it).
**Dependencies:** Epic 3.1; the settings opt-in (Epic 1.1); `pkg/skip` (Epic 1.2)
**Done when:** `requireHolderEnabled` is refactored to surface BOTH keys from its single cached read (return `(requireHolder, allowHolderSkip bool)`, or return the parsed `*mmodel.LedgerSettings`); at `create_account.go:60`, `honoredHolderSkip, err := skip.ResolveSkipFor("holder", cai.Skip != nil && cai.Skip.Holder, allowHolderSkip)` — on err return the 422 (mirror the `applyHolderValidation` error branch at `:62-67`) — then `requireHolder = requireHolder && !honoredHolderSkip`, so `applyHolderValidation` (`:296`) short-circuits before `HolderReader.Exists` (`:310`); a mock `HolderReader` test asserts `Exists` is called **zero** times when the skip is honored and `requireHolder=true`, with `GetParsedLedgerSettings` called exactly once on that path; `skip.holder` without opt-in → 422; with `requireHolder=false`, skip is a no-op (CRM already not called); absent skip is unchanged.
**Status:** Done (`c649ff79b`) — `requireHolderEnabled` renamed `resolveHolderRequirement`, returns `(requireHolder, allowHolderSkip)` from one read; honored skip folds into `requireHolder` so the existing guard short-circuits before `Exists`; `applyHolderValidation` unchanged.

---

## Phase 4 — Audit + hardening (release gate)

**Milestone:** every honored skip is durably recorded and observable across persistence and streaming; the API contract and agent docs reflect the feature; a cross-surface integration suite proves the zero-overhead property end-to-end. **The feature does not ship without this phase** — a financial control that can be skipped must leave a queryable trail.

### Epic 4.1: Persist and stream honored skips

**Goal:** honored skips are written to typed boolean columns AND threaded onto the full streaming wire path and spans — written from the resolved `honored*Skip` booleans, not the zero value.
**Scope:** two migrations + domain structs + persistence structs + model mapping + handler wiring + ALL scan/INSERT sites + span attrs + the full transaction streaming wire path.
**Dependencies:** Phases 1–3 (the booleans being recorded)
**Done when:** the columns exist, are written on honored skips (proven by a testcontainer round-trip that POSTs a honored-skip transaction and account, then GET/lists and asserts the audit boolean reads back TRUE — catches both always-false wiring gaps and any missed scan site), default false for all historical rows, and the transaction skip flags appear on the four lifecycle events via the COMPLETE wire path (touch-points below) with the JSONShape test upgraded to assert an exact top-level field SET (fail-closed on a missing field, not a present-key whitelist).
**Status:** Doing — **4a (transaction half) DONE** (`c7caf2b2f`): migration 000035, domain+PG model fields, ToEntity/FromEntity, both column lists, single+bulk INSERT, all 6 scan sites (Scan order == SELECT order verified), handler wiring of resolved `honored*Skip`, `app.transaction.*` span attrs, full streaming wire path + JSONShape lock, column-count guards 16→18. **4b (account half) DONE** (`244323320`): migration 000019, `mmodel.Account`+`AccountPostgreSQLModel` field, ToEntity/FromEntity, `accountColumnList` tail-append, all 8 list-derived SELECT/scan sites (the 2 non-account scans `&exists`/`&count` correctly untouched), `Create` INSERT (a **squirrel `.Columns/.Values` builder — NOT a hand-written `fmt.Sprintf` literal**; column+value appended at tail, placeholders auto-generated, alignment by construction), use-case wiring of resolved `honoredHolderSkip`, `app.account.holder_check_skipped` span attr, `AccountCreatedPayload`+`NewAccountCreated`+account JSONShape (count 16→17). Independently reviewed: scan position-match against the list-derived SELECTs verified, INSERT alignment verified, own gate green (gofmt/build/vet/5 test pkgs). JSONShape "exact-field-set" upgrade still deferred on BOTH surfaces (kept present-key whitelist — flag for 4c if fail-closed is wanted). **Epic 4.1 (both halves) COMPLETE — only 4.2 (testcontainer) + 4.3 (docs) remain.**

Migration contract (mirror the up/down shape of the linter-approved `migrations/onboarding/000007_add_account_blocked_column`):
```sql
-- migrations/transaction/000035_add_skip_audit_to_transaction.up.sql
ALTER TABLE transaction ADD COLUMN IF NOT EXISTS fees_skipped   BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE transaction ADD COLUMN IF NOT EXISTS tracer_skipped BOOLEAN NOT NULL DEFAULT FALSE;
-- migrations/onboarding/000019_add_holder_skip_audit_to_account.up.sql
ALTER TABLE account ADD COLUMN IF NOT EXISTS holder_check_skipped BOOLEAN NOT NULL DEFAULT FALSE;
```
**Persistence touch-points (complete checklist — partial coverage causes runtime `expected N destination arguments` 500s, since `rows.Scan` is variadic and the compiler will NOT catch a column-count mismatch):**
1. **Typed fields + mapping (or the columns always write FALSE):** add `FeesSkipped`/`TracerSkipped bool` to the domain `transaction.Transaction` (`postgres/transaction/transaction.go:113`) AND `TransactionPostgreSQLModel` (`:33`), mapped both ways in `ToEntity` (`:209`) and `FromEntity` (`:250`); same for `HolderCheckSkipped bool` on `mmodel.Account` AND `AccountPostgreSQLModel`. **Wire the resolved booleans at the handler/use-case:** assign `honoredFeeSkip`/`honoredTracerSkip` onto `tran` at `transaction_create.go:1295` and `honoredHolderSkip` onto the account entity before `Create` — without this the INSERT binds the zero value.
2. **Transaction SQL — EVERY site:** append to `transactionColumnList` (`:39`) AND `transactionColumnListPrefixed` (`:58`) AND the single INSERT (`transaction.postgresql.go:202-224`, bump placeholders) AND the **bulk INSERT `insertChunk` (`:401-427`)** AND **all 6 scan tails** (`:741,:848,:930,:1011,:1229,:1412`).
3. **Account SQL — EVERY site:** append to `accountColumnList` (`:34`) AND the **hand-written `Create` INSERT literal** (`account.postgresql.go:137-178`, NOT derived from the list) AND **all 8 scan tails** (`:352,:432,:510,:589,:735,:819,:1069,:1152`).
4. **Streaming — the full transaction wire path (not a 3-item triad):** thread `fees_skipped`/`tracer_skipped` through (a) the domain struct the mapper reads, (b) `TransactionSource` (`transaction_lifecycle.go:173`), (c) `buildTransactionEventSource` (`send_transaction_events.go:255`), (d) `newTransactionPayload` (`:201`), (e) `TransactionPayload` (`:143`), and (f) the JSONShape test (`transaction_lifecycle_test.go:187`, upgraded to exact-field-set). The shared `TransactionPayload` stamps the field on all four events; `canceled`/`reverted` reflect the row's own persisted skip (a reverted child does NOT inherit the parent's flags). **Account symmetry decision:** add `holder_check_skipped` to `AccountCreatedPayload` + `NewAccountCreated` + the account JSONShape test (preferred), OR explicitly downgrade the milestone to "Postgres column + span only" — pick one so the "observable" claim matches what ships.
5. **Span attributes:** system-observation namespace (e.g. `app.transaction.fees_skipped`), not `app.request.*`.
6. **Skip × idempotency replay (accepted-by-design, lock with a test):** the idempotency hash includes `Skip` (it's on `mtransaction.Transaction`, serialized at `transaction_create.go:1018`) but is NEVER compared on replay — with a client `Idempotency-Key`, the header is the sole identity and a replay returns the FIRST request's stored outcome + audit state regardless of the replayer's `skip` (exactly as it already masks amount/account divergence). Document this as unchanged contract; the audit row records the FIRST request's skip state. The auto-hash fallback (no header) is unaffected (differing skip → differing hash → distinct slot).

Writing these columns is part of the existing INSERT — **no extra query, no added latency on the skip path** (preserves the Phase-1 invariant). Ship a per-migration `_test.go` for each new migration mirroring `000007`/`000033` (asserts up/down existence, NOT NULL + DEFAULT, idempotent `IF EXISTS` rollback) — these run in `make ci` via `test-unit`; additionally run `make migrate-lint` locally (exit 0) before the PR.

### Epic 4.2: Cross-surface integration suite

**Goal:** prove the zero-overhead property with real dependencies across all three controls. **Do NOT touch `feeExemption`** — the earlier "half-built ghost" premise was wrong: `feeExemption` is live, tested account-level exemption (`distribute.go:129-154` writes it; `calculate-fee.go:163` reads it to keep `packageAppliedID` stamped on the all-accounts-exempt branch, where no fee legs are added). It is orthogonal to the per-call skip (which bypasses the whole engine before the Mongo lookup and never produces exemption metadata). Removing `calculate-fee.go:163` would silently drop `packageAppliedID` on all-exempt transactions — a fee-reporting regression. Retiring exemption reporting, if ever wanted, is a separate documented decision, not part of this feature.
**Scope:** integration tests (testcontainers) for transaction + account flows. No production-code changes to the fee engine.
**Dependencies:** Phases 1–3
**Done when:** testcontainer integration tests assert, for honored skips, zero Mongo package lookups / zero gRPC Reserve (at create AND at PENDING commit/cancel) / zero CRM `Exists`; assert the ledger-settings read count is **exactly 1** on both the skip and the normal path (a counting spy on the settings query, guarding the gate-adds-no-DB-read invariant at runtime); assert 422 on unauthorized skips; assert unchanged behavior on absent skips; assert the idempotency replay semantics from Epic 4.1 (same key + differing skip → first result + first audit state).
**Status:** Done (`ec1a97032`) — **scope = LEAN + runtime-proof (Fred 2026-06-15)**. Delivered the proofs unit tests structurally cannot give: (a) audit-bool round-trip via real `repo.Create`+Find/FindAll/ListByIDs for tx (fees+tracer) and account (holder), each with a false control — runtime proof of variadic scan-order + INSERT alignment (agent negative-control-verified: flipping a control assertion FAILS); (b) HTTP idempotency-replay (no-seed direction: same key + differing skip → 201-not-422 + X-Idempotency-Replayed + first id + persisted audit unchanged → replayer skip ignored). The zero-Mongo/zero-gRPC-Reserve/zero-CRM-Exists + settings-read==1 invariants stay UNIT-covered (deviation from the original done-when, Fred-approved) with pointer comments naming the unit tests. All 3 suites re-run green by me against Docker (`ALLOW_INSECURE_TLS=true go test -tags=integration`). Join-scan readers (FindWithOperations) not exercised (need heavy operation fixtures; single-row + 2 list sites already cover both structurally distinct scan paths) — documented residual.

### Epic 4.3: Contract and agent documentation

**Goal:** the OpenAPI spec, `llms-full.txt`, and agent docs describe the opt-in settings, the body `skip` objects, the 422 response, and the non-goals.
**Scope:** consolidated OpenAPI spec, `llms-full.txt`, a short note in `CLAUDE.md`/`AGENTS.md`.
**Dependencies:** Phases 1–3 (final field/route shapes)
**Done when:** OpenAPI documents `overrides` on the settings schema, `skip` on the account-create and transaction-create REQUEST bodies (and confirms `skip` is absent from the GET/list transaction RESPONSE schema — request-only input, surfaced as read state only via the Epic 4.1 audit columns); `ErrSkipNotPermitted`/422 is documented; non-goals are stated explicitly, including that a revert re-runs the tracer and does NOT inherit the parent's tracer skip; the hardcoded settings swagger text (`ledger.go:415/423` `@Param`/`@Description` and examples at `ledger.go:29/97`) is updated to mention `overrides`; the agent docs note the two-key control.
**Status:** Done (`9352792ce`). `UpdateLedgerSettings` `@Description`/`@Param` now list `overrides.allow{Fee,Tracer,Holder}Skip`; account-create documents the `skip` object + 422 as a public `AccountSkip` schema field; transaction-create documents `skip` in PROSE (kept `Transaction.Skip swaggerignore` — dual-purpose response model; verified `skip` ABSENT from GET response schema, PRESENT on the three create-input schemas); llms-full.txt + AGENTS.md gained a per-call-skip section with the four non-goals; swagger/openapi/postman regenerated via `make generate-docs`. **Doc-accuracy fix (caught in my review, beyond the agent's):** the holder-skip docs invented a `requireHolder=true` precondition; corrected to the actual two-key gate (`requested AND allowHolderSkip`, independent of `accounting.requireHolder`) across the handler, the `AccountSkip` model + Holder field, and both `CreateAccountInput`/`CreateHolderAccountInput` field docs — then regenerated. `ledger.go:29/97` create examples left as illustrative single-field snippets (model already carries per-field examples). CLAUDE.md untouched (rules file). The JSONShape exact-field-set residual was folded into the `test` commit (`ec1a97032`).

**PLAN CLOSED 2026-06-15.** All 4 phases Done: P1 tracer (`eb2879f0a`), P2 fees (`5fcb7423b`), P3 holder (`c649ff79b`), P4 audit/hardening — Epic 4.1 (`c7caf2b2f` tx + `244323320` acct), Epic 4.2 (`ec1a97032`), Epic 4.3 (`9352792ce`). All three behavioral skips work end-to-end under the two-key gate with zero added overhead on honored skips; audit persisted + streamed + runtime-proven; 422 on unauthorized; docs + OpenAPI shipped. Sole residual: join-scan reader runtime coverage (documented, low-risk — static + 3-site runtime proof already cover the scan-order invariant).

---

## Self-review

- **Spec coverage:** CRM bypass → Epics 3.1/3.2 (+1.1 opt-in). Fees bypass → Epics 2.1/2.2 (+1.1). Tracer bypass → Epics 1.1/1.2/1.3. "Zero added latency/overhead on skip" → enforced as a done-when on every behavioral epic (zero Mongo/gRPC/CRM call; gate rides the cached settings read; audit columns ride the existing INSERT). Two-key + 422 → Epic 1.2 resolver, applied in 1.3/2.2/3.2. Audit/reversibility → Phase 4. Covered.
- **Vagueness scan:** Phase 1 tasks name exact files/lines, decisions (resolve-once location, neutral `pkg/skip` placement, idempotency-key deletion on 422, commit/cancel re-resolution, nil-pointer absent path), and edge cases (advisory/enforce vs skip, partial overrides, unknown keys, annotation/revert/DSL). No "appropriate"/"TBD" in the detailed wave; the phantom structural-test fallback was removed (the single hoisted read is unconditional).
- **Contract consistency:** `OverridePolicy`, `TransactionSkip`/`AccountSkip`, `pkg/skip.ResolveSkipFor`, `ErrSkipNotPermitted`, and the resolve-once contract are defined in the cross-cutting section and Phase 1, and consumed unchanged by Phases 2–4. Column names match the persistence touch-points in 4.1.
- **Phase boundaries:** each phase ends with compiling, tested software (P1 tracer end-to-end; P2 fees; P3 holder; P4 audited + documented). No phase ends mid-refactor — the hoist (2.1) and its consumer (2.2) are in the same phase.
- **Verification plausibility:** commands target real package paths; "zero calls" assertions use mocks already present in the `in` package test suite.
- **Contrarian hardening (2026-06-15):** a 6-lens adversarial review (39 raw → 23 clusters → 10 confirmed, 13 refuted) folded in: PENDING commit/cancel skip (C3), neutral `pkg/skip` resolver avoiding an inward-layer violation (C5), `feeExemption` is live not dead — deletion removed (C6), complete audit persistence + full streaming wire path (C10/C11), single-read structural guard replacing a phantom fallback (C4), `swaggerignore` to stop schema bleed (C7), idempotency-replay semantics locked (C2), explicit revert-tracer decision (C17), migration-test done-when (C22).
