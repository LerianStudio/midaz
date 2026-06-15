// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Per-call tracer-skip wiring proofs. The honored-skip short-circuit behavior is
// proven at the helper level in transaction_reservation_anchor_test.go
// (TestReserveTransaction_HonoredSkip_Proceeds for create, the honored-skip
// subtests of TestConfirmReservationsByTransaction /
// TestReleaseReservationsByTransaction for commit/cancel). What those cannot see
// is whether the SEAMS actually feed the resolved boolean into the helpers, and
// whether the create-path 422 releases the idempotency key. Those are call-site
// facts, so — mirroring the fee-seam (transaction_fee_seam_structure_test.go) and
// fail-closed (transaction_reservation_failposture_test.go) gates — they are
// asserted over the live source AST. A future reorder that drops the resolution,
// stops threading the flag, or forgets the idempotency release fails these gates.

const (
	commitCancelFuncName = "commitOrCancelTransaction"
	stateHandlersFile    = "transaction_state_handlers.go"
)

// findFuncDecl parses src and returns the named top-level function declaration.
func findFuncDecl(t *testing.T, src, name string) *ast.FuncDecl {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == name {
			return d
		}
	}

	t.Fatalf("function %q not found", name)

	return nil
}

// createSkipSeamMetrics captures the ordering and reject-branch facts the
// create-path tracer-skip wiring relies on, all within executeCreateTransaction.
type createSkipSeamMetrics struct {
	settingsPos        int  // index of the GetParsedLedgerSettings call (-1 if absent)
	resolveSkipPos     int  // index of the skip.ResolveSkipFor call (-1)
	reservePos         int  // index of the reserveTransaction call (-1)
	rejectDeleteIdemp  bool // the ResolveSkipFor-error branch releases the idempotency key
	rejectReturns      bool // that branch returns (does not fall through to the reserve)
	reserveCarriesFlag bool // reserveTransaction is called with the honoredTracerSkip ident
}

// analyzeCreateSkipSeam walks executeCreateTransaction and extracts the tracer-skip
// resolution facts. The 422 guard is the `if err != nil` that immediately follows
// the `honoredTracerSkip, err := skip.ResolveSkipFor(...)` assignment.
func analyzeCreateSkipSeam(t *testing.T, src string) createSkipSeamMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, createSeamFuncName)

	m := createSkipSeamMetrics{settingsPos: -1, resolveSkipPos: -1, reservePos: -1}

	for i, stmt := range fn.Body.List {
		if m.settingsPos == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.settingsPos = i
		}

		if m.resolveSkipPos == -1 && stmtCallsFunc(stmt, "ResolveSkipFor") {
			m.resolveSkipPos = i
		}

		if m.reservePos == -1 && stmtCallsMethod(stmt, "reserveTransaction") {
			m.reservePos = i

			if call := findCallToMethod(stmt, "reserveTransaction"); call != nil {
				m.reserveCarriesFlag = callHasArgIdent(call, "honoredTracerSkip")
			}
		}

		// The 422 guard sits right after the resolve assignment. Identify it as the
		// first `if err != nil` whose block releases the idempotency key, appearing
		// after the resolve statement but before the reserve.
		if m.resolveSkipPos != -1 && i == m.resolveSkipPos+1 {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				m.rejectDeleteIdemp = blockCallsMethod(ifStmt.Body, "deleteIdempotencyKey")
				m.rejectReturns = blockEndsInReturn(ifStmt.Body)
			}
		}
	}

	return m
}

// findCallToMethod returns the first CallExpr in stmt whose selector method
// matches, or nil.
func findCallToMethod(stmt ast.Stmt, method string) *ast.CallExpr {
	var found *ast.CallExpr

	ast.Inspect(stmt, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
				found = call
			}
		}

		return true
	})

	return found
}

// callHasArgIdent reports whether the call passes a bare identifier with the
// given name as one of its arguments.
func callHasArgIdent(call *ast.CallExpr, name string) bool {
	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && id.Name == name {
			return true
		}
	}

	return false
}

