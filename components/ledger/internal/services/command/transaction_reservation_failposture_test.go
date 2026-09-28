// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fail-closed call-site proof. The branch outcomes of the reserve anchor
// (fail-open proceeds and marks the reservation skipped, fail-closed rejects
// with 0178) are asserted in transaction_reservation_anchor_test.go. What is
// proven here is the create seam's handling of a reject: it releases the
// idempotency key and removes the Redis-queue seed BEFORE — and instead of —
// the balance commit, so no balance is mutated. It is a structural guarantee
// asserted directly over the live executeCreateEngine source AST, mirroring the
// fee-seam structural gate. A "bites" fixture proves the gate fails if the
// release is dropped or the reject falls through to the balance commit.

// ---- Gate 5 (fail-closed): structural proof of the call-site mechanics --------

const createEngineSeamFuncName = "executeCreateEngine"

// failClosedSeamMetrics captures the statement-list ordering facts the Gate-5
// structural assertion relies on, all within CreateTransactionV2.
type failClosedSeamMetrics struct {
	reservePos          int  // index of the reservePreparedTransaction call (-1 if absent)
	rejectRollbackClaim bool // rollbackCreateClaim appears inside the reservationReject branch
	rejectReturnsBefore bool // the reject branch returns (no fall-through to the balance commit)
	executeEnginePos    int  // index of the top-level ExecutePreparedEngine call (-1)
}

// analyzeFailClosedSeam walks CreateTransactionV2 and extracts the ordering
// and reject-branch facts. The reservationReject guard is an `if` whose cond is
// `reservation.Kind == reservationReject`; the release mechanics must live in
// that block and the block must return.
func analyzeFailClosedSeam(t *testing.T, src string) failClosedSeamMetrics {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var fn *ast.FuncDecl

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == createEngineSeamFuncName {
			fn = d
			break
		}
	}

	if fn == nil || fn.Body == nil {
		t.Fatalf("function %q not found or has no body", createEngineSeamFuncName)
	}

	m := failClosedSeamMetrics{reservePos: -1, executeEnginePos: -1}

	for i, stmt := range fn.Body.List {
		if m.reservePos == -1 && stmtCallsMethod(stmt, "reservePreparedTransaction") {
			m.reservePos = i
		}

		if m.executeEnginePos == -1 && stmtCallsFunc(stmt, "ExecutePreparedEngine") {
			m.executeEnginePos = i
		}

		ast.Inspect(stmt, func(node ast.Node) bool {
			ifStmt, ok := node.(*ast.IfStmt)
			if !ok || !isReservationRejectGuard(ifStmt) {
				return true
			}

			m.rejectRollbackClaim = blockCallsMethod(ifStmt.Body, "rollbackCreateClaim")
			m.rejectReturnsBefore = blockEndsInReturn(ifStmt.Body)
			return false
		})
	}

	return m
}

// isReservationRejectGuard reports whether the if-cond is
// `reservation.Kind == reservationReject` (the fail-closed / denied reject gate
// that must precede the balance commit).
func isReservationRejectGuard(ifStmt *ast.IfStmt) bool {
	bin, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || bin.Op != token.EQL {
		return false
	}

	// RHS must reference the reservationReject identifier.
	rhs, ok := bin.Y.(*ast.Ident)
	if !ok || rhs.Name != "reservationReject" {
		return false
	}

	// LHS must be reservation.Kind.
	sel, ok := bin.X.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Kind" {
		return false
	}

	x, ok := sel.X.(*ast.Ident)

	return ok && x.Name == "reservation"
}

// blockCallsMethod reports whether the block contains a selector call to the
// named method.
func blockCallsMethod(block *ast.BlockStmt, method string) bool {
	for _, stmt := range block.List {
		if stmtCallsMethod(stmt, method) {
			return true
		}
	}

	return false
}

