# Keeping stored fees inside a lowered package minimum

**Goal:** a deductible fee is taken out of the payment it is charged on, so it can never
exceed that payment. One way of editing a fee package skipped that rule entirely, and the
update path measured fees against the wrong minimum.

**Scope:** `components/ledger/pkg/feeshared/model` (the shared validators) and
`components/ledger/internal/services/fees` (the update use case, where the stored package is
already loaded), plus one new error code. No schema change, no API shape change.

**Status:** complete, measured at `ca93bf0c0bc20b22065c026f6a1b02008456639a`.

## What changes for a caller, and what does not

This PR changes the UPDATE path only. **Creation behaves exactly as before**: `minimumAmount`
is `validate:"required"` on the create payload, and `createPackage` runs
`ValidateMinAndMaxAmount()` (which parses the minimum and refuses an empty one with `0204`)
before `ValidateFees()`, so the create-side validator is never reached without a minimum. A
create of a deductible percentage of 150 answered `0207` before this change and answers
`0207` now.

## Phase overview

| Phase | Outcome | Status |
|---|---|---|
| P1 | The update BOUNDARY caps a deductible percentage above 100 when the patch carries no minimum, answering `0207` | Complete, commit `c70f62af3` |
| P2 | A package update is judged against the minimum it will carry, for the fees it keeps and the fees it restates or adds | Complete, commit `1e1181c47` |
| P3 | A fee the same patch settles does not block the minimum change | Complete, commit `7bc74b175`, from review |
| P4 | An ambiguous patch is refused, one predicate decides a removal, and the added-fee branch is pinned | Complete, commits `60b6eb2f4` `20cb502da` `81e3a0dc5` `f2b3267b6` `ca93bf0c0`, from review |

## P1 - The percentage cap no longer waits for a minimum

### P1.1 What was wrong

`validateCalculationValues` capped a deductible percentage at 100 only inside
`if minAmount != "" && isDeductible`. The update-side validator
(`validatePercentageCalculation`) capped it with no minimum clause at all.

### P1.2 What changed, stated exactly

- [x] The percentage cap runs for every deductible calculation, minimum or no minimum, and
      answers `0207` as it already did on create.
- [x] The one reachable consequence is at the update boundary: a patch that carries fees,
      declares `isDeductibleFrom: true` in the body and sets a percentage above 100, while
      carrying no `minimumAmount`, is now refused there with `0207`. Measured at the head
      above: `BOUNDARY, no minimum, flag declared in body => 0207 - Calculation value is
      invalid, it cannot exceed 100%.`
- [x] **Not changed, and not claimed:** when the body does NOT restate the flag,
      `ValidateFees` reads `fee.GetIsDeductibleFrom()` off the payload, cannot see the stored
      record, and lets the request through; the service then answers `0210` with its existing
      wording. Measured at the same head: `SERVICE, no minimum, no flag in body => 0210 - Can
      not update deductible value to true. The calculation value is bigger than 100% for Fee
      fee1.` The same holds for the flat half: a patch that restates a fee's amount above the
      new minimum is refused by `0211`, and this PR's own test pins that code.
- [x] The flat cap still waits for a minimum, because without one it has no amount to
      measure the fee against.
- [x] Non-deductible fees are untouched: they are charged on top of the payment, so they may
      exceed it.
- [x] One existing case in `TestValidateCalculationValues` was renamed from "Empty minAmount
      - should skip deductible validation" to "Empty minAmount - deductible flat has no
      minimum to exceed". It pinned the flat half, which is preserved; only its name claimed
      more than the code does. No existing case changed its expectation.

## P2 - A minimum change is judged against the fees the package keeps

### P2.1 What was wrong

`UpdatePackageInput.ValidateFees` runs only when the patch carries fees, and
`buildUpdateFields` validated fees only when `up.Fee != nil`. A patch of `{"minimumAmount":
"1"}` against a package storing a deductible flat fee of 25 was therefore accepted and
written. The package then accepted payments of 1 that it could not charge its own fee on,
and every such payment failed later at transaction time under `0233`
(`ErrDeductibleFeeExceedsAmount`). A create of the same configuration is refused by `0208`.

A second, quieter half: fees the patch did carry were validated against the STORED minimum,
never the new one. That made a raised minimum refuse a fee the same call was setting, and
would have let the check above be evaded by restating the offending fee in the same call.

### P2.2 What changed

- [x] `UpdatePackageInput.EffectiveMinimumAmount` returns the minimum the package will carry:
      the patched value when the patch sets one, otherwise the stored value.
