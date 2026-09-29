// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
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
	canonicalAmount = "100.00"
	canonicalAsset  = "USD"
)

// newReserveRequest builds a valid proto reserve request whose timestamp sits
// inside the validation window relative to the injected fixed clock, so the
// model-level reserve validation accepts it.
func newReserveRequest(now time.Time, transactionID, requestID, accountID uuid.UUID) *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        transactionID.String(),
		RequestId:            requestID.String(),
		Amount:               canonicalAmount,
		Asset:                canonicalAsset,
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

	t.Run("allow maps proto to the same limit input and returns reservation ids", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		expected := expectedInput(now, requestID, accountID)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{}).
			DoAndReturn(func(_ context.Context, _ uuid.UUID, gotReq *model.ValidationRequest, _ services.ReserveOptions) (*services.ReserveResult, error) {
				// The gRPC server must hand the use case the validation request whose
				// limit input is the canonical ToCheckLimitsInput projection.
				require.NotNil(t, gotReq)
				require.Equal(t, requestID, gotReq.RequestID)

				gotInput := gotReq.ToCheckLimitsInput()
				require.True(t, gotInput.Amount.Equal(expected.Amount))
				require.Equal(t, expected.Asset, gotInput.Asset)
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
			Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{}).
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
			Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{LongLived: true}).
			Return(&services.ReserveResult{}, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.LongLived = true

		_, err = server.Reserve(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("revert hint is forwarded to the use case", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{Revert: true}).
			Return(&services.ReserveResult{}, nil)

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.Revert = true

		_, err = server.Reserve(context.Background(), req)
		require.NoError(t, err)
	})

	t.Run("rule cache not ready is Unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		svc := mocks.NewMockReservationService(ctrl)
		clk := testutil.NewMockClock(now)

		svc.EXPECT().
			Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{}).
			Return(nil, fmt.Errorf("rule evaluation failed: %w", constant.ErrRuleCacheNotReady))

		server, err := NewReservationServer(svc, clk)
		require.NoError(t, err)

		_, err = server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
		require.Equal(t, codes.Unavailable, status.Code(err))
		require.Equal(t, constant.ErrRuleCacheNotReady.Error(), status.Convert(err).Message())
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

// expectedInput mirrors what ToCheckLimitsInput produces for the canonical
// valid reserve request, so the server's proto->domain mapping can be
// asserted against the SAME shape the synchronous validate path uses.
func expectedInput(now time.Time, requestID, accountID uuid.UUID) *model.CheckLimitsInput {
	req := &model.ValidationRequest{
		RequestID:            requestID,
		TransactionType:      model.TransactionTypeCard,
		Amount:               decimal.RequireFromString(canonicalAmount),
		Asset:                canonicalAsset,
		TransactionTimestamp: now.Add(-1 * time.Second),
		Account:              model.AccountContext{ID: accountID},
	}

	return req.ToCheckLimitsInput()
}

func TestReservationServer_Reserve_Decision(t *testing.T) {
	now := testutil.FixedTime()
	transactionID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	ruleID := testutil.MustDeterministicUUID(5)

	tests := []struct {
		name            string
		serviceResult   *services.ReserveResult
		wantDenied      bool
		wantDecision    string
		wantReason      string
		wantMatchedRule []string
	}{
		{
			name:          "allow without a service decision falls back to ALLOW",
			serviceResult: &services.ReserveResult{},
			wantDecision:  string(model.DecisionAllow),
		},
		{
			name:          "denied without a service decision falls back to DENY",
			serviceResult: &services.ReserveResult{Denied: true},
			wantDenied:    true,
			wantDecision:  string(model.DecisionDeny),
		},
		{
			name: "service decision, reason and matched rules are carried verbatim",
			serviceResult: &services.ReserveResult{
				Denied:         true,
				Decision:       model.DecisionReview,
				Reason:         "manual review required",
				MatchedRuleIDs: []uuid.UUID{ruleID},
			},
			wantDenied:      true,
			wantDecision:    string(model.DecisionReview),
			wantReason:      "manual review required",
			wantMatchedRule: []string{ruleID.String()},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			svc := mocks.NewMockReservationService(ctrl)

			svc.EXPECT().
				Reserve(gomock.Any(), transactionID, gomock.Any(), services.ReserveOptions{}).
				Return(tt.serviceResult, nil)

			server, err := NewReservationServer(svc, testutil.NewMockClock(now))
			require.NoError(t, err)

			result, err := server.Reserve(context.Background(), newReserveRequest(now, transactionID, requestID, accountID))
			require.NoError(t, err)
			require.Equal(t, tt.wantDenied, result.GetDenied())
			require.Equal(t, tt.wantDecision, result.GetDecision())
			require.NotEmpty(t, result.GetDecision())
			require.Equal(t, tt.wantReason, result.GetReason())
			require.Equal(t, tt.wantMatchedRule, result.GetMatchedRuleIds())
		})
	}
}