// blockEndsInReturn reports whether the last statement of the block is a return
// — proving the reject branch does not fall through to the balance commit.
func blockEndsInReturn(block *ast.BlockStmt) bool {
	if len(block.List) == 0 {
		return false
	}

	_, ok := block.List[len(block.List)-1].(*ast.ReturnStmt)

	return ok
}

// TestTracerFailClosedReject_ReleasesIdempotencyAndSkipsBalanceCommit — Gate 5.
// The fail-closed reject (proven at the helper in
// TestReserveTransaction_FailClosed_Rejects and
// TestTracerFailClosedDoesNotMarkSkipped) must, at the call site, release the
// idempotency key and the Redis-queue seed and return BEFORE
// ProcessBalanceOperations — so no balance is mutated. Asserted over the live
// source so a future reorder that drops the release or falls through to the
// balance commit fails this gate.
func TestTracerFailClosedReject_ReleasesIdempotencyAndSkipsBalanceCommit(t *testing.T) {
	src := readTransportSource(t, "create_transaction_engine.go", "func (uc *UseCase) executeCreateEngine")

	m := analyzeFailClosedSeam(t, src)

	require.NotEqual(t, -1, m.reservePos, "reservePreparedTransaction call not found in CreateTransactionV2")
	require.NotEqual(t, -1, m.executeEnginePos, "ExecutePreparedEngine call not found")

	assert.Less(t, m.reservePos, m.executeEnginePos,
		"the reserve anchor must precede the balance commit (reject before any balance move)")

	assert.True(t, m.rejectRollbackClaim,
		"fail-closed reject branch must roll back the idempotency claim")
	assert.True(t, m.rejectReturnsBefore,
		"fail-closed reject branch must return — it must NOT fall through to ProcessBalanceOperations")
}

// TestTracerFailClosedSeam_Bites proves the Gate-5 analyzer actually fails on a
// reject branch that drops the rollback or falls through to the balance commit — a
// gate that cannot bite is not a guard.
func TestTracerFailClosedSeam_Bites(t *testing.T) {
	// Fixture 1: reject branch missing the rollback and the return.
	leaky := `package command
func (uc *UseCase) executeCreateEngine() error {
	reservation := uc.reservePreparedTransaction()
	if reservation.Kind == reservationReject {
		// BUG: neither rolls back the claim and seed nor returns
		_ = reservation.Err
	}
	result, err := ExecutePreparedEngine()
	_ = result
	return err
}`

	m := analyzeFailClosedSeam(t, leaky)

	if m.reservePos == -1 || m.executeEnginePos == -1 {
		t.Fatalf("Gate 5 fixture sanity: missing positions reserve=%d executeEngine=%d", m.reservePos, m.executeEnginePos)
	}

	if m.rejectRollbackClaim {
		t.Error("Gate 5 failed to bite: a reject branch with no rollbackCreateClaim was reported as rolling back")
	}

	if m.rejectReturnsBefore {
		t.Error("Gate 5 failed to bite: a reject branch with no return was reported as returning before the balance commit")
	}

	// Fixture 2: the canonical, correct shape must pass both reject facts.
	correct := `package command
func (uc *UseCase) executeCreateEngine() error {
	reservation := uc.reservePreparedTransaction()
	if reservation.Kind == reservationReject {
		uc.rollbackCreateClaim()
		return reservation.Err
	}
	result, err := ExecutePreparedEngine()
	_ = result
	return err
}`

	mc := analyzeFailClosedSeam(t, correct)
	if !(mc.rejectRollbackClaim && mc.rejectReturnsBefore) {
		t.Errorf("Gate 5 fixture sanity: the correct shape was not fully recognized: rollback=%v returns=%v",
			mc.rejectRollbackClaim, mc.rejectReturnsBefore)
	}

	if !(mc.reservePos < mc.executeEnginePos) {
		t.Error("Gate 5 fixture sanity: reserve should precede the balance commit in the correct shape")
	}
}
