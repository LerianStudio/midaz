package panicguard

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// NoRawGoroutineAnalyzer detects raw 'go' statements and requires the use of
// mruntime.SafeGo() or mruntime.SafeGoWithContext() for proper panic recovery.
var NoRawGoroutineAnalyzer = &analysis.Analyzer{
	Name:     "norawgoroutine",
	Doc:      "detects raw 'go' statements that should use mruntime.SafeGo()",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      runNoRawGoroutine,
}

func runNoRawGoroutine(pass *analysis.Pass) (any, error) {
	// Build exclusion matcher
	patterns := append([]string{}, CommonExclusions...)
	patterns = append(patterns, MRuntimeExclusions...)
	matcher := NewPathMatcher(patterns)

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	nodeFilter := []ast.Node{
		(*ast.GoStmt)(nil),
	}

	insp.Preorder(nodeFilter, func(n ast.Node) {
		goStmt := n.(*ast.GoStmt)

		// Get file position for exclusion check
		pos := pass.Fset.Position(goStmt.Pos())
		if matcher.ShouldExclude(pos.Filename) {
			return
		}

		pass.Reportf(goStmt.Pos(),
			"raw goroutine detected; use mruntime.SafeGo() or mruntime.SafeGoWithContext() "+
				"instead of raw 'go' statements to ensure panic recovery. "+
				"See pkg/mruntime/ for documentation.")
	})

	return nil, nil
}