func TestReservationServer_ToValidationRequest_AccountTypeAndMetadata(t *testing.T) {
	now := testutil.FixedTime()
	transactionID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)

	ctrl := gomock.NewController(t)
	server, err := NewReservationServer(mocks.NewMockReservationService(ctrl), testutil.NewMockClock(now))
	require.NoError(t, err)

	t.Run("account type and metadata reach the validation request", func(t *testing.T) {
		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.Account.Type = "deposit"
		req.Metadata = map[string]string{"channel": "app"}

		validationReq, err := server.toValidationRequest(req)
		require.NoError(t, err)
		require.Equal(t, accountID, validationReq.Account.ID)
		require.Equal(t, "deposit", validationReq.Account.Type)
		require.Equal(t, map[string]any{"channel": "app"}, validationReq.Metadata)
	})

	t.Run("empty metadata stays nil", func(t *testing.T) {
		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.Metadata = map[string]string{}

		validationReq, err := server.toValidationRequest(req)
		require.NoError(t, err)
		require.Nil(t, validationReq.Metadata)
	})

	t.Run("account type without an account id is carried", func(t *testing.T) {
		req := newReserveRequest(now, transactionID, requestID, accountID)
		req.Account = &reservationv1.ReserveAccount{Type: "deposit"}

		validationReq, err := server.toValidationRequest(req)
		require.NoError(t, err)
		require.Equal(t, uuid.Nil, validationReq.Account.ID)
		require.Equal(t, "deposit", validationReq.Account.Type)
	})
}

func TestReservationServer_Reserve_ForwardsRuleContext(t *testing.T) {
	now := testutil.FixedTime()
	transactionID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)
	ruleID := testutil.MustDeterministicUUID(5)

	ctrl := gomock.NewController(t)
	svc := mocks.NewMockReservationService(ctrl)

	svc.EXPECT().
		Reserve(gomock.Any(), transactionID, gomock.Cond(func(req *model.ValidationRequest) bool {
			return req != nil &&
				req.Account.Type == "deposit" &&
				req.Metadata["channel"] == "app"
		}), services.ReserveOptions{}).
		Return(&services.ReserveResult{
			Denied:         true,
			Decision:       model.DecisionDeny,
			Reason:         "blocked by rule",
			MatchedRuleIDs: []uuid.UUID{ruleID},
		}, nil)

	server, err := NewReservationServer(svc, testutil.NewMockClock(now))
	require.NoError(t, err)

	req := newReserveRequest(now, transactionID, requestID, accountID)
	req.Account.Type = "deposit"
	req.Metadata = map[string]string{"channel": "app"}

	result, err := server.Reserve(context.Background(), req)
	require.NoError(t, err)
	require.True(t, result.GetDenied())
	require.Equal(t, string(model.DecisionDeny), result.GetDecision())
	require.Equal(t, "blocked by rule", result.GetReason())
	require.Equal(t, []string{ruleID.String()}, result.GetMatchedRuleIds())
	require.Empty(t, result.GetReservationIds())
}

func TestReservationServer_Reserve_InvalidMetadataKey(t *testing.T) {
	now := testutil.FixedTime()
	transactionID := testutil.MustDeterministicUUID(1)
	requestID := testutil.MustDeterministicUUID(2)
	accountID := testutil.MustDeterministicUUID(3)

	ctrl := gomock.NewController(t)
	svc := mocks.NewMockReservationService(ctrl)

	server, err := NewReservationServer(svc, testutil.NewMockClock(now))
	require.NoError(t, err)

	req := newReserveRequest(now, transactionID, requestID, accountID)
	req.Metadata = map[string]string{"bad-key": "value"}

	_, err = server.Reserve(context.Background(), req)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Contains(t, status.Convert(err).Message(), constant.ErrMetadataKeyInvalidChars.Error())
}
