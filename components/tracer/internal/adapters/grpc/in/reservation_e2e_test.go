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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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
	client    reservationv1.ReservationServiceClient
	evaluator *servicesMocks.MockRuleEvaluator
	resolver  *servicesMocks.MockLimitResolver
	now       time.Time
}

func newReserveE2E(t *testing.T) *reserveE2E {
	t.Helper()

	ctrl := gomock.NewController(t)
	now := testutil.FixedTime()

	evaluator := servicesMocks.NewMockRuleEvaluator(ctrl)
	resolver := servicesMocks.NewMockLimitResolver(ctrl)

	svc, err := services.NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		resolver,
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
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
		client:    reservationv1.NewReservationServiceClient(conn),
		evaluator: evaluator,
		resolver:  resolver,
		now:       now,
	}
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
}
