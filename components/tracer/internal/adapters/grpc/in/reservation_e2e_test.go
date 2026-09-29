// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	reservationv1 "github.com/LerianStudio/midaz/v4/pkg/proto/reservation/v1"
)

// reserveE2E is a real ReservationServer over a real ReservationService on an
// in-memory gRPC listener. Only the rule evaluator and the persistence ports are
// doubles, so the proto mapping, the reserve rule policy and the response
// mapping all run as in production. The client is the generated stub the
// ledger's gRPC client wraps.
type reserveE2E struct {
	client      reservationv1.ReservationServiceClient
	evaluator   *servicesMocks.MockRuleEvaluator
	resolver    *servicesMocks.MockLimitResolver
	conn        *pgdbMocks.MockTxBeginner
	tx          *pgdbMocks.MockTx
	repo        *servicesMocks.MockReservationRepository
	auditWriter *servicesMocks.MockReservationAuditWriter
	now         time.Time
}

func newReserveE2E(t *testing.T) *reserveE2E {
	t.Helper()

	ctrl := gomock.NewController(t)
	now := testutil.FixedTime()

	evaluator := servicesMocks.NewMockRuleEvaluator(ctrl)
	resolver := servicesMocks.NewMockLimitResolver(ctrl)
	txBeginner := pgdbMocks.NewMockTxBeginner(ctrl)
	tx := pgdbMocks.NewMockTx(ctrl)
	repo := servicesMocks.NewMockReservationRepository(ctrl)
	auditWriter := servicesMocks.NewMockReservationAuditWriter(ctrl)

	svc, err := services.NewReservationService(
		txBeginner,
		resolver,
		repo,
		auditWriter,
		evaluator,
		testutil.NewMockClock(now),
	)
	require.NoError(t, err)

	server, err := NewReservationServer(svc, testutil.NewMockClock(now))
	require.NoError(t, err)

	lis := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	reservationv1.RegisterReservationServiceServer(grpcServer, server)

	go func() { _ = grpcServer.Serve(lis) }()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		grpcServer.Stop()
		_ = lis.Close()
	})

	return &reserveE2E{
		client:      reservationv1.NewReservationServiceClient(conn),
		evaluator:   evaluator,
		resolver:    resolver,
		conn:        txBeginner,
		tx:          tx,
		repo:        repo,
		auditWriter: auditWriter,
		now:         now,
	}
}

// expectTxCommit wires one BeginTx handing out the shared mock Tx and one Commit.
func (e *reserveE2E) expectTxCommit() {
	e.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(e.tx, nil).Times(1)
	e.tx.EXPECT().Commit().Return(nil).Times(1)
}

// expectTxRollback wires one BeginTx handing out the shared mock Tx and one Rollback.
func (e *reserveE2E) expectTxRollback() {
	e.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(e.tx, nil).Times(1)
	e.tx.EXPECT().Rollback().Return(nil).Times(1)
}

// ledgerShapedRequest is the reserve the ledger sends: free-form account type,
// flat metadata, no transaction type.
func (e *reserveE2E) ledgerShapedRequest(revert bool) *reservationv1.ReserveRequest {
	return &reservationv1.ReserveRequest{
		TransactionId:        testutil.MustDeterministicUUID(9101).String(),
		RequestId:            testutil.MustDeterministicUUID(9102).String(),
		Amount:               "100.00",
		Asset:                "BRL",
		Account:              &reservationv1.ReserveAccount{AccountId: testutil.MustDeterministicUUID(9103).String(), Type: "deposit"},
		TransactionTimestamp: e.now.Add(-1 * time.Minute).Format(time.RFC3339),
		Metadata:             map[string]string{"channel": "app"},
		Revert:               revert,
	}
}

