# Keeping stored fees inside a lowered package minimum

**Goal:** a deductible fee is taken out of the payment it is charged on, so it can never
exceed that payment. One way of editing a fee package skipped that rule entirely, and the
update path measured fees against the wrong minimum.

**Scope:** `components/ledger/pkg/feeshared/model` (the shared validators) and
`components/ledger/internal/services/fees` (the update use case, where the stored package is
already loaded), plus one new error code. No schema change, no API shape change.

**Status:** complete, measured at `8fac078fb6ae0b82b1ebb35e6903adec001702de`.

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
| P4 | An ambiguous patch is refused, one predicate decides a removal, and the added-fee branch is pinned | Complete, commits `60b6eb2f4` `20cb502da` `81e3a0dc5` `f2b3267b6` `ca93bf0c0` `1eca67b9d` `8fac078fb`, from review |

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
- The ambiguous-key refusal is on the UPDATE payload only. The create-side mapper
  (`adapters/mongodb/fees/pack/package.go:300`) lower-camels fee keys the same way, so a
  create body naming one fee twice still collapses into a single stored fee chosen by Go map
  order. Closing that is a create-side behaviour change, which this PR deliberately does not
  make; it is left as its own piece of work.

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
same fee. Only the check read the patch by first match, and only inside this branch: measured
at the pre-guard commit `7bc74b175`, 200 identical calls came back 174 accepted and 26
refused. The write never picked a winner at all. On every one of those 174 accepted calls, and
on 200 of 200 at `origin/develop`, it merged both entries into one update document naming
`fees.fee1` under `$set` (as `fees.fee1.fee_label`) and under `$unset` at the same time, so
what the stored document kept was decided last-wins by map order. That self-conflicting write
is the defect that reached `develop`. The accept-or-refuse coin toss never left this branch.

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
- [x] A negative control pins the other side of the guard: a body carrying three genuinely
      different fees written in three different spellings (`fee1`, `fee_2`, `feeThree`, which
      normalise to three distinct keys) is accepted, through both entry points. Without it a
      guard that refused every multi-fee body would leave the suite green.

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
      the service test file added in the same PR. Twenty-eight paused tests under the filter
      the parallel block below runs: the five test functions that filter selects and their
      twenty-three subtests. The two files hold seven test functions in all.
- [x] The live proof now reads back the stored fee as well as the minimum and the timestamp.
- [x] Two refusal assertions pin the amount the operator is told, not only the error code: the
      lowered-minimum row of the stored-fee table and the single-fee naming test. The other
      three lowered-minimum refusal rows assert the code alone. A refusal that still refuses
      while quoting a number the request never set is caught by those two and by nothing else
      (mutant M5 below).
- [x] This document's verification section is rebuilt: every block carries the three evidence
      header lines except the five mutant blocks, which share one header disclosed at the top
      of that section; the outputs are transcripts rather than counts; and the `golangci-lint`
      line is the binary that was actually invoked.

## Found by

Console lane #963 measured the two validators against midaz `origin/develop` while closing
the console-side drift: the console refuses both configurations at its own boundary, and the
ledger did not agree with itself for direct API callers. P3 and P4 came from review rounds on
PR #2494.

## Verification

Every block below carries `date -u`, `git rev-parse HEAD` and `git status --porcelain` before
the command, then the command, its output and its exit code. The one exception is the mutant
section, whose five blocks share the header printed once at its top and say so there. Where
output is filtered, the filter is on the command line. The RED evidence has two halves:
the lane's tests against current `develop`, which is what the defect looks like today, and
mutants applied at the code-final head, which any reader can reproduce.

The code-final head is `8fac078fb6ae0b82b1ebb35e6903adec001702de`, the last commit that
touches anything outside `docs/`. Every mutant, GREEN, live-proof, gate and linter-liveness
block below was measured at that commit, with no exception, and the gates header names it.
The single commit after it changes this file only. The one block measured elsewhere is the
base RED, which is measured at `origin/develop` on purpose and says so in its own header.

### RED, the lane's tests against `origin/develop`

A detached worktree at `origin/develop` with the lane's three final test files copied in, as
they stand at the code-final head.

