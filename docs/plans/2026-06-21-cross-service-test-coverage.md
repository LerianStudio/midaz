# Cross-Service Test Coverage Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave: implement the
> detailed phase → user checkpoint → detail the next phase → implement → repeat),
> or ring:running-dev-cycle for the full subagent-orchestrated workflow.
> This document is the living source of truth — task elaboration for later
> phases is written back into it during execution.

**Goal:** Extend the `tests/e2e` black-box suite to cover the cross-seam surfaces of the consolidated ledger — fees and CRM (now in-process) and the ledger↔tracer integration — exercising each boundary's correctness, money-direction, state-machine, and integrity behavior, and turning each gap found into a fix or a documented finding.

**Architecture:** All tests are build-tagged `e2e` Go tests under `tests/e2e/`, driving the live `make up` stack over HTTP (the harness in `tests/e2e/harness_test.go`). The suite self-gates: a test skips when its required capability (stack reachable, tracer wired, broker present, multi-tenant enabled) is absent. New capabilities (tracer-wired detection + tracer limit-rule seeding, broker, MT) are added as harness helpers behind `requireX(t)` gates mirroring the existing `requireStack`/`requireTracer` pattern. Fees and CRM are in-process but retain real seams (Mongo stores, the synchronous fee seam in `transaction_create.go`, the CRM holder lookup on account create, per-`(org,ledger)` fee cache, the authz namespace); the tests target those seams, not just the HTTP surface.

**Tech Stack:** Go `testing` (no framework), stdlib `net/http` client, `github.com/google/uuid`; the existing `tests/e2e` harness; k6 only where a load dimension is relevant (out of scope here — this plan is correctness coverage). Tracer-wired tests reuse the documented local wiring (tracer compose up + `TRACER_BASE_URL`/`TRACER_TRANSPORT=rest` on the ledger). Streaming uses a local Redpanda/Kafka broker; MT uses `MULTI_TENANT_ENABLED=true` + tenant-manager.

## Phase Overview