// TestExecuteCreateTransaction_TracerSkip — the create-path wiring proof. Asserts
// that executeCreateTransaction resolves the tracer skip AFTER the settings read
// and BEFORE the reserve anchor, that the 422 (unauthorized skip) branch releases
// the idempotency key and returns before the reserve, and that the resolved
// honoredTracerSkip boolean is threaded into reserveTransaction.
func TestExecuteCreateTransaction_TracerSkip(t *testing.T) {
	src := readSeamSource(t) // transaction_create.go

	m := analyzeCreateSkipSeam(t, src)

	require.NotEqual(t, -1, m.settingsPos, "GetParsedLedgerSettings call not found")
	require.NotEqual(t, -1, m.resolveSkipPos, "skip.ResolveSkipFor call not found")
	require.NotEqual(t, -1, m.reservePos, "reserveTransaction call not found")

	assert.Greater(t, m.resolveSkipPos, m.settingsPos,
		"the tracer skip must be resolved AFTER the settings read (it reads ledgerSettings.Overrides)")
	assert.Less(t, m.resolveSkipPos, m.reservePos,
		"the tracer skip must be resolved BEFORE the reserve anchor it gates")

	assert.True(t, m.rejectDeleteIdemp,
		"an unauthorized skip (422) must release the idempotency key — mirror the fee error path")
	assert.True(t, m.rejectReturns,
		"the 422 branch must return — it must NOT fall through to the reserve anchor")

	assert.True(t, m.reserveCarriesFlag,
		"reserveTransaction must receive the resolved honoredTracerSkip flag")
}

// TestExecuteCreateTransaction_TracerSkip_Bites proves the create-path analyzer
// bites: it must reject a seam that drops the idempotency release on the 422
// branch or stops threading the flag into the reserve.
func TestExecuteCreateTransaction_TracerSkip_Bites(t *testing.T) {
	leaky := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, err := skip.ResolveSkipFor()
	if err != nil {
		// BUG: neither releases the idempotency key nor returns
		_ = err
	}
	reservation := handler.reserveTransaction() // BUG: flag not threaded
	_ = reservation
	_ = ledgerSettings
	_ = honoredTracerSkip
	return nil
}`

	m := analyzeCreateSkipSeam(t, leaky)

	require.NotEqual(t, -1, m.resolveSkipPos, "fixture sanity: ResolveSkipFor must be present")
	require.NotEqual(t, -1, m.reservePos, "fixture sanity: reserveTransaction must be present")

	assert.False(t, m.rejectDeleteIdemp, "gate failed to bite: a 422 branch with no release was reported as releasing")
	assert.False(t, m.rejectReturns, "gate failed to bite: a 422 branch with no return was reported as returning")
	assert.False(t, m.reserveCarriesFlag, "gate failed to bite: a reserve without the flag was reported as carrying it")

	correct := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, err := skip.ResolveSkipFor()
	if err != nil {
		handler.deleteIdempotencyKey()
		return handler.WithError(err)
	}
	reservation := handler.reserveTransaction(honoredTracerSkip)
	_ = reservation
	_ = ledgerSettings
	return nil
}`

	mc := analyzeCreateSkipSeam(t, correct)
	assert.True(t, mc.rejectDeleteIdemp && mc.rejectReturns && mc.reserveCarriesFlag,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.True(t, mc.settingsPos < mc.resolveSkipPos && mc.resolveSkipPos < mc.reservePos,
		"fixture sanity: settings -> resolve -> reserve ordering")
}

// commitCancelSkipMetrics captures the commit/cancel tracer-skip wiring facts,
// all within commitOrCancelTransaction.
type commitCancelSkipMetrics struct {
	settingsPos          int  // index of the GetParsedLedgerSettings re-fetch (-1)
	resolveSkipPos       int  // index of the skip.ResolveSkipFor re-resolution (-1)
	confirmCarriesFlag   bool // confirmReservationsByTransaction receives honoredTracerSkip
	releaseCarriesFlag   bool // releaseReservationsByTransaction receives honoredTracerSkip
	resolveReadsBodySkip bool // ResolveSkipFor is fed from tran.Body.Skip
}

// analyzeCommitCancelSkipSeam walks commitOrCancelTransaction.
func analyzeCommitCancelSkipSeam(t *testing.T, src string) commitCancelSkipMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, commitCancelFuncName)

	m := commitCancelSkipMetrics{settingsPos: -1, resolveSkipPos: -1}

	for i, stmt := range fn.Body.List {
		if m.settingsPos == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.settingsPos = i
		}

		if m.resolveSkipPos == -1 && stmtCallsFunc(stmt, "ResolveSkipFor") {
			m.resolveSkipPos = i
			m.resolveReadsBodySkip = stmtReferencesBodySkip(stmt)
		}

		if call := findCallToMethod(stmt, "confirmReservationsByTransaction"); call != nil {
			m.confirmCarriesFlag = callHasArgIdent(call, "honoredTracerSkip")
		}

		if call := findCallToMethod(stmt, "releaseReservationsByTransaction"); call != nil {
			m.releaseCarriesFlag = callHasArgIdent(call, "honoredTracerSkip")
		}
	}

	return m
}

