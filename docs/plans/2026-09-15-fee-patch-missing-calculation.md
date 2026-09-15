# A fee added without calculation values is refused, not crashed

- Repository: LerianStudio/midaz, branch `fix/fee-patch-missing-calculation`, base `develop`
- Date: 2026-09-15
- Code-final commit: `d8b2b5e61036b88fc0252a35f220d9ba69ab6d3a`
- Measured at: `d8b2b5e61036b88fc0252a35f220d9ba69ab6d3a`, the branch merged with
  `origin/develop` at `b8457c2817f2ded83b2a63de2a60b4e043569b15`. Every block in
  Verification was run on that tree, not on the branch point: `origin/develop`
  moved inside both files this change touches while the branch sat behind it.

## Phase overview

| Phase | What it changes | State |
| --- | --- | --- |
| 1 | A PATCH that adds a fee with no calculation model is refused with 0187 instead of crashing the request | Done, `7d6154ceb` |
| 2 | The refusal is pinned by its whole sentence, at the endpoint, at the service and in the package that owns the check | Done, `660c01615` |
| 3 | The guards that keep a nil calculation model away from the unguarded read on the existing-fee path are pinned | Done, `d8b2b5e61` |
| 4 | This plan document | Done |

## What an operator could not trust before

A fee package is patched by sending the fees to add or change. A fee that is
being added must carry its calculation model: the application rule and the
calculations that decide how much the fee is.

Sending a fee entry that carried a label and nothing else produced HTTP 500. The
request was refused, so no fee was created, but the caller was told only that the
server failed. Nothing named the missing field, nothing distinguished it from an
outage, and the same body repeated forever. A console or an integration driving
fee setup could not tell a malformed request from a broken service.

The only reason the defect stayed narrow is an accident of ordering: a fee entry
with no label at all was turned down by the label check before the missing
calculation model was ever read. A label was enough to reach it.

## How the two endpoints answer this body, measured

The earlier version of this document and of the pull request body said that
patching and creating now give the same answer. They do not, and the difference
is worth stating plainly, because an integration keying on a code will branch on
it.

Both endpoints refuse the body with HTTP 400 and both name the missing thing.
They name it in different places, because they refuse it at different layers:

| Endpoint | Refused by | Code | What the caller reads |
| --- | --- | --- | --- |
| POST (create) | request-body validation | 0009 | a `fields` map: `fees[adminFee].calculationModel is a required field` |
| PATCH (update) | the package service | 0187 | a message: `The calculation model is required for fee adminFee.` |

The mechanism: `CreatePackageInput.Fee` carries `validate:"required,min=1,dive"`,
so the struct validator descends into every fee in the map and enforces the
`required` on `CalculationModel` before any handler code runs.
`UpdatePackageInput.Fee` carries no validate tag at all, so there is no descent
and the body reaches the service intact. That is not an oversight to correct: a
PATCH of a fee that already exists may legitimately omit its calculation model,
and only the service knows which fees in the map are new. So the service is the
only layer that can tell a new fee off, and 0187 is the code it uses.

`CreatePackageInput.ValidateFees` does hold a 0187 branch for a nil calculation
model, but over HTTP nothing reaches it for this body: the request-body
validation answers first. Measured, both endpoints, same fee shape:

```
== POST (create), fee complete EXCEPT the calculation model ==
1. request-body validation (ValidateStruct)          -> {"title":"Missing Fields in Request","code":"0009","fields":{"calculationModel":"fees[adminFee].calculationModel is a required field"}}

== POST (create), same fee shape as the PATCH under test ==
1. request-body validation (ValidateStruct)          -> {"title":"Missing Fields in Request","code":"0009","fields":{"calculationModel":"fees[adminFee].calculationModel is a required field","creditAccount":"fees[adminFee].creditAccount is a required field","isDeductibleFrom":"fees[adminFee].isDeductibleFrom is a required field"}}
2. input.ValidateFees() [only if 1 passed]           -> "0187 - The calculation model is required for fee adminFee."

== PATCH (update), same fee shape ==
1. request-body validation (ValidateStruct)          -> nil (accepted at this layer)
2. input.ValidateFees() [only if 1 passed]           -> nil (accepted at this layer)
3. the service's ValidateNewFee, per fee             -> "0187 - The calculation model is required for fee adminFee."
```