- [x] `UpdatePackageInput.ValidateStoredFeesAgainstMinimum` checks the fees already stored
      against that new minimum, through the same shared validator a create uses, so the
      answer carries the create codes `0207` and `0208`.
- [x] Fees whose calculations the patch restates are skipped there and validated as they are
      applied, against the same new minimum. Patch keys are matched by their lower camel
      form, the way the update applies them, so a key differing only in case is the same fee.
- [x] The use case passes the effective minimum to `validationFeesSetUnset`, so both a fee
      the package does not yet carry and a fee it already carries are measured against where
      the package lands.
- [x] The check runs after the existing min/max validation, so a malformed or inverted
      minimum still answers its own error first.

### P2.3 Deliberately not changed

- The check triggers on the patch CARRYING a minimum, not on the minimum CHANGING. A package
  already stored in violation is therefore refused by a patch that restates the same minimum
  value it already has, which a client submitting a whole form on an unrelated edit does.
  Refusing there is the safe direction: the alternative is to compare against the stored
  value and let a form round-trip quietly re-confirm an invalid package. A patch that carries
  no `minimumAmount` at all still succeeds.
- A package already stored in violation (written before this change) is not swept. It stays
  editable and enable-able, and its payments keep failing at transaction time under `0233`
  until someone repairs it. A sweep that refused every update to such a package would lock it
  out of being repaired.
- The transaction-time guard `0233` stays. It is the last line for a payment whose amount sits
  between the minimum and the fee, which package-level validation cannot see.
- `ValidateStoredFeesAgainstMinimum` skips a stored fee whose `CalculationModel` is nil. The
  MongoDB mapper (`ToEntityFeeMap`) always builds a non-nil model, so that guard and its test
  case cover a shape only a mock produces today. It is kept as a nil-dereference guard on a
  money-path validator, not as a claim about stored data.
- One shape the create path would refuse is still applied on update: a patch setting a
  `percentage` calculation under an unchanged `flatFee` application rule. That is pre-existing
  on the update path and outside this PR.

## P3 - A fee the same patch settles does not block the minimum change

### P3.1 What was wrong

P2 skipped a stored fee only when the patch restated its amounts. Two other patches leave
no conflict behind and were refused anyway:

- the patch removes the fee, which is an entry that sets no field at all;
- the patch sets `isDeductibleFrom` to `false`, after which the fee is charged on top of
  the payment and no cap applies to it.

Both are legitimate single-call edits: lower the minimum and clear what stood in its way.

### P3.2 What changed

- [x] A stored fee is skipped when its patch entry settles it, in any of the three ways:
      removal, stopping the deduction, or restating the amounts.
- [x] A patch that only renames the fee, that only sets a route, or that confirms
      `isDeductibleFrom: true`, leaves the fee in place and is still refused.

Raised by CodeRabbit on PR #2494 and confirmed against the code before fixing: both cases
were refusals the operator did not deserve, not accepted bad states.

## P4 - What the review round closed

### P4.1 An ambiguous patch is refused instead of answered at random

Fee keys are applied by their lower camel form, so `fee1` and `Fee_1` in one body name the
same fee. Both the check and the write picked whichever entry the Go map yielded first, so
the same request was accepted on some calls and refused on others. Measured before the fix,
inside one test run, the same body returned `<nil>` in one subtest and `0208` in another.

- [x] A body naming one fee twice is refused with the new code `0236`, which names the key
      the entries collide on. The refusal is raised at the request boundary
      (`UpdatePackageInput.ValidateFees`) and again where the stored fees are matched, so a
      caller reaching the use case directly gets the same answer.
- [x] One function (`normalisedFees`) indexes the patch under the applied key; the
      first-match lookup it replaces is gone.
- [x] The stored fees are walked in sorted order, so a package breaking the new minimum in
      more than one fee names the same fee on every call.
- [x] `0236` is a `ValidationError`, so it renders as HTTP 400. The golden net proves the
      tuple through the real dispatcher:
      `TestGolden_BusinessErrorCodeStatus/ErrDuplicateFeeKey_0236_400`.

### P4.2 One predicate decides what removes a fee

`removesTheFee` (the check) and `SetAndValidateHasFieldsToUpdate` (the write) each decided
separately what deletes a fee, and disagreed on two shapes:

- a patch entry whose `calculationModel` is present but carries neither an application rule
  nor calculations, which a form-driven client sends on every remove-fee call;
- a field whose value the writer reads as empty but a `== ""` test does not, such as the
  string `"null"`, which `commons.IsNilOrEmpty` treats as empty.

