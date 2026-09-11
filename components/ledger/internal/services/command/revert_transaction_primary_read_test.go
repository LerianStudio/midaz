// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
)

const (
	revertPrepareFuncName = "prepareRevertTransaction"
	revertFile            = "revert_transaction.go"
)

// TestRevert_PrimaryReadWrapPlacement is the placement guard over the live source of
// prepareRevertTransaction — the eligibility gate both RevertTransactionV1 and
// RevertTransactionV2 run. It proves the dedicated-var wrap
// `readCtx := readrouting.WithPrimaryRead(ctx)` exists and precedes every
// transaction/parent read of the gate, so a revert issued right after its create reads
// its own write instead of a lagging replica (which answers not-found for a transaction
// that exists). It also proves the marker stays scoped: the operation-route lookup is
// not a read of the create and keeps the unmarked ctx.
func TestRevert_PrimaryReadWrapPlacement(t *testing.T) {
	src := readTransportSource(t, revertFile, "func (uc *UseCase) "+revertPrepareFuncName)

	positions := analyzeRevertWrap(t, src, revertPrepareFuncName)

	if positions.wrap == -1 {
		t.Fatal("no dedicated `readCtx := readrouting.WithPrimaryRead(ctx)` wrap found in prepareRevertTransaction; the revert eligibility gate must mark the primary-read intent on a dedicated ctx var, not reassign ctx")
	}

	reads := []struct {
		name     string
		position int
		takesCtx bool
	}{
		{"GetParentByTransactionID", positions.getParent, positions.getParentTakesReadCtx},
		{"GetTransactionWithOperationsByID", positions.getWithOperations, positions.getWithOperationsTakesReadCtx},
		{"GetTransactionByID", positions.getTransaction, positions.getTransactionTakesReadCtx},
	}

	for _, read := range reads {
		if read.position == -1 {
			t.Fatalf("no %s call found in prepareRevertTransaction; the read call site moved", read.name)
		}

		if positions.wrap >= read.position {
			t.Errorf("the readCtx wrap (stmt %d) must precede the %s read (stmt %d) so the eligibility gate observes the mark", positions.wrap, read.name, read.position)
		}

		if !read.takesCtx {
			t.Errorf("the %s read must receive the dedicated readCtx (the primary-read marker); passing the unmarked ctx leaves the revert reading a lagging replica", read.name)
		}
	}

	if positions.getOperationRouteTakesReadCtx {
		t.Error("GetOperationRouteByID must NOT receive the dedicated readCtx: a route is not a read of the create being reverted and deliberately keeps the unmarked ctx")
	}
}

// revertWrapPositions holds the top-level statement indices of the marker wrap and the
// eligibility-gate reads it must precede, plus the arg-identity checks that scope the
// marker to the transaction and parent reads.
type revertWrapPositions struct {
	wrap              int
	getParent         int
	getWithOperations int
	getTransaction    int

	getParentTakesReadCtx         bool
	getWithOperationsTakesReadCtx bool
	getTransactionTakesReadCtx    bool
	getOperationRouteTakesReadCtx bool
}

// analyzeRevertWrap returns, within the named function body, the top-level statement
// indices of the dedicated `readCtx := readrouting.WithPrimaryRead(ctx)` wrap and of the
// three eligibility-gate reads (each -1 when absent), and which reads receive `readCtx`
// as their context argument. Top-level indices are sufficient even for the fallback read:
// it sits inside a top-level if-block, whose own index still orders it against the wrap.
func analyzeRevertWrap(t *testing.T, src, funcName string) revertWrapPositions {
	t.Helper()

	fn := findFuncDecl(t, src, funcName)

	if fn.Body == nil {
		t.Fatalf("function %q has no body", funcName)
	}

	positions := revertWrapPositions{wrap: -1, getParent: -1, getWithOperations: -1, getTransaction: -1}

	for i, stmt := range fn.Body.List {
		if positions.wrap == -1 && stmtDefinesReadCtxFromPrimaryRead(stmt) {
			positions.wrap = i
		}

		if positions.getParent == -1 && stmtCallsMethod(stmt, "GetParentByTransactionID") {
			positions.getParent = i
		}

		if positions.getWithOperations == -1 && stmtCallsMethod(stmt, "GetTransactionWithOperationsByID") {
			positions.getWithOperations = i
		}

		if positions.getTransaction == -1 && stmtCallsMethod(stmt, "GetTransactionByID") {
			positions.getTransaction = i
		}

		positions.getParentTakesReadCtx = positions.getParentTakesReadCtx ||
			callFirstArgIsIdent(stmt, "GetParentByTransactionID", "readCtx")

		positions.getWithOperationsTakesReadCtx = positions.getWithOperationsTakesReadCtx ||
			callFirstArgIsIdent(stmt, "GetTransactionWithOperationsByID", "readCtx")

		positions.getTransactionTakesReadCtx = positions.getTransactionTakesReadCtx ||
			callFirstArgIsIdent(stmt, "GetTransactionByID", "readCtx")

		positions.getOperationRouteTakesReadCtx = positions.getOperationRouteTakesReadCtx ||
			callFirstArgIsIdent(stmt, "GetOperationRouteByID", "readCtx")
	}

	return positions
}

// TestAnalyzeRevertWrap_DetectsUnmarkedReads proves the analyzer bites: a gate shaped
// like the real one but reading through the unmarked ctx reports the reads as unmarked,
// so the placement guard above cannot pass over a regressed source.
func TestAnalyzeRevertWrap_DetectsUnmarkedReads(t *testing.T) {
	src := "package p\n\nfunc unmarkedGate() {\n" +
		"\treadCtx := readrouting.WithPrimaryRead(ctx)\n" +
		"\tuc.TransactionReader.GetParentByTransactionID(ctx)\n" +
		"\tuc.TransactionReader.GetTransactionWithOperationsByID(ctx)\n" +
		"\tuc.TransactionReader.GetTransactionByID(ctx)\n" +
		"\t_ = readCtx\n}\n"

	positions := analyzeRevertWrap(t, src, "unmarkedGate")

	if positions.wrap == -1 {
		t.Fatal("the analyzer failed to find the wrap it is built to find")
	}

	if positions.getParentTakesReadCtx || positions.getWithOperationsTakesReadCtx || positions.getTransactionTakesReadCtx {
		t.Error("the analyzer reported an unmarked read as marked; the placement guard would pass over a regressed gate")
	}
}
