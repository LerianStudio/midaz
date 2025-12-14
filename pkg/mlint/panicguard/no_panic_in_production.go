package panicguard

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NoPanicInProductionAnalyzer warns against panic() calls in production code.
// Panics should be replaced with proper error handling.
var NoPanicInProductionAnalyzer = &analysis.Analyzer{
	Name:     "nopanicinproduction",
	Doc:      "warns against panic() calls in production code",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runNoPanicInProduction,
}

func runNoPanicInProduction(pass *analysis.Pass) (interface{}, error) {
	// Build exclusion matcher
	patterns := append([]string{}, CommonExclusions...)
	patterns = append(patterns, PanicAllowedExclusions...)
	matcher := NewPathMatcher(patterns)

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.CallExpr)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		call := n.(*ast.CallExpr)

		// Check if this is a panic() call
		if !isPanicCall(pass, call) {
			return
		}

		// Get file position for exclusion check
		pos := pass.Fset.Position(call.Pos())
		if matcher.ShouldExclude(pos.Filename) {
			return
		}

		pass.Reportf(call.Pos(),
			"panic() should not be used in production code. Return an error instead. "+
				"If this is a truly unrecoverable situation, document why and request "+
				"an exception in code review.")
	})

	return nil, nil
}

// isPanicCall checks if the call expression is a call to the built-in panic().
func isPanicCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}

	if ident.Name != "panic" {
		return false
	}

	if pass.TypesInfo == nil {
		return true
	}

	if b, ok := pass.TypesInfo.Uses[ident].(*types.Builtin); ok {
		return b.Name() == "panic"
	}

	return false
}
