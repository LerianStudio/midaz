// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Gate 5 — POSITION. The route gate must be the FIRST statement of the two
// by-transaction tracer seams, the ones commit and cancel still address with an explicit
// policy. Ordering is the guarantee, not a detail: a /v1 request has to return before a
// request is built or a connection dialled, so moving the gate below the nil/mode guard
// would still no-op but would no longer prove that nothing left the process. Nothing at
// runtime distinguishes the two.
//
// The create-side reserve anchor needs no gate: the /v1 pipelines never call it, which
// TestCreateTransactionV1_NeverReferencesVersionedSeams asserts over the source.

// firstStatementGatesRouteV1 reports whether the named function's FIRST statement is
// `if policy == RouteV1`. A function that is absent from src is reported separately so
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

// isRouteV1Guard matches `if policy == RouteV1 { ... }` with no else branch.
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

	return okLeft && okRight && left.Name == "policy" && right.Name == "RouteV1"
}

func TestRouteVersionStructure_TracerSeamsGateFirst(t *testing.T) {
	src := readTransportSource(t, "transaction_reservation_anchor.go", "ConfirmReservationsByTransaction")

	for _, fn := range []string{
		"ConfirmReservationsByTransaction",
		"ReleaseReservationsByTransaction",
	} {
		gated, found := firstStatementGatesRouteV1(t, src, fn)

		if !found {
			t.Errorf("Gate 5: %s not found in transaction_reservation_anchor.go — the gate is pointed at a renamed seam", fn)

			continue
		}

		if !gated {
			t.Errorf("Gate 5: the first statement of %s is not `if policy == RouteV1` — a /v1 request must return before any reserve request is built or dialled", fn)
		}
	}
}

func TestRouteVersionStructure_GateFirstBites(t *testing.T) {
	// Gate 5 must reject a gate that sits below the availability guard: it still no-ops
	// on /v1, but it no longer proves nothing was built or dialled first.
	const gateMoved = `package command

func (uc *UseCase) ConfirmReservationsByTransaction() {
	if uc.TracerReserver == nil {
		return
	}

	if policy == RouteV1 {
		return
	}

	_ = uc.TracerReserver.ConfirmByTransaction()
}
`

	if gated, found := firstStatementGatesRouteV1(t, gateMoved, "ConfirmReservationsByTransaction"); !found || gated {
		t.Fatalf("Gate 5 bite: a gate below the nil guard must be reported as ungated, got gated=%v found=%v", gated, found)
	}

	// An absent gate is not the same as an absent function; both must fail, distinctly.
	const noGate = `package command

func (uc *UseCase) ConfirmReservationsByTransaction() {
	_ = uc.TracerReserver.ConfirmByTransaction()
}
`

	if gated, found := firstStatementGatesRouteV1(t, noGate, "ConfirmReservationsByTransaction"); !found || gated {
		t.Fatalf("Gate 5 bite: a seam with no gate must be reported as ungated, got gated=%v found=%v", gated, found)
	}

	if _, found := firstStatementGatesRouteV1(t, noGate, "renamedSeam"); found {
		t.Fatal("Gate 5 bite: a missing function must be reported as not found, never as gated")
	}

	// The correct shape must satisfy the gate, or the gate is unsatisfiable.
	const correct = `package command

func (uc *UseCase) ConfirmReservationsByTransaction() {
	if policy == RouteV1 {
		return
	}

	_ = uc.TracerReserver.ConfirmByTransaction()
}
`

	if gated, found := firstStatementGatesRouteV1(t, correct, "ConfirmReservationsByTransaction"); !found || !gated {
		t.Fatalf("Gate 5 bite: fixture sanity — the correct shape must pass, got gated=%v found=%v", gated, found)
	}
}