// stmtReferencesBodySkip reports whether the statement selects `.Skip` off a
// `.Body` selector (i.e. tran.Body.Skip), proving the commit/cancel re-resolution
// reads the persisted body and not a fresh request.
func stmtReferencesBodySkip(stmt ast.Stmt) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Skip" {
			return true
		}

		if inner, ok := sel.X.(*ast.SelectorExpr); ok && inner.Sel.Name == "Body" {
			found = true
		}

		return true
	})

	return found
}

// TestCommitCancel_TracerSkip — the PENDING-flow wiring proof. A PENDING
// transaction defers the tracer confirm/release to /commit and /cancel; without
// this wiring an honored create-time skip would merely relocate the gRPC cost to
// the state transition. Asserts commitOrCancelTransaction re-resolves the skip
// from the persisted body (tran.Body.Skip) AFTER its settings re-fetch and threads
// the resolved boolean into BOTH the by-transaction confirm and release. The
// zero-call behavior given the boolean is proven directly at the helpers in
// transaction_reservation_anchor_test.go (the honored-skip subtests).
func TestCommitCancel_TracerSkip(t *testing.T) {
	src := readStateHandlersSource(t)

	m := analyzeCommitCancelSkipSeam(t, src)

	require.NotEqual(t, -1, m.settingsPos, "GetParsedLedgerSettings re-fetch not found in commitOrCancelTransaction")
	require.NotEqual(t, -1, m.resolveSkipPos, "skip.ResolveSkipFor re-resolution not found")

	assert.Greater(t, m.resolveSkipPos, m.settingsPos,
		"the skip must be re-resolved AFTER the settings re-fetch (it reads ledgerSettings.Overrides)")
	assert.True(t, m.resolveReadsBodySkip,
		"the commit/cancel re-resolution must read the persisted skip from tran.Body.Skip")
	assert.True(t, m.confirmCarriesFlag,
		"confirmReservationsByTransaction must receive the resolved honoredTracerSkip flag")
	assert.True(t, m.releaseCarriesFlag,
		"releaseReservationsByTransaction must receive the resolved honoredTracerSkip flag")
}

// TestCommitCancel_TracerSkip_Bites proves the commit/cancel analyzer bites on a
// seam that stops threading the flag or no longer reads the persisted body skip.
func TestCommitCancel_TracerSkip_Bites(t *testing.T) {
	leaky := `package in
func (handler *TransactionHandler) commitOrCancelTransaction() error {
	ledgerSettings, _ := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, _ := skip.ResolveSkipFor(req.Skip) // BUG: not tran.Body.Skip
	switch status {
	case constant.APPROVED:
		handler.confirmReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
	case constant.CANCELED:
		handler.releaseReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
	}
	_ = honoredTracerSkip
	return nil
}`

	m := analyzeCommitCancelSkipSeam(t, leaky)

	require.NotEqual(t, -1, m.resolveSkipPos, "fixture sanity: ResolveSkipFor must be present")

	assert.False(t, m.resolveReadsBodySkip, "gate failed to bite: a non-body skip source was reported as reading tran.Body.Skip")
	assert.False(t, m.confirmCarriesFlag, "gate failed to bite: a confirm without the flag was reported as carrying it")
	assert.False(t, m.releaseCarriesFlag, "gate failed to bite: a release without the flag was reported as carrying it")

	correct := `package in
func (handler *TransactionHandler) commitOrCancelTransaction() error {
	ledgerSettings, _ := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", tran.Body.Skip != nil && tran.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)
	switch status {
	case constant.APPROVED:
		handler.confirmReservationsByTransaction(ledgerSettings.Tracer, txID, honoredTracerSkip)
	case constant.CANCELED:
		handler.releaseReservationsByTransaction(ledgerSettings.Tracer, txID, honoredTracerSkip)
	}
	return nil
}`

	mc := analyzeCommitCancelSkipSeam(t, correct)
	assert.True(t, mc.resolveReadsBodySkip && mc.confirmCarriesFlag && mc.releaseCarriesFlag,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.Less(t, mc.settingsPos, mc.resolveSkipPos, "fixture sanity: settings re-fetch precedes the re-resolution")
}

// readStateHandlersSource reads transaction_state_handlers.go from disk so the
// commit/cancel wiring gate runs against the live source.
func readStateHandlersSource(t *testing.T) string {
	t.Helper()

	data, err := os.ReadFile(stateHandlersFile)
	if err != nil {
		t.Fatalf("read %s: %v", stateHandlersFile, err)
	}

	src := string(data)
	if !strings.Contains(src, "func (handler *TransactionHandler) "+commitCancelFuncName) {
		t.Fatalf("%s does not contain %s — the gate is pointed at the wrong file", stateHandlersFile, commitCancelFuncName)
	}

	return src
}
