// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// lockedReservation builds the row a by-id confirm/release reads under its lock,
// in the given status.
func lockedReservation(t *testing.T, id uuid.UUID, status model.ReservationStatus) *model.Reservation {
	t.Helper()

	res, err := model.NewReservation(
		testutil.MustDeterministicUUID(7501),
		testutil.MustDeterministicUUID(7502),
		"acct:7501",
		"2026-06",
		decimal.RequireFromString("123.45"),
		testutil.FixedTime().Add(5*time.Minute),
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	res.ID = id
	res.Status = status

	return res
}

// Sequential: newReservationServiceDeps swaps the process-global tracer provider.
func TestReservationService_ByIDSettleAuditContext(t *testing.T) {
	t.Run("confirm by id audits the locked row's correlation", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		resID := testutil.MustDeterministicUUID(7500)
		locked := lockedReservation(t, resID, model.StatusReserved)

		var got command.ReservationAuditContext

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(locked, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, resID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ model.AuditEventType, _ model.AuditAction, _ uuid.UUID, rc command.ReservationAuditContext) error {
				got = rc
				return nil
			}).
			Times(1)

		outcome, err := svc.Confirm(deps.ctx(), resID)
		require.NoError(t, err)
		assert.Equal(t, ConfirmOutcome{Confirmed: 1}, outcome)

		assert.Equal(t, locked.TransactionID, got.TransactionID)
		assert.Equal(t, locked.LimitID, got.LimitID)
		assert.Equal(t, locked.ScopeKey, got.ScopeKey)
		assert.Equal(t, locked.PeriodKey, got.PeriodKey)
		assert.True(t, locked.Amount.Equal(got.Amount), "amount: want %s, got %s", locked.Amount, got.Amount)
		assert.Equal(t, string(model.StatusConfirmed), got.Status, "the audit carries the status the row now holds")
	})

	t.Run("late confirm of an EXPIRED row still audits CONFIRMED with the row's correlation", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		resID := testutil.MustDeterministicUUID(7510)
		locked := lockedReservation(t, resID, model.StatusExpired)

		var got command.ReservationAuditContext

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(locked, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, resID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ model.AuditEventType, _ model.AuditAction, _ uuid.UUID, rc command.ReservationAuditContext) error {
				got = rc
				return nil
			}).
			Times(1)

		_, err := svc.Confirm(deps.ctx(), resID)
		require.NoError(t, err)

		assert.Equal(t, locked.TransactionID, got.TransactionID)
		assert.Equal(t, locked.LimitID, got.LimitID)
		assert.Equal(t, string(model.StatusConfirmed), got.Status)
	})

	t.Run("release by id audits the locked row's correlation", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		resID := testutil.MustDeterministicUUID(7520)
		locked := lockedReservation(t, resID, model.StatusReserved)

		var got command.ReservationAuditContext

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ReleaseWithTx(gomock.Any(), deps.tx, resID, model.StatusReleased).
			Return(locked, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReleased, model.AuditActionRelease, resID, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, _ model.AuditEventType, _ model.AuditAction, _ uuid.UUID, rc command.ReservationAuditContext) error {
				got = rc
				return nil
			}).
			Times(1)

		require.NoError(t, svc.Release(deps.ctx(), resID))

		assert.Equal(t, locked.TransactionID, got.TransactionID)
		assert.Equal(t, locked.LimitID, got.LimitID)
		assert.Equal(t, locked.ScopeKey, got.ScopeKey)
		assert.Equal(t, locked.PeriodKey, got.PeriodKey)
		assert.True(t, locked.Amount.Equal(got.Amount), "amount: want %s, got %s", locked.Amount, got.Amount)
		assert.Equal(t, string(model.StatusReleased), got.Status)
	})
}