Both were refused with `0208` although the package would have landed valid.

- [x] `removesTheFee` is now the exact negation of the eight field writers, read through the
      same emptiness test they use, and `SetAndValidateHasFieldsToUpdate` asks it first. The
      two cannot disagree by construction.
- [x] A table feeds the same twelve patch shapes to both and asserts they agree.
- [x] At the service level, both removal shapes are proved to land the same end state: the
      write carries `minimum_amount: "1"` and `$unset` on `fees.fee1`.

### P4.3 The added-fee branch is pinned

`validationFeesSetUnset` uses the effective minimum in two places, and only the existing-fee
branch had a test. The unpinned branch is the one that wrote the live invalid packages.

- [x] A service-level table adds a NEW deductible fee in the same call that moves the
      minimum, in both directions: lowered under it, refused with nothing written; raised
      above it, applied with the fee in the write.
- [x] Replacing the effective minimum with the stored one on that branch now turns both rows
      RED (mutant M1 below).

### P4.4 Test hygiene and evidence

- [x] The two model test files call `t.Parallel()` at function and subtest level, matching
      the service test file added in the same PR. Twenty-six paused tests, four functions and
      twenty-two subtests.
- [x] The live proof now reads back the stored fee as well as the minimum and the timestamp.
- [x] This document's verification section is rebuilt: every block carries the three evidence
      header lines, the outputs are transcripts, and the `golangci-lint` line is the binary
      that was actually invoked.

## Found by

Console lane #963 measured the two validators against midaz `origin/develop` while closing
the console-side drift: the console refuses both configurations at its own boundary, and the
ledger did not agree with itself for direct API callers. P3 and P4 came from review rounds on
PR #2494.

## Verification

Every block below carries `date -u`, `git rev-parse HEAD` and `git status --porcelain` before
the command, then the command, its output and its exit code. The RED evidence has two halves:
the lane's tests against current `develop`, which is what the defect looks like today, and
mutants applied at the code-final head, which any reader can reproduce.

### RED, the lane's tests against `origin/develop`

A detached worktree at `origin/develop` with the lane's test files copied in.

```
$ date -u
Tue Sep 15 12:47:49 UTC 2026
$ git rev-parse HEAD
753bd40ef85af9da73beb214c698bb10ce713c97
$ git status --porcelain
A  components/ledger/internal/services/fees/update_package_minimum_test.go
A  components/ledger/pkg/feeshared/model/deductible_caps_test.go
A  components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/
# github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model [.../model.test]
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:109:14: up.ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:128:24: up.EffectiveMinimumAmount undefined (type *UpdatePackageInput has no field or method EffectiveMinimumAmount)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:137:24: up.EffectiveMinimumAmount undefined (type *UpdatePackageInput has no field or method EffectiveMinimumAmount)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:146:16: up.EffectiveMinimumAmount undefined (type *UpdatePackageInput has no field or method EffectiveMinimumAmount)
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model [build failed]
--- FAIL: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
    update-package-by-id.go:73: Unexpected call to *pack.MockRepository.Update([context.Background.WithValue(trace.traceContextKeyType, global.nonRecordingSpan) 5c2322e0-effa-4b7c-806b-858abaf482a1 44b06224-7996-489f-abc2-a87bc0f5331a 00000000-0000-0000-0000-000000000000 {"$set":{"minimum_amount":"1","updated_at":{"$date":{"$numberLong":"1789476472739"}}}}]) at /tmp/rv-feeval-impl-base/components/ledger/internal/services/fees/update-package-by-id.go:73 because: there are no expected calls of the method "Update" for that receiver
--- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.01s)
    --- FAIL: .../minimum_lowered_under_the_fee_the_patch_sets (0.00s)
            	Error:      	An error is expected but got nil.
    --- FAIL: .../minimum_raised_above_the_fee_the_patch_sets (0.00s)
            	Error:      	Received unexpected error:
            	            	0211 - Can not update deductible value to true. Calculation value is bigger than the minimum amount 100 for Fee fee1.
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.039s
rc=1
```

The first failure is the defect itself: the write went through, carrying
`minimum_amount: "1"` to a package whose stored deductible fee is 25.

The percentage cap alone, in the same worktree with the model test file removed so the
package builds:

