// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"testing"
)

// The version split is enforced by construction rather than by a runtime flag: the
// /v1 pipelines simply do not name the seams the /v1 contract excludes. Nothing at
// runtime distinguishes "the seam ran and no-oped" from "the seam was never reached",
// so these gates read the source instead.
//
// Negative gate: a /v1 pipeline naming any versioned seam is the regression — it would
// mean a /v1 client can acquire fee legs, a tenant fee-DB resolution failure, or a
// reservation rejection from a version upgrade it never asked for.
//
// Positive gate: the /v2 create pipeline must name them in the one order the contract
// allows — skips resolve off the single settings read, the fee engine mutates the send,
// the reserve observes fee-inclusive amounts before the balance commit, and the confirm
// follows the commit.

// versionedSeams are the seams the /v1 contract excludes.
var versionedSeams = []string{
	"resolveTransactionSkips",
	"applyFees",
	"reserveTransaction",
	"confirmReservations",
	"releaseReservations",
}

// calledNames returns every function and method name called anywhere inside the named
// function, in source order and with duplicates kept, so an ordering assertion can read
// the sequence directly.
func calledNames(t *testing.T, src, funcName string) []string {
	t.Helper()

	fn := findFuncDecl(t, src, funcName)

	var names []string

	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch f := call.Fun.(type) {
		case *ast.Ident:
			names = append(names, f.Name)
		case *ast.SelectorExpr:
			names = append(names, f.Sel.Name)
		}

		return true
	})

	return names
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}

	return false
}

// indexOfName returns the position of want in names, or -1.
func indexOfName(names []string, want string) int {
	for i, n := range names {
		if n == want {
			return i
		}
	}

	return -1
}

func TestCreateTransactionV1_NeverReferencesVersionedSeams(t *testing.T) {
	for _, tc := range []struct {
		file string
		fn   string
	}{
		{file: "create_transaction_v1.go", fn: "CreateTransactionV1"},
		{file: "revert_transaction.go", fn: "createRevertV1"},
	} {
		names := calledNames(t, readTransportSource(t, tc.file, "func (uc *UseCase) "+tc.fn), tc.fn)

		for _, seam := range versionedSeams {
			if containsName(names, seam) {
				t.Errorf("%s references %s — the /v1 contract carries neither the fee engine nor the tracer reservation", tc.fn, seam)
			}
		}
	}
}

func TestCreateTransactionV2_ReferencesVersionedSeamsInOrder(t *testing.T) {
	names := calledNames(t, readTransportSource(t, "create_transaction_v2.go", "func (uc *UseCase) CreateTransactionV2"), "CreateTransactionV2")

	order := []string{
		"resolveTransactionSkips",
		"applyFees",
		"reserveTransaction",
		"ProcessBalanceOperations",
		"confirmReservations",
	}

	previous := -1

	for _, name := range order {
		at := indexOfName(names, name)
		if at == -1 {
			t.Fatalf("CreateTransactionV2 does not call %s — the /v2 contract includes it", name)
		}

		if at <= previous {
			t.Errorf("CreateTransactionV2 calls %s out of order (expected after the previous seam in %v)", name, order)
		}

		previous = at
	}
}

// TestRevertV2_NeverAppliesFees locks the one way the revert pipeline differs from the
// create pipeline on the same contract: the reverse transaction already carries the
// reversed fee legs reconstructed by TransactionRevert, so charging again would double
// the fees.
func TestRevertV2_NeverAppliesFees(t *testing.T) {
	names := calledNames(t, readTransportSource(t, "revert_transaction.go", "func (uc *UseCase) createRevertV2"), "createRevertV2")

	if containsName(names, "applyFees") {
		t.Error("createRevertV2 references applyFees — a revert already carries the reversed fee legs, so re-charging would double the fees")
	}

	if !containsName(names, "reserveTransaction") {
		t.Error("createRevertV2 must still reserve: limits measure GROSS activity, so a revert is a chargeable transaction of its own")
	}
}
