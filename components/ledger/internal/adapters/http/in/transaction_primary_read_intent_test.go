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
)

// TestCommitCancel_PrimaryReadWrapPlacement is the placement half of the
// commit/cancel primary-read contract: a structural guard over the live source of
// commitOrCancelTransaction proving the dedicated-var wrap
// `readCtx := readrouting.WithPrimaryRead(ctx)` exists AND sits AFTER the
// validation-only reads (GetParsedLedgerSettings) and BEFORE the GetBalances read;
// that GetBalances and the cancel overdraft read receive readCtx while
// ValidateAccountingRules keeps the unmarked ctx, so only the pre-write balance reads
// are routed to primary. The mechanism half lives beside the reads it exercises, in
// the command package.
func TestCommitCancel_PrimaryReadWrapPlacement(t *testing.T) {
	src := readStateHandlerSource(t)

	positions := analyzeCommitCancelWrap(t, src, "commitOrCancelTransaction")

	if positions.wrap == -1 {
		t.Fatal("no dedicated `readCtx := readrouting.WithPrimaryRead(ctx)` wrap found in commitOrCancelTransaction; the commit/cancel flow must mark the primary-read intent on a dedicated ctx var, not reassign ctx")
	}

	if positions.getBalances == -1 {
		t.Fatal("no GetBalances call found in commitOrCancelTransaction; the read call site moved")
	}

	if positions.getLedgerSettings == -1 {
		t.Fatal("no GetParsedLedgerSettings call found in commitOrCancelTransaction; the validation-only read moved")
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
		t.Error("the cancel EnrichOverdraftOperations read must receive the dedicated readCtx so its internal GetBalances is routed to primary")
	}

	if positions.validateTakesReadCtx {
		t.Error("ValidateAccountingRules must NOT receive the dedicated readCtx: it is validation-only (route-cache) and deliberately keeps the unmarked ctx")
	}
}

// commitCancelWrapPositions holds the top-level statement indices of the marker
// wrap and the reads it must sit between within commitOrCancelTransaction, plus
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
// call (each -1 when absent), and whether GetBalances, EnrichOverdraftOperations,
// and ValidateAccountingRules receive `readCtx` as their context argument.
// Statement indices are sufficient because the reads live at the top level of the
// sequential commit/cancel flow.
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

		if callFirstArgIsIdent(stmt, "EnrichOverdraftOperations", "readCtx") {
			positions.enrichTakesReadCtx = true
		}

		if callFirstArgIsIdent(stmt, "ValidateAccountingRules", "readCtx") {
			positions.validateTakesReadCtx = true
		}
	}

	return positions
}

// stmtDefinesReadCtxFromPrimaryRead reports whether stmt is a short-var
// definition `readCtx := readrouting.WithPrimaryRead(...)` — the dedicated-var
// form that scopes the primary-read marker without reassigning ctx.
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

	return ok && pkg.Name == "readrouting"
}

// callFirstArgIsIdent reports whether stmt contains a call to callee whose first
// argument is exactly the identifier argName. callee matches either a method call
// (`x.callee(...)`) or a plain function call (`callee(...)`), so both
// `handler.Query.GetBalances(readCtx, ...)` and `command.EnrichOverdraftOperations(readCtx, ...)`
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

// readStateHandlerSource reads transaction_state_handlers.go from disk so the
// placement guard runs against the live source, not a snapshot, and fails the
// moment the commit/cancel seam is edited.
func readStateHandlerSource(t *testing.T) string {
	t.Helper()

	const path = "transaction_state_handlers.go"

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	src := string(data)
	if !strings.Contains(src, "func (handler *TransactionHandler) commitOrCancelTransaction") {
		t.Fatalf("%s does not contain commitOrCancelTransaction — the gate is pointed at the wrong file", path)
	}

	return src
}

// stmtCallsMethod reports whether the statement contains a selector call whose
// method name matches (e.g. handler.Query.GetBalances(...)).
func stmtCallsMethod(stmt ast.Stmt, method string) bool {
	found := false

	ast.Inspect(stmt, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
			found = true
		}

		return true
	})

	return found
}
