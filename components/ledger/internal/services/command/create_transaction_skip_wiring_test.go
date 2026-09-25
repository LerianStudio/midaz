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

// Per-call tracer-skip wiring proof for the create path. The honored-skip
// short-circuit behavior is proven at the helper level in
// transaction_reservation_anchor_test.go (TestReserveTransaction_HonoredSkip_Proceeds).
// What that cannot see is whether the SEAM actually feeds the resolved boolean into the
// helper, and whether the 422 releases the idempotency key. Those are call-site facts,
// so — mirroring the fee-seam and fail-closed gates — they are asserted over the live
// source AST. A future reorder that drops the resolution, stops threading the flag, or
// forgets the idempotency release fails these gates.

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
// create-path tracer-skip wiring relies on, all within CreateTransactionV2.
type createSkipSeamMetrics struct {
	settingsPos        int  // index of the GetParsedLedgerSettings call (-1 if absent)
	resolveSkipPos     int  // index of the resolveTransactionSkips call (-1)
	reservePos         int  // index of the reservePreparedTransaction call (-1)
	rejectDeleteIdemp  bool // the ResolveSkipFor-error branch releases the idempotency claim
	rejectReturns      bool // that branch returns (does not fall through to the reserve)
	reserveCarriesFlag bool // reservePreparedTransaction carries the honoredTracerSkip field
}