## Epic 1: refuse the fee, name what is missing

### Task 1.1: check the calculation model on its own, before anything reads it

`components/ledger/pkg/feeshared/model/package.go`

The required-field check tested seven things in one chained condition, two of
which read through the calculation model pointer. The model is now checked by
itself, ahead of the chain, and `validateRequiredFields` takes the fee key so the
refusal can name the fee.

The fee label was the first clause of that same chain, and it is now its own
check, ahead of the model, so that every shape already refused keeps the exact
answer it had. An entirely empty fee entry still answers `0192`, not `0187`: it
was refused before this change, and a refusal code an integration may branch on
is a contract, unlike a crash. Mutant 5 below holds that in place.

### Task 1.2: pin the refusal table, and pin that nothing is written

`components/ledger/internal/services/fees/update-package-fee-calculation_test.go`

Eight request bodies, each one a real JSON payload unmarshalled into the update
input, each asserted on two things: the refusal code, and which of the two layers
produced it. The request-body validation the handler runs is called first, as the
handler calls it, so the test states plainly which bodies it catches and which
fall through to the service.

The first case pins the whole rendered sentence, not the code. The code says only
that something is missing; the fee name is the half the caller acts on, and a
package may carry a dozen fees.

Nothing may be written: the repository mock carries an explicit `Times(0)`
expectation on `Update`, so the write is what fails the case rather than a later
assertion on an error the write would not have stopped. That is a real assertion
for the five bodies that reach the service; the three refused by request-body
validation never build a repository, so nothing about writes is claimed for them.

| Body | Refused by | Code |
| --- | --- | --- |
| fee carries only a label, no calculation model | service | 0187 (was a crash) |
| label plus an empty calculation model object | service | 0192 |
| calculation model carries no application rule | service | 0192 |
| calculation model carries no calculations | request body | 0189 |
| fee carries no credit account | service | 0192 |
| calculation carries no type | request body | 0191 |
| calculation carries no value | request body | 0204 |
| fee entry is entirely empty | service | 0192 |

### Task 1.3: pin the status the endpoint answers

`components/ledger/internal/adapters/http/in/fees_huma_test.go`

The product claim of this change is a status change, and nothing drove the
handler. `TestUpdatePackage_MissingCalculationModel_Canonical400` now sits beside
the three sibling refusals already pinned on this endpoint, routes the update
through the real fee service rather than a stub that would answer whatever the
test invented, and mounts the recovery boundary the unified server installs in
production. Measured without the fix, that request answers HTTP 500 with code
0046; with it, 400 with 0187 naming the fee.

### Task 1.4: give the owning package a case for the shape it never had

`components/ledger/pkg/feeshared/model/package_test.go`

`TestFee_ValidateNewFee` is the table written for this function, and all sixteen
of its cases built a non-nil calculation model, which is why the crash was never
seen in the package that owns the check. It gains two cases: a fee with a label
and no calculation model, pinned by its whole sentence, and a fee with neither,
pinned on 0192 so the shape that was already refused is held in place.

### Task 1.5: pin the guards on the existing-fee path

`components/ledger/pkg/feeshared/model/package_test.go`

`hasNoCalculationModelUpdates` reads the same pointer with no check of its own.
It is safe because both routes into it refuse a nil model first: `removesTheFee`
short-circuits on it, and `updateCalculationModel` wraps the call in a nil test.
Nothing said so, so a third route, or either guard dropped in a refactor, brings
the same crash back on the patch of a fee that already exists.

