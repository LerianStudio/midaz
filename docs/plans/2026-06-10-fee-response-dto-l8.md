# Fee Response DTO Split (L8) Implementation Plan

> **For implementers:** Use ring:executing-plans (rolling wave) or ring:running-dev-cycle.
> This is a single-phase, single-subsystem plan — small enough that the whole thing is
> detailed at plan time. This document is the living source of truth.

**Goal:** Stop `FeeCalculate` (an internal, mutable fee-engine carrier) from appearing on the HTTP wire; give the `POST /estimates` response a purpose-built output DTO and drop the deprecated `Route` field from the fee-estimate transaction shape.

**Architecture:** Keep `FeeCalculate`/`FeeEstimate` exactly as they are for the engine — the fee engine *mutates the embedded transaction in place* (`Send.Source.From`, `Send.Distribute.To`, `Send.Value`, `Metadata`), so the type must stay a mutable, full-`Transaction`-carrying envelope. Introduce an **output-only response DTO** and project into it at the one handler that serializes fee results. Engine and its ~50 test construction sites stay untouched.

**Tech Stack:** Go 1.26, Fiber v2, swaggo/swag v1.16.6, `pkg/mtransaction`.

---

## ⚠️ Read this before committing to execution — severity & scope honesty

L8 is the **lowest audit tier (L)**, and the recon weakened its original justification:

- The audit framed it as "persistence/bson structs leaking onto the wire." **False here** — `mtransaction.Transaction` (`pkg/mtransaction/transaction.go:287-300`) has **zero bson tags, no unexported fields**, and already carries `// @name TransactionInput`. It is a wire-clean *input* type, not a DB row.
- `FeeCalculate` reaches the wire in exactly **one place**: nested as `feesApplied` inside `FeeEstimateResponse` on `POST /v1/organizations/{org}/estimates` (`fees_handler.go:91,99`). The `/fees` calculate endpoint is **not mounted** (`fees_routes.go:29-31`). `FeeCalculate` is never a request body and never a top-level response.
- `FeeEstimate` is request-IN only; it never enters the engine.

So the **actual** residual defect is narrow and twofold:
1. An **input-shaped** type (`TransactionInput`) is echoed back as part of a **response** — a modeling smell, not a data leak.
2. The fee-estimate response transaction carries the **deprecated `Route` field** (`transaction.go:294`, read under `//nolint:staticcheck` at `calculate-fee.go:125`).

**Recommendation:** This clears the bar for the v4 breaking window **only if** we want the fee-estimate response contract to be self-describing and free of deprecated/input-only fields. It is a polish change, not a correctness or security fix. If the v4 window is tight, **defer it** — the wire is already clean of persistence concerns, and `feesApplied.transaction` rendering as `TransactionInput` is cosmetically acceptable. Two execution variants below; **Variant A (Minimal)** is recommended if we proceed.

---

## Phase Overview

| Phase | Milestone | Epics | Status |
|-------|-----------|-------|--------|
| 1 | `POST /estimates` returns a purpose-built `FeeEstimateResult` DTO; `FeeCalculate` no longer appears in the generated spec; engine + engine tests unchanged | 1.1, 1.2 | Detailed |

---

## Variant decision (lock before Epic 1.1)

**Variant A — Minimal (recommended).** Introduce `FeeEstimateResult` mirroring `FeeCalculate`'s public output fields (`ledgerId`, `segmentId`, `transaction`), but type its `transaction` against a **new output projection** `FeeAdjustedTransaction` that excludes the deprecated `Route` field. Resolves both sub-defects with one new wire type + one transaction projection. Blast radius: the estimate use case's return type + the handler + swagger regen + the estimate-layer tests. Engine untouched.