```
$ date -u
Tue Sep 15 14:31:08 UTC 2026
$ git rev-parse HEAD
753bd40ef85af9da73beb214c698bb10ce713c97
$ git status --porcelain
A  components/ledger/internal/services/fees/update_package_minimum_test.go
A  components/ledger/pkg/feeshared/model/deductible_caps_test.go
A  components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/
# github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model [github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model.test]
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:129:14: up.ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:203:41: patch.removesTheFee undefined (type Fee has no field or method removesTheFee)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:231:42: undefined: constant.ErrDuplicateFeeKey
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:238:22: ambiguous().ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:240:42: undefined: constant.ErrDuplicateFeeKey
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:250:31: up.ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:250:82: undefined: constant.ErrDuplicateFeeKey
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:268:25: up.ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:277:41: ambiguous().ValidateStoredFeesAgainstMinimum undefined (type *UpdatePackageInput has no field or method ValidateStoredFeesAgainstMinimum)
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:277:92: undefined: constant.ErrDuplicateFeeKey
components/ledger/pkg/feeshared/model/update_package_minimum_test.go:277:92: too many errors
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model [build failed]
--- FAIL: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
    update-package-by-id.go:73: Unexpected call to *pack.MockRepository.Update([context.Background.WithValue(trace.traceContextKeyType, global.nonRecordingSpan) d2be7d04-2a0d-4e35-b9d1-d2aaefc54366 1f46033b-39ac-424e-8e99-9fedd5de2f06 00000000-0000-0000-0000-000000000000 {"$set":{"minimum_amount":"1","updated_at":{"$date":{"$numberLong":"1789482669297"}}}}]) at /tmp/hand2494-develop/components/ledger/internal/services/fees/update-package-by-id.go:73 because: there are no expected calls of the method "Update" for that receiver
--- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.00s)
    --- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_sets (0.00s)
        update_package_minimum_test.go:354: 
            	Error Trace:	/tmp/hand2494-develop/components/ledger/internal/services/fees/update_package_minimum_test.go:354
            	Error:      	An error is expected but got nil.
            	Test:       	TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_sets
    --- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_sets (0.00s)
        update_package_minimum_test.go:350: 
            	Error Trace:	/tmp/hand2494-develop/components/ledger/internal/services/fees/update_package_minimum_test.go:350
            	Error:      	Received unexpected error:
            	            	0211 - Can not update deductible value to true. Calculation value is bigger than the minimum amount 100 for Fee fee1.
            	Test:       	TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_sets
--- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum (0.01s)
    --- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_adds (0.00s)
        update_package_minimum_test.go:271: 
            	Error Trace:	/tmp/hand2494-develop/components/ledger/internal/services/fees/update_package_minimum_test.go:271
            	Error:      	An error is expected but got nil.
            	Test:       	TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_adds
    --- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_adds (0.00s)
        update_package_minimum_test.go:277: 
            	Error Trace:	/tmp/hand2494-develop/components/ledger/internal/services/fees/update_package_minimum_test.go:277
            	Error:      	Received unexpected error:
            	            	0211 - Can not update deductible value to true. Calculation value is bigger than the minimum amount 100 for Fee fee2.
            	Test:       	TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_adds
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.043s
FAIL
rc=1
```

The first failure is the defect itself: the write went through, carrying
`minimum_amount: "1"` to a package whose stored deductible fee is 25. The two rows under
`MeasuresAddedFeesAgainstTheNewMinimum` are the branch that wrote the live invalid packages,
and they appear here only because this block runs the final test files.

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

### RED, five mutants at the code-final head

Each mutant is applied in a detached worktree at the code-final head, run, then reverted with
`git checkout --`. The header lines are from the start of the block; the worktree is clean
before each mutant and clean again after the last. Test output is filtered through the grep
shown on each command line, and `rc` is the exit code of `go test`, not of the pipeline.

```
$ date -u
Tue Sep 15 13:15:26 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)
```

**M1 - the added-fee branch reads a fixed minimum instead of the effective one.** This is the
mutant that survived the whole suite, integration tests included, before this round.

```
$ sed -i 's|err := fee.ValidateNewFee(key, minAmount)|err := fee.ValidateNewFee(key, decimal.NewFromInt(100))|' components/ledger/internal/services/fees/update-package-by-id.go
$ go test -count=1 ./components/ledger/internal/services/fees/ 2>&1 | /usr/bin/grep -E 'FAIL|PASS|ok |---'
--- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum (0.01s)
    --- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_adds (0.00s)
    --- FAIL: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_adds (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.040s
FAIL
rc=1
$ git checkout -- components/ledger/internal/services/fees/update-package-by-id.go
$ git status --porcelain
(no output)
```

