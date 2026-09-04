// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestCommitCancel_PrimaryReadWrapPlacement is the placement half of the
// commit/cancel primary-read contract: a structural guard over the live source of
// preparePendingTransition proving the dedicated-var wrap
// `readCtx := readrouting.WithPrimaryRead(ctx)` exists AND sits AFTER the
// validation-only reads (GetParsedLedgerSettings) and BEFORE the GetBalances read;
// that GetBalances and the cancel overdraft read receive readCtx while
// ValidateAccountingRules keeps the unmarked ctx, so only the pre-write balance reads
// are routed to primary. The mechanism half lives beside the reads it exercises.
func TestCommitCancel_PrimaryReadWrapPlacement(t *testing.T) {
	src := readTransportSource(t, pendingStepsFile, "func (uc *UseCase) "+pendingPrepareFuncName)

	positions := analyzeCommitCancelWrap(t, src, pendingPrepareFuncName)

	if positions.wrap == -1 {
		t.Fatal("no dedicated `readCtx := readrouting.WithPrimaryRead(ctx)` wrap found in preparePendingTransition; the commit/cancel flow must mark the primary-read intent on a dedicated ctx var, not reassign ctx")
	}

	if positions.getBalances == -1 {
		t.Fatal("no GetBalances call found in preparePendingTransition; the read call site moved")
	}

	if positions.getLedgerSettings == -1 {
		t.Fatal("no GetParsedLedgerSettings call found in preparePendingTransition; the validation-only read moved")
	}

	if positions.wrap >= positions.getBalances {
		t.Errorf("the readCtx wrap (stmt %d) must precede the GetBalances read (stmt %d) so both pre-write balance reads observe the mark", positions.wrap, positions.getBalances)
	}

	if positions.wrap <= positions.getLedgerSettings {
		t.Errorf("the readCtx wrap (stmt %d) must FOLLOW the validation-only GetParsedLedgerSettings read (stmt %d) so validation-only reads are NOT marked", positions.wrap, positions.getLedgerSettings)
	}

	// Arg identity: the marker must be scoped to the pre-write balance reads.
	if !positions.getBalancesTakesReadCtx {
		t.Error("the GetBalances read must receive the dedicated readCtx (the primary-read marker); passing the unmarked ctx would stop routing the balance seed to primary")
	}

	if !positions.enrichTakesReadCtx {
		t.Error("the cancel enrichOverdraftOperations read must receive the dedicated readCtx so its internal GetBalances is routed to primary")
	}

	if positions.validateTakesReadCtx {
		t.Error("ValidateAccountingRules must NOT receive the dedicated readCtx: it is validation-only (route-cache) and deliberately keeps the unmarked ctx")
	}
}

// commitCancelWrapPositions holds the top-level statement indices of the marker
// wrap and the reads it must sit between within the preparation step, plus
// the arg-identity checks that scope the marker to the pre-write balance reads.
type commitCancelWrapPositions struct {
	wrap              int
	getBalances       int
	getLedgerSettings int

	getBalancesTakesReadCtx bool
	enrichTakesReadCtx      bool
	validateTakesReadCtx    bool
}

