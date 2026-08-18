// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

const (
	canonicalAmount   = "100.00"
	canonicalCurrency = "USD"
)

// newReserveRequest builds a valid proto reserve request whose timestamp sits
// inside the validation window relative to the injected fixed clock, so the
// model-level reserve validation (shared with the REST path) accepts it.
func newReserveRequest(now time.Time, transactionID, requestID, accountID uuid.UUID) *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        transactionID.String(),
		RequestId:            requestID.String(),
		Amount:               canonicalAmount,
		Currency:             canonicalCurrency,
		Account:              &reservationv1.ReserveAccount{AccountId: accountID.String()},
		TransactionType:      string(model.TransactionTypeCard),
		TransactionTimestamp: now.Add(-1 * time.Second).Format(time.RFC3339),
	}
}

func TestNewReservationServer_NilDeps(t *testing.T) {
	clk := testutil.NewDefaultMockClock()

	t.Run("nil service", func(t *testing.T) {
		_, err := NewReservationServer(nil, clk)
		require.Error(t, err)
	})

	t.Run("nil clock", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)

		_, err := NewReservationServer(svc, nil)
		require.Error(t, err)
	})
}

func TestReservationServer_Reserve(t *testing.T) {
	now := testutil.FixedTime()
	transactionID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	reservationID := testutil.MustDeterministicUUID(4)

	t.Run("allow maps proto to the same CheckLimitsInput and returns reservation ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		expected := expectedInput(now, requestID, accountID)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), false).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, gotInput *model.CheckLimitsInput, _ bool, _ ...model.ReservationDeliveryMode) (*services.ReserveResult, error) {
				// The gRPC server must hand the use case the SAME CheckLimitsInput
				// the REST path produces (no fork).
				require.True(t, gotInput.Amount.Equal(expected.Amount))
				require.Equal(t, expected.Currency, gotInput.Currency)
				require.Equal(t, expected.AccountID, gotInput.AccountID)
				require.NotNil(t, gotInput.TransactionType)
				require.Equal(t, *expected.TransactionType, *gotInput.TransactionType)

				return &services.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}, nil
			})

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		result, err := server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
		require.NoError(t, err)
		require.False(t, result.GetDenied())
		require.Equal(t, transactionID.String(), result.GetTransactionId())
		require.Equal(t, []string{reservationID.String()}, result.GetReservationIds())
	})

	t.Run("denied returns denied=true and no reservation ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), false).
			Return(&services.ReserveResult{Denied: true}, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		result, err := server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
		require.NoError(t, err)
		require.True(t, result.GetDenied())
		require.Empty(t, result.GetReservationIds())
	})

	t.Run("long_lived hint is forwarded to the use case", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), true).
			Return(&services.ReserveResult{}, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.LongLived = true

		_, err = server.Reserve(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("ledger outcome v2 mode is forwarded", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Reserve(gomock.Any(), transactionID, gomock.Any(), false, model.DeliveryModeLedgerOutcomeV2).
			Return(&services.ReserveResult{}, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.DeliveryMode = reservationv1.ReservationDeliveryMode_RESERVATION_DELIVERY_MODE_LEDGER_OUTCOME_V2
		_, err = server.Reserve(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("unknown delivery mode is InvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		server, err := NewReservationServer(svc, testutil.NewMockClock(now))
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.DeliveryMode = reservationv1.ReservationDeliveryMode(99)
		_, err = server.Reserve(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("invalid transaction id is InvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.TransactionId = "not-a-uuid"

		_, err = server.Reserve(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("non-positive amount fails validation with InvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.Amount = "0"

		_, err = server.Reserve(context.Background(), req)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("idempotency conflict maps to AlreadyExists", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Reserve(gomock.Any(), transactionID, gomock.Any(), false).Return(nil, constant.ErrIdempotencyKey)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
		require.Equal(t, codes.AlreadyExists, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), constant.ErrIdempotencyKey.Error())
	})

	t.Run("terminal reservation retry maps to FailedPrecondition", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Reserve(gomock.Any(), transactionID, gomock.Any(), false).Return(nil, constant.ErrReservationAlreadyTerminal)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), constant.ErrReservationAlreadyTerminal.Error())
	})
}

func TestReservationServer_ApplyOutcome(t *testing.T) {
	transactionID := testutil.MustDeterministicUUID(50)
	outcomeID := testutil.MustDeterministicUUID(51)
	now := testutil.FixedTime()

	t.Run("committed receipt", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		svc.EXPECT().ApplyOutcome(gomock.Any(), transactionID, outcomeID, model.OutcomeCommitted).
			Return(&services.ApplyOutcomeResult{TransactionID: transactionID, OutcomeID: outcomeID, Outcome: model.OutcomeCommitted, ReservationCount: 3}, nil)

		server, err := NewReservationServer(svc, testutil.NewMockClock(now))
		require.NoError(t, err)
		got, err := server.ApplyOutcome(context.Background(), &reservationv1.ApplyOutcomeRequest{
			TransactionId: transactionID.String(), OutcomeId: outcomeID.String(),
			Outcome: reservationv1.ReservationOutcome_RESERVATION_OUTCOME_COMMITTED,
		})
		require.NoError(t, err)
		require.Equal(t, transactionID.String(), got.GetTransactionId())
		require.Equal(t, outcomeID.String(), got.GetOutcomeId())
		require.EqualValues(t, 3, got.GetReservationCount())
		require.False(t, got.GetReplayed())
	})

	t.Run("aborted exact replay", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		svc.EXPECT().ApplyOutcome(gomock.Any(), transactionID, outcomeID, model.OutcomeAborted).
			Return(&services.ApplyOutcomeResult{TransactionID: transactionID, OutcomeID: outcomeID, Outcome: model.OutcomeAborted, Replayed: true}, nil)

		server, err := NewReservationServer(svc, testutil.NewMockClock(now))
		require.NoError(t, err)
		got, err := server.ApplyOutcome(context.Background(), &reservationv1.ApplyOutcomeRequest{
			TransactionId: transactionID.String(), OutcomeId: outcomeID.String(),
			Outcome: reservationv1.ReservationOutcome_RESERVATION_OUTCOME_ABORTED,
		})
		require.NoError(t, err)
		require.True(t, got.GetReplayed())
		require.Equal(t, reservationv1.ReservationOutcome_RESERVATION_OUTCOME_ABORTED, got.GetOutcome())
	})

	t.Run("opposite outcome conflict", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		svc.EXPECT().ApplyOutcome(gomock.Any(), transactionID, outcomeID, model.OutcomeAborted).
			Return(nil, constant.ErrReservationOutcomeConflict)

		server, err := NewReservationServer(svc, testutil.NewMockClock(now))
		require.NoError(t, err)
		_, err = server.ApplyOutcome(context.Background(), &reservationv1.ApplyOutcomeRequest{
			TransactionId: transactionID.String(), OutcomeId: outcomeID.String(),
			Outcome: reservationv1.ReservationOutcome_RESERVATION_OUTCOME_ABORTED,
		})
		require.Equal(t, codes.AlreadyExists, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), constant.ErrReservationOutcomeConflict.Error())
	})

	for _, tc := range []struct {
		name string
		req  *reservationv1.ApplyOutcomeRequest
	}{
		{"bad transaction id", &reservationv1.ApplyOutcomeRequest{TransactionId: "bad", OutcomeId: outcomeID.String(), Outcome: reservationv1.ReservationOutcome_RESERVATION_OUTCOME_COMMITTED}},
		{"bad outcome id", &reservationv1.ApplyOutcomeRequest{TransactionId: transactionID.String(), OutcomeId: "bad", Outcome: reservationv1.ReservationOutcome_RESERVATION_OUTCOME_COMMITTED}},
		{"unspecified outcome", &reservationv1.ApplyOutcomeRequest{TransactionId: transactionID.String(), OutcomeId: outcomeID.String()}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			server, err := NewReservationServer(mocks.NewMockReservationService(ctrl), testutil.NewMockClock(now))
			require.NoError(t, err)
			_, err = server.ApplyOutcome(context.Background(), tc.req)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
		})
	}
}