A partial patch of an existing fee may legitimately omit its calculation model,
so the answer is the guards, not a refusal. The test pins them: a fee entry with
a label and no calculation model goes through both routes and must come back
without a write and without a crash.

## Found by this work, not fixed

- `make test` does not run on a Debian box. `mk/tests.mk:92` invokes
  `sh ./scripts/run-tests.sh`, and that script is bash: line 8 reads
  `${BASH_SOURCE[0]}`, which dash cannot expand. `/bin/sh` is dash here, so the
  target reports `Bad substitution`, resolves the project root to the parent
  directory, and exits 2 with `pattern ./...: directory prefix . does not contain
  main module`. It works wherever `/bin/sh` is bash, which is why nobody has hit
  it; no CI workflow invokes the target, so it is latent rather than broken in
  the pipeline. Given a bash the same harness passes, 107 ok and 0 FAIL. The fix
  is one word in the Makefile or in the script's use of `$0`, and it belongs in a
  change about the build, not in a change about fees.
- `.github/pull_request_template.md` asks for `make test`, `make test-int`,
  `make sec` and `make vulncheck`. Of those only `make test` exists, and only
  under the constraint above; `make test-integration` exists but under a
  different name. Every pull request that ticks those boxes ticks something it
  did not run.
- A fee stored without a calculation model does NOT crash fee calculation. An
  earlier version of this document said it did, and a review of this branch
  raised the same thing; both are wrong, and the correction is worth recording
  because it is the money path. `calculate-fee.go` does read
  `fee.CalculationModel.ApplicationRule` with no nil check, but nothing can hand
  it a nil: the Mongo-side `pack.Fee.CalculationModel` is a value type, not a
  pointer, so a document with the key absent decodes to the zero struct, and
  `ToEntityFeeMap` then wraps it unconditionally in a non-nil
  `&model.CalculationModel{}`. Both constructions of a package for reading
  (`find.go:330` and `PackageMongoDBModel.ToEntity`) go through that function.
  Measured, decoding a package document whose fee carries no `calculation_model`:

  ```
  mongo-side CalculationModel (value type): pack.CalculationModel{ApplicationRule:"", Calculations:[]pack.Calculation(nil)}
  after ToEntity, model.Fee.CalculationModel == nil ? false
  ApplicationRule="" len(Calculations)=0
  ```

  `CalculateFee` then switches on an empty application rule, which falls to the
  default arm and returns `ErrApplicationRule`, "unknown application rule: ". An
  error, not a panic. So this change closes the last door through which a fee
  with no calculation values could be written, and anything already persisted is
  refused at calculation time rather than crashing the transaction. No production
  data was inspected.

## Verification

Every block below was run verbatim on this box, in the order shown, at the head
each block names.

The first version of this document claimed RED came first and carried `date -u`
headers that said otherwise: GREEN at 15:59:08, gates at 16:00:42, RED at
16:03:06, all at the branch point. So the test was written against a crash that
had already been diagnosed and fixed, and the RED was reconstructed afterwards in
a throwaway worktree. The defect was real either way, but the discipline was not
what the document asserted. Everything below is the re-take, in order, on the
merged tree: RED at 17:22:45, GREEN at 17:23:29, gates at 17:30:52.

### RED, at the merge head, with only the tests applied

The three tests were left in place and the production guard alone was removed, so
this is exactly the tree that would ship if the tests had been written and the
fix had not.

```
$ date -u
Tue Sep 15 17:22:45 UTC 2026
$ git rev-parse HEAD
6c94b0df36ebc547cff99d1ff071dfd59b173220
$ git status --porcelain
 M components/ledger/pkg/feeshared/model/package.go
$ git diff -- components/ledger/pkg/feeshared/model/package.go
-	if f.CalculationModel == nil {
-		return pkg.ValidateBusinessError(constant.ErrCalculationRequired, "", feeKey)
-	}
-
 	if f.CalculationModel.ApplicationRule == "" ||
```

