# A fee added without calculation values is refused, not crashed

- Repository: LerianStudio/midaz, branch `fix/fee-patch-missing-calculation`, base `develop`
- Date: 2026-09-15
- Code-final commit: `7d6154cebc1f290c69a7b805f67a12a6511ef625`
- Measured at: `792eb32bd076cb719aba1f2648bffff113ce11f3` (the branch point, an ancestor of `origin/develop`)

## Phase overview

One phase, one code commit, plus this document.

| Phase | What it changes | State |
| --- | --- | --- |
| 1 | A PATCH that adds a fee with no calculation model is refused with 0187 instead of crashing the request | Done, `7d6154ceb` |
| 2 | This plan document | Done |

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

## Epic 1: refuse the fee, name what is missing

### Task 1.1: check the calculation model on its own, before anything reads it

`components/ledger/pkg/feeshared/model/package.go`

The required-field check tested seven things in one chained condition, two of
which read through the calculation model pointer. The model is now checked by
itself, ahead of the chain.

The refusal is `0187`, "The calculation model is required for fee X". That code
is not new and not invented for this fix: creating a package with a fee that
lacks a calculation model already answers exactly that, through
`validateCalculationModel`. Patching one now gives the same answer as creating
one, which is the behaviour a caller can reason about.

The label keeps its own check, first, so that every shape already refused keeps
the exact answer it had. An entirely empty fee entry still answers `0192`.

### Task 1.2: pin the whole refusal table, and pin that nothing is written

`components/ledger/internal/services/fees/update-package-fee-calculation_test.go`

Eight request bodies, each one a real JSON payload unmarshalled into the update
input, each asserted on two things: the refusal code, and which of the two layers
produced it. The request-body validation the handler runs is called first, as the
handler calls it, so the test states plainly which bodies it catches and which
fall through to the service.

Nothing may be written: when the service is reached, its repository mock carries
no expectation for `Update`, so any write fails the case.

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

## Found by this work, not fixed

- The brief named `0217` as the refusal code. `0217` is `ErrInvalidPricingModel`
  ("Valid models are 'tiered' and 'fixed'"), which is about something else
  entirely. The brief also asked for "the same code and shape the other
  required-field refusals use", and those two halves point at different codes.
  The code shipped is `0187`, on the evidence that the create path already
  answers `0187` for this exact missing thing.
- The equivalent read in the update path for an existing fee,
  `hasNoCalculationModelUpdates` in `update_package_input.go`, dereferences the
  same pointer, and is safe only because its one caller checks for nil first.
  Nothing pins that; a second caller added later would reintroduce the crash. Not
  changed here, because guarding it would mean guessing whether a partial patch
  of an existing fee may omit its calculation model, which it may.
- `docs/plans/` in this repository holds `2026-08-17-integration-test-efficiency.md`
  and `huma-migration-research/`. The brief expected a
  `2026-09-14-fee-deductible-validators.md`, which is not present at this head.

## Verification

Every block below was run verbatim, in the order shown.

### RED, at the branch point, from the committed test

The test file was copied into a throwaway worktree detached at
`792eb32bd076cb719aba1f2648bffff113ce11f3`, which carries no fix.

```
$ date -u
Tue Sep 15 16:03:06 UTC 2026
$ git rev-parse HEAD
792eb32bd076cb719aba1f2648bffff113ce11f3
$ git status --porcelain
?? components/ledger/internal/services/fees/update-package-fee-calculation_test.go
$ go test -count=1 -run 'TestUpdatePackageByID_FeeWithoutCalculationIsRefused' ./components/ledger/internal/services/fees/
--- FAIL: TestUpdatePackageByID_FeeWithoutCalculationIsRefused (0.00s)
    --- FAIL: TestUpdatePackageByID_FeeWithoutCalculationIsRefused/fee_carries_only_a_label,_no_calculation_model_at_all (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
[signal SIGSEGV: segmentation violation code=0x1 addr=0x8 pc=0xb49d39]
...
        .../components/ledger/pkg/feeshared/model/package.go:211 +0x19
        .../components/ledger/pkg/feeshared/model/package.go:193 +0x34
        .../components/ledger/internal/services/fees/update-package-by-id.go:197 +0x5ea
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.021s
FAIL
rc=1
```