**M2 - the removal predicate goes back to the narrow form**, that is the whole body of
`removesTheFee` replaced by
`return f.ValidateIfFeeIsNil() && commons.IsNilOrEmpty(f.RouteFrom) && commons.IsNilOrEmpty(f.RouteTo)`.

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/ 2>&1 | /usr/bin/grep -E 'FAIL|ok |---'
--- FAIL: TestFeeRemovalPredicateAgreesWithTheApplyPath (0.01s)
    --- FAIL: TestFeeRemovalPredicateAgreesWithTheApplyPath/an_entry_whose_only_field_the_writer_reads_as_empty (0.00s)
    --- FAIL: TestFeeRemovalPredicateAgreesWithTheApplyPath/an_entry_whose_calculation_model_carries_nothing (0.00s)
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.01s)
    --- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum/patch_removes_the_offending_fee_with_an_empty_calculation_model (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.058s
--- FAIL: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
    --- FAIL: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee/the_entry_carries_an_empty_calculation_model (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.040s
FAIL
rc=1
$ git checkout -- components/ledger/pkg/feeshared/model/update_package_input.go
$ git status --porcelain
(no output)
```

The two agreement rows that go red here are the point of the mutant: an entry carrying an
empty calculation model, and an entry whose only field is one the field writer reads as empty
(`FeeLabel: "null"`, which lib-commons treats as empty). The narrow predicate calls both of
them a real edit while the apply path writes nothing, which is exactly the disagreement this
round removed.

**M3 - colliding patch keys are resolved instead of refused**, that is the duplicate guard
inside `normalisedFees` is deleted and the later entry simply overwrites the earlier one.

```
$ date -u
Tue Sep 15 13:16:02 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ 2>&1 | /usr/bin/grep -E 'FAIL|ok |---'
--- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys (0.00s)
    --- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys/at_the_request_boundary (0.00s)
    --- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys/a_patch_carrying_no_minimum_is_refused_just_the_same (0.00s)
    --- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys/when_the_stored_fees_are_measured_against_the_new_minimum (0.00s)
    --- FAIL: TestUpdatePackageInputRefusesAmbiguousFeeKeys/every_call_answers_the_same_way (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.055s
FAIL
rc=1
```

The fifth row of that test, the negative control where a body carries three genuinely
different fees, stays green under this mutant, as it must: deleting the guard cannot make a
non-colliding body refused. It is the row that proves the guard refuses ambiguity rather than
refusing several fees.

**M4 - the stored fees are walked in Go map order again**, that is
`for key, storedFee := range storedFees` with the sort kept only as a discarded call.

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ 2>&1 | /usr/bin/grep -E 'FAIL|ok |---'
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.054s
FAIL
rc=1
```

**M5 - the refusal quotes a fixed minimum instead of the one the request sets**, that is
`validateCalculationValues(storedFee.CalculationModel, "2", ...)`. The number is wrong but
still low enough to keep refusing, so on the rows that lower the minimum to 1 the verdict and
the code are unchanged and only the sentence the operator reads moves. Three rows go red.
Removing the two message assertions added in this round, in the second run below, leaves two
of the three green: the only survivor is the row whose verdict itself flips, the one lowering
the minimum to exactly the stored fee. Without those assertions, a refusal telling someone who
set a minimum of 1 to check it against 2 ships with the suite green.

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ 2>&1 | /usr/bin/grep -E 'FAIL|ok |---'
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee (0.00s)
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.01s)
    --- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum/minimum_lowered_to_exactly_the_stored_deductible_flat_fee (0.00s)
    --- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum/minimum_lowered_below_a_stored_deductible_flat_fee (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.052s
FAIL
rc=1
$ sed -i '300d' components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ sed -i '43d' components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ 2>&1 | /usr/bin/grep -E 'FAIL|ok |---'
--- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.01s)
    --- FAIL: TestUpdatePackageInputValidateStoredFeesAgainstMinimum/minimum_lowered_to_exactly_the_stored_deductible_flat_fee (0.00s)
FAIL
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.058s
FAIL
rc=1
$ git checkout -- components/ledger/pkg/feeshared/model/update_package_input.go components/ledger/pkg/feeshared/model/update_package_minimum_test.go
$ git status --porcelain
(no output)
```

### GREEN, every test this PR adds, at the code-final head

```
$ date -u
Tue Sep 15 14:27:17 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)
$ go test -count=1 -v ./components/ledger/pkg/feeshared/model/ ./components/ledger/internal/services/fees/ -run 'CapsDeductiblePercentageWithoutMinimum|ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount|RefusesAmbiguousFeeKeys|RemovalPredicateAgreesWithTheApplyPath|MinimumUnderStoredDeductibleFee|AcceptsALoweredMinimumWhenThePatchRemovesTheFee|MeasuresPatchedFeesAgainstTheNewMinimum|MeasuresAddedFeesAgainstTheNewMinimum' 2>&1 | /usr/bin/grep -E '^(--- |ok  |FAIL)'
--- PASS: TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum (0.00s)
--- PASS: TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum (0.00s)
--- PASS: TestUpdatePackageInputEffectiveMinimumAmount (0.00s)
--- PASS: TestFeeRemovalPredicateAgreesWithTheApplyPath (0.00s)
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.00s)
--- PASS: TestUpdatePackageInputRefusesAmbiguousFeeKeys (0.00s)
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimumNamesOneFee (0.02s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.045s
--- PASS: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresAddedFeesAgainstTheNewMinimum (0.00s)
--- PASS: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.031s
rc=0
```

The refusal tests declare no `Update` expectation on the repository mock, or assert the
captured payload is nil, so they pass only while nothing is written.

The subtests of the two model test files run in parallel. The run below pauses twenty-eight
tests: the five test functions its filter selects, and their twenty-three subtests.

```
$ date -u
Tue Sep 15 14:27:25 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)
$ go test -count=1 -v ./components/ledger/pkg/feeshared/model/ -run 'CapsDeductiblePercentageWithoutMinimum|ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount' 2>&1 | /usr/bin/grep -c "=== PAUSE"
28
rc=0
```

### Live proof against a real MongoDB

The same update runs against a MongoDB testcontainer through the real repository. At
`origin/develop` the lowered minimum is accepted and the write leaves for the repository,
which is what the first RED block above catches against a mock; at the code-final head it is
refused and the stored document keeps its minimum, its fee and its timestamp, all three
asserted against a real database.

```
$ date -u
Tue Sep 15 14:27:35 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)
$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 -v -run 'TestIntegration_UpdatePackage_' ./components/ledger/internal/services/fees/ 2>&1 | /usr/bin/grep -E '^(--- |ok  |FAIL|PASS)'
--- PASS: TestIntegration_UpdatePackage_LoweredMinimumLeavesTheStoredPackageUntouched (1.65s)
--- PASS: TestIntegration_UpdatePackage_MinimumAboveTheStoredFeeIsApplied (2.36s)
PASS
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	5.314s
rc=0
```

The second case passes at both commits: a minimum of 30, still above the stored fee of 25, is
applied before and after. The fix refuses the invalid move, not the valid one.

### Gates at `8fac078fb6ae0b82b1ebb35e6903adec001702de`

```
$ date -u
Tue Sep 15 13:09:55 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)

