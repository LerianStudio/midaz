# Deductible fee caps: one answer on create and on a minimum change

**Goal:** make the fee package validators agree with each other. A deductible fee is taken
out of the payment it is charged on, so it can never exceed that payment. Two paths read
that rule differently, and one package update skipped it entirely.

**Scope:** `components/ledger/pkg/feeshared/model` (the shared validators) and
`components/ledger/internal/services/fees` (the update use case, where the stored package is
already loaded). No schema change, no API shape change, no new error code.

**Status:** both defects closed on `fix/fee-deductible-validators`, plus one review round,
measured at `7bc74b1753a3b19d7d7875258dea3325e795fad7`.

## Phase overview

| Phase | Outcome | Status |
|---|---|---|
| P1 | A deductible percentage above 100 is refused whether or not the package declares a minimum | Complete, commit `c70f62af3` |
| P2 | A package update is judged against the minimum it will carry, for the fees it keeps and the fees it restates | Complete, commit `1e1181c47` |
| P3 | A fee the same patch settles does not block the minimum change | Complete, commit `7bc74b175`, from review |

## P1 - The percentage cap no longer waits for a minimum

### P1.1 What was wrong

`validateCalculationValues` capped a deductible percentage at 100 only inside
`if minAmount != "" && isDeductible`. The update-side validator
(`validatePercentageCalculation`) capped it with no minimum clause at all. The same fee was
therefore judged by two different rules depending on which validator saw it.

Reachable consequence: a package update whose body carries fees and no `minimumAmount`
passed the boundary validator untouched and was only refused deeper, under `0210`, whose
message reads "Can not update deductible value to true" even when the caller never touched
the deductible flag.

### P1.2 What changed

- [x] The percentage cap runs for every deductible calculation, minimum or no minimum, and
      answers `0207` as it already did on create.
- [x] The flat cap still waits for a minimum, because without one it has no amount to
      measure the fee against.
- [x] Non-deductible fees are untouched: they are charged on top of the payment, so they may
      exceed it.
- [x] One existing case in `TestValidateCalculationValues` was renamed from "Empty minAmount
      - should skip deductible validation" to "Empty minAmount - deductible flat has no
      minimum to exceed". It pinned the flat half, which is preserved; only its name claimed
      more than the code now does. No existing case changed its expectation.

## P2 - A minimum change is judged against the fees the package keeps

### P2.1 What was wrong

`UpdatePackageInput.ValidateFees` runs only when the patch carries fees, and
`buildUpdateFields` validated fees only when `up.Fee != nil`. A patch of `{"minimumAmount":
"1"}` against a package storing a deductible flat fee of 25 was therefore accepted and
written. The package then accepted payments of 1 that it could not charge its own fee on,
and every such payment failed later at transaction time under `0233`
(`ErrDeductibleFeeExceedsAmount`). A create of the same configuration is refused by `0208`.

A second, quieter half: fees the patch did carry were validated against the STORED minimum,
never the new one. That made a raised minimum refuse a fee that fits inside it, and would
have let the check above be evaded by restating the offending fee in the same call.

### P2.2 What changed

- [x] `UpdatePackageInput.EffectiveMinimumAmount` returns the minimum the package will carry:
      the patched value when the patch sets one, otherwise the stored value.
- [x] `UpdatePackageInput.ValidateStoredFeesAgainstMinimum` checks the fees already stored
      against that new minimum, through the same shared validator a create uses, so the
      answer carries the create codes `0207` and `0208`.
- [x] Fees whose calculations the patch restates are skipped there and validated as they are
      applied, against the same new minimum. Patch keys are matched by their lower camel
      form, the way the update applies them, so a key differing only in case is the same fee.
- [x] The use case passes the effective minimum to `validationFeesSetUnset`, so both a new
      fee and a patched fee are measured against where the package lands.
- [x] The check runs after the existing min/max validation, so a malformed or inverted
      minimum still answers its own error first.

### P2.3 Deliberately not changed

- A package already stored in violation (written before this change) is refused only when a
  patch moves its minimum. An unrelated patch, a label or a description, still succeeds. A
  sweep that refuses every update to a legacy-invalid package would lock those packages out
  of editing entirely.
- The transaction-time guard `0233` stays. It is the last line for a payment whose amount sits
  between the minimum and the fee, which package-level validation cannot see.

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
- [x] Removal is recognised exactly as the update applies it, every field empty including
      the two routes `ValidateIfFeeIsNil` does not cover, so a patch that only sets a route
      keeps the fee and still faces the check.
