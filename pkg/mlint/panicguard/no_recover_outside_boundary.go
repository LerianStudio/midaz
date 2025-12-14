package panicguard

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NoRecoverOutsideBoundaryAnalyzer restricts recover() calls to boundary packages
// (HTTP handlers, gRPC interceptors, message queue consumers) and pkg/mruntime/.
var NoRecoverOutsideBoundaryAnalyzer = &analysis.Analyzer{
	Name:     "norecoveroutsideboundary",
	Doc:      "restricts recover() to boundary packages only",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runNoRecoverOutsideBoundary,
}

func runNoRecoverOutsideBoundary(pass *analysis.Pass) (interface{}, error) {
	// Build exclusion matcher - test files and boundary packages
	patterns := append([]string{}, CommonExclusions...)
	patterns = append(patterns, BoundaryPackageExclusions...)
	matcher := NewPathMatcher(patterns)

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// Check if this is a recover() call
		if !isRecoverCall(pass, call) {
			return
		}

		// Get file position for exclusion check
		pos := pass.Fset.Position(call.Pos())
		if matcher.ShouldExclude(pos.Filename) {
			return
		}

		pass.Reportf(call.Pos(),
			"recover() is only allowed in boundary packages (HTTP/gRPC/RabbitMQ adapters, "+
				"bootstrap workers) and pkg/mruntime/. For goroutine protection, use "+
				"mruntime.SafeGo() which handles recovery automatically. "+
				"If you need panic recovery in a new boundary, discuss with the team first.")
	})

	return nil, nil
}

// isRecoverCall checks if the call expression is a call to the built-in recover().
func isRecoverCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}

	if ident.Name != "recover" {
		return false
	}

	if pass.TypesInfo == nil {
		return true
	}

	if b, ok := pass.TypesInfo.Uses[ident].(*types.Builtin); ok {
		return b.Name() == "recover"
	}

	return false
}