RED 1, the package that owns the check:

```
$ go test -count=1 -run 'TestFee_ValidateNewFee' ./components/ledger/pkg/feeshared/model/
--- FAIL: TestFee_ValidateNewFee (0.00s)
    --- FAIL: TestFee_ValidateNewFee/Nil_CalculationModel (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x8 pc=0xe3d0fd]
	.../components/ledger/pkg/feeshared/model.(*Fee).validateRequiredFields
	/srv/worktrees/midaz-fee-patch-500/components/ledger/pkg/feeshared/model/package.go:229
	.../components/ledger/pkg/feeshared/model.(*Fee).ValidateNewFee
	/srv/worktrees/midaz-fee-patch-500/components/ledger/pkg/feeshared/model/package.go:200
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.022s
rc=1
```

RED 2, the fee update service:

```
$ go test -count=1 -run 'TestUpdatePackageByID_FeeWithoutCalculationIsRefused' ./components/ledger/internal/services/fees/
--- FAIL: TestUpdatePackageByID_FeeWithoutCalculationIsRefused (0.00s)
    --- FAIL: .../fee_carries_only_a_label,_no_calculation_model_at_all (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
	.../model.(*Fee).validateRequiredFields  package.go:229
	.../model.(*Fee).ValidateNewFee          package.go:200
	.../services/fees.(*UseCase).validationFeesSetUnset  update-package-by-id.go:210
rc=1
```

RED 3, the endpoint, which is the answer this change is about:

```
$ go test -count=1 -run 'TestUpdatePackage_MissingCalculationModel_Canonical400' ./components/ledger/internal/adapters/http/in/
--- FAIL: TestUpdatePackage_MissingCalculationModel_Canonical400 (0.00s)
    fees_huma_test.go:905:
        	Error:      	Not equal:
        	            	expected: 400
        	            	actual  : 500
        	Messages:   	body: {"type":"https://errors.lerian.studio/v1/0046","title":"Internal Server Error","status":500,"detail":"The server encountered an unexpected error. Please try again later or contact support.","code":"0046"}
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in	0.030s
rc=1
```

A reader chasing these on `origin/develop` wants different numbers. There the
function is one seven-clause chain, `validateRequiredFields()` takes no fee key,
and the first clause reading through the pointer is `package.go:218`, reached
from `package.go:200` and `update-package-by-id.go:210`. The 229 above is that
same read in the tree this branch ships, lower in the file because the fee label
now has its own check and the guard has a comment explaining why it exists.

### GREEN, guard restored

The restored file is byte-identical to the committed one
(`sha256 f8b9a942967eeb56e6b97f0179b2ce58664336a034b6b79d1ec08d7e18b4a66f`, and
`git status --porcelain` is empty).

```
$ date -u
Tue Sep 15 17:23:29 UTC 2026
$ git rev-parse HEAD
6c94b0df36ebc547cff99d1ff071dfd59b173220
$ git status --porcelain
(empty)

$ go test -count=1 -v -run 'TestFee_ValidateNewFee' ./components/ledger/pkg/feeshared/model/
--- PASS: TestFee_ValidateNewFee/Nil_CalculationModel (0.00s)
--- PASS: TestFee_ValidateNewFee/Nil_CalculationModel_and_no_fee_label (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.020s
rc=0

$ go test -count=1 -v -run 'TestUpdatePackageByID_FeeWithoutCalculationIsRefused' ./components/ledger/internal/services/fees/
--- PASS: TestUpdatePackageByID_FeeWithoutCalculationIsRefused (0.00s)
    --- PASS: .../calculation_model_carries_no_application_rule (0.00s)
    --- PASS: .../calculation_model_carries_no_calculations (0.00s)
    --- PASS: .../calculation_carries_no_type (0.00s)
    --- PASS: .../fee_carries_a_label_and_an_empty_calculation_model_object (0.00s)
    --- PASS: .../fee_entry_is_entirely_empty (0.00s)
    --- PASS: .../fee_carries_only_a_label,_no_calculation_model_at_all (0.00s)
    --- PASS: .../calculation_carries_no_value (0.01s)
    --- PASS: .../fee_carries_no_credit_account (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.027s
rc=0

$ go test -count=1 -v -run 'TestUpdatePackage_MissingCalculationModel_Canonical400|TestUpdatePackage_PriorityOneWrongReference_Canonical400|TestUpdatePackage_DuplicatePriorities_Canonical400|TestUpdatePackage_MinGreaterThanMax_422' ./components/ledger/internal/adapters/http/in/
--- PASS: TestUpdatePackage_PriorityOneWrongReference_Canonical400 (0.00s)
--- PASS: TestUpdatePackage_DuplicatePriorities_Canonical400 (0.00s)
--- PASS: TestUpdatePackage_MinGreaterThanMax_422 (0.00s)
--- PASS: TestUpdatePackage_MissingCalculationModel_Canonical400 (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in	0.052s
rc=0
```

