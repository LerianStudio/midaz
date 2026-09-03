// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
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
	resolveSkipPos     int  // index of the resolveTransactionSkips call (-1)
	reservePos         int  // index of the reserveTransaction call (-1)
	rejectDeleteIdemp  bool // the ResolveSkipFor-error branch releases the idempotency key
	rejectReturns      bool // that branch returns (does not fall through to the reserve)
	reserveCarriesFlag bool // reserveTransaction is called with the honoredTracerSkip ident
}

// analyzeCreateSkipSeam walks executeCreateTransaction and extracts the tracer-skip
// resolution facts. The skip is resolved through the resolveTransactionSkips helper
// (which calls skip.ResolveSkipFor for both controls); the 422 guard is the
// `if err != nil` that immediately follows that resolution call.
func analyzeCreateSkipSeam(t *testing.T, src string) createSkipSeamMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, createSeamFuncName)

	m := createSkipSeamMetrics{settingsPos: -1, resolveSkipPos: -1, reservePos: -1}

	for i, stmt := range fn.Body.List {
		if m.settingsPos == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.settingsPos = i
		}

		if m.resolveSkipPos == -1 && stmtCallsFunc(stmt, "resolveTransactionSkips") {
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
	require.NotEqual(t, -1, m.resolveSkipPos, "resolveTransactionSkips call not found")
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

	// The orchestrator delegates resolution to resolveTransactionSkips; prove that
	// helper terminates at the real two-key gate (skip.ResolveSkipFor) rather than a
	// stub, so the displaced authz call-site fact stays covered after the extraction.
	helper := findFuncDecl(t, src, "resolveTransactionSkips")
	callsResolver := false

	for _, stmt := range helper.Body.List {
		if stmtCallsFunc(stmt, "ResolveSkipFor") {
			callsResolver = true
			break
		}
	}

	assert.True(t, callsResolver,
		"resolveTransactionSkips must call skip.ResolveSkipFor — the orchestrator's skip must terminate at the real two-key gate")
}