func TestReservationServer_ConfirmReleaseById(t *testing.T) {
	reservationID := testutil.MustDeterministicUUID(10)
	now := testutil.FixedTime()

	t.Run("confirm by id succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Confirm(gomock.Any(), reservationID).Return(nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ConfirmById(context.Background(), &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
		require.NoError(t, err)
	})

	t.Run("release by id succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Release(gomock.Any(), reservationID).Return(nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ReleaseById(context.Background(), &reservationv1.ReleaseByIdRequest{ReservationId: reservationID.String()})
		require.NoError(t, err)
	})

	t.Run("not found maps to NotFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().Confirm(gomock.Any(), reservationID).Return(constant.ErrReservationNotFound)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ConfirmById(context.Background(), &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("invalid id is InvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ReleaseById(context.Background(), &reservationv1.ReleaseByIdRequest{ReservationId: "nope"})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

func TestReservationServer_ConfirmReleaseByTransaction(t *testing.T) {
	transactionID := testutil.MustDeterministicUUID(20)
	now := testutil.FixedTime()

	t.Run("confirm by transaction succeeds (idempotent zero flips)", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).Return(0, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ConfirmByTransaction(context.Background(), &reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
		require.NoError(t, err)
	})

	t.Run("release by transaction succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().ReleaseByTransaction(gomock.Any(), transactionID).Return(2, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ReleaseByTransaction(context.Background(), &reservationv1.ReleaseByTransactionRequest{TransactionId: transactionID.String()})
		require.NoError(t, err)
	})

	t.Run("V2 transaction rejects the V1 endpoint", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().ConfirmByTransaction(gomock.Any(), transactionID).
			Return(0, constant.ErrReservationOutcomeConflict)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ConfirmByTransaction(context.Background(), &reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
		require.Equal(t, codes.AlreadyExists, status.Code(err))
		require.Contains(t, status.Convert(err).Message(), constant.ErrReservationOutcomeConflict.Error())
	})

	t.Run("empty transaction id is InvalidArgument", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.ConfirmByTransaction(context.Background(), &reservationv1.ConfirmByTransactionRequest{TransactionId: ""})
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

// expectedInput mirrors what the REST path's ToCheckLimitsInput produces for the
// canonical valid reserve request, so the server's proto->domain mapping can be
// asserted against the SAME shape the synchronous validate path uses.
func expectedInput(now time.Time, requestID, accountID uuid.UUID) *model.CheckLimitsInput {
	req := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString(canonicalAmount),
		Currency:             canonicalCurrency,
		TransactionTimestamp: now.Add(-1 * time.Second),
		Account:              model.AccountContext{ID: accountID},
	}

	return req.ToCheckLimitsInput()
}
