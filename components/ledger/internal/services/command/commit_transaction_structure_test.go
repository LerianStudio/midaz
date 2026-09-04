// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/token"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Per-call tracer-skip and overdraft wiring proofs for the commit/cancel transition.
// The honored-skip short-circuit behavior is proven at the helper level (the
// honored-skip subtests of TestConfirmReservationsByTransaction /
// TestReleaseReservationsByTransaction). What those cannot see is whether the pipeline
// actually feeds the resolved boolean into the helpers. That is a call-site fact, so it
// is asserted over the live source AST. A future reorder that drops the resolution or
// stops threading the flag fails these gates. The same proof for the create path lives
// beside the create seam.

const (
	pendingPrepareFuncName  = "preparePendingTransition"
	pendingFinalizeFuncName = "finalizePendingTransition"
	pendingTransitionV2Func = "transitionPendingV2"
	pendingStepsFile        = "transition_pending_steps.go"
	pendingPipelineFile     = "commit_transaction.go"
)

// commitCancelSkipMetrics captures the commit/cancel tracer-skip wiring facts. The
// resolution lives in preparePendingTransition; the confirm/release calls it feeds live
// in transitionPendingV2.
type commitCancelSkipMetrics struct {
	settingsPos          int  // index of the GetParsedLedgerSettings read (-1)
	resolveSkipPos       int  // index of the skip.ResolveSkipFor re-resolution (-1)
	confirmCarriesFlag   bool // confirmReservationsByTransaction receives honoredTracerSkip
	releaseCarriesFlag   bool // releaseReservationsByTransaction receives honoredTracerSkip
	resolveReadsBodySkip bool // ResolveSkipFor is fed from tran.Body.Skip
}

// analyzeCommitCancelSkipSeam walks the preparation step for the resolution facts and
// the /v2 pipeline for the flag-threading facts. Both sources may be the same string,
// which is what the bite fixtures rely on.
func analyzeCommitCancelSkipSeam(t *testing.T, prepareSrc, pipelineSrc string) commitCancelSkipMetrics {
	t.Helper()

	m := commitCancelSkipMetrics{settingsPos: -1, resolveSkipPos: -1}

	prepare := findFuncDecl(t, prepareSrc, pendingPrepareFuncName)

	for i, stmt := range prepare.Body.List {
		if m.settingsPos == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			m.settingsPos = i
		}

		if m.resolveSkipPos == -1 && stmtCallsFunc(stmt, "ResolveSkipFor") {
			m.resolveSkipPos = i
			m.resolveReadsBodySkip = stmtReferencesBodySkip(stmt)
		}
	}

	pipeline := findFuncDecl(t, pipelineSrc, pendingTransitionV2Func)

	for _, stmt := range pipeline.Body.List {
		if call := findCallToMethod(stmt, "confirmReservationsByTransaction"); call != nil {
			m.confirmCarriesFlag = callHasSelectorArg(call, "honoredTracerSkip")
		}

		if call := findCallToMethod(stmt, "releaseReservationsByTransaction"); call != nil {
			m.releaseCarriesFlag = callHasSelectorArg(call, "honoredTracerSkip")
		}
	}

	return m
}

