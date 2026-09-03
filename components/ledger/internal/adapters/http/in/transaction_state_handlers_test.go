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

// Per-call tracer-skip wiring proof for the commit/cancel transition. The honored-skip
// short-circuit behavior is proven at the helper level in the command package (the
// honored-skip subtests of TestConfirmReservationsByTransaction /
// TestReleaseReservationsByTransaction). What those cannot see is whether the SEAM
// actually feeds the resolved boolean into the helpers. That is a call-site fact, so it
// is asserted over the live source AST. A future reorder that drops the resolution or
// stops threading the flag fails these gates. The same proof for the create path lives
// beside the create seam, in the command package.

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

// stmtCallsFunc reports whether the statement contains a call to a function with the
// given name, package-qualified or not.
func stmtCallsFunc(stmt ast.Node, name string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		switch f := call.Fun.(type) {
		case *ast.Ident:
			if f.Name == name {
				found = true
			}
		case *ast.SelectorExpr:
			if f.Sel.Name == name {
				found = true
			}
		}

		return true
	})

	return found
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

		if call := findCallToMethod(stmt, "ConfirmReservationsByTransaction"); call != nil {
			m.confirmCarriesFlag = callHasArgIdent(call, "honoredTracerSkip")
		}

		if call := findCallToMethod(stmt, "ReleaseReservationsByTransaction"); call != nil {
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
		"ConfirmReservationsByTransaction must receive the resolved honoredTracerSkip flag")
	assert.True(t, m.releaseCarriesFlag,
		"ReleaseReservationsByTransaction must receive the resolved honoredTracerSkip flag")
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
		handler.Command.ConfirmReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
	case constant.CANCELED:
		handler.Command.ReleaseReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
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
		handler.Command.ConfirmReservationsByTransaction(ledgerSettings.Tracer, txID, honoredTracerSkip)
	case constant.CANCELED:
		handler.Command.ReleaseReservationsByTransaction(ledgerSettings.Tracer, txID, honoredTracerSkip)
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
			if ifStmt, ok := stmt.(*ast.IfStmt); ok && stmtCallsFunc(ifStmt.Body, "EnrichOverdraftOperations") {
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
		balanceOps, companionFromTos, _ = command.EnrichOverdraftOperations(readCtx, balanceOps, validate)
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
		balanceOps, companionFromTos, _ = command.EnrichOverdraftOperations(readCtx, balanceOps, validate)
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