$ go build ./...
rc=0

$ gofmt -l components/ledger/pkg/feeshared/model components/ledger/internal/services/fees pkg
rc=0

$ go vet ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/... ./pkg/...
rc=0

$ GOLANGCI_LINT_CACHE=/tmp/gcl-midaz2494 /tmp/gcl-bin-midaz-fee/golangci-lint run --allow-parallel-runners ./components/ledger/pkg/feeshared/model/... ./components/ledger/internal/services/fees/...
0 issues.
rc=0
```

```
$ date -u
Tue Sep 15 14:27:52 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
(no output)

$ go test -count=1 ./components/ledger/... ./pkg/...
ok  	github.com/LerianStudio/midaz/v4/components/ledger	0.028s
?   	github.com/LerianStudio/midaz/v4/components/ledger/cmd/app	[no test files]
?   	github.com/LerianStudio/midaz/v4/components/ledger/cmd/backfill	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in	1.073s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware	0.017s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees	0.020s
?   	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack	0.223s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/onboarding	0.028s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction	0.026s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account	0.024s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/accounttype	0.030s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset	0.029s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/assetrate	0.029s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance	0.026s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/completion	0.045s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger	0.022s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation	0.056s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute	0.026s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization	0.024s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio	0.025s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/readseam	0.022s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment	0.022s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction	0.072s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionquarantine	0.022s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute	0.025s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq	0.648s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache	0.033s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/engine	2.697s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/onboarding	0.027s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction	0.065s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction/balance	0.033s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer	0.082s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/bootstrap	1.473s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/audit	0.027s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/dupkey	0.021s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/encryption	0.033s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/holder	0.040s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/adapters/mongodb/instrument	0.040s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services	0.040s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/crm/services/encryption	0.101s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting	0.023s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services	0.023s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/backfill	0.029s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command	3.507s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/composition	0.033s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.057s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/midaz	0.024s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query	0.065s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/migrations/onboarding	0.023s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/migrations/transaction	0.026s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/migrations-image	0.036s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/fee	0.079s
?   	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal	0.022s
?   	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.072s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp	0.041s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting	0.017s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/spanattr	0.030s
ok  	github.com/LerianStudio/midaz/v4/pkg	0.022s
ok  	github.com/LerianStudio/midaz/v4/pkg/buildinfo	0.026s
ok  	github.com/LerianStudio/midaz/v4/pkg/constant	0.024s
ok  	github.com/LerianStudio/midaz/v4/pkg/crypto	0.022s
ok  	github.com/LerianStudio/midaz/v4/pkg/crypto/kms/vault	3.456s
ok  	github.com/LerianStudio/midaz/v4/pkg/crypto/tink	0.029s
?   	github.com/LerianStudio/midaz/v4/pkg/mbootstrap	[no test files]
ok  	github.com/LerianStudio/midaz/v4/pkg/mmodel	0.052s
ok  	github.com/LerianStudio/midaz/v4/pkg/mongo	0.021s
ok  	github.com/LerianStudio/midaz/v4/pkg/mtransaction	0.055s
ok  	github.com/LerianStudio/midaz/v4/pkg/net/http	0.174s
?   	github.com/LerianStudio/midaz/v4/pkg/pagination	[no test files]
?   	github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1	[no test files]
ok  	github.com/LerianStudio/midaz/v4/pkg/rabbitmq	0.026s
?   	github.com/LerianStudio/midaz/v4/pkg/repository	[no test files]
ok  	github.com/LerianStudio/midaz/v4/pkg/servicediscovery	0.022s
ok  	github.com/LerianStudio/midaz/v4/pkg/skip	0.017s
ok  	github.com/LerianStudio/midaz/v4/pkg/streaming	0.038s
ok  	github.com/LerianStudio/midaz/v4/pkg/streaming/events	0.036s
ok  	github.com/LerianStudio/midaz/v4/pkg/utils	0.028s
rc=0