| Phase | Milestone | Epics | Tier coverage | Status |
|-------|-----------|-------|---------------|--------|
| 1 | Fees + CRM in-process correctness fully covered against the current stack; no new infra | 1.1, 1.2, 1.3 | Tier 1 (#4 requireHolder, #5 deductible/multi-fee) + Tier 2 (all) | **Done** (committed `fa27b5222`, live-verified) |
| 2 | Ledger↔tracer integration proven end-to-end behind a tracer-wired capability gate | 2.1, 2.2, 2.3, 2.4 | Tier 1 (#1 fee×tracer, #2 denial, #3 lifecycle) | **Done** (committed `0249321dd`, live-verified) |
| 3 | Each infra-gated seam covered behind a capability flag that skips cleanly when absent | 3.1, 3.2, 3.3, 3.4, 3.5 | Tier 3 (all) | Detailed |

**Why this ordering (not tier-numbered):** dependency/infra first. Phase 1 needs zero new infra and carries the highest density of money-direction bugs, and its fixture builders (multi-fee packages, requireHolder ledgers, holder+instrument) are reused by Phase 2. Phase 2 introduces one new harness capability (wire + seed tracer limits) and holds the single highest-risk cross-seam test (fee×tracer amount) — it is the program's risk front-load, gated behind that capability. Phase 3's seams each need heavier env (broker, MT, certs, auth, async) and are independent. **If you want maximal risk-first, pull Epic 2.4 (fee×tracer) ahead — flagged in its Dependencies.**

**Calibration-first principle:** several fee/CRM contracts are unverified at plan time (deductible-fee creation currently 400s with an apparently-valid payload; multi-fee accumulation; min/max eligibility boundaries; instrument referential rules; holder cascade). Each detailed task begins by calibrating the live contract with concrete requests, then asserts the observed contract. A divergence from the documented/expected contract is itself a deliverable — record it as a finding (Fn) in `scripts/k6/results/BENCHMARKS.md` and, if it is a 5xx-on-bad-input or an integrity/money defect, fix it.

---

## Phase 1 — Fees & CRM in-process correctness

### Epic 1.1: Fee correctness matrix

**Goal:** the fee engine's money math is proven across the fee-model matrix — deductible vs additive direction, multi-fee priority and `maxBetweenTypes`, and eligibility boundaries — with double-entry reconciliation on every case.
**Scope:** `tests/e2e/` (new `fees_matrix_test.go`, helper additions to `harness_test.go`); exercises `components/ledger/pkg/feeshared/model` + the fee seam in `components/ledger/internal/adapters/http/in/transaction_create.go` + `internal/services/fees`.
**Dependencies:** none.
**Done when:** deductible and additive fee directions, a multi-fee package with priority ordering, `maxBetweenTypes`, and min/max eligibility boundaries each have a passing assertion (or a recorded finding where the live contract diverges); every transaction case reconciles source/destination/fee balances to the cent.
**Status:** Pending

#### Task 1.1.1: Deductible vs additive fee direction

- [x] Done

**Context:** The suite covers only additive fees (`isDeductibleFrom:false` → sender pays principal + fee; `tests/e2e/flow_test.go` `TestFullLedgerFlow`, `tests/e2e/positive_test.go` `TestPercentageFee`). `Fee.IsDeductibleFrom` (`components/ledger/pkg/feeshared/model/package.go:63`) flips who bears the fee. `ValidateFees` (`create_package_input.go:41`) enforces: a deductible fee's `ReferenceAmount` must be `originalAmount`. Calibration during planning showed a deductible package built with `{applicationRule:"flatFee", calculations:[{type:"flat","10"}], referenceAmount:"originalAmount", isDeductibleFrom:true, priority:1}` is **rejected at creation with HTTP 400** — an unverified corner.

**Implementation vision:** First calibrate: POST the deductible package above and capture the exact 400 body (error code + fields). Decide from the error which constraint is unmet, and find a *valid* deductible payload (vary `applicationRule`/`priority`/`referenceAmount` per `ValidateFees` rules until 201). If no valid deductible payload exists with otherwise-legal inputs, that is a finding (record as F5: "deductible fee package cannot be created" or "deductible creation misclassifies a valid payload as 400") — and the test asserts the *current* rejection behavior so it is pinned, not silently broken. If a valid deductible package creates, assert the money direction: for a 100 transfer with a deductible flat 10 fee, the **recipient nets 90**, the **fee account gets 10**, the **sender is debited 100** (fee comes out of the transfer), and top-level `amount` reflects the contract (likely `100`, not `110`). Contrast in the same test with the additive case (already known: sender debited 110, recipient 100, fee 10). Edge cases: deductible fee larger than the transfer amount (expect a 4xx/422, not a negative recipient credit — assert no negative balance); deductible with `referenceAmount:"afterFeesAmount"` (per `ValidateFees` must be rejected → assert the validation error).

**Files:**
- Create: `tests/e2e/fees_matrix_test.go`
- Modify: `tests/e2e/harness_test.go` (add `createDeductibleFeePackage` helper if a valid payload exists; reuse `createFeePackage`)

**Verification:** `go test -tags e2e -run TestFeeDirection -count=1 -v ./tests/e2e/...` — additive and deductible cases pass with reconciled balances, or the deductible-creation finding is asserted and recorded.

**Done when:** the additive and deductible directions are each asserted (or the deductible-creation gap is pinned as a finding with the exact error), and the over-large-deductible edge produces no negative balance.

#### Task 1.1.2: Multi-fee priority ordering and maxBetweenTypes

- [x] Done

**Context:** A package's `fees` is a `map[string]Fee`; each fee has a `Priority` (`package.go:62`) and a `CalculationModel.ApplicationRule` one of `maxBetweenTypes|flatFee|percentual` (`package.go:99`). `ValidateFees` requires `priority==1` fees to use `referenceAmount:"originalAmount"`. Calibration verified `maxBetweenTypes` with `[percentage 10%, flat 5]` on a 100 transfer charges **10** (the max), `amount` 110. Untested: a package with **multiple distinct fees** at different priorities (e.g. priority 1 flat 10 + priority 2 percentage 5%) — application order and whether later fees compute on `originalAmount` vs `afterFeesAmount`.

**Implementation vision:** Two sub-cases. (a) `maxBetweenTypes`: assert the verified contract — `[percentage 10, flat 5]` on 100 → fee 10, recipient 100, fee account 10, sender 110. Add the inverse (`[percentage 1, flat 50]` → fee 50) to prove it is genuinely the max, not always-percentage. (b) Multi-fee: build a package with two fees, priority 1 (flat 10, `originalAmount`) and priority 2 (percentage 5%, `referenceAmount:"afterFeesAmount"`). Calibrate the resulting operations first (record each fee leg's amount and credit account), then assert: priority-1 fee = 10 on the original 100; priority-2 fee = 5% computed on the contract's reference (originalAmount=5, or afterFeesAmount over 100+10=5.5 — pin whichever the engine produces); total fee = sum; balances reconcile. Edge case: two fees crediting the **same** account (assert the credits aggregate correctly, no lost leg) and crediting **different** accounts (assert each gets its leg).

**Files:**
- Modify: `tests/e2e/fees_matrix_test.go`
- Modify: `tests/e2e/harness_test.go` (add `createMultiFeePackage(fees ...feeSpec)` builder taking per-fee priority/rule/type/value/credit)

**Verification:** `go test -tags e2e -run 'TestFeeMaxBetweenTypes|TestFeeMultiPriority' -count=1 -v ./tests/e2e/...` — both pass with reconciled balances against the calibrated contract.

**Done when:** `maxBetweenTypes` (both directions) and a two-fee priority package are asserted with the exact per-leg amounts the engine produces, and the same-account credit aggregation edge is covered.

#### Task 1.1.3: Fee eligibility boundaries (min/max amount) and waivedAccounts

- [x] Done

**Context:** `CreatePackageInput` carries `MinAmount`/`MaxAmount` (`create_package_input.go:25-26`) and `WaivedAccounts *[]string` (`:27`). A package applies only to transactions within `[min,max]`; waived accounts are exempt. The suite always used `min:0,max:1e8` (everything in range) and never set waivedAccounts.

**Implementation vision:** Create a package with `min:50, max:200` and a flat 10 fee. Calibrate first (record fee-applied vs not at each amount), then assert: transfer of 49 → **no fee** (below min, `amount`=49, no fee leg); 50 → fee applies (boundary inclusive — pin whether `>=min` or `>min` from the observed result and assert it); 200 → applies; 201 → **no fee**. Name the boundary decision explicitly from calibration (inclusive vs exclusive) — do not assume. Second: a package over `[0,1e8]` with `waivedAccounts:["@waived"]`; transfer from a normal account → fee applies; transfer whose fee-bearing account is `@waived` → **no fee**. Calibrate which party the waiver keys on (source vs destination vs fee-bearing) and assert that. Edge case: a transfer exactly at `min` and exactly at `max` (boundary), and a waived account in a multi-leg transfer.

**Files:**
- Modify: `tests/e2e/fees_matrix_test.go`
- Modify: `tests/e2e/harness_test.go` (extend the package builder to accept min/max/waived)

**Verification:** `go test -tags e2e -run 'TestFeeBoundaries|TestFeeWaived' -count=1 -v ./tests/e2e/...` — eligibility boundaries and waiver behave per the calibrated contract.

**Done when:** below/at/above min and max are asserted with the boundary inclusivity pinned, and waivedAccounts exemption is asserted with the keyed party identified.

### Epic 1.2: Fee package lifecycle & scoping

**Goal:** the fee cache invalidates correctly on package change, and segment/transaction-route scoping applies the right package to the right transaction.
**Scope:** `tests/e2e/`; exercises the per-`(org,ledger)` fee cache (`internal/services/fees`), the Mongo fee store (`internal/adapters/mongodb/fees`), and segment/route scoping in `CreatePackageInput.SegmentID`/`TransactionRoute`.
**Dependencies:** Epic 1.1 (fee assertion + builder helpers established there).
**Done when:** a package update is reflected on the next transaction (no stale cache); a segment-scoped package applies only to transactions in that segment; a route-scoped package applies only to its route; reconciliation holds in every case.
**Status:** Pending

#### Task 1.2.1: Fee package cache invalidation on update

- [x] Done

**Context:** Enabled packages are cached per `(org,ledger)` and (per project notes) invalidated on package create/update; the store is Mongo while the transaction runs on Postgres — a cross-store consistency seam. No test exercises an update-then-transact sequence.

**Implementation vision:** Create a flat-10 package; transfer 100 → assert fee 10 (warms the cache). PATCH/PUT the package to flat-20 (find the update route — check `routes.go`/postman for `PUT|PATCH .../packages/:id`; calibrate the update payload and success status). Immediately transfer 100 again → assert fee **20**, not a stale 10. Then disable the package (`enable:false` via update) and transfer → assert **no fee**. Edge cases: update that changes `enable` true→false→true (re-enable reflected); update racing a transaction is out of scope (single-threaded e2e). If the cache does NOT invalidate (stale fee charged), that is a finding (Fn: "fee cache not invalidated on package update") — assert the bug explicitly and record it.

**Files:**
- Modify: `tests/e2e/fees_lifecycle_test.go` (create)
- Modify: `tests/e2e/harness_test.go` (add `updateFeePackage` helper once the update route/payload is calibrated)

**Verification:** `go test -tags e2e -run TestFeeCacheInvalidation -count=1 -v ./tests/e2e/...` — the post-update transaction charges the new fee.

**Done when:** value-change, disable, and re-enable updates are each reflected on the next transaction, or a stale-cache finding is pinned.

#### Task 1.2.2: Fee scoping by segment and transaction route

- [x] Done

**Context:** `CreatePackageInput.SegmentID` and `TransactionRoute` (`create_package_input.go:22,24`) scope a package. Accounts carry a `segmentId`; transactions carry a `routeId`/`route`. No test verifies that a scoped package applies only to matching transactions.

**Implementation vision:** Segment case: create two segments (calibrate the create-segment route/payload from `routes.go:99`/postman), two accounts each in a different segment, and a package scoped to segment A. Transfer from the segment-A account → fee applies; transfer from the segment-B account → **no fee**. Pin from calibration whether scoping keys on the source or destination account's segment. Route case: create a transaction route (calibrate `routes.go` `transaction-routes` create), a package scoped to it, and assert a transaction carrying that `routeId` gets the fee while one without it does not. Edge case: a package with both a segment and a route scope (assert AND semantics — both must match — or pin whatever the engine does).

**Files:**
- Modify: `tests/e2e/fees_lifecycle_test.go`
- Modify: `tests/e2e/harness_test.go` (add `createSegment`, `createTransactionRoute` helpers)

**Verification:** `go test -tags e2e -run TestFeeScoping -count=1 -v ./tests/e2e/...` — scoped packages apply only to matching transactions.

**Done when:** segment-scoped and route-scoped application is asserted with the keyed party/field identified, and the combined-scope semantics are pinned.

### Epic 1.3: CRM enforcement & integrity

**Goal:** the ledger↔CRM seam is proven — holder-requirement enforcement with the two-key skip, instrument referential validation, atomic account+instrument composition, and holder soft-delete ownership integrity.
**Scope:** `tests/e2e/` (new `crm_enforcement_test.go`); exercises `internal/services/command/create_account.go:61` (`resolveHolderRequirement`), the CRM holder/instrument services (`internal/crm`), and `internal/adapters/http/in/composition.go`.
**Dependencies:** none (independent of fees epics).
**Done when:** `requireHolder:true` rejects an account with no/unknown holder and honors `skip.holder` only under `allowHolderSkip`; instrument creation validates account/ledger references (422 on bogus); the composition endpoint's account+instrument creation is atomic; a soft-deleted holder's ownership integrity is asserted.
**Status:** Pending

#### Task 1.3.1: requireHolder enforcement and the two-key holder skip

- [x] Done

**Context:** `AccountingValidation.RequireHolder` (`pkg/mmodel/settings.go:66`) defaults false; all current tests use it false (`tests/e2e/harness_test.go` `createLedger`). `create_account.go:61` resolves `requireHolder` + `allowHolderSkip`, then `skip.ResolveSkipFor("holder", ...)`. The two-key model: a `skip.holder:true` is honored only when `overrides.allowHolderSkip` is set, else 422. `AccountSkip.Holder` is `json:"holder,omitempty"` (`account.go:117`) — the F1 fix means explicit `false` is now accepted.

**Implementation vision:** Add a `createLedger` variant (or extend the existing one) that sets `settings.accounting.requireHolder:true` with a complete settings block (recall: partial settings → 0176). Cases: (a) `requireHolder:true`, create account with no `holderId` → calibrate then assert the rejection (likely 422; pin the code). (b) same ledger, account referencing a **non-existent** holderId → assert rejection. (c) account referencing a **valid** holder (create holder first) → 201. (d) `requireHolder:true` + `overrides.allowHolderSkip:true`, account with `skip.holder:true` and no holder → 201 with `holderCheckSkipped:true` (assert that response field, seen in earlier responses). (e) `requireHolder:true` + `allowHolderSkip:false`, `skip.holder:true` → 422 (skip requested without opt-in). Edge case: `skip.holder:false` under requireHolder (must be accepted and still enforce the holder — F1 regression interaction).

**Files:**
- Create: `tests/e2e/crm_enforcement_test.go`
- Modify: `tests/e2e/harness_test.go` (add `createLedgerRequiringHolder` / extend settings builder)

**Verification:** `go test -tags e2e -run TestRequireHolder -count=1 -v ./tests/e2e/...` — all five cases behave per the two-key model.

**Done when:** the no-holder, unknown-holder, valid-holder, skip-with-opt-in, and skip-without-opt-in cases are asserted, plus the `skip.holder:false` interaction.

#### Task 1.3.2: Instrument referential validation and atomic composition

- [x] Done

**Context:** Instrument create is `POST /v1/organizations/{org}/holders/{holderId}/instruments` with `{accountId, ledgerId, ...}` (`crm_routes.go:49`); project notes say Epic 4.3 shipped instrument referential validation (422). The composition endpoint `POST .../ledgers/{id}/holders/{holderId}/accounts` (`composition.go`) opens an account and *optionally* creates an instrument in one call, returning `{account, instrument}`. Current tests cover only the happy instrument link and account-only composition.

**Implementation vision:** Referential: create a holder, then POST an instrument with (a) a bogus `accountId` (random UUID) → calibrate then assert 422 (referential validation); (b) a bogus `ledgerId` → assert 422; (c) an `accountId` that exists but in a **different ledger** than `ledgerId` → assert the cross-ledger mismatch is rejected; (d) valid account+ledger → 201 (regression of existing). Composition atomicity: drive the holder-owned-account endpoint with instrument fields that are **invalid** (so the instrument leg fails after the account leg) → assert the whole call fails AND no orphan account is left (query the holder's accounts / the alias afterward to confirm absence). Pin from calibration whether the endpoint is transactional; if an orphan account survives a failed instrument leg, that is a finding (Fn: "non-atomic holder-account composition leaves orphan account").

**Files:**
- Modify: `tests/e2e/crm_enforcement_test.go`
- Modify: `tests/e2e/harness_test.go` (helper to drive composition with instrument fields; calibrate the instrument-in-composition body shape from `composition.go`/postman)

**Verification:** `go test -tags e2e -run 'TestInstrumentReferential|TestCompositionAtomicity' -count=1 -v ./tests/e2e/...` — bogus references 422; failed composition leaves no orphan (or the orphan finding is pinned).

**Done when:** the three bad-reference cases 422, the valid case 201, and the atomicity behavior is asserted (transactional, or the orphan finding recorded).

#### Task 1.3.3: Holder soft-delete ownership integrity

- [x] Done

**Context:** Holders/accounts/instruments use soft delete (`DeletedAt`, status `DELETED`). A holder owns accounts (via `holderId`) and instruments. The behavior when deleting a holder that still owns accounts/instruments is unspecified by the current suite.

**Implementation vision:** Create a holder, a holder-owned account, and an instrument. Calibrate the delete-holder route (`DELETE .../holders/:id` — confirm from `routes.go`/postman) and its status. Then assert the chosen integrity contract, pinned from calibration: either (a) delete is **blocked** while the holder owns active accounts/instruments (assert 409/422 + the holder still readable), or (b) delete **cascades/soft-deletes** dependents (assert the instrument is then DELETED and the account's state), or (c) delete succeeds and leaves accounts referencing a deleted holder (assert the account is still usable or report the dangling reference). Whatever the live behavior, the test pins it and names which contract holds; a silent dangling-reference with no documented contract is a finding. Edge case: deleting an instrument while its holder is active (independent soft-delete) → assert the holder is unaffected.

**Files:**
- Modify: `tests/e2e/crm_enforcement_test.go`
- Modify: `tests/e2e/harness_test.go` (add `deleteHolder`, `deleteInstrument` helpers)

**Verification:** `go test -tags e2e -run TestHolderDeleteIntegrity -count=1 -v ./tests/e2e/...` — the holder-delete-with-dependents contract is asserted and the instrument independent-delete leaves the holder intact.

**Done when:** the holder-delete-with-owned-dependents behavior is pinned and asserted, and independent instrument soft-delete is covered.

---

## Phase 2 — Ledger↔Tracer integration depth

### Epic 2.1: Tracer-wired harness capability

**Goal:** the suite can run tracer-integration tests against a ledger actually wired to a running tracer, can seed limit rules on the tracer, and gates those tests so they skip cleanly when the ledger is not wired.
**Scope:** `tests/e2e/` harness; the documented local wiring (tracer compose up + `TRACER_BASE_URL`/`TRACER_TRANSPORT=rest`); the tracer limit/rule admin API.
**Dependencies:** none (foundation for 2.2–2.4).
**Done when:** a `requireTracerWired(t)` gate detects whether the ledger forwards reserves to the tracer (skips otherwise), and a `seedLimitRule(...)` helper creates an active limit rule on the tracer and confirms it is queryable; a smoke test reserves through the ledger and observes the tracer participated.
**Status:** Pending

### Epic 2.2: Tracer denial and fail-posture from the ledger

**Goal:** a ledger transaction that exceeds a seeded tracer limit is denied end-to-end, and `failPosture` open vs closed behaves correctly when the tracer is unavailable.
**Scope:** `tests/e2e/`; ledger `tracer.mode:enforce`, seeded limit rules, tracer up/down.
**Dependencies:** Epic 2.1.
**Done when:** an over-limit transaction is rejected by the ledger with the tracer's denial surfaced; with `failPosture:open` a tracer outage lets the transaction proceed (audited skipped), with `failPosture:closed` it is rejected; the error classification is correct (business vs technical).
**Status:** Pending

### Epic 2.3: Reserve lifecycle driven by the ledger

**Goal:** the ledger drives the two-phase reserve correctly — commit confirms, cancel/revert releases, pending (longLived) reservations are held then resolved — with no stuck RESERVED rows.
**Scope:** `tests/e2e/`; ledger transaction lifecycle (commit/cancel/revert) against an enforce ledger; tracer reservation state.
**Dependencies:** Epic 2.1.
**Done when:** a committed transaction's reservations are CONFIRMED, a cancelled/reverted transaction's are RELEASED, a pending transaction holds a longLived reservation that is confirmed on commit / released on cancel, and a reserve retry does not double-count (idempotency across the seam).
**Status:** Pending

### Epic 2.4: Fee × tracer amount interaction (highest-risk cross-seam)

**Goal:** determine and pin whether the tracer reserves the pre-fee or post-fee amount when a ledger has both an enabled fee package and tracer enforce, and assert the limit is enforced against the correct amount.
**Scope:** `tests/e2e/`; a ledger with both an enabled fee package and `tracer.mode:enforce` + a seeded limit; the fee seam (`transaction_create.go`) vs the reserve anchor (`transaction_reservation_anchor.go`) ordering.
**Dependencies:** Epic 2.1, Epic 2.2 (denial fixture), Epic 1.1 (fee builders). **Pull-forward candidate:** if running risk-first, this epic can lead Phase 2 immediately after 2.1.
**Done when:** the reserved amount (original vs fee-inclusive) is determined from observed behavior and asserted; a transaction whose fee-inflated amount crosses a seeded limit is handled correctly (denied if post-fee should count, allowed if not) and the chosen contract is documented; any sub-reservation / limit-bypass-via-fee defect is recorded as a finding and, if it is a money/integrity defect, fixed.
**Status:** Pending

---

## Phase 3 — Infra-gated cross-service seams

> **Scope decision (2026-06-21, Fred — option "author all 5, verify 2"):** the workflow authors all five epics as self-gating tests. The supervisor live-verifies the two infra-feasible epics now (3.5 async — flip `RABBITMQ_TRANSACTION_ASYNC=true` + recreate ledger; 3.1 streaming — start a Redpanda broker + flip `STREAMING_ENABLED`). 3.2/3.3/3.4 ship compiling + self-gating + SKIPping; live-verification is explicitly **deferred to a full-infra env** (CI/devops) and recorded as such — NOT marked verified.
>
> **Cross-cutting design (mirrors Phase 2):** each epic's test lives in its own `tests/e2e/<file>.go` with a unique package-private prefix and its own `requireX(t)` gate + helpers — **no edits to `harness_test.go`**, so the five files compose with zero shared-symbol/parallel-edit conflict. Prefixes: `strm` (3.1), `mt` (3.2), `tgrpc` (3.3), `auth` (3.4), `async` (3.5). All reuse the existing harness helpers: `createOrg(t)`, `createLedger(t,orgID,allowSkips)`, `createAccount(t,f,alias)`, `fund`, `transferBody`, `call`, `mustCreate`, `str`, `availableBalance`, `atoiDecimal`, `newFixture` (`harness_test.go:95-422`); URLs from `ledgerURL()`/`tracerURL()` (`:34-35`, env-overridable). Every gate follows the `requireStack`/`requireTracer` `sync.Once`+probe+`t.Skipf` pattern (`:57`, `:314`).

### Epic 3.1: Streaming events — LIVE-VERIFY: feasible (start Redpanda)

**Goal:** account and transaction lifecycle events are emitted to the broker with the correct CloudEvents wire contract, and emit failures never fail the request.
**Scope:** `tests/e2e/streaming_events_test.go` (new); `pkg/streaming/events`; a local Redpanda/Kafka broker; `STREAMING_ENABLED=true`.
**Dependencies:** none beyond the harness; `requireBroker(t)` gate + a franz-go consumer.
**Done when:** creating an account produces `account.created` (ce-type `studio.lerian.account.created`, subject = account id, payload top-level field set matching the events package); a posted transaction produces `transaction.posted`; with the broker down, the create still returns 201 (IMPORTANT-posture non-propagation).
**Status:** Detailed

**Execution (detailed wave):** prefix `strm`. **Gate** `requireBroker(t)`: TCP-dial `STREAMING_BROKERS` (default `localhost:19092`); `t.Skipf` if closed. **Consumer:** build a `*kgo.Client` (`github.com/twmb/franz-go/pkg/kgo` v1.21.3, already in go.mod) consuming from the start of the relevant topic. **Events** (read exact payload shapes from `pkg/streaming/events/*.go` + their JSONShape `_test.go`): `account.created` → topic `lerian.streaming.account.created`, ce-type `studio.lerian.account.created`, subject = account id, 17 top-level payload fields (incl. `holderCheckSkipped`, `feesSkipped`/`tracerSkipped` are transaction-only); `transaction.posted` → topic `lerian.streaming.transaction.posted`, 16 fields. Emit anchor is post-commit/pre-metadata, IMPORTANT posture (`create_account.go:231`, `send_transaction_events.go:128`). **Holder.created emits NO event** (CRM-internal — `crm/services/create-holder.go` has no emit) — do NOT assert a holder event; optionally assert its absence. **Non-propagation:** with `STREAMING_ENABLED=true` pointed at a DEAD broker address, an account create still returns 201 (`EmitImportant`, `pkg/streaming/emit.go:34-63`, logs Warn, never propagates; bounded by `STREAMING_IMPORTANT_EMIT_TIMEOUT_MS`, default 5s). Note `STREAMING_ENABLED=true` + empty `STREAMING_BROKERS` → NoopEmitter fallback (`bootstrap/streaming.go:72-101`), so the dead-broker test must set a NON-empty unreachable address. **Live-verify (supervisor):** start Redpanda on `infra-network` bound to host `19092`; set `STREAMING_ENABLED=true`, `STREAMING_BROKERS=midaz-redpanda:9092` (container) / consumer reads `localhost:19092` (host), `STREAMING_CLOUDEVENTS_SOURCE=lerian.midaz.ledger`; recreate ledger; pre-provision topics.

### Epic 3.2: Multi-tenant scoping — LIVE-VERIFY: DEFERRED (needs auth + tenant-manager)

**Goal:** tenant isolation holds across the ledger — resources under tenant A are invisible to tenant B.
**Scope:** `tests/e2e/multitenant_test.go` (new); `MULTI_TENANT_ENABLED=true` + tenant-manager + auth; the `MT` code paths.
**Dependencies:** `requireMultiTenant(t)` gate; a tenant-token helper. **Heavy infra — verification deferred.**
**Done when:** with an MT-configured stack, a resource created under tenant A's token is not readable under tenant B's token (404/empty), and the same logical alias can coexist across tenants; absent MT, the test SKIPS.
**Status:** Detailed (live-verify deferred)

**Execution (detailed wave):** prefix `mt`. **Gate** `requireMultiTenant(t)`: skip unless `E2E_MULTI_TENANT=1` (operator asserts an MT+auth stack is up) — there is **no dev bypass header** on user-facing routes (the trusted `x-tenant-id` exists only on the tracer reservation seam, `seamtenant/resolver.go:33`), so MT genuinely requires the auth path. **Token:** an unsigned JWT carrying the `tenantId` claim is accepted because the auth middleware uses `ParseUnverified` (`pkg/net/http/protected_routes.go:68-70`) and a mock plugin-auth approves it — build a `mtTenantToken(tenantID)` helper minting `jwt.SigningMethodNone` tokens with `{sub, tenantId, iat, exp}` (pattern proven in `components/tracer/tests/integration/14a_multitenant_mt_harness_test.go:286-313`). **Test:** create an org under tenant A (Authorization: Bearer tokenA), attempt to GET it under tokenB → expect not-visible; assert tenant DB resolution via context (`tmcore.GetTenantIDContext`). **Live-verify deferred:** requires standing up `MULTI_TENANT_ENABLED=true` + `PLUGIN_AUTH_ENABLED=true` + a mock plugin-auth (httptest) + mock tenant-manager (`MULTI_TENANT_URL`/`MULTI_TENANT_SERVICE_API_KEY`) + Redis (valkey is up) — devops/full-infra env.

### Epic 3.3: Tracer gRPC + mTLS transport — LIVE-VERIFY: DEFERRED (no cert provisioning)

**Goal:** the reserve contract holds over the production-default gRPC+mTLS transport, not just REST — no contract drift between transports.
**Scope:** `tests/e2e/tracer_grpc_test.go` (new); the ledger gRPC reserve client (`TRACER_TRANSPORT=grpc`, `TRACER_TLS_MODE=mtls`); cert material.
**Dependencies:** Epic 2.1 reserve fixtures (reuse `seedLimitRule`, `newEnforceFixture` from `tracer_harness_test.go`); cert provisioning; `requireTracerGRPC(t)` gate. **Heavy infra — verification deferred.**
**Done when:** with the stack wired over gRPC+mTLS, an over-limit transfer is denied (422/0177) and an in-limit transfer commits (201) — identical to the REST outcomes Phase 2 proved; absent gRPC, the test SKIPS.
**Status:** Detailed (live-verify deferred)

**Execution (detailed wave):** prefix `tgrpc`. **Gate** `requireTracerGRPC(t)`: skip unless `E2E_TRACER_GRPC=1` (operator asserts a gRPC+mTLS-wired stack). **Test:** reuse the Phase 2 enforce-mode + denial scenario verbatim — it is transport-agnostic at the HTTP layer (the ledger's transport choice is internal), so the SAME `seedLimitRule`/over-limit-transfer → 422 and in-limit → 201 assertions prove no drift. The only new thing is the gate. **Live-verify deferred:** no cert-provisioning script exists in the repo (recon). Requires hand-generating a CA + ledger-client + tracer-server cert chain, mounting both (`TRACER_TLS_CERT_FILE`/`KEY_FILE`/`CA_FILE` on ledger; `..._CLIENT_CA_FILE` on tracer — `bootstrap/tls_seam.go` both sides), and setting `TRACER_TRANSPORT=grpc` + `TRACER_TLS_MODE=mtls` + `TRACER_GRPC_PORT` — devops/full-infra env. Boot fails fast if cert material is missing, so a misconfigured run won't false-pass.

### Epic 3.4: Auth-on and CRM namespace flip — LIVE-VERIFY: DEFERRED (no plugin-auth)

**Goal:** with auth enabled, protected routes reject unauthenticated calls and the CRM holder/instrument routes authorize under the `midaz` namespace (the X1 flip from plugin-crm).
**Scope:** `tests/e2e/auth_namespace_test.go` (new); `PLUGIN_AUTH_ENABLED=true` + plugin-auth; the `midaz:{holders,instruments}` authz namespace.
**Dependencies:** `requireAuth(t)` gate + token helper. **Heavy infra — verification deferred.**
**Done when:** an unauthenticated CRM call returns 401; an authorized call under the `midaz` namespace succeeds; absent auth, the test SKIPS.
**Status:** Detailed (live-verify deferred)

**Execution (detailed wave):** prefix `auth`. **Gate** `requireAuth(t)`: skip unless `E2E_AUTH=1`. **Test:** (a) an unauthenticated POST to a holders or instruments route → 401 (`MarkTrustedAuthAssertion` returns `fiber.StatusUnauthorized`, `pkg/net/http/protected_routes.go:48-49`); (b) the namespace flip is a static fact to pin — the holders/instruments routes register `auth.Authorize("midaz", "holders"|"instruments", verb)` with `ApplicationName="midaz"` (`crm_routes.go:15-20`, `:36-53`), NOT `plugin-crm`; the test can assert an authorized call under `midaz` succeeds (with a mock-approved token). **Live-verify deferred:** no plugin-auth container in any compose; needs `PLUGIN_AUTH_ENABLED=true` + `PLUGIN_AUTH_HOST` pointed at a real or httptest-mock plugin-auth (recon: integration suite uses an httptest fake returning `{authorized:true}`) — devops/full-infra env. Token = unsigned JWT (ParseUnverified) as in 3.2.

### Epic 3.5: Async transaction processing — LIVE-VERIFY: feasible (flag flip)

**Goal:** transactions created in async mode settle correctly with eventual balance consistency, equivalent to the sync result.
**Scope:** `tests/e2e/async_transaction_test.go` (new); `RABBITMQ_TRANSACTION_ASYNC=true`; the in-process RabbitMQ consumer goroutine.
**Dependencies:** `requireAsync(t)` gate. RabbitMQ is already up (`midaz-rabbitmq`); the consumer is an in-process goroutine in the unified binary (`bootstrap/service.go:66`) — no separate worker.
**Done when:** an async-created transfer returns 201 immediately, balances converge to the sync-equivalent final value within a poll-with-timeout, and two sequential transfers on one account both settle to the correct cumulative balance; absent async, the test SKIPS.
**Status:** Detailed

**Execution (detailed wave):** prefix `async`. **Gate** `requireAsync(t)`: skip unless `E2E_ASYNC=1` (a poll-with-timeout test passes trivially in sync mode and proves nothing, so gate on the operator's intent flag rather than a behavioral probe). **Test:** fund a source (poll until the funding inflow settles); transfer 100 → expect 201 with status `CREATED`/`APPROVED` (`transaction_create.go:1429`); **poll** `GET .../accounts/alias/{alias}/balances` until `available` reflects the debit, with a bounded timeout + clear failure on non-convergence (the async-specific behavior: balance mutation is deferred to the consumer). Equivalence: assert the final balance equals the sync arithmetic (before − 100). Ordering: two sequential transfers on one source → poll until both settle → assert cumulative balance. Idempotency key dedups replays at the API layer before enqueue (`transaction_create.go:1037-1060`). **Live-verify (supervisor):** set `RABBITMQ_TRANSACTION_ASYNC=true` in `components/ledger/.env`, recreate ledger, set `E2E_ASYNC=1`, run.

---

## Self-Review

- **Spec coverage:** Tier 1 → #4 requireHolder (1.3.1), #5 deductible/multi-fee (1.1.1/1.1.2), #1 fee×tracer (2.4), #2 denial (2.2), #3 lifecycle (2.3). Tier 2 → cache (1.2.1), scoping (1.2.2), boundaries/waived (1.1.3), instrument referential (1.3.2), cascade (1.3.3). Tier 3 → streaming (3.1), MT (3.2), gRPC/mTLS (3.3), auth/namespace (3.4), async (3.5). All tiers mapped. The Tier-2 "composition atomicity" lands in 1.3.2.
- **Vagueness scan:** detailed-wave tasks name concrete cases and pin "calibrate then assert the observed contract"; no "appropriate"/"TBD". The calibration steps are concrete (exact requests, exact fields to capture), not deferrals.
- **Contract consistency:** all tasks reuse the `tests/e2e/harness_test.go` helpers and the verified bodies; new helpers (`createMultiFeePackage`, `updateFeePackage`, `createSegment`, `createLedgerRequiringHolder`, `deleteHolder`) are introduced where first needed and named consistently.
- **Phase boundaries:** Phase 1 ends with a green, infra-free fees/CRM suite; Phase 2 ends with tracer integration green behind a gate; Phase 3 ends with each seam gated. Every phase ships working tests.
- **Verification plausibility:** every detailed task gives a real `go test -tags e2e -run ... ./tests/e2e/...` command against the real suite path.