// callHasSelectorArg reports whether the call passes the named value as one of its
// arguments, either as a bare identifier or as a field selected off the run state
// (run.honoredTracerSkip).
func callHasSelectorArg(call *ast.CallExpr, name string) bool {
	if callHasArgIdent(call, name) {
		return true
	}

	for _, arg := range call.Args {
		if sel, ok := arg.(*ast.SelectorExpr); ok && sel.Sel.Name == name {
			return true
		}
	}

	return false
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
// the state transition. Asserts the preparation step re-resolves the skip from the
// persisted body (tran.Body.Skip) AFTER its settings read and that the /v2 pipeline
// threads the resolved boolean into BOTH the by-transaction confirm and release. The
// zero-call behavior given the boolean is proven directly at the helpers in
// transaction_reservation_anchor_test.go (the honored-skip subtests).
func TestCommitCancel_TracerSkip(t *testing.T) {
	prepareSrc := readTransportSource(t, pendingStepsFile, "func (uc *UseCase) "+pendingPrepareFuncName)
	pipelineSrc := readTransportSource(t, pendingPipelineFile, "func (uc *UseCase) "+pendingTransitionV2Func)

	m := analyzeCommitCancelSkipSeam(t, prepareSrc, pipelineSrc)

	require.NotEqual(t, -1, m.settingsPos, "GetParsedLedgerSettings read not found in "+pendingPrepareFuncName)
	require.NotEqual(t, -1, m.resolveSkipPos, "skip.ResolveSkipFor re-resolution not found")

	assert.Greater(t, m.resolveSkipPos, m.settingsPos,
		"the skip must be re-resolved AFTER the settings read (it reads ledgerSettings.Overrides)")
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
	leaky := `package command
func (uc *UseCase) preparePendingTransition() error {
	ledgerSettings, _ := uc.TransactionReader.GetParsedLedgerSettings()
	honoredTracerSkip, _ := skip.ResolveSkipFor(req.Skip) // BUG: not tran.Body.Skip
	_ = honoredTracerSkip
	return nil
}

func (uc *UseCase) transitionPendingV2() error {
	switch run.status {
	case constant.APPROVED:
		uc.confirmReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
	case constant.CANCELED:
		uc.releaseReservationsByTransaction(ledgerSettings.Tracer, txID) // BUG: no flag
	}
	return nil
}`

	m := analyzeCommitCancelSkipSeam(t, leaky, leaky)

	require.NotEqual(t, -1, m.resolveSkipPos, "fixture sanity: ResolveSkipFor must be present")

	assert.False(t, m.resolveReadsBodySkip, "gate failed to bite: a non-body skip source was reported as reading tran.Body.Skip")
	assert.False(t, m.confirmCarriesFlag, "gate failed to bite: a confirm without the flag was reported as carrying it")
	assert.False(t, m.releaseCarriesFlag, "gate failed to bite: a release without the flag was reported as carrying it")

	correct := `package command
func (uc *UseCase) preparePendingTransition() error {
	ledgerSettings, _ := uc.TransactionReader.GetParsedLedgerSettings()
	honoredTracerSkip, _ := skip.ResolveSkipFor("tracer", run.tran.Body.Skip != nil && run.tran.Body.Skip.Tracer, ledgerSettings.Overrides.AllowTracerSkip)
	run.honoredTracerSkip = honoredTracerSkip
	return nil
}

func (uc *UseCase) transitionPendingV2() error {
	switch run.status {
	case constant.APPROVED:
		uc.confirmReservationsByTransaction(run.ledgerSettings.Tracer, txID, run.honoredTracerSkip)
	case constant.CANCELED:
		uc.releaseReservationsByTransaction(run.ledgerSettings.Tracer, txID, run.honoredTracerSkip)
	}
	return nil
}`

	mc := analyzeCommitCancelSkipSeam(t, correct, correct)
	assert.True(t, mc.resolveReadsBodySkip && mc.confirmCarriesFlag && mc.releaseCarriesFlag,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.Less(t, mc.settingsPos, mc.resolveSkipPos, "fixture sanity: the settings read precedes the re-resolution")
}

// commitCancelOverdraftMetrics captures the overdraft-enrichment wiring facts of the
// pending transition: which transitions enrich (preparePendingTransition) and whether
// the companions reach the operation builder (finalizePendingTransition).
type commitCancelOverdraftMetrics struct {
	// enrichStatuses holds the transaction-status constants named in the
	// condition guarding the enrichment call.
	enrichStatuses map[string]bool
	// enrichPos is the index of the guarded enrichment statement (-1 if absent).
	enrichPos int
	// validatePos is the index of the ValidateAccountingRules call (-1).
	validatePos int
	// companionsReachFromTo reports whether the companion FromTo entries are
	// appended into the fromTo slice BuildOperations consumes.
	companionsReachFromTo bool
}

// analyzeCommitCancelOverdraftSeam walks the preparation step for the enrichment guard
// and the finalize step for the companion splice. Both sources may be the same string,
// which is what the bite fixtures rely on.
func analyzeCommitCancelOverdraftSeam(t *testing.T, prepareSrc, finalizeSrc string) commitCancelOverdraftMetrics {
	t.Helper()

	m := commitCancelOverdraftMetrics{
		enrichStatuses: map[string]bool{},
		enrichPos:      -1,
		validatePos:    -1,
	}

	prepare := findFuncDecl(t, prepareSrc, pendingPrepareFuncName)

	for i, stmt := range prepare.Body.List {
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
	}

	finalize := findFuncDecl(t, finalizeSrc, pendingFinalizeFuncName)

	for _, stmt := range finalize.Body.List {
		if stmtAppendsInto(stmt, "fromTo", "companionFromTos") {
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

// stmtAppendsInto reports whether stmt assigns to `target` the result of an append that
// spreads `source` (i.e. target = append(target, source...)). Both names are matched as
// a bare identifier or as the trailing field of a selector, so state carried on the run
// struct (run.fromTo, run.companionFromTos) is recognised.
func stmtAppendsInto(stmt ast.Stmt, target, source string) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}

	if !exprNames(assign.Lhs[0], target) {
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
		if exprNames(arg, source) {
			return true
		}
	}

	return false
}

// exprNames reports whether expr is the identifier name or a selector ending in it.
func exprNames(expr ast.Expr, name string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return e.Sel.Name == name
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
	src := readTransportSource(t, pendingStepsFile, "func (uc *UseCase) "+pendingPrepareFuncName)

	m := analyzeCommitCancelOverdraftSeam(t, src, src)

	require.NotEqual(t, -1, m.enrichPos, "guarded enrichOverdraftOperations call not found in "+pendingPrepareFuncName)
	require.NotEqual(t, -1, m.validatePos, "ValidateAccountingRules call not found")

	assert.True(t, m.enrichStatuses["APPROVED"],
		"the commit transition must enrich overdraft companions: its destination credit is what repays outstanding overdraft")
	assert.True(t, m.enrichStatuses["CANCELED"],
		"the cancel transition must enrich overdraft companions: it restores the capacity the hold consumed")

	assert.Less(t, m.enrichPos, m.validatePos,
		"enrichment must precede ValidateAccountingRules so the companion is subject to the route's overdraft rubric")
	assert.True(t, m.companionsReachFromTo,
		"the companion FromTo entries must be appended into fromTo so BuildOperations persists the overdraft operation")
}

// TestCommitCancel_OverdraftEnrichmentCoversBothTransitions_Bites proves the
// analyzer bites on a seam that enriches on cancel only, validates before
// enriching, or never threads the companions into fromTo.
func TestCommitCancel_OverdraftEnrichmentCoversBothTransitions_Bites(t *testing.T) {
	leaky := `package command
func (uc *UseCase) preparePendingTransition() error {
	routeCache, _ := uc.TransactionReader.ValidateAccountingRules(ctx, balanceOps, validate, action)
	var companionFromTos []mtransaction.FromTo
	if run.status == constant.CANCELED { // BUG: commit is not enriched
		balanceOps, companionFromTos, _ = enrichOverdraftOperations(readCtx, balanceOps, validate)
	}
	_, _ = routeCache, companionFromTos
	return nil
}

func (uc *UseCase) finalizePendingTransition() error {
	return nil // BUG: the companions never reach fromTo
}`

	m := analyzeCommitCancelOverdraftSeam(t, leaky, leaky)

	require.NotEqual(t, -1, m.enrichPos, "fixture sanity: the guarded enrichment must be present")

	assert.False(t, m.enrichStatuses["APPROVED"],
		"gate failed to bite: a cancel-only guard was reported as covering the commit")
	assert.True(t, m.enrichStatuses["CANCELED"], "fixture sanity: the cancel status must be detected")
	assert.Greater(t, m.enrichPos, m.validatePos,
		"fixture sanity: this fixture validates before enriching")
	assert.False(t, m.companionsReachFromTo,
		"gate failed to bite: companions never appended into fromTo were reported as reaching it")

	correct := `package command
func (uc *UseCase) preparePendingTransition() error {
	var companionFromTos []mtransaction.FromTo
	if run.status == constant.APPROVED || run.status == constant.CANCELED {
		balanceOps, companionFromTos, _ = enrichOverdraftOperations(readCtx, balanceOps, validate)
	}
	routeCache, _ := uc.TransactionReader.ValidateAccountingRules(ctx, balanceOps, validate, action)
	_, _ = routeCache, companionFromTos
	return nil
}

func (uc *UseCase) finalizePendingTransition() error {
	run.fromTo = append(run.fromTo, run.companionFromTos...)
	return nil
}`

	mc := analyzeCommitCancelOverdraftSeam(t, correct, correct)
	assert.True(t, mc.enrichStatuses["APPROVED"] && mc.enrichStatuses["CANCELED"] && mc.companionsReachFromTo,
		"fixture sanity: the correct shape must satisfy every fact")
	assert.Less(t, mc.enrichPos, mc.validatePos, "fixture sanity: enrichment precedes validation")
}