$ go test -race -count=1 ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/...
?   	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/bsondecimal	1.036s
?   	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/constant	[no test files]
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	1.240s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/nethttp	1.096s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	1.102s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/midaz	1.040s
rc=0

$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 ./components/ledger/internal/adapters/mongodb/fees/... ./components/ledger/internal/services/fees/...
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees	13.669s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/billing_package	24.302s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/fees/pack	27.807s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	3.605s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/midaz	0.017s
rc=0
```

The repository's own gates, at the same head: `make check-tests` rc=0, `make check-telemetry`
rc=0 with all five gates green. `make check-logs` exits 2 on this machine because the Makefile
invokes the script with `sh` while the script is written in bash; run with `bash` it exits 0
with 62 advisory warnings, none of them in the files this PR touches, and this branch changes
nothing under `scripts/`.

`golangci-lint` is invoked by absolute path because the version on `PATH` is `v2.12.2` built
with `go1.26`, which refuses a module targeting `1.27.0`. The binary at
`/tmp/gcl-bin-midaz-fee/golangci-lint` is the CI pin from
`.github/workflows/pr-validation.yml:40`, `v2.13.2`, rebuilt locally with `go1.27.0`. The
line above is the one that was run, verbatim.

That gate answers `0 issues.` on a clean tree, so it is proved live rather than assumed:
planting one unused variable in `update_package_input.go`, in a detached worktree, turns it
red.

```
$ date -u
Tue Sep 15 13:39:03 UTC 2026
$ git rev-parse HEAD
8fac078fb6ae0b82b1ebb35e6903adec001702de
$ git status --porcelain
 M components/ledger/pkg/feeshared/model/update_package_input.go
$ GOLANGCI_LINT_CACHE=/tmp/gcl-midaz2494-probe2 /tmp/gcl-bin-midaz-fee/golangci-lint run --allow-parallel-runners ./components/ledger/pkg/feeshared/model/... ./components/ledger/internal/services/fees/...
components/ledger/pkg/feeshared/model/update_package_input.go:623:5: var implUnusedProbe is unused (unused)
var implUnusedProbe = 1
    ^
1 issues:
* unused: 1
rc=1
```

The probe runs in a detached worktree at the code-final head, and the tree is clean again
after `git checkout --`. The planted variable is the last line of the file, which is why the
report names line 623.
