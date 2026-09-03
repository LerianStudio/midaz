// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRevertNoReservationRefund_StructuralGuard asserts the revert core in
// transaction_state_handlers.go invokes neither Release nor Confirm. The
// confirm/release transport lives only in the create seam; the revert core must stay
// refund-free, because limits measure GROSS activity: a revert reserves on its own and
// never refunds the original transaction's reservation (Q9 no-refund). The behavioral
// half of the lock lives beside the reserve anchor, in the command package.
func TestRevertNoReservationRefund_StructuralGuard(t *testing.T) {
	src, err := os.ReadFile("transaction_state_handlers.go")
	require.NoError(t, err, "read transaction_state_handlers.go")

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "transaction_state_handlers.go", src, 0)
	require.NoError(t, err, "parse transaction_state_handlers.go")

	// Walk the revert core and assert no Release/Confirm method call appears in it.
	revertFuncs := map[string]bool{"revertTransaction": true}
	seen := false

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !revertFuncs[fn.Name.Name] {
			continue
		}

		seen = true

		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			if sel.Sel.Name == "Release" || sel.Sel.Name == "Confirm" {
				t.Errorf("revert function %q calls %q — a revert must not refund the original reservation (Q9 no-refund)",
					fn.Name.Name, sel.Sel.Name)
			}

			return true
		})
	}

	require.True(t, seen, "revertTransaction not found in transaction_state_handlers.go — the guard is pointed at a renamed core")
}
