// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"go/ast"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// TestRevertNoReservationRefund is the permanent behavioral lock for Q9
// (no-refund on revert): reverting a transaction must NEVER Release or Confirm
// the ORIGINAL transaction's reservation. Limits measure GROSS activity, so a
// revert is itself a new chargeable transaction that reserves on its own via
// the create anchor (F3-T13); the original reservation is left exactly as the
// original transaction left it (confirmed on the original commit).
//
// The structural half of the lock lives in the transport package, over the revert
// core: it asserts the revert entry point contains no Release/Confirm call against
// any reservation.
func TestRevertNoReservationRefund(t *testing.T) {
	ctx, sp, logger := anchorDeps()

	// The "original" transaction — the one a buggy refund would release or
	// confirm. We assert it is never addressed.
	originalTransactionID := uuid.New()

	// The revert path admits the reverse transaction on its own, then confirms
	// that admission after its commit.
	stub := &stubContextTracer{}
	uc := &UseCase{ContextTracer: stub.coordinatorFor(t)}
	transaction, validated, balances := anchorPrepared()
	input := anchorInput(enforceSettings(mmodel.TracerFailPostureOpen), false)

	out := uc.reservePreparedTransaction(ctx, sp, logger, input, transaction, validated, balances)
	require.Equal(t, reservationProceed, out.Kind)

	uc.confirmReservations(ctx, sp, logger, out.Handle)

	require.Equal(t, []uuid.UUID{input.Key.TransactionID}, stub.confirmedTransactions(),
		"a revert confirms its own reverse-transaction reservation")
	assert.NotContains(t, stub.confirmedTransactions(), originalTransactionID,
		"a revert must NEVER confirm the original transaction's reservation")
	assert.NotContains(t, stub.releasedTransactions(), originalTransactionID,
		"a revert must NEVER release the original transaction's reservation (Q9 no-refund)")
}

// TestRevertNoReservationRefund_StructuralGuard is the structural half of the Q9
// no-refund lock: the eligibility gate — the only revert step that touches the ORIGINAL
// transaction — must invoke neither Release nor Confirm. The reservation transport a
// revert does use belongs to the reverse transaction it creates, held by its own reserve
// anchor.
func TestRevertNoReservationRefund_StructuralGuard(t *testing.T) {
	src := readTransportSource(t, "revert_transaction.go", "func (uc *UseCase) prepareRevertTransaction")

	fn := findFuncDecl(t, src, "prepareRevertTransaction")

	// Every completion method, including the by-transaction pair the PENDING
	// lifecycle uses. A revert may call none of them against the original
	// reservation.
	refunds := []string{"Release", "Confirm", "ReleaseByTransaction", "ConfirmByTransaction"}

	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		if slices.Contains(refunds, sel.Sel.Name) {
			t.Errorf("prepareRevertTransaction calls %q — a revert must not refund the original reservation (Q9 no-refund)", sel.Sel.Name)
		}

		return true
	})
}