- [x] A patch that only renames the fee, or that confirms `isDeductibleFrom: true`, leaves
      the fee in place and is still refused.

Raised by CodeRabbit on PR #2494 and confirmed against the code before fixing: both cases
were refusals the operator did not deserve, not accepted bad states.

## Found by

Console lane #963 measured the two validators against midaz `origin/develop` while closing
the console-side drift: the console refuses both configurations at its own boundary, and the
ledger did not agree with itself for direct API callers.

## Verification

Every command below was run verbatim in `/srv/worktrees/midaz-fee-validators`. The RED runs
are at the parent of each fix; the gates are at the code-final head
`1e1181c477594de6d8a1e67f5fbbfde2ee4dddf9`.

### RED, P1, at `94ada9bd34987b9a05009b4e5103692045a23394`

```
$ go test ./components/ledger/pkg/feeshared/model/ -run 'CapsDeductiblePercentageWithoutMinimum' -v
--- FAIL: TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum (0.00s)
    --- FAIL: .../deductible_percentage_over_100_without_a_minimum (0.00s)
        deductible_caps_test.go:102: An error is expected but got nil.
    --- PASS: .../deductible_percentage_at_100_without_a_minimum (0.00s)
    --- PASS: .../percentage_over_100_that_is_not_deductible (0.00s)
    --- PASS: .../deductible_flat_without_a_minimum_has_nothing_to_exceed (0.00s)
    --- PASS: .../deductible_percentage_over_100_with_a_minimum (0.00s)
    --- PASS: .../deductible_flat_over_the_minimum (0.00s)
--- FAIL: TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum (0.00s)
        deductible_caps_test.go:116: An error is expected but got nil.
FAIL
```

### GREEN, P1, at `c70f62af3d431fb73529065db9300257ff1e3bbb`

```
$ go test ./components/ledger/pkg/feeshared/model/ -run 'CapsDeductiblePercentageWithoutMinimum' -v
rc=0
--- PASS: TestValidateCalculationValuesCapsDeductiblePercentageWithoutMinimum (0.00s)  [6/6 cases]
--- PASS: TestUpdatePackageInputValidateFeesCapsDeductiblePercentageWithoutMinimum (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.019s
```

### RED, P2, at `c70f62af3d431fb73529065db9300257ff1e3bbb`

```
$ go test ./components/ledger/internal/services/fees/ -run 'MinimumUnderStoredDeductibleFee|MeasuresPatchedFeesAgainstTheNewMinimum' -v
rc=1
--- FAIL: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
    update-package-by-id.go:73: Unexpected call to *pack.MockRepository.Update(...
      {"$set":{"minimum_amount":"1","updated_at":...}}) because: there are no expected calls
--- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_lowered_under_the_fee_the_patch_sets
        An error is expected but got nil.
--- FAIL: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum/minimum_raised_above_the_fee_the_patch_sets
        Received unexpected error: 0211 - Can not update deductible value to true.
        Calculation value is bigger than the minimum amount 100 for Fee fee1.
FAIL
```

The first failure is the defect itself: the write went through, carrying
`minimum_amount: "1"` to a package whose stored deductible fee is 25.

```
$ go test ./components/ledger/pkg/feeshared/model/ -run 'ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount'
rc=1
update_package_minimum_test.go:83:14: up.ValidateStoredFeesAgainstMinimum undefined
update_package_minimum_test.go:102:24: up.EffectiveMinimumAmount undefined
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model [build failed]
```

### GREEN, P2, at `1e1181c477594de6d8a1e67f5fbbfde2ee4dddf9`

```
$ go test ./components/ledger/internal/services/fees/ -run 'MinimumUnderStoredDeductibleFee|MeasuresPatchedFeesAgainstTheNewMinimum' -v
rc=0
--- PASS: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.00s)
    --- PASS: .../minimum_raised_above_the_fee_the_patch_sets (0.00s)
    --- PASS: .../minimum_lowered_under_the_fee_the_patch_sets (0.00s)

$ go test ./components/ledger/pkg/feeshared/model/ -run 'ValidateStoredFeesAgainstMinimum|EffectiveMinimumAmount' -v
rc=0
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.00s)  [9/9 cases]
--- PASS: TestUpdatePackageInputEffectiveMinimumAmount (0.00s)  [3/3 cases]
```

The refusal test declares no `Update` expectation on the repository mock, so it passes only
while nothing is written.

### Live proof against a real MongoDB