```
$ date -u
Tue Sep 15 12:48:01 UTC 2026
$ git rev-parse HEAD
753bd40ef85af9da73beb214c698bb10ce713c97
$ git status --porcelain
A  components/ledger/internal/services/fees/update_package_minimum_test.go
A  components/ledger/pkg/feeshared/model/deductible_caps_test.go
AD components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ go test -count=1 -v ./components/ledger/pkg/feeshared/model/ -run 'CapsDeductiblePercentageWithoutMinimum'
--- FAIL: TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum (0.00s)
    --- FAIL: .../deductible_percentage_over_100_without_a_minimum (0.00s)
    --- PASS: .../deductible_percentage_at_100_without_a_minimum (0.00s)
    --- PASS: .../percentage_over_100_that_is_not_deductible (0.00s)
    --- PASS: .../deductible_flat_without_a_minimum_has_nothing_to_exceed (0.00s)
    --- PASS: .../deductible_percentage_over_100_with_a_minimum (0.00s)
    --- PASS: .../deductible_flat_over_the_minimum (0.00s)
--- FAIL: TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.018s
rc=1
```

### RED, four mutants at the code-final head

Each mutant is applied in a detached worktree at `ca93bf0c0`, run, then reverted. The header
lines are from the start of the block; `git status --porcelain` shows the mutated file at the
moment each test ran.

**M1 - the added-fee branch reads the stored minimum instead of the effective one.** This is
the mutant that survived the whole suite before this round.

```
$ date -u
Tue Sep 15 12:48:09 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(clean before the mutant)
$ sed -i 's|err := fee.ValidateNewFee(key, minAmount)|err := fee.ValidateNewFee(key, decimal.NewFromInt(100))|' components/ledger/internal/services/fees/update-package-by-id.go
$ grep -n "ValidateNewFee(key," components/ledger/internal/services/fees/update-package-by-id.go
210:			err := fee.ValidateNewFee(key, decimal.NewFromInt(100))
$ go test -count=1 ./components/ledger/internal/services/fees/
--- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum (0.00s)
    --- FAIL: .../minimum_lowered_under_the_fee_the_patch_adds (0.00s)
    --- FAIL: .../minimum_raised_above_the_fee_the_patch_adds (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.038s
rc=1
```

**M2 - the removal predicate goes back to the narrow form.**

```
$ date -u
Tue Sep 15 12:48:19 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(clean before the mutant)
$ grep -n "func (f \*Fee) removesTheFee" -A 3 components/ledger/pkg/feeshared/model/update_package_input.go
181:func (f *Fee) removesTheFee() bool {
182-	return f.ValidateIfFeeIsNil() && commons.IsNilOrEmpty(f.RouteFrom) && commons.IsNilOrEmpty(f.RouteTo)
183-}
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/
--- FAIL: TestFeeRemovalPredicateAgreesWithTheApplyPath (0.01s)
    --- FAIL: .../an_entry_whose_only_field_the_writer_reads_as_empty (0.00s)
    --- FAIL: .../an_entry_whose_calculation_model_carries_nothing (0.00s)
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.01s)
    --- FAIL: .../patch_removes_the_offending_fee_with_an_empty_calculation_model (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.062s
--- FAIL: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
    --- FAIL: .../the_entry_carries_an_empty_calculation_model (0.00s)
FAIL
rc=1
```

**M3 - colliding patch keys are resolved instead of refused.**

```
$ date -u
Tue Sep 15 12:48:31 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(clean before the mutant; the duplicate guard in normalisedFees is deleted)
$ go test -count=1 ./components/ledger/pkg/feeshared/model/
--- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys (0.01s)
    --- FAIL: .../every_call_answers_the_same_way (0.00s)
    --- FAIL: .../at_the_request_boundary (0.00s)
    --- FAIL: .../a_patch_carrying_no_minimum_is_refused_just_the_same (0.00s)
    --- FAIL: .../when_the_stored_fees_are_measured_against_the_new_minimum (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.056s
rc=1
```

