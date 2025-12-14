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

func runNoBareRecover(pass *analysis.Pass) (interface{}, error) {
	// Build exclusion matcher - only test files excluded
	matcher := NewPathMatcher(CommonExclusions)

	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	// We need stack context to distinguish:
	//   - allowed: r := recover() / r = recover() (incl. if-init)
	//   - disallowed: recover() (including defer recover(), _ = recover(), and recover() used in expressions)
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

		var parent ast.Node
		if len(stack) >= 2 {
			parent = stack[len(stack)-2]
		}

		var grandparent ast.Node
		if len(stack) >= 3 {
			grandparent = stack[len(stack)-3]
		}

		switch p := parent.(type) {
		case *ast.AssignStmt:
			// Allow any assignment to a non-blank identifier (Semgrep parity).
			// Flag blank identifier discard: _ = recover().
			for i, rhs := range p.Rhs {
				if rhs != call {
					continue
				}

				if i < len(p.Lhs) {
					if ident, ok := p.Lhs[i].(*ast.Ident); ok && ident.Name == "_" {
						pass.Reportf(call.Pos(),
							"recover() result is discarded with blank identifier. "+
								"The panic value must be captured and logged. "+
								"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) }")
						return true
					}
				}
				// Allow r := recover() / r = recover()
				return true
			}

			return true

		case *ast.IfStmt:
			// If the call is part of the if init (e.g., if r := recover(); r != nil { ... })
			// it will usually be under an AssignStmt, not directly under IfStmt.
			_ = p

		case *ast.DeferStmt:
			pass.Reportf(call.Pos(),
				"recover() call should capture and log the panic value. "+
					"Silently swallowing panics makes debugging impossible. "+
					"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) } "+
					"Or use mruntime.RecoverAndLog(logger, \"name\").")
			return true

		case *ast.ExprStmt:
			pass.Reportf(call.Pos(),
				"recover() call should capture and log the panic value. "+
					"Silently swallowing panics makes debugging impossible. "+
					"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) } "+
					"Or use mruntime.RecoverAndLog(logger, \"name\").")
			return true
		}

		// If this recover() is in an if-init assign, allow it.
		if gpIf, ok := grandparent.(*ast.IfStmt); ok {
			if as, ok := parent.(*ast.AssignStmt); ok && gpIf.Init == as {
				return true
			}
		}

		// Any other usage (e.g., fmt.Println(recover())) is treated as "bare".
		pass.Reportf(call.Pos(),
			"recover() call should capture and log the panic value. "+
				"Silently swallowing panics makes debugging impossible. "+
				"Use: if r := recover(); r != nil { logger.Errorf(\"panic: %%v\", r) } "+
				"Or use mruntime.RecoverAndLog(logger, \"name\").")

		return true
	})

	return nil, nil
}
