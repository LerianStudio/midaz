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
		{"loadLifecycleTransaction", positions.loadTransaction, positions.loadTransactionTakesReadCtx},
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

// TestRevert_ProjectionHelperForwardsContext closes the indirection the placement guard
// above depends on: prepareRevertTransaction hands readCtx to loadLifecycleTransaction,
// so the marker only reaches the database if every helper on the way forwards its own
// ctx to each read instead of substituting a fresh one.
func TestRevert_ProjectionHelperForwardsContext(t *testing.T) {
	src := readTransportSource(t, "transaction_reader.go", "func resolveTransactionProjection")

	for _, helper := range []struct {
		name  string
		reads []string
	}{
		{"loadLifecycleTransaction", []string{"loadIndexedOrPersistedTransaction", "GetWriteBehindTransaction"}},
		{"loadIndexedOrPersistedTransaction", []string{"resolveTransactionProjection", "GetTransactionByID"}},
		{"resolveTransactionProjection", []string{"ResolveTransactionProjection", "GetTransactionWithOperationsByID"}},
	} {
		fn := findFuncDecl(t, src, helper.name)

		if fn.Body == nil {
			t.Fatalf("%s has no body", helper.name)
		}

		for _, read := range helper.reads {
			forwards := false

			for _, stmt := range fn.Body.List {
				forwards = forwards || callFirstArgIsIdent(stmt, read, "ctx")
			}

			if !forwards {
				t.Errorf("%s must pass its own ctx to %s; a substituted context drops the caller's primary-read marker", helper.name, read)
			}
		}
	}
}

// revertWrapPositions holds the top-level statement indices of the marker wrap and the
// eligibility-gate reads it must precede, plus the arg-identity checks that scope the
// marker to the transaction and parent reads.
type revertWrapPositions struct {
	wrap            int
	getParent       int
	loadTransaction int

	getParentTakesReadCtx         bool
	loadTransactionTakesReadCtx   bool
	getOperationRouteTakesReadCtx bool
}

// analyzeRevertWrap returns, within the named function body, the top-level statement
// indices of the dedicated `readCtx := readrouting.WithPrimaryRead(ctx)` wrap and of the
// eligibility-gate reads (each -1 when absent), and which reads receive `readCtx` as
// their context argument.
func analyzeRevertWrap(t *testing.T, src, funcName string) revertWrapPositions {
	t.Helper()

	fn := findFuncDecl(t, src, funcName)

	if fn.Body == nil {
		t.Fatalf("function %q has no body", funcName)
	}

	positions := revertWrapPositions{wrap: -1, getParent: -1, loadTransaction: -1}

	for i, stmt := range fn.Body.List {
		if positions.wrap == -1 && stmtDefinesReadCtxFromPrimaryRead(stmt) {
			positions.wrap = i
		}

		if positions.getParent == -1 && stmtCallsMethod(stmt, "GetParentByTransactionID") {
			positions.getParent = i
		}

		if positions.loadTransaction == -1 && stmtCallsFunc(stmt, "loadLifecycleTransaction") {
			positions.loadTransaction = i
		}

		positions.getParentTakesReadCtx = positions.getParentTakesReadCtx ||
			callFirstArgIsIdent(stmt, "GetParentByTransactionID", "readCtx")

		positions.loadTransactionTakesReadCtx = positions.loadTransactionTakesReadCtx ||
			callFirstArgIsIdent(stmt, "loadLifecycleTransaction", "readCtx")

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
		"\tuc.loadLifecycleTransaction(ctx)\n" +
		"\t_ = readCtx\n}\n"

	positions := analyzeRevertWrap(t, src, "unmarkedGate")

	if positions.wrap == -1 {
		t.Fatal("the analyzer failed to find the wrap it is built to find")
	}

	if positions.getParentTakesReadCtx || positions.loadTransactionTakesReadCtx {
		t.Error("the analyzer reported an unmarked read as marked; the placement guard would pass over a regressed gate")
	}
}
