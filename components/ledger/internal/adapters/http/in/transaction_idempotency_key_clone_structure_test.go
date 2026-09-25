// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// Idempotency key ownership gate. The X-Idempotency string every create route hands to
// executeCreateTransaction is a view over fasthttp's request buffer, and the key outlives the
// request in the asynchronous idempotency value write. executeCreateTransaction must replace
// it with its own copy before the idempotency claim; otherwise a later request on the same
// connection rewrites the key, and this request's transaction is stored under that request's
// key and replayed to it.
//
// The behavior is proven by TestIntegration_TransactionCreate_IdempotencyKeySurvivesRequestBufferReuse,
// which needs Docker and does not run in the unit suite. This gate keeps the copy in place
// there: a clone that is removed, or moved below the claim, still passes every unit test.

// cloneAndClaimPositions returns the source positions, inside funcName, of the first
// `idempotencyKey = strings.Clone(idempotencyKey)` assignment and of the first
// CreateOrCheckTransactionIdempotency call. A missing element yields token.NoPos.
func cloneAndClaimPositions(t *testing.T, src, funcName string) (clonePos, claimPos token.Pos, found bool) {
	t.Helper()

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "src.go", src, 0)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName || fn.Body == nil {
			continue
		}

		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				if clonePos == token.NoPos && isIdempotencyKeySelfClone(node) {
					clonePos = node.Pos()
				}
			case *ast.CallExpr:
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "CreateOrCheckTransactionIdempotency" && claimPos == token.NoPos {
					claimPos = node.Pos()
				}
			}

			return true
		})

		return clonePos, claimPos, true
	}

	return token.NoPos, token.NoPos, false
}

// isIdempotencyKeySelfClone matches `idempotencyKey = strings.Clone(idempotencyKey)`.
func isIdempotencyKeySelfClone(stmt *ast.AssignStmt) bool {
	if stmt.Tok != token.ASSIGN || len(stmt.Lhs) != 1 || len(stmt.Rhs) != 1 {
		return false
	}

	lhs, ok := stmt.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != "idempotencyKey" {
		return false
	}

	call, ok := stmt.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}

	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Clone" {
		return false
	}

	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "strings" {
		return false
	}

	arg, ok := call.Args[0].(*ast.Ident)

	return ok && arg.Name == "idempotencyKey"
}

func TestIdempotencyKeyStructure_CreateFunnelClonesKeyBeforeClaim(t *testing.T) {
	src := readTransportSource(t, "transaction_create.go", "executeCreateTransaction")

	clonePos, claimPos, found := cloneAndClaimPositions(t, src, "executeCreateTransaction")
	if !found {
		t.Fatal("executeCreateTransaction not found in transaction_create.go — the gate is pointed at a renamed funnel")
	}

	if claimPos == token.NoPos {
		t.Fatal("executeCreateTransaction makes no CreateOrCheckTransactionIdempotency call — the gate no longer sees the idempotency claim")
	}

	if clonePos == token.NoPos {
		t.Fatal("executeCreateTransaction does not assign idempotencyKey = strings.Clone(idempotencyKey) — the header view would reach the asynchronous idempotency value write")
	}

	if clonePos > claimPos {
		t.Error("idempotencyKey is cloned after the idempotency claim — the key must be copied before any use, so the claim and the value write name the same slot")
	}
}