// analyzeCreateSkipSeam walks CreateTransactionV2 and extracts the tracer-skip
// resolution facts. The skip is resolved through the resolveTransactionSkips helper
// (which calls skip.ResolveSkipFor for both controls); the 422 guard is the
// `if err != nil` that immediately follows that resolution call.
func analyzeCreateSkipSeam(t *testing.T, src, engineSrc string) createSkipSeamMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, seamFuncName)

	m := createSkipSeamMetrics{settingsPos: -1, resolveSkipPos: -1, reservePos: -1}

	for i, stmt := range fn.Body.List {
		if m.settingsPos == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.settingsPos = i
		}

		if m.resolveSkipPos == -1 && stmtCallsFunc(stmt, "resolveTransactionSkips") {
			m.resolveSkipPos = i
		}

		// The 422 guard sits right after the resolve assignment. Identify it as the
		// first `if err != nil` whose block releases the idempotency key, appearing
		// after the resolve statement but before the reserve.
		if m.resolveSkipPos != -1 && i == m.resolveSkipPos+1 {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok {
				m.rejectDeleteIdemp = blockCallsMethod(ifStmt.Body, "rollbackCreateClaim")
				m.rejectReturns = blockEndsInReturn(ifStmt.Body)
			}
		}
	}

	engine := findFuncDecl(t, engineSrc, "executeCreateEngine")
	for i, stmt := range engine.Body.List {
		if m.reservePos == -1 && stmtCallsMethod(stmt, "reservePreparedTransaction") {
			m.reservePos = i
			if call := findCallToMethod(stmt, "reservePreparedTransaction"); call != nil {
				m.reserveCarriesFlag = callHasArgIdent(call, "honoredTracerSkip")
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

// callHasArgIdent reports whether the call passes the named value as one of its
// arguments, either as a bare identifier or as a field of the per-request run state
// (run.<name>).
func callHasArgIdent(call *ast.CallExpr, name string) bool {
	for _, arg := range call.Args {
		found := false
		ast.Inspect(arg, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.Ident:
				found = found || value.Name == name
			case *ast.SelectorExpr:
				found = found || value.Sel.Name == name
			}

			return !found
		})
		if found {
			return true
		}
	}

	return false
}

// TestCreateTransactionV2_TracerSkip — the create-path wiring proof. Asserts
// that CreateTransactionV2 resolves the tracer skip AFTER the settings read
// and BEFORE the reserve anchor, that the 422 (unauthorized skip) branch releases
// the idempotency key and returns before the reserve, and that the resolved
// honoredTracerSkip boolean is threaded into reserveTransaction.
func TestCreateTransactionV2_TracerSkip(t *testing.T) {
	src := readSeamSource(t) // create_transaction_v2.go
	engineSrc := readTransportSource(t, "create_transaction_engine.go", "func (uc *UseCase) executeCreateEngine")

	m := analyzeCreateSkipSeam(t, src, engineSrc)

	require.NotEqual(t, -1, m.settingsPos, "GetParsedLedgerSettings call not found")
	require.NotEqual(t, -1, m.resolveSkipPos, "resolveTransactionSkips call not found")
	require.NotEqual(t, -1, m.reservePos, "reservePreparedTransaction call not found")

	assert.Greater(t, m.resolveSkipPos, m.settingsPos,
		"the tracer skip must be resolved AFTER the settings read (it reads ledgerSettings.Overrides)")
	assert.True(t, m.rejectDeleteIdemp,
		"an unauthorized skip (422) must release the idempotency claim — mirror the fee error path")
	assert.True(t, m.rejectReturns,
		"the 422 branch must return — it must NOT fall through to the reserve anchor")

	assert.True(t, m.reserveCarriesFlag,
		"reservePreparedTransaction must receive the resolved honoredTracerSkip flag")

	// The pipeline delegates resolution to resolveTransactionSkips; prove that helper
	// terminates at the real two-key gate (skip.ResolveSkipFor) rather than a stub, so
	// the displaced authz call-site fact stays covered after the extraction.
	helper := findFuncDecl(t, readTransportSource(t, "create_transaction.go", "func resolveTransactionSkips"), "resolveTransactionSkips")
	callsResolver := false

	for _, stmt := range helper.Body.List {
		if stmtCallsFunc(stmt, "ResolveSkipFor") {
			callsResolver = true
			break
		}
	}

	assert.True(t, callsResolver,
		"resolveTransactionSkips must call skip.ResolveSkipFor — the pipeline's skip must terminate at the real two-key gate")
}

// TestCreateTransactionV2_TracerSkip_Bites proves the create-path analyzer
// bites: it must reject a seam that drops the idempotency release on the 422
// branch or stops threading the flag into the reserve.
func TestCreateTransactionV2_TracerSkip_Bites(t *testing.T) {
	leaky := `package command
func (uc *UseCase) CreateTransactionV2() error {
	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings()
	honoredTracerSkip, err := resolveTransactionSkips()
	if err != nil {
		// BUG: neither rolls back the claim nor returns
		_ = err
	}
	_ = ledgerSettings
	_ = honoredTracerSkip
	return nil
}
func (uc *UseCase) executeCreateEngine() error {
	reservation := uc.reservePreparedTransaction() // BUG: flag not threaded
	_ = reservation
	return nil
}`

	m := analyzeCreateSkipSeam(t, leaky, leaky)

	require.NotEqual(t, -1, m.resolveSkipPos, "fixture sanity: resolveTransactionSkips must be present")
	require.NotEqual(t, -1, m.reservePos, "fixture sanity: reservePreparedTransaction must be present")

	assert.False(t, m.rejectDeleteIdemp, "gate failed to bite: a 422 branch with no release was reported as releasing")
	assert.False(t, m.rejectReturns, "gate failed to bite: a 422 branch with no return was reported as returning")
	assert.False(t, m.reserveCarriesFlag, "gate failed to bite: a reserve without the flag was reported as carrying it")

	correct := `package command
func (uc *UseCase) CreateTransactionV2() error {
	ledgerSettings, err := uc.TransactionReader.GetParsedLedgerSettings()
	honoredTracerSkip, err := resolveTransactionSkips()
	if err != nil {
		uc.rollbackCreateClaim()
		return err
	}
	_ = ledgerSettings
	return nil
}
func (uc *UseCase) executeCreateEngine() error {
	reservation := uc.reservePreparedTransaction(ContextTracerInput{HonoredSkip: run.honoredTracerSkip})
	_ = reservation
	return nil
}`

	mc := analyzeCreateSkipSeam(t, correct, correct)
	assert.True(t, mc.rejectDeleteIdemp && mc.rejectReturns && mc.reserveCarriesFlag,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.True(t, mc.settingsPos < mc.resolveSkipPos,
		"fixture sanity: settings must precede skip resolution")
}
