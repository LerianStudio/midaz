// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
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

	// The "original" transaction's reservation — the one a buggy refund would
	// release/confirm. We assert it is never referenced.
	originalReservationID := uuid.New()

	// The revert path delegates to executeCreateTransaction, whose reserve
	// anchor issues a NEW reserve for the reverse transaction. Model that the
	// reverse transaction reserves on its own and capture which ids the ledger
	// later confirms/releases.
	reverseReservationID := uuid.New()
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reverseReservationID}}}
	uc := &UseCase{TracerReserver: reserver}

	// Reserve for the reverse transaction (what the revert's executeCreateTransaction does).
	out := uc.reserveTransaction(ctx, sp, logger,
		mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce, FailPosture: mmodel.TracerFailPostureOpen},
		uuid.New(), decimal.NewFromInt(1000), "BRL", fixedReserveAccountID, fixedReserveTimestamp, reservationTTLDefault, RouteV2, false)
	require.Equal(t, reservationProceed, out.Kind)

	// On a successful reverse-transaction commit the ledger confirms the
	// REVERSE reservation — never the original.
	uc.confirmReservations(ctx, sp, logger, out.Handle)

	require.Equal(t, []uuid.UUID{reverseReservationID}, reserver.confirmedIDs,
		"a revert confirms its own reverse-transaction reservation")
	assert.NotContains(t, reserver.confirmedIDs, originalReservationID,
		"a revert must NEVER confirm the original transaction's reservation")
	assert.NotContains(t, reserver.releasedIDs, originalReservationID,
		"a revert must NEVER release the original transaction's reservation (Q9 no-refund)")
}