**Variant B — Rename-only (cheapest).** Keep echoing `TransactionInput` in the response (accept sub-defect #1), only stop `FeeCalculate` itself from being the wire type by wrapping its output fields in `FeeEstimateResult`. Does **not** drop `Route`, does **not** resolve the input-shape-as-response smell. Cheaper, but only half-fixes L8 — arguably not worth a breaking-change slot. **Not recommended:** if we touch the contract at all, do it once and properly (Variant A).

Tasks below assume **Variant A**.

---

### Epic 1.1: Introduce the fee-estimate output DTOs

**Goal:** New wire-only types exist, fully annotated, with no embedded engine/input semantics; they compile and have unit coverage.
**Scope:** `components/ledger/pkg/feeshared/model/` (new file or extend `fees.go`).
**Dependencies:** none
**Done when:** `FeeEstimateResult` and `FeeAdjustedTransaction` exist with `swagger:model`+`@name`+`@Description`+field examples; a `projectFeeEstimate(*FeeCalculate) FeeEstimateResult` (or method) maps engine output → wire DTO; unit test covers the projection including metadata pass-through and `Route` exclusion.
**Status:** Pending

#### Task 1.1.1: Define `FeeAdjustedTransaction` + `FeeEstimateResult` and the projection

- [ ] Done

**Context:** `FeeCalculate` (`components/ledger/pkg/feeshared/model/fees.go:17-21`) embeds `mtransaction.Transaction` by value and is mutated in place by the engine (`pkg/fee/calculate-fee.go:149-150`, `pkg/fee/distribute.go:67`, metadata at `distribute.go:129-153` and `internal/services/fees/calculate-fee.go:163-171`). It must remain the engine's mutable carrier. The estimate handler currently returns `FeeEstimateResponse{Message, FeesApplied *FeeCalculate}` (`fees.go:28-31`), so the engine type leaks onto the wire as `feesApplied`.

**Implementation vision:** In `feeshared/model` add:
- `FeeAdjustedTransaction` — mirror the *output-relevant* fields of `mtransaction.Transaction` (`transaction.go:287-300`): `ChartOfAccountsGroupName`, `Description`, `Code`, `Pending`, `Metadata`, `RouteID`, `TransactionDate`, `Send`. **Omit the deprecated `Route` field.** Reuse `mtransaction.Send` for the `Send` field (it is itself wire-typed and `@name`d) rather than re-mirroring the leg structure — only the top-level `Route` drop is in scope, do not deep-fork `Send`. Carry `// @name FeeAdjustedTransaction`, `@Description`, and field examples.
- `FeeEstimateResult` — `{LedgerID uuid.UUID, SegmentID *uuid.UUID, Transaction FeeAdjustedTransaction}` with `// @name FeeEstimateResult` + annotations.
- A pure mapper `func NewFeeEstimateResult(fc *FeeCalculate) FeeEstimateResult` that copies the scalar/uuid fields and projects `fc.Transaction` into `FeeAdjustedTransaction` (drop `Route`, pass `Send`/`Metadata`/`RouteID`/`TransactionDate` through). No mutation of `fc`.

Follow the existing annotation idiom in `fees.go` (which Phase 5 just deepened). Do NOT add bson tags. Do NOT embed `mtransaction.Transaction` directly.

**Files:**
- Modify: `components/ledger/pkg/feeshared/model/fees.go` (or Create `components/ledger/pkg/feeshared/model/fee_estimate_result.go`)
- Test: `components/ledger/pkg/feeshared/model/fee_estimate_result_test.go`

**Verification:** `go test ./components/ledger/pkg/feeshared/model/... -run FeeEstimateResult -v` — projection copies all fields, excludes `Route`, preserves `Metadata` (including injected `packageAppliedID`/`feeExemption` keys) and the rewritten `Send` legs.

**Done when:** types compile, are annotated to the `fees.go` bar, and the projection is unit-covered with a fixture whose `FeeCalculate.Transaction` has a non-empty `Route` proven absent from the result.

---

### Epic 1.2: Repoint the estimate response path to the DTO and regenerate

**Goal:** `POST /estimates` serializes `FeeEstimateResult`; `FeeCalculate`/`FeeEstimate` no longer appear in the ledger spec's `definitions` (except where `FeeEstimate` legitimately remains the request body — see note); spec regenerated and verified.
**Scope:** `components/ledger/internal/services/fees/estimate-fee-calculation.go`, `components/ledger/internal/adapters/http/in/fees_handler.go`, the fee service interface, `components/ledger/api/*` (regen).
**Dependencies:** Epic 1.1
**Done when:** the estimate use case returns `*model.FeeEstimateResult` (projected from the internal `FeeCalculate`); `FeeEstimateResponse.FeesApplied` is `*FeeEstimateResult`; `make generate-docs` is green; `feesApplied` in the spec references `FeeEstimateResult`→`FeeAdjustedTransaction` (no `FeeCalculate`, no `Route` on that path); ledger security still 111/111; parity green.
**Status:** Pending

#### Task 1.2.1: Project at the use-case boundary and update the response wrapper

- [ ] Done

**Context:** `EstimateFeeCalculation` use case constructs and returns `*model.FeeCalculate` (`internal/services/fees/estimate-fee-calculation.go:74`); the handler wraps it as `FeeEstimateResponse{FeesApplied: ...}` and responds (`fees_handler.go:91,99`). The service interface that the handler depends on (`fees_handler.go:27`) types the return as `*model.FeeCalculate`.

**Implementation vision:** Choose the projection seam = **the use case** (keeps the handler thin and the wire type out of the engine-facing service entirely). Change the estimate use case to compute its `*FeeCalculate` internally as today, then `return model.NewFeeEstimateResult(fc), nil`. Update the service interface return type and `FeeEstimateResponse.FeesApplied` to `*model.FeeEstimateResult`. The handler change is mechanical (type of the local). Do **not** touch `transaction_fee_application.go` — that seam folds `cf.Transaction` back into the live transaction in-process (`transaction_fee_application.go:96`) and never serializes; it must keep using `FeeCalculate`.

**Files:**
- Modify: `components/ledger/internal/services/fees/estimate-fee-calculation.go:74`
- Modify: `components/ledger/internal/adapters/http/in/fees_handler.go:27,91,99`
- Modify: `components/ledger/pkg/feeshared/model/fees.go` (`FeeEstimateResponse.FeesApplied` type)
- Test: `components/ledger/internal/services/fees/estimate-fee-calculation_test.go` (assertions at L212,L239 move from `*FeeCalculate` to `*FeeEstimateResult`)

**Verification:** `go test ./components/ledger/internal/services/fees/... ./components/ledger/internal/adapters/http/in/... -run 'Estimate|Fee' -v` — estimate path returns the DTO; engine/service mutation tests untouched and still green. `go build ./...` exits 0.

**Done when:** estimate use case + handler return `FeeEstimateResult`; engine (`pkg/fee`) and its ~50 construction-site tests are byte-unchanged; the only test churn is the estimate use case + a new projection test.

#### Task 1.2.2: Regenerate specs and verify the wire contract

- [ ] Done

**Context:** swagger currently names `FeeCalculate` (`docs.go:12611`) and refs it from `FeeEstimateResponse.FeesApplied` (`docs.go:12667`). After the repoint it must reference `FeeEstimateResult`.

**Implementation vision:** Run `make generate-docs`; `git checkout -- postman/MIDAZ.postman_collection.json postman/MIDAZ.postman_environment.json` (random-UUID noise). Verify with jq/python that ledger `definitions` contains `FeeEstimateResult` + `FeeAdjustedTransaction`, that `FeeEstimateResponse.feesApplied` $refs `FeeEstimateResult`, that `FeeCalculate` is **gone** from `definitions` (it no longer reaches the wire), that `FeeEstimate` remains only as the request body schema, and that `FeeAdjustedTransaction` has no `route` property. Confirm `make check-docs` parity + security-coverage pass and ledger security is 111/111.

**Files:**
- Modify: `components/ledger/api/{docs.go,swagger.json,swagger.yaml,openapi.yaml}`, `postman/specs/ledger/*` (regenerated)

**Verification:** `make check-docs` green; jq assertion script (above) passes; `make test-unit` for the fees packages green.

**Done when:** spec shows the new DTOs, `FeeCalculate` absent from `definitions`, `route` absent from `FeeAdjustedTransaction`, security 111/111, parity green — all reproducible via regen on a clean tree.

---

## Self-Review

- **Spec coverage:** L8's two real sub-defects (input-shape-as-response; deprecated `Route` on output) are each closed by Epic 1.1 (`FeeAdjustedTransaction` drops `Route`; `FeeEstimateResult` is a response-only shape) and Epic 1.2 (repoint + regen).
- **Contract consistency:** `FeeEstimateResult`/`FeeAdjustedTransaction` names are defined in 1.1.1 and referenced by 1.2.1/1.2.2; the projection function name `NewFeeEstimateResult` is used consistently.
- **Phase boundary:** single phase ends with a compiling, regenerated, verified state; engine untouched throughout.
- **Vagueness scan:** every task names exact files/lines and a concrete verification (jq assertions, named test runs). The only deliberate open item is the Variant A/B lock, surfaced explicitly above with a recommendation.
- **Blast-radius claim verified by recon:** engine + ~50 `&model.FeeCalculate{}` construction sites need zero changes because the engine keeps `FeeCalculate`; churn is confined to the estimate use case, handler, response wrapper, and one new projection test.

## Verification Checklist

- [x] Plan header (Goal/Architecture/Tech Stack/Phase Overview)
- [x] Phase ends in working, regenerable software
- [x] Every epic has Goal/Scope/Dependencies/Done-when/Status
- [x] Phase 1 fully broken into dispatch-ready tasks
- [x] No vague tasks (exact files/lines + concrete verification)
- [x] Code-shape constraints stated as prose/decisions, not speculative snippets
- [x] Severity/scope honesty surfaced up front (this is L-tier polish, defer-able)
