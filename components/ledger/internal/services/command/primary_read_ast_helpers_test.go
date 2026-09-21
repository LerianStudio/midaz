// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/token"
)

func stmtDefinesReadCtxFromPrimaryRead(stmt ast.Stmt) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != "readCtx" {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "WithPrimaryRead" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "readrouting" || len(call.Args) != 1 {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	return ok && arg.Name == "ctx"
}

func callFirstArgIsIdent(stmt ast.Stmt, callee, argName string) bool {
	found := false
	ast.Inspect(stmt, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		var name string
		switch function := call.Fun.(type) {
		case *ast.SelectorExpr:
			name = function.Sel.Name
		case *ast.Ident:
			name = function.Name
		default:
			return true
		}
		if name != callee {
			return true
		}
		if argument, ok := call.Args[0].(*ast.Ident); ok && argument.Name == argName {
			found = true
		}
		return true
	})
	return found
}