// analyzeCommitCancelWrap returns, within the named function body, the top-level
// statement indices of the dedicated `readCtx := readrouting.WithPrimaryRead(...)`
// wrap, the first GetBalances call, and the validation-only GetParsedLedgerSettings
// call (each -1 when absent), and whether GetBalances, enrichOverdraftOperations,
// and ValidateAccountingRules receive `readCtx` as their context argument.
// Statement indices are sufficient because the reads live at the top level of the
// sequential preparation flow.
func analyzeCommitCancelWrap(t *testing.T, src, funcName string) commitCancelWrapPositions {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	var fn *ast.FuncDecl

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.FuncDecl); ok && d.Name.Name == funcName {
			fn = d
			break
		}
	}

	if fn == nil || fn.Body == nil {
		t.Fatalf("function %q not found or has no body", funcName)
	}

	positions := commitCancelWrapPositions{wrap: -1, getBalances: -1, getLedgerSettings: -1}

	for i, stmt := range fn.Body.List {
		if positions.wrap == -1 && stmtDefinesReadCtxFromPrimaryRead(stmt) {
			positions.wrap = i
		}

		if positions.getBalances == -1 && stmtCallsMethod(stmt, "GetBalances") {
			positions.getBalances = i
		}

		if positions.getLedgerSettings == -1 && stmtCallsMethod(stmt, "GetParsedLedgerSettings") {
			positions.getLedgerSettings = i
		}

		if callFirstArgIsIdent(stmt, "GetBalances", "readCtx") {
			positions.getBalancesTakesReadCtx = true
		}

		if callFirstArgIsIdent(stmt, "enrichOverdraftOperations", "readCtx") {
			positions.enrichTakesReadCtx = true
		}

		if callFirstArgIsIdent(stmt, "ValidateAccountingRules", "readCtx") {
			positions.validateTakesReadCtx = true
		}
	}

	return positions
}

// stmtDefinesReadCtxFromPrimaryRead reports whether stmt is a short-var
// definition `readCtx := readrouting.WithPrimaryRead(ctx)` — the dedicated-var
// form that scopes the primary-read marker without reassigning ctx. The lone
// `ctx` argument is part of the shape: a wrap over an already-derived context
// would not establish the marker over the function's unmarked ctx.
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

	if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "readrouting" {
		return false
	}

	if len(call.Args) != 1 {
		return false
	}

	arg, ok := call.Args[0].(*ast.Ident)

	return ok && arg.Name == "ctx"
}

// TestStmtDefinesReadCtxFromPrimaryRead_RequiresUnmarkedCtx proves the argument
// shape above bites: only a single-argument wrap over the unmarked ctx counts as
// the dedicated-var marker.
func TestStmtDefinesReadCtxFromPrimaryRead_RequiresUnmarkedCtx(t *testing.T) {
	tests := []struct {
		name string
		stmt string
		want bool
	}{
		{name: "wraps ctx", stmt: "readCtx := readrouting.WithPrimaryRead(ctx)", want: true},
		{name: "wraps a derived ctx", stmt: "readCtx := readrouting.WithPrimaryRead(readCtx)", want: false},
		{name: "no argument", stmt: "readCtx := readrouting.WithPrimaryRead()", want: false},
		{name: "extra argument", stmt: "readCtx := readrouting.WithPrimaryRead(ctx, true)", want: false},
		{name: "other package", stmt: "readCtx := routing.WithPrimaryRead(ctx)", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn := findFuncDecl(t, "package p\n\nfunc f() {\n\t"+tc.stmt+"\n}\n", "f")

			if got := stmtDefinesReadCtxFromPrimaryRead(fn.Body.List[0]); got != tc.want {
				t.Errorf("stmtDefinesReadCtxFromPrimaryRead(%q) = %v, want %v", tc.stmt, got, tc.want)
			}
		})
	}
}

// callFirstArgIsIdent reports whether stmt contains a call to callee whose first
// argument is exactly the identifier argName. callee matches either a method call
// (`x.callee(...)`) or a plain function call (`callee(...)`), so both
// `uc.TransactionReader.GetBalances(readCtx, ...)` and `enrichOverdraftOperations(readCtx, ...)`
// are recognized — used to assert which reads receive the dedicated readCtx versus
// the unmarked ctx.
func callFirstArgIsIdent(stmt ast.Stmt, callee, argName string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}

		var name string

		switch fun := call.Fun.(type) {
		case *ast.SelectorExpr:
			name = fun.Sel.Name
		case *ast.Ident:
			name = fun.Name
		default:
			return true
		}

		if name != callee {
			return true
		}

		if id, ok := call.Args[0].(*ast.Ident); ok && id.Name == argName {
			found = true
		}

		return true
	})

	return found
}