func TestReservationE2E_RuleDecisionsReachTheClient(t *testing.T) {
	ruleID := testutil.MustDeterministicUUID(9104)

	t.Run("matched REVIEW rule refuses with the rule id and holds nothing", func(t *testing.T) {
		e := newReserveE2E(t)

		e.evaluator.EXPECT().
			Execute(gomock.Any(), gomock.Cond(func(req *model.ValidationRequest) bool {
				return req.Account.Type == "deposit" && req.Metadata["channel"] == "app"
			})).
			Return(&model.EvaluationResult{
				Decision:       model.DecisionReview,
				MatchedRuleIDs: []uuid.UUID{ruleID},
				Reason:         "manual review required",
			}, nil)
		// No resolver expectation: a REVIEW must not reach the limits.

		got, err := e.client.Reserve(context.Background(), e.ledgerShapedRequest(false))
		require.NoError(t, err)
		assert.True(t, got.GetDenied())
		assert.Equal(t, string(model.DecisionReview), got.GetDecision())
		assert.Equal(t, "manual review required", got.GetReason())
		assert.Equal(t, []string{ruleID.String()}, got.GetMatchedRuleIds())
		assert.Empty(t, got.GetReservationIds())
	})

	t.Run("REVIEW default with no matched rule proceeds to limits as ALLOW", func(t *testing.T) {
		e := newReserveE2E(t)

		e.evaluator.EXPECT().
			Execute(gomock.Any(), gomock.Any()).
			Return(&model.EvaluationResult{Decision: model.DecisionReview, Reason: "no rule matched"}, nil)
		e.resolver.EXPECT().
			ResolveReservations(gomock.Any(), gomock.Any()).
			Return(nil, false, nil)

		got, err := e.client.Reserve(context.Background(), e.ledgerShapedRequest(false))
		require.NoError(t, err)
		assert.False(t, got.GetDenied())
		assert.Equal(t, string(model.DecisionAllow), got.GetDecision())
		assert.Empty(t, got.GetReason())
		assert.Empty(t, got.GetMatchedRuleIds())
	})

	t.Run("rule evaluation error refuses as REVIEW with the failing rule", func(t *testing.T) {
		e := newReserveE2E(t)

		e.evaluator.EXPECT().
			Execute(gomock.Any(), gomock.Any()).
			Return(nil, fmt.Errorf("failed to evaluate rules: %w", &query.RuleEvaluationError{
				RuleID: ruleID,
				Err:    fmt.Errorf("%w: no such overload", constant.ErrExpressionEvaluation),
			}))

		got, err := e.client.Reserve(context.Background(), e.ledgerShapedRequest(false))
		require.NoError(t, err)
		assert.True(t, got.GetDenied())
		assert.Equal(t, string(model.DecisionReview), got.GetDecision())
		assert.Equal(t, "rule_evaluation_error", got.GetReason())
		assert.Equal(t, []string{ruleID.String()}, got.GetMatchedRuleIds())
	})

	t.Run("revert skips the rules and still resolves limits", func(t *testing.T) {
		e := newReserveE2E(t)

		// No evaluator expectation: a rule evaluation on a revert fails the test.
		e.resolver.EXPECT().
			ResolveReservations(gomock.Any(), gomock.Any()).
			Return(nil, false, nil)

		got, err := e.client.Reserve(context.Background(), e.ledgerShapedRequest(true))
		require.NoError(t, err)
		assert.False(t, got.GetDenied())
		assert.Equal(t, string(model.DecisionAllow), got.GetDecision())
	})

	t.Run("replay onto a settled row reaches the client as FailedPrecondition 0533", func(t *testing.T) {
		e := newReserveE2E(t)

		e.evaluator.EXPECT().
			Execute(gomock.Any(), gomock.Any()).
			Return(&model.EvaluationResult{Decision: model.DecisionAllow}, nil)
		e.resolver.EXPECT().
			ResolveReservations(gomock.Any(), gomock.Any()).
			Return([]query.ReservationSpec{{
				LimitID:   testutil.MustDeterministicUUID(9105),
				ScopeKey:  "acct:9103",
				PeriodKey: "2026-06",
				Amount:    decimal.NewFromInt(100),
				MaxAmount: decimal.NewFromInt(10000),
			}}, false, nil)

		e.expectTxRollback()
		e.repo.EXPECT().AcquireReserveScopeLock(gomock.Any(), e.tx, gomock.Any()).Return(nil)
		e.repo.EXPECT().
			ReserveWithTx(gomock.Any(), e.tx, gomock.Any(), gomock.Any()).
			Return(false, constant.ErrReservationAlreadySettled)
		// No audit expectation: a settled replay writes no row.

		_, err := e.client.Reserve(context.Background(), e.ledgerShapedRequest(false))
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		assert.Equal(t, constant.ErrReservationAlreadySettled.Error(), status.Convert(err).Message())
	})
}

// TestReservationE2E_ConfirmOutcomesReachTheClient drives the two confirm RPCs
// through the real server and service over bufconn, so the proto response
// fields are proven populated from the service outcome, not from the mock.
func TestReservationE2E_ConfirmOutcomesReachTheClient(t *testing.T) {
	transactionID := testutil.MustDeterministicUUID(9201)
	reservationID := testutil.MustDeterministicUUID(9202)

	t.Run("confirm by transaction carries confirmed and already_released", func(t *testing.T) {
		e := newReserveE2E(t)

		confirmed, err := model.NewReservation(
			testutil.MustDeterministicUUID(9203), transactionID, "acct:9103", "2026-06",
			decimal.NewFromInt(100), e.now.Add(5*time.Minute), e.now,
		)
		require.NoError(t, err)

		e.expectTxCommit()
		e.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), e.tx, transactionID).
			Return([]*model.Reservation{confirmed}, nil)
		e.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), e.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, confirmed.ID, gomock.Any()).
			Return(nil)
		e.repo.EXPECT().
			CountReleasedByTransactionWithTx(gomock.Any(), e.tx, transactionID).
			Return(1, nil)

		got, err := e.client.ConfirmByTransaction(context.Background(), &reservationv1.ConfirmByTransactionRequest{TransactionId: transactionID.String()})
		require.NoError(t, err)
		assert.EqualValues(t, 1, got.GetConfirmed())
		assert.EqualValues(t, 1, got.GetAlreadyReleased())
	})

	t.Run("confirm by id on a RELEASED row carries already_released", func(t *testing.T) {
		e := newReserveE2E(t)

		e.expectTxRollback()
		e.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), e.tx, reservationID).
			Return(model.StatusReleased, constant.ErrReservationAlreadyTerminal)

		got, err := e.client.ConfirmById(context.Background(), &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
		require.NoError(t, err)
		assert.True(t, got.GetAlreadyReleased())
	})

	t.Run("confirm by id that settles the row is not already_released", func(t *testing.T) {
		e := newReserveE2E(t)

		e.expectTxCommit()
		e.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), e.tx, reservationID).
			Return(model.StatusReserved, nil)
		e.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), e.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, reservationID, gomock.Any()).
			Return(nil)

		got, err := e.client.ConfirmById(context.Background(), &reservationv1.ConfirmByIdRequest{ReservationId: reservationID.String()})
		require.NoError(t, err)
		assert.False(t, got.GetAlreadyReleased())
	})
}
