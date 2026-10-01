// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestAtomicTransactionBatchReservations_KnownSuccessLeavesPendingItemsToCommitOrCancel(t *testing.T) {
	t.Parallel()

	createdReservationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000201")
	pendingReservationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000202")
	fake := &atomicTransactionBatchTracerFake{results: []*tracer.ReserveResult{
		{ReservationIDs: []uuid.UUID{createdReservationID}},
		{ReservationIDs: []uuid.UUID{pendingReservationID}},
	}}
	run := atomicTransactionBatchTracerTestRun(2)
	run.items[1].status = constant.PENDING
	uc := &UseCase{TracerReserver: fake}
	ctx, span, logger := anchorDeps()

	require.NoError(t, uc.reserveAtomicTransactionBatch(ctx, span, logger, run))

	uc.settleAtomicTransactionBatchReservations(ctx, span, logger, run, atomicTransactionBatchReservationKnownSuccess)

	_, confirmed, released := fake.snapshot()
	assert.Equal(t, []uuid.UUID{createdReservationID}, confirmed,
		"a CREATED item is confirmed; a PENDING hold keeps its reservation held for its commit or cancel")
	assert.Empty(t, released)
}
