// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	commandMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

func reservationAuditCtx() ReservationAuditContext {
	return ReservationAuditContext{
		TransactionID: testutil.MustDeterministicUUID(400),
		LimitID:       testutil.MustDeterministicUUID(401),
		ScopeKey:      "acct:400",
		PeriodKey:     "2026-06",
		Amount:        400,
		Status:        string(model.StatusReserved),
	}
}

func TestRecordReservationEventWithTx_WritesOneRowInTx(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	resID := testutil.MustDeterministicUUID(410)

	// Exactly one InsertWithTx for a single transition, carrying the reservation
	// resource type and the RESERVED event/action.
	mockRepo.EXPECT().InsertWithTx(
		gomock.Any(),
		mockDB,
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, _ any, event *model.AuditEvent) error {
		assert.Equal(t, model.AuditEventReservationReserved, event.EventType)
		assert.Equal(t, model.AuditActionReserve, event.Action)
		assert.Equal(t, model.ResourceTypeReservation, event.ResourceType)
		assert.Equal(t, resID.String(), event.ResourceID)
		assert.Equal(t, resID.String(), event.Context["reservationId"])
		return nil
	}).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordReservationEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventReservationReserved,
		model.AuditActionReserve,
		resID,
		reservationAuditCtx(),
	)
	require.NoError(t, err)
}

func TestRecordReservationEventWithTx_RepoError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)
	mockDB := pgdbMocks.NewMockDB(ctrl)

	dbErr := errors.New("insert failed")
	mockRepo.EXPECT().InsertWithTx(gomock.Any(), mockDB, gomock.Any()).Return(dbErr).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordReservationEventWithTx(
		context.Background(),
		mockDB,
		model.AuditEventReservationConfirmed,
		model.AuditActionConfirm,
		testutil.MustDeterministicUUID(411),
		reservationAuditCtx(),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, dbErr, "repository error must propagate so the tx rolls back")
}

func TestRecordReservationEvent_SkippedUsesNonTxInsert(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)

	resID := testutil.MustDeterministicUUID(420)

	// SKIPPED is the ledger fail-open decision: no counter move, no tx — recorded
	// via the non-tx Insert surface.
	mockRepo.EXPECT().Insert(
		gomock.Any(),
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, event *model.AuditEvent) error {
		assert.Equal(t, model.AuditEventReservationSkipped, event.EventType)
		assert.Equal(t, model.AuditActionSkip, event.Action)
		assert.Equal(t, model.ResourceTypeReservation, event.ResourceType)
		return nil
	}).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordReservationEvent(
		context.Background(),
		model.AuditEventReservationSkipped,
		model.AuditActionSkip,
		resID,
		ReservationAuditContext{
			TransactionID: testutil.MustDeterministicUUID(421),
			LimitID:       testutil.MustDeterministicUUID(422),
			ScopeKey:      "acct:420",
			PeriodKey:     "2026-06",
			Amount:        400,
			Status:        "SKIPPED",
		},
	)
	require.NoError(t, err)
}

func TestRecordReservationExpiryBatch_WritesExactlyOneRowForNExpiries(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockRepo := commandMocks.NewMockAuditEventRepository(ctrl)

	oldest := testutil.FixedTime().Add(-10 * time.Minute) // 10 min before sweep

	// ONE summary row for N expiries (Q11): exactly one Insert, never N.
	mockRepo.EXPECT().Insert(
		gomock.Any(),
		gomock.AssignableToTypeOf(&model.AuditEvent{}),
	).DoAndReturn(func(_ context.Context, event *model.AuditEvent) error {
		assert.Equal(t, model.AuditEventReservationExpired, event.EventType)
		assert.Equal(t, model.AuditActionExpire, event.Action)
		assert.Equal(t, model.ResourceTypeReservation, event.ResourceType)
		assert.Equal(t, 137, event.Context["expiredCount"], "batch row must carry the sweep count")
		return nil
	}).Times(1)

	cmd := NewRecordAuditEventCommand(mockRepo)
	err := cmd.RecordReservationExpiryBatch(
		context.Background(),
		ReservationExpiryBatchSummary{
			ExpiredCount: 137,
			SweptAt:      testutil.FixedTime(),
			OldestExpiry: &oldest,
		},
	)
	require.NoError(t, err)
}