**M4 - the stored fees are walked in Go map order again.**

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.056s
rc=1
```

### GREEN, every test this PR adds, at the code-final head

```
$ date -u
Tue Sep 15 12:48:56 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(no output)
$ go test -count=1 -v ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/ -run 'CapsDeductiblePercentageWithoutMinimum|ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount|RefusesAmbiguousFeeKeys|RemovalPredicateAgreesWithTheApplyPath|MinimumUnderStoredDeductibleFee|AcceptsALoweredMinimumWhenThePatchRemovesTheFee|MeasuresPatchedFeesAgainstTheNewMinimum|MeasuresAddedFeesAgainstTheNewMinimum'
--- PASS: TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum (0.00s)
--- PASS: TestUpdatePackageInputEffectiveMinimumAmount (0.00s)
--- PASS: TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum (0.00s)
--- PASS: TestFeeRemovalPredicateAgreesWithTheApplyPath (0.00s)
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.00s)
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee (0.02s)
--- PASS: TestUpdatePackageInputRefusesAmbiguousFeeKeys (0.00s)
PASS
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.044s
--- PASS: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
--- PASS: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.00s)
PASS
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.032s
rc=0
```

The refusal tests declare no `Update` expectation on the repository mock, or assert the
captured payload is nil, so they pass only while nothing is written.

The 22 subtests of the two model test files run in parallel:

```
$ go test -count=1 -v ./components/ledger/pkg/feeshared/model/ -run 'CapsDeductiblePercentageWithoutMinimum|ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount' 2>&1 | grep -c "=== PAUSE"
26
```

### Live proof against a real MongoDB

The same update runs against a MongoDB testcontainer through the real repository. At
`origin/develop` the lowered minimum is accepted and persisted; at the code-final head it is
refused and the stored document keeps its minimum, its fee and its timestamp, all three now
asserted.

```
$ date -u
Tue Sep 15 12:43:42 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
 M components/ledger/internal/services/fees/update_package_minimum_integration_test.go
$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 -v -run 'TestIntegration_UpdatePackage_' ./components/ledger/internal/services/fees/
--- PASS: TestIntegration_UpdatePackage_LoweredMinimumLeavesTheStoredPackageUntouched (1.60s)
--- PASS: TestIntegration_UpdatePackage_MinimumAboveTheStoredFeeIsApplied (0.94s)
PASS
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	3.863s
rc=0
```

The status line shows the assertion being added in that run; it was committed as
`ca93bf0c0` immediately after, and the integration gate below re-runs it from a clean tree.

The second case passes at both commits: a minimum of 30, still above the stored fee of 25, is
applied before and after. The fix refuses the invalid move, not the valid one.

### Gates at `ca93bf0c0bc20b22065c026f6a1b02008456639a`

```
$ date -u
Tue Sep 15 12:44:01 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(no output)

$ go build ./...
rc=0

$ gofmt -l components/ledger/pkg/feeshared/model components/ledger/internal/services/fees pkg
rc=0   (no output)

$ go vet ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/... ./pkg/...
rc=0

$ GOLANGCI_LINT_CACHE=/tmp/gcl-midaz2494 /tmp/gcl-bin-midaz-fee/golangci-lint run --allow-parallel-runners ./components/ledger/pkg/feeshared/model/... ./components/ledger/internal/services/fees/...
0 issues.
rc=0
```

```
$ date -u
Tue Sep 15 12:44:13 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
(no output)

$ go test -count=1 ./components/ledger/... ./pkg/...
rc=0   ok=71 fail=0

$ go test -race -count=1 ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/...
rc=0   (5 packages ok, 2 with no test files)

$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 ./components/ledger/internal/adapters/mongodb/fees/... ./components/ledger/internal/services/fees/...
ok  	.../adapters/mongodb/fees	14.894s
ok  	.../adapters/mongodb/fees/billing_package	32.491s
ok  	.../adapters/mongodb/fees/pack	31.661s
ok  	.../services/fees	5.172s
ok  	.../services/fees/midaz	0.018s
rc=0
```

`golangci-lint` is invoked by absolute path because the version on `PATH` is `v2.12.2` built
with `go1.26`, which refuses a module targeting `1.27.0`. The binary at
`/tmp/gcl-bin-midaz-fee/golangci-lint` is the CI pin from
`.github/workflows/pr-validation.yml:40`, `v2.13.2`, rebuilt locally with `go1.27.0`. The
line above is the one that was run, verbatim.

That gate answers `0 issues.` on a clean tree, so it is proved live rather than assumed:
planting one unused variable in `update_package_input.go`, in a detached worktree at the same
head, turns it red.

```
$ date -u
Tue Sep 15 12:46:15 UTC 2026
$ git rev-parse HEAD
ca93bf0c0bc20b22065c026f6a1b02008456639a
$ git status --porcelain
 M components/ledger/pkg/feeshared/model/update_package_input.go
$ GOLANGCI_LINT_CACHE=/tmp/gcl-midaz2494-probe /tmp/gcl-bin-midaz-fee/golangci-lint run --allow-parallel-runners ./components/ledger/pkg/feeshared/model/... ./components/ledger/internal/services/fees/...
components/ledger/pkg/feeshared/model/update_package_input.go:623:5: var implUnusedProbe is unused (unused)
1 issues:
* unused: 1
rc=1
```
