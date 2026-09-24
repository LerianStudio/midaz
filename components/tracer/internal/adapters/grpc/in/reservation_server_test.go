// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/grpc/in/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

const (
	canonicalAmount = "100.00"
	canonicalAsset  = "USD"
)

func newReserveRequest(now time.Time, transactionID, requestID, accountID uuid.UUID) *reservationv1.ReserveRequest {
	blocked, longLived := false, false
	asset := &reservationv1.AssetRef{Namespace: "official", Id: "asset", Code: canonicalAsset}
	return &reservationv1.ReserveRequest{
		ContractRevision: tracercontract.ReserveContractRevision,
		TransactionId:    transactionID.String(), RequestId: requestID.String(),
		ContextId: "context", ValidationMode: string(tracercontract.ValidationLimits),
		Amount: canonicalAmount, Asset: asset, LongLived: &longLived,
		TransactionTimestamp: now.Add(-time.Second).Format(time.RFC3339Nano),
		Context: &reservationv1.EvaluationContext{
			Accounts: []*reservationv1.ContextAccount{{Id: accountID.String(), Type: "native", Status: "ACTIVE", Blocked: &blocked, Asset: asset}},
			Entries:  []*reservationv1.ContextEntry{{AccountId: accountID.String(), Direction: string(tracercontract.Debit), Amount: canonicalAmount, Asset: asset}},
		},
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
	for _, scenario := range []string{"allow", "deny", "review", "long lived", "invalid id", "zero amount", "missing presence", "identity absent", "deadline", "mismatched result", "oversize"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			legacy := mocks.NewMockReservationService(ctrl)
			admission := mocks.NewMockContextReserveAdmitter(ctrl)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			config := ContextReservationConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536, MaxReservations: 100}
			server, err := NewContextReservationServer(legacy, testutil.NewDefaultMockClock(), admission, completion, mocks.NewMockContextReserveIDCompleter(ctrl), config)
			require.NoError(t, err)
			transaction := testutil.MustDeterministicUUID(1)
			request := newReserveRequest(testutil.FixedTime(), transaction, testutil.MustDeterministicUUID(2), testutil.MustDeterministicUUID(3))
			ctx := contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "official"})
			expectedCode := codes.OK
			decision := tracercontract.DecisionAllow
			switch scenario {
			case "invalid id":
				request.TransactionId = "invalid"
				expectedCode = codes.InvalidArgument
			case "zero amount":
				request.Amount = "0"
				expectedCode = codes.InvalidArgument
			case "missing presence":
				request.LongLived = nil
				expectedCode = codes.InvalidArgument
			case "oversize":
				request.Amount = strings.Repeat("1", 65536)
				expectedCode = codes.ResourceExhausted
			case "identity absent":
				ctx = t.Context()
				expectedCode = codes.PermissionDenied
			default:
				if scenario == "deny" {
					decision = tracercontract.DecisionDeny
				}
				if scenario == "review" {
					decision = tracercontract.DecisionReview
					request.ValidationMode = string(tracercontract.ValidationRulesAndLimits)
				}
				if scenario == "long lived" {
					*request.LongLived = true
				}
				result := &tracercontract.ReserveResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: transaction, EvaluationID: testutil.MustDeterministicUUID(4), Decision: decision, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}
				if scenario == "review" {
					result.Controls.Rules = tracercontract.RulesEvaluated
					result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonRuleReview}
				}
				if scenario == "deny" {
					result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
				}
				var serviceErr error
				if scenario == "deadline" {
					serviceErr = context.DeadlineExceeded
					expectedCode = codes.DeadlineExceeded
				}
				if scenario == "mismatched result" {
					result.TransactionID = testutil.MustDeterministicUUID(99)
					expectedCode = codes.Internal
				}
				admission.EXPECT().Execute(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, got tracercontract.ReserveRequest) (*tracercontract.ReserveResult, error) {
					require.Equal(t, transaction, got.TransactionID)
					require.Equal(t, tracercontract.Amount(canonicalAmount), got.Amount)
					require.Equal(t, scenario == "long lived", *got.LongLived)
					require.Len(t, got.Context.Accounts, 1)
					require.Equal(t, "native", got.Context.Accounts[0].Type)
					return result, serviceErr
				})
			}
			result, err := server.Reserve(ctx, request)
			require.Equal(t, expectedCode, status.Code(err))
			if expectedCode == codes.OK {
				require.Equal(t, string(decision), result.GetDecision())
				require.Equal(t, transaction.String(), result.GetTransactionId())
			} else {
				require.Nil(t, result)
			}
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

func TestContextReservationCompletion(t *testing.T) {
	for _, scenario := range []string{"confirmed", "released", "before admission", "unsupported", "identity absent", "wrong transaction", "conflict"} {
		t.Run(scenario, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			completion := mocks.NewMockContextReserveCompleter(ctrl)
			config := ContextReservationConfig{Bounds: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxBodyBytes: 65536, MaxReservations: 100}
			server, err := NewContextReservationServer(mocks.NewMockReservationService(ctrl), testutil.NewDefaultMockClock(), mocks.NewMockContextReserveAdmitter(ctrl), completion, mocks.NewMockContextReserveIDCompleter(ctrl), config)
			require.NoError(t, err)
			transaction := testutil.MustDeterministicUUID(88101)
			evaluation := testutil.MustDeterministicUUID(88102)
			ctx := contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: "producer", AssetNamespace: "official"})
			revision := tracercontract.ReserveContractRevision
			expected := codes.OK
			outcome := model.OperationConfirmed
			if scenario == "released" {
				outcome = model.OperationReleased
			}
			switch scenario {
			case "unsupported":
				revision = "unsupported"
				expected = codes.InvalidArgument
			case "identity absent":
				ctx = t.Context()
				expected = codes.PermissionDenied
			default:
				result := &tracercontract.TransactionCompletionResult{ContractRevision: revision, TransactionID: transaction, Status: string(outcome), EvaluationID: &evaluation}
				if scenario == "before admission" {
					result.EvaluationID = nil
				}
				if scenario == "wrong transaction" {
					result.TransactionID = uuid.Nil
					expected = codes.Internal
				}
				var serviceErr error
				if scenario == "conflict" {
					serviceErr = constant.ErrReserveOperationConflict
					expected = codes.FailedPrecondition
				}
				completion.EXPECT().ExecuteReport(gomock.Any(), transaction, outcome).Return(result, serviceErr)
			}
			if scenario == "released" {
				result, err := server.ReleaseByTransaction(ctx, &reservationv1.ReleaseByTransactionRequest{ContractRevision: revision, TransactionId: transaction.String()})
				require.NoError(t, err)
				require.Equal(t, string(outcome), result.GetStatus())
				require.Equal(t, evaluation.String(), result.GetEvaluationId())
			} else {
				result, err := server.ConfirmByTransaction(ctx, &reservationv1.ConfirmByTransactionRequest{ContractRevision: revision, TransactionId: transaction.String()})
				require.Equal(t, expected, status.Code(err))
				if expected != codes.OK {
					require.Nil(t, result)
					return
				}
				require.Equal(t, revision, result.GetContractRevision())
				require.Equal(t, transaction.String(), result.GetTransactionId())
				require.Zero(t, result.GetFlipped())
				if scenario == "before admission" {
					require.Nil(t, result.EvaluationId)
				} else {
					require.Equal(t, evaluation.String(), result.GetEvaluationId())
				}
			}
		})
	}
}