The pin added in Task 1.5, at the code-final head:

```
$ date -u
Tue Sep 15 17:29:43 UTC 2026
$ go test -count=1 -v -run 'TestFee_NilCalculationModelNeverReachesTheUnguardedRead' ./components/ledger/pkg/feeshared/model/
--- PASS: TestFee_NilCalculationModelNeverReachesTheUnguardedRead (0.00s)
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	0.019s
rc=0
```

### Mutants

Seven mutations, each applied to the working tree and reverted, with both
production files proved byte-identical afterwards by sha256 and
`git status --porcelain` empty. The earlier version of this branch left one of
these alive: with the fee name unasserted, a refusal that named no fee at all
passed all 55 ledger packages.

| # | Mutation | Verdict |
| --- | --- | --- |
| 1 | the fee name dropped from the refusal (`feeKey` to `""`) | killed at all three levels |
| 2 | the fee name replaced with `TOTALLY-WRONG-FEE-NAME` | killed at all three levels |
| 3 | the nil guard deleted | killed, panic returns (this is RED above) |
| 4 | refusal code 0187 swapped for 0192 | killed at all three levels |
| 5 | the label refusal swapped from 0192 to 0187 | killed, the unchanged shape is held |
| 6 | `removesTheFee` loses its nil check | killed, panic at `update_package_input.go:518` |
| 7 | `updateCalculationModel` loses its nil check | killed, panic at `update_package_input.go:514` |

```
MUTANT 1, feeKey -> "":
    --- FAIL: TestFee_ValidateNewFee/Nil_CalculationModel (0.00s)
            expected: "0187 - The calculation model is required for fee fee1."
            actual  : "0187 - The calculation model is required for fee ."
$ go test -count=1 ./components/ledger/...
rc=1, FAIL in .../adapters/http/in, .../services/fees, .../pkg/feeshared/model (52 ok)

MUTANT 2, feeKey -> "TOTALLY-WRONG-FEE-NAME":
            expected: "0187 - The calculation model is required for fee adminFee."
            actual  : "0187 - The calculation model is required for fee TOTALLY-WRONG-FEE-NAME."
rc=1, all three packages FAIL

MUTANT 4, ErrCalculationRequired -> ErrFeeFieldsRequired:
            expected: "0187 - The calculation model is required for fee adminFee."
            actual  : "0192 - All fields of a new Fee must be filled. Please check again the payload passed."
            (endpoint) expected: "0187"  actual: "0192"
rc=1, all three packages FAIL

MUTANT 5, the label refusal ErrFeeFieldsRequired -> ErrCalculationRequired:
    --- FAIL: TestFee_ValidateNewFee/Missing_FeeLabel (0.00s)
    --- FAIL: TestFee_ValidateNewFee/Nil_CalculationModel_and_no_fee_label (0.00s)
rc=1

MUTANT 6, removesTheFee loses "f.CalculationModel != nil &&":
    --- FAIL: TestFee_NilCalculationModelNeverReachesTheUnguardedRead (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference
	update_package_input.go:518 (hasNoCalculationModelUpdates)
	update_package_input.go:182 (removesTheFee)
rc=1

MUTANT 7, updateCalculationModel loses "if f.CalculationModel != nil":
    --- FAIL: TestFee_NilCalculationModelNeverReachesTheUnguardedRead (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference
	update_package_input.go:514 (hasNoCalculationModelUpdates)
	update_package_input.go:495 (setAndValidateCalculationModel)
	update_package_input.go:347 (updateCalculationModel)
rc=1

$ sha256sum components/ledger/pkg/feeshared/model/package.go components/ledger/pkg/feeshared/model/update_package_input.go
f8b9a942967eeb56e6b97f0179b2ce58664336a034b6b79d1ec08d7e18b4a66f  .../package.go
d66d229e2ba5241483cb663109a6b596dd3b9bf1e07720aabae269422a225f1b  .../update_package_input.go
(both equal the copies taken before the first mutation; git status --porcelain empty)
```