The same update runs against a MongoDB testcontainer through the real repository, first at the
base commit and then at the fix.

```
$ cd /tmp/rv-midaz-fee-validators-base   # detached worktree at 94ada9bd3
$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 -run 'TestIntegration_UpdatePackage_' ./components/ledger/internal/services/fees/
rc=1
--- FAIL: TestIntegration_UpdatePackage_LoweredMinimumLeavesTheStoredPackageUntouched (4.77s)
        update_package_minimum_integration_test.go:77: An error is expected but got nil.
FAIL
```

```
$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 -run 'TestIntegration_UpdatePackage_' ./components/ledger/internal/services/fees/ -v
rc=0
--- PASS: TestIntegration_UpdatePackage_LoweredMinimumLeavesTheStoredPackageUntouched (2.01s)
--- PASS: TestIntegration_UpdatePackage_MinimumAboveTheStoredFeeIsApplied (1.11s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	4.756s
```

The second case passes at both commits: a minimum of 30, still above the stored fee of 25, is
applied before and after. The fix refuses the invalid move, not the valid one.

### RED, P3, at `d89f3794ed432da0543671553cc50eecc886a407`

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ -run 'ValidateStoredFeesAgainstMinimum'
rc=1
--- FAIL: .../patch_removes_the_offending_fee_in_the_same_call (0.00s)
        Received unexpected error
--- FAIL: .../patch_stops_the_offending_fee_being_deducted_from_the_payment (0.00s)
        Received unexpected error
FAIL

$ go test -count=1 ./components/ledger/internal/services/fees/ -run 'AcceptsALoweredMinimumWhenThePatchRemovesTheFee' -v
rc=1
--- FAIL: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
        Received unexpected error
FAIL
```

The two guard cases added in the same round, a route-only patch and a patch confirming the
fee stays deductible, passed at this commit and still pass: the skip must not widen into
them.

### GREEN, P3, at `7bc74b1753a3b19d7d7875258dea3325e795fad7`

```
$ go test -count=1 ./components/ledger/pkg/feeshared/model/ -run 'ValidateStoredFeesAgainstMinimum' -v
rc=0
--- PASS: TestUpdatePackageInputValidateStoredFeesAgainstMinimum (0.00s)  [13/13 cases]

$ go test -count=1 ./components/ledger/internal/services/fees/ -run 'AcceptsALoweredMinimumWhenThePatchRemovesTheFee|MinimumUnderStoredDeductibleFee|MeasuresPatchedFees' -v
rc=0
--- PASS: TestUpdatePackageByIDAcceptsALoweredMinimumWhenThePatchRemovesTheFee (0.00s)
--- PASS: TestUpdatePackageByIDRefusesMinimumUnderStoredDeductibleFee (0.00s)
--- PASS: TestUpdatePackageByIDMeasuresPatchedFeesAgainstTheNewMinimum (0.00s)  [2/2 cases]
```

### Gates at `7bc74b1753a3b19d7d7875258dea3325e795fad7`

```
$ go test -count=1 ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/... ./components/ledger/internal/adapters/http/in/...
rc=0   (7 packages ok)

$ go test -race -count=1 ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/...
rc=0   (5 packages ok, 2 with no test files)

$ go build ./...
rc=0

$ gofmt -l components/ledger/pkg/feeshared/model components/ledger/internal/services/fees
rc=0   (no output)

$ go vet ./components/ledger/pkg/feeshared/... ./components/ledger/internal/services/fees/...
rc=0

$ GOLANGCI_LINT_CACHE=/tmp/gcl-midaz-fee-validators golangci-lint run --allow-parallel-runners \
    ./components/ledger/pkg/feeshared/model/... ./components/ledger/internal/services/fees/...
rc=0
0 issues.

$ go test -count=1 ./components/ledger/... ./pkg/...
rc=0   ok=71 fail=0

$ ALLOW_INSECURE_TLS=true go test -tags integration -p=1 -count=1 \
    ./components/ledger/internal/adapters/mongodb/fees/... ./components/ledger/internal/services/fees/...
rc=0
ok  	.../adapters/mongodb/fees	19.761s
ok  	.../adapters/mongodb/fees/billing_package	30.854s
ok  	.../adapters/mongodb/fees/pack	39.671s
ok  	.../services/fees	4.368s
ok  	.../services/fees/midaz	0.017s
```

`golangci-lint` is the CI-pinned `v2.13.2`, rebuilt locally with the go1.27.0 toolchain: the
version on PATH was built with go1.26 and refuses a module targeting 1.27.0.
