package panicguard

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NoBareRecoverAnalyzer ensures recover() calls properly capture and log the
// panic value. Silently swallowing panics makes debugging impossible.
var NoBareRecoverAnalyzer = &analysis.Analyzer{
	Name:     "nobarerecover",
	Doc:      "ensures recover() calls capture and log the panic value",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runNoBareRecover,
}

// Stack depth constants for AST traversal.
const (
	minStackDepthForParent      = 2
	minStackDepthForGrandparent = 3
)

// recoverCheckResult represents the outcome of checking a recover() call.
type recoverCheckResult int

const (
	// resultContinue means continue to next checks.
	resultContinue recoverCheckResult = iota
	// resultAllowed means the recover() usage is allowed.
	resultAllowed
	// resultReported means a diagnostic was already reported.
	resultReported
)

// bareRecoverMessage is the standard diagnostic message for bare recover() calls.
const bareRecoverMessage = "recover() call should capture and log the panic value. " +
	"Silently swallowing panics makes debugging impossible. " +
	"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) } " +
	"Or use mruntime.RecoverAndLog(logger, \"name\")."

// blankRecoverMessage is the diagnostic message for discarded recover() results.
const blankRecoverMessage = "recover() result is discarded with blank identifier. " +
	"The panic value must be captured and logged. " +
	"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) }"

// checkAssignStmt checks if a recover() in an AssignStmt is allowed or should be flagged.
func checkAssignStmt(pass *analysis.Pass, call *ast.CallExpr, assign *ast.AssignStmt) recoverCheckResult {
	for i, rhs := range assign.Rhs {
		if rhs != call {
			continue
		}

		if i < len(assign.Lhs) {
			if ident, ok := assign.Lhs[i].(*ast.Ident); ok && ident.Name == "_" {
				pass.Reportf(call.Pos(), blankRecoverMessage)
				return resultReported
			}
		}
		// Allow r := recover() / r = recover()
		return resultAllowed
	}

	return resultAllowed
}

// checkParentNode checks the parent node type and returns the appropriate result.
func checkParentNode(pass *analysis.Pass, call *ast.CallExpr, parent ast.Node) recoverCheckResult {
	switch p := parent.(type) {
	case *ast.AssignStmt:
		return checkAssignStmt(pass, call, p)

	case *ast.IfStmt:
		// If the call is part of the if init (e.g., if r := recover(); r != nil { ... })
		// it will usually be under an AssignStmt, not directly under IfStmt.
		return resultContinue

	case *ast.DeferStmt, *ast.ExprStmt:
		pass.Reportf(call.Pos(), bareRecoverMessage)
		return resultReported
	}

	return resultContinue
}

// isIfInitAssignment checks if the recover() is in an if-init assignment.
func isIfInitAssignment(parent, grandparent ast.Node) bool {
	gpIf, ok := grandparent.(*ast.IfStmt)
	if !ok {
		return false
	}

	as, ok := parent.(*ast.AssignStmt)

	return ok && gpIf.Init == as
}

// runNoBareRecover inspects recover() calls to ensure they capture and log the panic value.
// The cognitive complexity is inherent to distinguishing between allowed patterns
// (r := recover()) and disallowed patterns (bare recover(), _ = recover()).
func runNoBareRecover(pass *analysis.Pass) (any, error) {
	matcher := NewPathMatcher(CommonExclusions)
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	nodeFilter := []ast.Node{(*ast.CallExpr)(nil)}

	insp.WithStack(nodeFilter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}

		call := n.(*ast.CallExpr)
		if !isRecoverCall(pass, call) {
			return true
		}

		pos := pass.Fset.Position(call.Pos())
		if matcher.ShouldExclude(pos.Filename) {
			return true
		}

		var parent, grandparent ast.Node
		if len(stack) >= minStackDepthForParent {
			parent = stack[len(stack)-minStackDepthForParent]
		}

		if len(stack) >= minStackDepthForGrandparent {
			grandparent = stack[len(stack)-minStackDepthForGrandparent]
		}

		result := checkParentNode(pass, call, parent)
		if result != resultContinue {
			return true
		}

		if isIfInitAssignment(parent, grandparent) {
			return true
		}

		// Any other usage (e.g., fmt.Println(recover())) is treated as "bare".
		pass.Reportf(call.Pos(), bareRecoverMessage)

		return true
	})

	return nil, nil
}
