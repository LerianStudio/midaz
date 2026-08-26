// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Route-version structural gates. The fee seam's Gate 4
// (transaction_fee_seam_structure_test.go) proves every createTransactionShell call
// names its policy; these two cover what that gate cannot see.
//
// Gate 5 — POSITION. The route gate must be the FIRST statement of each tracer seam
// entry point. Ordering is the guarantee, not a detail: a /v1 request has to return
// before a reserve request is built or a connection dialled, so moving the gate below
// the nil/mode guard would still no-op but would no longer prove that nothing left the
// process. Nothing at runtime distinguishes the two.
//
// Gate 6 — COVERAGE. commit, cancel and revert reach the tracer through their own
// cores, not through createTransactionShell, so Gate 4 is blind to them. A new /v1
// state-transition route wired with the wrong constant would silently start reaching
// the tracer.

// callRouteVersionArgs returns the route-version identifier passed by every call to
// callee in src, in source order. An absent or non-identifier policy argument yields an
// empty entry, which fails the gates below.
func callRouteVersionArgs(t *testing.T, src, callee string) []string {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var policies []string

	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != callee {
			return true
		}

		policies = append(policies, routeVersionArgName(call.Args))

		return true
	})

	return policies
}

// firstStatementGatesRouteV1 reports whether the named function's FIRST statement is
// `if policy == routeV1`. A function that is absent from src is reported separately so
// a renamed seam fails loudly instead of passing vacuously.
func firstStatementGatesRouteV1(t *testing.T, src, funcName string) (gated, found bool) {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Body == nil || len(fn.Body.List) == 0 {
			continue
		}

		return isRouteV1Guard(fn.Body.List[0]), true
	}

	return false, false
}

// isRouteV1Guard matches `if policy == routeV1 { ... }` with no else branch.
func isRouteV1Guard(stmt ast.Stmt) bool {
	ifStmt, ok := stmt.(*ast.IfStmt)
	if !ok || ifStmt.Init != nil || ifStmt.Else != nil {
		return false
	}

	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}

	left, okLeft := bin.X.(*ast.Ident)
	right, okRight := bin.Y.(*ast.Ident)

	return okLeft && okRight && left.Name == "policy" && right.Name == "routeV1"
}

func TestRouteVersionStructure_TracerSeamsGateFirst(t *testing.T) {
	src := readTransportSource(t, "transaction_reservation_anchor.go", "reserveTransaction")

	for _, fn := range []string{
		"reserveTransaction",
		"confirmReservationsByTransaction",
		"releaseReservationsByTransaction",
	} {
		gated, found := firstStatementGatesRouteV1(t, src, fn)

		if !found {
			t.Errorf("Gate 5: %s not found in transaction_reservation_anchor.go — the gate is pointed at a renamed seam", fn)

			continue
		}

		if !gated {
			t.Errorf("Gate 5: the first statement of %s is not `if policy == routeV1` — a /v1 request must return before any reserve request is built or dialled", fn)
		}
	}
}

func TestRouteVersionStructure_V1StateRoutesPassRouteV1(t *testing.T) {
	src := readTransportSource(t, "transaction_handler.go", "commitTransaction")

	for _, callee := range []string{"commitTransaction", "revertTransaction"} {
		policies := callRouteVersionArgs(t, src, callee)
		if len(policies) == 0 {
			t.Fatalf("Gate 6: no %s call found in transaction_handler.go", callee)
		}

		for i, got := range policies {
			if got != "routeV1" {
				t.Errorf("Gate 6: %s call #%d in transaction_handler.go passes %q, want routeV1 — the /v1 contract carries no tracer", callee, i, got)
			}
		}
	}
}

func TestRouteVersionStructure_V2StateRoutesPassRouteV2(t *testing.T) {
	src := readTransportSource(t, "transaction_handler_v2.go", "commitTransaction")

	for _, callee := range []string{"commitTransaction", "revertTransaction"} {
		policies := callRouteVersionArgs(t, src, callee)
		if len(policies) == 0 {
			t.Fatalf("Gate 6: no %s call found in transaction_handler_v2.go", callee)
		}

		for i, got := range policies {
			if got != "routeV2" {
				t.Errorf("Gate 6: %s call #%d in transaction_handler_v2.go passes %q, want routeV2", callee, i, got)
			}
		}
	}
}

func TestRouteVersionStructure_GatesBite(t *testing.T) {
	// Gate 5 must reject a gate that sits below the availability guard: it still no-ops
	// on /v1, but it no longer proves nothing was built or dialled first.
	const gateMoved = `package in

func (handler *TransactionHandler) reserveTransaction() reservationOutcome {
	if handler.TracerReserver == nil {
		return reservationOutcome{}
	}

	if policy == routeV1 {
		return reservationOutcome{}
	}

	return reservationOutcome{}
}
`

	if gated, found := firstStatementGatesRouteV1(t, gateMoved, "reserveTransaction"); !found || gated {
		t.Fatalf("Gate 5 bite: a gate below the nil guard must be reported as ungated, got gated=%v found=%v", gated, found)
	}

	// An absent gate is not the same as an absent function; both must fail, distinctly.
	const noGate = `package in

func (handler *TransactionHandler) reserveTransaction() reservationOutcome {
	return reservationOutcome{}
}
`

	if gated, found := firstStatementGatesRouteV1(t, noGate, "reserveTransaction"); !found || gated {
		t.Fatalf("Gate 5 bite: a seam with no gate must be reported as ungated, got gated=%v found=%v", gated, found)
	}

	if _, found := firstStatementGatesRouteV1(t, noGate, "renamedSeam"); found {
		t.Fatal("Gate 5 bite: a missing function must be reported as not found, never as gated")
	}

	// The correct shape must satisfy the gate, or the gate is unsatisfiable.
	const correct = `package in

func (handler *TransactionHandler) reserveTransaction() reservationOutcome {
	if policy == routeV1 {
		return reservationOutcome{}
	}

	return reservationOutcome{}
}
`

	if gated, found := firstStatementGatesRouteV1(t, correct, "reserveTransaction"); !found || !gated {
		t.Fatalf("Gate 5 bite: fixture sanity — the correct shape must pass, got gated=%v found=%v", gated, found)
	}

	// Gate 6 must report the wrong constant, and an absent one as empty.
	const wrongConstant = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED, routeV2)
}
`

	if got := callRouteVersionArgs(t, wrongConstant, "commitTransaction"); len(got) != 1 || got[0] != "routeV2" {
		t.Fatalf("Gate 6 bite: analyzer must report the wrong constant, got %v", got)
	}

	const noPolicy = `package in

func (handler *TransactionHandler) CommitTransaction(ctx context.Context) error {
	return handler.commitTransaction(ctx, orgID, ledgerID, txID, constant.APPROVED)
}
`

	if got := callRouteVersionArgs(t, noPolicy, "commitTransaction"); len(got) != 1 || got[0] != "" {
		t.Fatalf("Gate 6 bite: analyzer must report an absent policy as empty, got %v", got)
	}
}