The crash is at `package.go:211`, the first clause of the chain that reads
through the calculation model pointer, reached from the fee update service.

### GREEN, with the fix in the tree

```
$ date -u
Tue Sep 15 15:59:08 UTC 2026
$ git rev-parse HEAD
792eb32bd076cb719aba1f2648bffff113ce11f3
$ git status --porcelain
 M components/ledger/pkg/feeshared/model/package.go
?? components/ledger/internal/services/fees/update-package-fee-calculation_test.go
$ go test -count=1 -v -run 'TestUpdatePackageByID_FeeWithoutCalculationIsRefused' ./components/ledger/internal/services/fees/
--- PASS: TestUpdatePackageByID_FeeWithoutCalculationIsRefused (0.00s)
    --- PASS: .../fee_carries_only_a_label,_no_calculation_model_at_all (0.00s)
    --- PASS: .../fee_entry_is_entirely_empty (0.00s)
    --- PASS: .../calculation_carries_no_type (0.00s)
    --- PASS: .../calculation_carries_no_value (0.00s)
    --- PASS: .../fee_carries_no_credit_account (0.00s)
    --- PASS: .../calculation_model_carries_no_calculations (0.00s)
    --- PASS: .../calculation_model_carries_no_application_rule (0.00s)
    --- PASS: .../fee_carries_a_label_and_an_empty_calculation_model_object (0.00s)
PASS
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.027s
rc=0
```

### Mutants

Both mutations were applied to a copy of the file taken beforehand and restored
from that copy afterwards; the restored file's sha256 is the one recorded before
the first mutation, and the suite was re-run green after the restore.

```
$ sha256sum components/ledger/pkg/feeshared/model/package.go
4efc82da6249a084b69662e7854ea955fe0b84eb409b240ccec10a4c3d6d9d25

MUTANT 1, the nil guard deleted:
--- FAIL: TestUpdatePackageByID_FeeWithoutCalculationIsRefused (0.00s)
panic: runtime error: invalid memory address or nil pointer dereference [recovered, repanicked]
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.021s
rc=1

MUTANT 2, ErrCalculationRequired swapped for ErrFeeFieldsRequired:
    --- FAIL: .../fee_carries_only_a_label,_no_calculation_model_at_all (0.00s)
            expected: "0187"
            actual  : "0192"
FAIL	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.025s
rc=1

restored sha256=4efc82da6249a084b69662e7854ea955fe0b84eb409b240ccec10a4c3d6d9d25
post-restore run: ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	0.025s
rc=0
```

### Gates, at the code-final commit

```
$ date -u
Tue Sep 15 16:00:42 UTC 2026
$ git rev-parse HEAD
7d6154cebc1f290c69a7b805f67a12a6511ef625
$ git status --porcelain
(empty)

$ go build ./...
rc=0

$ go vet ./...
rc=0

$ go test -count=1 -race ./components/ledger/...
rc=0
ok lines: 55, FAIL lines: 0, no-test-files: 5
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees	1.167s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/internal/services/fees/midaz	1.102s
ok  	github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model	1.193s

$ GOLANGCI_LINT_CACHE=/tmp/gcl-feepatch golangci-lint run --allow-parallel-runners ./components/ledger/...
golangci-lint has version 2.13.2 built with go1.27.0
0 issues.
rc=0
```

The golangci-lint on this box is 2.12.2, built with go1.26, and refuses this
repository outright: "the Go language version (go1.26) used to build
golangci-lint is lower than the targeted Go version (1.27.0)", rc=3, deciding
nothing. The version CI pins, v2.13.2, was installed to a scratch GOBIN and is
the one whose result is recorded above.

### Not run

No live stack and no browser run. This change is a refusal inside the ledger
component, with no user-facing surface of its own; the behaviour it fixes is the
HTTP status and body an API caller receives, which the service-level table above
pins directly.
