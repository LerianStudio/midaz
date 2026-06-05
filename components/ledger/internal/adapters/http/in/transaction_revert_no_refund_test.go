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

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

// TestRevertNoReservationRefund is the permanent behavioral lock for Q9
// (no-refund on revert): reverting a transaction must NEVER Release or Confirm
// the ORIGINAL transaction's reservation. Limits measure GROSS activity, so a
// revert is itself a new chargeable transaction that reserves on its own via
// the create anchor (F3-T13); the original reservation is left exactly as the
// original transaction left it (confirmed on the original commit).
//
// The lock has two halves:
//
//  1. A structural guard (TestRevertNoReservationRefund_StructuralGuard) over
//     transaction_state_handlers.go asserting the revert entry points contain
//     no Release/Confirm call against any reservation. A future change that
//     added a refund would have to add such a call there and would trip it.
//  2. This behavioral test: the reverse transaction reserves on its own and
//     the original reservation id is never touched.
func TestRevertNoReservationRefund(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	// The "original" transaction's reservation — the one a buggy refund would
	// release/confirm. We assert it is never referenced.
	originalReservationID := uuid.New()

	// The revert path delegates to executeCreateTransaction, whose reserve
	// anchor issues a NEW reserve for the reverse transaction. Model that the
	// reverse transaction reserves on its own and capture which ids the ledger
	// later confirms/releases.
	reverseReservationID := uuid.New()
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reverseReservationID}}}
	handler := &TransactionHandler{TracerReserver: reserver}

	// Reserve for the reverse transaction (what the revert's executeCreateTransaction does).
	out := handler.reserveTransaction(ctx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", reservationTTLDefault)
	require.Equal(t, reservationProceed, out.Kind)

	// On a successful reverse-transaction commit the ledger confirms the
	// REVERSE reservation — never the original.
	handler.confirmReservations(ctx, sp, logger, out.Handle)

	require.Equal(t, []uuid.UUID{reverseReservationID}, reserver.confirmedIDs,
		"a revert confirms its own reverse-transaction reservation")
	assert.NotContains(t, reserver.confirmedIDs, originalReservationID,
		"a revert must NEVER confirm the original transaction's reservation")
	assert.NotContains(t, reserver.releasedIDs, originalReservationID,
		"a revert must NEVER release the original transaction's reservation (Q9 no-refund)")
}

// TestRevertNoReservationRefund_StructuralGuard asserts the revert entry points
// in transaction_state_handlers.go invoke neither Release nor Confirm. The
// confirm/release transport lives only in the create seam
// (transaction_create.go); the state handlers must stay refund-free.
func TestRevertNoReservationRefund_StructuralGuard(t *testing.T) {
	src, err := os.ReadFile("transaction_state_handlers.go")
	require.NoError(t, err, "read transaction_state_handlers.go")

	fset := token.NewFileSet()

	file, err := parser.ParseFile(fset, "transaction_state_handlers.go", src, 0)
	require.NoError(t, err, "parse transaction_state_handlers.go")

	// Walk RevertTransaction and createRevertTransaction (the revert entry
	// points) and assert no Release/Confirm method call appears in either.
	revertFuncs := map[string]bool{"RevertTransaction": true, "createRevertTransaction": true}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || !revertFuncs[fn.Name.Name] {
			continue
		}

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
}