// TestExecuteCreateTransaction_TracerSkip_Bites proves the create-path analyzer
// bites: it must reject a seam that drops the idempotency release on the 422
// branch or stops threading the flag into the reserve.
func TestExecuteCreateTransaction_TracerSkip_Bites(t *testing.T) {
	leaky := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, err := resolveTransactionSkips()
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

	require.NotEqual(t, -1, m.resolveSkipPos, "fixture sanity: resolveTransactionSkips must be present")
	require.NotEqual(t, -1, m.reservePos, "fixture sanity: reserveTransaction must be present")

	assert.False(t, m.rejectDeleteIdemp, "gate failed to bite: a 422 branch with no release was reported as releasing")
	assert.False(t, m.rejectReturns, "gate failed to bite: a 422 branch with no return was reported as returning")
	assert.False(t, m.reserveCarriesFlag, "gate failed to bite: a reserve without the flag was reported as carrying it")

	correct := `package in
func (handler *TransactionHandler) executeCreateTransaction() error {
	ledgerSettings, err := handler.Query.GetParsedLedgerSettings()
	honoredTracerSkip, err := resolveTransactionSkips()
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

// commitCancelOverdraftMetrics captures the overdraft-enrichment wiring facts of
// commitOrCancelTransaction.
type commitCancelOverdraftMetrics struct {
	// enrichStatuses holds the transaction-status constants named in the
	// condition guarding the enrichOverdraftOperations call.
	enrichStatuses map[string]bool
	// enrichPos is the index of the guarded enrichment statement (-1 if absent).
	enrichPos int
	// validatePos is the index of the ValidateAccountingRules call (-1).
	validatePos int
	// companionsReachFromTo reports whether companionFromTos is appended into the
	// fromTo slice that BuildOperations consumes.
	companionsReachFromTo bool
}

// analyzeCommitCancelOverdraftSeam walks commitOrCancelTransaction and extracts
// which transitions enrich overdraft companions and whether those companions
// reach validation and the operation-record builder.
func analyzeCommitCancelOverdraftSeam(t *testing.T, src string) commitCancelOverdraftMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, commitCancelFuncName)

	m := commitCancelOverdraftMetrics{
		enrichStatuses: map[string]bool{},
		enrichPos:      -1,
		validatePos:    -1,
	}

	for i, stmt := range fn.Body.List {
		if m.enrichPos == -1 {
			if ifStmt, ok := stmt.(*ast.IfStmt); ok && stmtCallsFunc(ifStmt.Body, "enrichOverdraftOperations") {
				m.enrichPos = i

				for _, name := range constantSelectorNames(ifStmt.Cond) {
					m.enrichStatuses[name] = true
				}
			}
		}

		if m.validatePos == -1 && stmtCallsMethod(stmt, "ValidateAccountingRules") {
			m.validatePos = i
		}

		if stmtAppendsIdentInto(stmt, "fromTo", "companionFromTos") {
			m.companionsReachFromTo = true
		}
	}

	return m
}

// constantSelectorNames returns the selector names of every `constant.X`
// reference in the expression.
func constantSelectorNames(expr ast.Expr) []string {
	names := make([]string, 0, 2)

	ast.Inspect(expr, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "constant" {
			names = append(names, sel.Sel.Name)
		}

		return true
	})

	return names
}

// stmtAppendsIdentInto reports whether stmt assigns to `target` the result of an
// append that spreads `source` (i.e. target = append(target, source...)).
func stmtAppendsIdentInto(stmt ast.Stmt, target, source string) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}

	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != target {
		return false
	}

	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}

	fun, ok := call.Fun.(*ast.Ident)
	if !ok || fun.Name != "append" || call.Ellipsis == token.NoPos {
		return false
	}

	for _, arg := range call.Args {
		if id, ok := arg.(*ast.Ident); ok && id.Name == source {
			return true
		}
	}

	return false
}

// TestCommitCancel_OverdraftEnrichmentCoversBothTransitions — the two-phase
// overdraft wiring proof. Both transitions move funds on an overdrafted balance:
// a cancel restores the held capacity, and a commit posts the destination credit
// that repays outstanding overdraft. Enriching is what puts the companion leg in
// front of ValidateAccountingRules (so the route's overdraft rubric is enforced),
// into the atomic batch (so the companion balance moves in lock-step) and into
// the fromTo slice (so BuildOperations persists the overdraft leg). Gating the
// enrichment on the cancel alone silently drops all three on commit, which is a
// money-correctness bug the enrichment unit tests cannot see — they never reach
// this call site. Asserted over the live source AST.
func TestCommitCancel_OverdraftEnrichmentCoversBothTransitions(t *testing.T) {
	src := readStateHandlersSource(t)

	m := analyzeCommitCancelOverdraftSeam(t, src)

	require.NotEqual(t, -1, m.enrichPos, "guarded enrichOverdraftOperations call not found in commitOrCancelTransaction")
	require.NotEqual(t, -1, m.validatePos, "ValidateAccountingRules call not found")

	assert.True(t, m.enrichStatuses["APPROVED"],
		"the commit transition must enrich overdraft companions: its destination credit is what repays outstanding overdraft")
	assert.True(t, m.enrichStatuses["CANCELED"],
		"the cancel transition must enrich overdraft companions: it restores the capacity the hold consumed")

	assert.Less(t, m.enrichPos, m.validatePos,
		"enrichment must precede ValidateAccountingRules so the companion is subject to the route's overdraft rubric")
	assert.True(t, m.companionsReachFromTo,
		"companionFromTos must be appended into fromTo so BuildOperations persists the overdraft operation")
}

// TestCommitCancel_OverdraftEnrichmentCoversBothTransitions_Bites proves the
// analyzer bites on a seam that enriches on cancel only, validates before
// enriching, or never threads the companions into fromTo.
func TestCommitCancel_OverdraftEnrichmentCoversBothTransitions_Bites(t *testing.T) {
	leaky := `package in
func (handler *TransactionHandler) commitOrCancelTransaction() error {
	routeCache, _ := handler.Query.ValidateAccountingRules(ctx, balanceOps, validate, action)
	var companionFromTos []mtransaction.FromTo
	if transactionStatus == constant.CANCELED { // BUG: commit is not enriched
		balanceOps, companionFromTos, _ = enrichOverdraftOperations(readCtx, balanceOps, validate)
	}
	_, _ = routeCache, companionFromTos
	return nil
}`

	m := analyzeCommitCancelOverdraftSeam(t, leaky)

	require.NotEqual(t, -1, m.enrichPos, "fixture sanity: the guarded enrichment must be present")

	assert.False(t, m.enrichStatuses["APPROVED"],
		"gate failed to bite: a cancel-only guard was reported as covering the commit")
	assert.True(t, m.enrichStatuses["CANCELED"], "fixture sanity: the cancel status must be detected")
	assert.Greater(t, m.enrichPos, m.validatePos,
		"fixture sanity: this fixture validates before enriching")
	assert.False(t, m.companionsReachFromTo,
		"gate failed to bite: companions never appended into fromTo were reported as reaching it")

	correct := `package in
func (handler *TransactionHandler) commitOrCancelTransaction() error {
	var companionFromTos []mtransaction.FromTo
	if transactionStatus == constant.APPROVED || transactionStatus == constant.CANCELED {
		balanceOps, companionFromTos, _ = enrichOverdraftOperations(readCtx, balanceOps, validate)
	}
	routeCache, _ := handler.Query.ValidateAccountingRules(ctx, balanceOps, validate, action)
	fromTo = append(fromTo, companionFromTos...)
	_ = routeCache
	return nil
}`

	mc := analyzeCommitCancelOverdraftSeam(t, correct)
	assert.True(t, mc.enrichStatuses["APPROVED"] && mc.enrichStatuses["CANCELED"] && mc.companionsReachFromTo,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.Less(t, mc.enrichPos, mc.validatePos, "fixture sanity: enrichment precedes validation")
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

// =============================================================================
// ACCOUNT-BLOCK SEMANTICS MATRIX (REVISED ADR-004)
// =============================================================================
// One test per row of the frozen semantics matrix, named for the semantics it
// proves rather than for the function it calls. The rule the whole matrix
// expresses is a single sentence: EVALUATE AT EVERY MUTATION OF BALANCE, never
// inherit a decision taken at an earlier one.
//
// | Moment            | Behaviour                                              |
// |-------------------|--------------------------------------------------------|
// | Creation          | blocked && !grant => denied at the atomic point; the    |
// |                   | grant is computed by the enrichment when the            |
// |                   | transaction carries an operationalTypeCode              |
// | Commit of pending | RE-EVALUATED: the code is recovered from the body and   |
// |                   | the exception set is read again. An exception that      |
// |                   | expired since the pending denies; one created since     |
// |                   | the pending allows                                      |
// | Cancel of pending | ALWAYS allowed on a blocked account (exempt)            |
// | Revert            | A new transaction: gated as a creation                  |
// | Idempotent replay | Returns the original decision, unchanged                |
//
// The denial itself belongs to balance_atomic_operation.lua and is proven in the
// redis adapter suite; what is proven here is which grant each moment carries
// into it, which is what decides the denial.

// matrixLiveException is an exception with no validity bounds: live at every
// instant, so a row that does not grant cannot be blamed on the clock.
func matrixLiveException(id string) []*mmodel.AccountException {
	return []*mmodel.AccountException{exc(id, []string{"PIX_IN"}, nil, nil, nil)}
}

// TestAccountBlockSemanticsMatrix_GrantCarriedAtEachMutation walks the creation
// and commit rows of the matrix. Each case states the moment, what the exception
// store holds AT THAT MOMENT, and the grant the transaction consequently carries
// into the atomic script.
func TestAccountBlockSemanticsMatrix_GrantCarriedAtEachMutation(t *testing.T) {
	t.Parallel()

	hourAgo := time.Now().UTC().Add(-time.Hour)

	tests := []struct {
		name string
		// commit=false exercises the creation entry (block signal: the balance
		// projection); commit=true exercises the commit re-evaluation (block
		// signal: the blocked-accounts index).
		commit          bool
		accountBlocked  bool
		indexedBlocked  bool
		code            string
		exceptions      []*mmodel.AccountException
		wantGranted     bool
		wantAppliedID   string
		wantLoaderCalls int
	}{
		{
			name:           "creation of a blocked account with no exception carries no grant, so the script denies it",
			accountBlocked: true, code: "PIX_IN",
			exceptions:  nil,
			wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name:           "creation of a blocked account with a live exception carries the grant that transpasses the gate",
			accountBlocked: true, code: "PIX_IN",
			exceptions:  matrixLiveException("exc-at-create"),
			wantGranted: true, wantAppliedID: "exc-at-create", wantLoaderCalls: 1,
		},
		{
			name:           "creation without an operational type code never reaches the exception store",
			accountBlocked: true, code: "",
			exceptions:  matrixLiveException("exc-unreachable"),
			wantGranted: false, wantLoaderCalls: 0,
		},
		{
			name:           "commit of a pending whose exception is still live carries a fresh grant",
			commit:         true,
			indexedBlocked: true, code: "PIX_IN",
			exceptions:  matrixLiveException("exc-still-live"),
			wantGranted: true, wantAppliedID: "exc-still-live", wantLoaderCalls: 1,
		},
		{
			name:           "commit of a pending whose exception expired since creation carries no grant, so the commit is denied",
			commit:         true,
			indexedBlocked: true, code: "PIX_IN",
			exceptions:  []*mmodel.AccountException{exc("exc-expired-since", []string{"PIX_IN"}, nil, nil, &hourAgo)},
			wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name:           "commit of a pending whose exception was created after it carries a grant, so the commit is allowed",
			commit:         true,
			indexedBlocked: true, code: "PIX_IN",
			// The create-time read returned nothing; this is the commit-time read,
			// and it is the only one that decides the commit.
			exceptions:  matrixLiveException("exc-created-after-pending"),
			wantGranted: true, wantAppliedID: "exc-created-after-pending", wantLoaderCalls: 1,
		},
		{
			name:           "commit of a pending with no exception at all carries no grant",
			commit:         true,
			indexedBlocked: true, code: "PIX_IN",
			exceptions:  nil,
			wantGranted: false, wantLoaderCalls: 1,
		},
		{
			name:           "commit of an account the index does not hold never reaches the exception store",
			commit:         true,
			indexedBlocked: false, code: "PIX_IN",
			exceptions:  matrixLiveException("exc-unreachable"),
			wantGranted: false, wantLoaderCalls: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			org, ledger := uuid.New(), uuid.New()
			accID := uuid.New()

			validate, key := fromValidate("@acc", "default")
			bal := exceptionEnrichBalance(accID.String(), "@acc", "default", tt.accountBlocked, true, true)

			loader, loaderCalls := countingLoader(tt.exceptions, nil)

			var applied *string

			if tt.commit {
				var blocked []uuid.UUID
				if tt.indexedBlocked {
					blocked = []uuid.UUID{accID}
				}

				resolver, _ := countingResolver(blocked, nil)

				var err error

				applied, err = reevaluateAccountExceptionGrants(context.Background(), resolver, loader, nil,
					org, ledger, tt.code, validate, []*mmodel.Balance{bal})
				require.NoError(t, err)
			} else {
				applied = enrichAccountExceptionGrants(context.Background(), loader, nil,
					org, ledger, tt.code, validate, []*mmodel.Balance{bal}, nil)
			}

			assert.Equal(t, tt.wantLoaderCalls, *loaderCalls, "exception store reads")

			got := validate.From[key]
			assert.Equal(t, tt.wantGranted, got.BlockBypassGranted, "grant carried into the atomic script")

			if tt.wantAppliedID == "" {
				assert.Nil(t, applied)
				assert.Empty(t, got.GrantedExceptionID)
			} else {
				require.NotNil(t, applied)
				assert.Equal(t, tt.wantAppliedID, *applied)
				assert.Equal(t, tt.wantAppliedID, got.GrantedExceptionID)
			}
		})
	}
}

// TestAccountBlockSemanticsMatrix_ReplayPreservesTheOriginalDecision covers the
// replay row. The commit's decision is recorded on the span and in the log and
// deliberately NOT written into the transaction body, so the body a replay
// re-hashes is byte-identical to the one the create path hashed: the idempotency
// key cannot move, and the replay answers with the original decision.
func TestAccountBlockSemanticsMatrix_ReplayPreservesTheOriginalDecision(t *testing.T) {
	t.Parallel()

	org, ledger := uuid.New(), uuid.New()
	accID := uuid.New()

	body := mtransaction.Transaction{
		Description:         "pending transfer",
		OperationalTypeCode: "PIX_IN",
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: decimal.NewFromInt(100),
		},
	}

	before, err := json.Marshal(body)
	require.NoError(t, err)

	validate, key := fromValidate("@acc", "default")
	bal := exceptionEnrichBalance(accID.String(), "@acc", "default", false, true, true)

	resolver, _ := countingResolver([]uuid.UUID{accID}, nil)
	loader, _ := countingLoader(matrixLiveException("exc-at-commit"), nil)

	applied, err := reevaluateAccountExceptionGrants(context.Background(), resolver, loader, nil,
		org, ledger, body.OperationalTypeCode, validate, []*mmodel.Balance{bal})
	require.NoError(t, err)
	require.NotNil(t, applied, "fixture sanity: this commit must actually take a decision")
	require.True(t, validate.From[key].BlockBypassGranted)

	after, err := json.Marshal(body)
	require.NoError(t, err)

	assert.JSONEq(t, string(before), string(after),
		"the commit decision must not enter the body, or the idempotency preimage would move under a replay")
}

// commitExceptionSeamMetrics captures the call-site facts of the commit-time
// re-evaluation that no unit test of the enrichment itself can observe.
type commitExceptionSeamMetrics struct {
	reevaluatePos    int  // top-level stmt index of the guarded re-evaluation (-1)
	buildOpsPos      int  // top-level stmt index of buildBalanceOperations (-1)
	cancelIsExempt   bool // the re-evaluation sits under a `!= constant.CANCELED` guard
	errorReleasesLck bool // the failure branch releases the pending-transaction lock
	errorReturns     bool // the failure branch returns instead of proceeding unguarded
}

// findCallToFunc finds a call to a bare (non-method) function by name.
func findCallToFunc(node ast.Node, name string) *ast.CallExpr {
	var found *ast.CallExpr

	ast.Inspect(node, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if id, ok := call.Fun.(*ast.Ident); ok && id.Name == name {
				found = call
			}
		}

		return true
	})

	return found
}

// analyzeCommitExceptionSeam walks commitOrCancelTransaction and extracts where
// the re-evaluation sits, whether a cancel is exempt from it, and whether an
// unreadable index fails the transition closed.
func analyzeCommitExceptionSeam(t *testing.T, src string) commitExceptionSeamMetrics {
	t.Helper()

	fn := findFuncDecl(t, src, commitCancelFuncName)

	m := commitExceptionSeamMetrics{reevaluatePos: -1, buildOpsPos: -1}

	for i, stmt := range fn.Body.List {
		if m.buildOpsPos < 0 && findCallToFunc(stmt, "buildBalanceOperations") != nil {
			m.buildOpsPos = i
		}

		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok || findCallToFunc(ifStmt, "reevaluateAccountExceptionGrants") == nil {
			continue
		}

		if m.reevaluatePos >= 0 {
			continue
		}

		m.reevaluatePos = i

		// The guard must EXCLUDE cancel, not merely mention it: `== CANCELED`
		// would invert the exemption and re-evaluate exactly the transition the
		// script waives.
		if bin, ok := ifStmt.Cond.(*ast.BinaryExpr); ok && bin.Op == token.NEQ {
			for _, name := range constantSelectorNames(bin) {
				if name == "CANCELED" {
					m.cancelIsExempt = true
				}
			}
		}

		// The failure branch is the `if <err> != nil` inside the guarded block.
		for _, inner := range ifStmt.Body.List {
			innerIf, ok := inner.(*ast.IfStmt)
			if !ok || findCallToFunc(innerIf, "reevaluateAccountExceptionGrants") != nil {
				continue
			}

			if findCallToFunc(innerIf, "deleteLockOnError") != nil {
				m.errorReleasesLck = true
			}

			for _, branchStmt := range innerIf.Body.List {
				if _, ok := branchStmt.(*ast.ReturnStmt); ok {
					m.errorReturns = true
				}
			}
		}
	}

	return m
}

// TestAccountBlockSemanticsMatrix_CommitSeam covers the two structural rows of
// the matrix — the commit re-evaluates, the cancel is exempt — plus the
// fail-posture of an unreadable index. All three are call-site facts, invisible
// to the enrichment's own unit tests, so they are asserted over the live source
// AST in the same style as the tracer-skip and overdraft seams above.
func TestAccountBlockSemanticsMatrix_CommitSeam(t *testing.T) {
	t.Parallel()

	m := analyzeCommitExceptionSeam(t, readStateHandlersSource(t))

	require.GreaterOrEqual(t, m.reevaluatePos, 0,
		"the commit must re-evaluate the exception set at its own instant")
	require.GreaterOrEqual(t, m.buildOpsPos, 0)

	assert.Less(t, m.reevaluatePos, m.buildOpsPos,
		"the re-evaluation must precede buildBalanceOperations, which is what carries the grant into the script")
	assert.True(t, m.cancelIsExempt,
		"a cancel returns the hold to the account's own balance and is waived by the script: it must not be re-evaluated")
	assert.True(t, m.errorReleasesLck,
		"an unreadable index must release the pending-transaction lock, or the transaction is stuck until the TTL")
	assert.True(t, m.errorReturns,
		"an unreadable index must fail the transition, never fall through to move money ungated")
}

// TestAccountBlockSemanticsMatrix_CommitSeam_Bites proves the gate above has
// teeth: a shape that re-evaluates on every transition (cancel included) and
// swallows the index failure must fail every fact the real shape satisfies.
func TestAccountBlockSemanticsMatrix_CommitSeam_Bites(t *testing.T) {
	t.Parallel()

	const wrong = `package in

func (handler *TransactionHandler) commitOrCancelTransaction() error {
	if transactionStatus == constant.CANCELED {
		commitAppliedExceptionID, exceptionErr := reevaluateAccountExceptionGrants(ctx, resolve, loader, nil, org, ledger, code, validate, balances)
		if exceptionErr != nil {
			logger.Log(ctx, libLog.LevelError, "swallowed")
		}

		_ = commitAppliedExceptionID
	}

	balanceOps := buildBalanceOperations(ctx, organizationID, ledgerID, validate, balances)

	return nil
}
`

	m := analyzeCommitExceptionSeam(t, wrong)

	require.GreaterOrEqual(t, m.reevaluatePos, 0, "fixture sanity: the wrong shape still calls the re-evaluation")
	assert.False(t, m.cancelIsExempt, "an `== CANCELED` guard inverts the exemption and must not read as exempt")
	assert.False(t, m.errorReleasesLck, "a swallowed failure must not read as releasing the lock")
	assert.False(t, m.errorReturns, "a swallowed failure must not read as failing closed")
}

// TestAccountBlockSemanticsMatrix_RevertIsGatedAsACreation covers the revert row:
// a revert is not a state transition of the origin, it is a NEW transaction, so
// it goes through the create path and is gated by the create-time enrichment
// like any other creation. Nothing about the origin's grant carries over.
func TestAccountBlockSemanticsMatrix_RevertIsGatedAsACreation(t *testing.T) {
	t.Parallel()

	src := readStateHandlersSource(t)

	revert := findFuncDecl(t, src, "revertTransaction")

	assert.NotNil(t, findCallToMethod(revert.Body, "createRevertTransaction"),
		"a revert must be created through the create path, which is what gates it")
	assert.Nil(t, findCallToFunc(revert, "reevaluateAccountExceptionGrants"),
		"a revert must not re-evaluate as if it were a transition of the origin")
}