### Gates, at the code-final commit

```
$ date -u
Tue Sep 15 17:30:52 UTC 2026
$ git rev-parse HEAD
d8b2b5e61036b88fc0252a35f220d9ba69ab6d3a
$ git status --porcelain
(empty)

$ go build ./...
rc=0

$ go vet ./...
rc=0

$ go test -count=1 -race ./components/ledger/...
rc=0
ok lines: 55, FAIL lines: 0, no-test-files: 5
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in	6.443s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	1.125s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	1.267s

$ make test
./scripts/run-tests.sh: 8: Bad substitution
Running go test ./... from /srv/worktrees ...
pattern ./...: directory prefix . does not contain main module or its selected dependencies
FAIL	./... [setup failed]
rc=2
(the target is broken on a dash /bin/sh, see Found by this work; not caused by
this change, and no CI workflow invokes it)

$ bash ./scripts/run-tests.sh          # the same harness, given a bash
rc=0
ok lines: 107, FAIL lines: 0, no-test-files: 29
Duration: 0m:27s
[ok] All tests passed successfully

$ make lint                            # four legs, golangci-lint v2.13.2
rc=0
./components/ledger   Issues before processing: 1197, after processing: 0
./components/tracer   Issues before processing: 437,  after processing: 0
./tests               Issues before processing: 42,   after processing: 0
./pkg                 Issues before processing: 130,  after processing: 0
[ok] Linting completed successfully

$ GOLANGCI_LINT_CACHE=/tmp/gcl-feepatch /tmp/gclbin-feepatch/golangci-lint run --allow-parallel-runners ./components/ledger/...
golangci-lint has version 2.13.2 built with go1.27.0
0 issues.
rc=0

$ gofmt -l <the four .go files this branch touches>
(empty)
rc=0

$ git diff origin/develop...HEAD | python3 -c "import sys; print(sum(l.count(chr(0x2014)) for l in sys.stdin))"
0   # U+2014 count over the whole branch diff
```

The linter on this box refuses the repository, and the reason is the toolchain
that built it rather than its version number: "the Go language version (go1.26)
used to build golangci-lint is lower than the targeted Go version (1.27.0)",
rc=3, deciding nothing, which reads as a silent pass. `make lint` never hits it,
because it runs `go run ...golangci-lint@v2.13.2`, compiling the linter with the
repository's own toolchain. The standalone binary used above is v2.13.2 built
with go1.27.0; a v2.13.2 built with go1.26 refuses exactly the same way.

### Not run

No live stack and no browser run. There is no HTTP server for this component on
this box and the change has no browser surface. What it changes is the status and
body an API caller receives, and that is measured at the endpoint in Task 1.3,
through the real fee service and the production recovery boundary, rather than
inferred from reading the code: 500 with 0046 before, 400 with 0187 after.
