// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func completionDecision() (*model.ReserveDecision, *model.Reservation) {
	id := testutil.MustDeterministicUUID(78001)
	res := &model.Reservation{
		ID: testutil.MustDeterministicUUID(78002), LimitID: testutil.MustDeterministicUUID(78003), TransactionID: id,
		Status: model.StatusReserved, ScopeKey: "account", PeriodKey: "2026-09", Amount: decimal.RequireFromString("10.125"), ReservationExpiresAt: testutil.FixedTime(), CreatedAt: testutil.FixedTime(),
	}
	return &model.ReserveDecision{
		Key:       model.ReserveOperationKey{IntegrationID: "verified-producer", TransactionID: id, RequestID: testutil.MustDeterministicUUID(78004)},
		ContextID: "official-context", Fingerprint: sha256.Sum256([]byte("frozen")), ValidationMode: tracercontract.ValidationLimits, CreatedAt: testutil.FixedTime(),
		Result: tracercontract.ReserveResult{
			ContractRevision: tracercontract.ReserveContractRevision, TransactionID: id, EvaluationID: testutil.MustDeterministicUUID(78005),
			Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated},
			ReservationIDs: []uuid.UUID{res.ID}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied},
		},
	}, res
}

func completionAuth(ctx context.Context) context.Context {
	return contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: "verified-producer", AssetNamespace: "assets"})
}

func TestCompleteReserveOperationTransactionAndFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected transaction failure")
	for _, stage := range []string{"success", "begin", "operation", "read", "capacity", "audit", "commit", "replay", "missing capacity", "wrong owner"} {
		t.Run(stage, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			operations := mocks.NewMockReserveOperationCompleter(ctrl)
			decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
			capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			clk := clock.NewFixedClock(testutil.FixedTime())
			c, err := NewCompleteReserveOperationCommand(operations, decisions, capacity, audit, beginner, clk, ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
			require.NoError(t, err)
			d, res := completionDecision()
			key := d.Key.Identity()
			at := testutil.FixedTime()
			state := &model.ReserveOperationState{Status: model.OperationConfirmed, CompletedAt: &at}
			want := failure
			if stage == "missing capacity" || stage == "wrong owner" {
				want = constant.ErrInternalServer
			}
			if stage == "begin" {
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, failure)
			} else {
				calls := []any{beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)}
				operationErr := error(nil)
				if stage == "operation" {
					operationErr = failure
				}
				calls = append(calls, operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationConfirmed, at).Return(state, stage != "replay", operationErr))
				if operationErr == nil && stage != "replay" {
					readErr := error(nil)
					if stage == "read" {
						readErr = failure
					}
					if stage == "wrong owner" {
						d.Key.IntegrationID = "unrelated-producer"
					}
					calls = append(calls, decisions.EXPECT().GetByOperationWithTx(gomock.Any(), tx, key).Return(d, readErr))
					if readErr == nil && stage != "wrong owner" {
						capacityErr := error(nil)
						if stage == "capacity" {
							capacityErr = failure
						}
						rows := []*model.Reservation{res}
						if stage == "missing capacity" {
							rows = nil
						}
						calls = append(calls, capacity.EXPECT().SettleDecisionWithTx(gomock.Any(), tx, d.Result.EvaluationID, model.StatusConfirmed).Return(rows, capacityErr))
						if capacityErr == nil && stage != "missing capacity" {
							auditErr := error(nil)
							if stage == "audit" {
								auditErr = failure
							}
							calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, e *model.AuditEvent) error {
								require.Equal(t, model.AuditEventOperationConfirmed, e.EventType)
								require.Equal(t, model.ResourceTypeReserveOperation, e.ResourceType)
								require.Equal(t, key.IntegrationID, e.Actor.ID)
								require.Equal(t, model.ActorTypeSystem, e.Actor.ActorType)
								require.Equal(t, model.AuditResultSuccess, e.Result)
								require.Equal(t, at, e.CreatedAt)
								require.Equal(t, key.IntegrationID, e.Context["integrationId"])
								details, ok := e.Context["reservations"].([]ReserveCompletionReservation)
								require.True(t, ok)
								require.Len(t, details, 1)
								require.Equal(t, res.ID, details[0].ID)
								require.Equal(t, model.StatusReserved, details[0].Before)
								require.Equal(t, model.StatusConfirmed, details[0].After)
								return auditErr
							}))
						}
					}
				}
				if stage == "success" || stage == "replay" || stage == "commit" {
					commitErr := error(nil)
					if stage == "commit" {
						commitErr = failure
					}
					calls = append(calls, tx.EXPECT().Commit().Return(commitErr))
				}
				if stage != "success" && stage != "replay" {
					calls = append(calls, tx.EXPECT().Rollback().Return(nil))
				}
				gomock.InOrder(calls...)
			}
			got, err := c.Execute(completionAuth(t.Context()), key.TransactionID, model.OperationConfirmed)
			if stage == "success" || stage == "replay" {
				require.NoError(t, err)
				require.Equal(t, state, got)
				require.NotSame(t, state, got)
			} else {
				require.ErrorIs(t, err, want)
				require.Nil(t, got)
			}
		})
	}
}

func TestCompleteReserveOperationRejectsBeforeTransaction(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	operations := mocks.NewMockReserveOperationCompleter(ctrl)
	decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
	capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	c, err := NewCompleteReserveOperationCommand(operations, decisions, capacity, audit, beginner, clock.NewFixedClock(testutil.FixedTime()), ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
	require.NoError(t, err)
	d, _ := completionDecision()
	_, err = c.Execute(t.Context(), d.Key.TransactionID, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrInsufficientPrivileges)
	principal := contextutil.WithPrincipal(t.Context(), contextutil.Principal{ID: "admin", Type: "user"})
	_, err = c.Execute(principal, d.Key.TransactionID, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrInsufficientPrivileges, "administrative principal is not a verified producer")
	canceled, cancel := context.WithCancel(completionAuth(t.Context()))
	cancel()
	_, err = c.Execute(canceled, d.Key.TransactionID, model.OperationConfirmed)
	require.ErrorIs(t, err, context.Canceled)
	_, err = c.Execute(completionAuth(t.Context()), uuid.Nil, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	for _, status := range []model.ReserveOperationStatus{model.OperationOpen, "EXPIRED", ""} {
		_, err = c.Execute(completionAuth(t.Context()), d.Key.TransactionID, status)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
}

func TestCompleteReserveOperationReportDoesNotInventDecision(t *testing.T) {
	ctrl := gomock.NewController(t)
	operations := mocks.NewMockReserveOperationCompleter(ctrl)
	decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
	capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	tx := dbmocks.NewMockTx(ctrl)
	at := testutil.FixedTime()
	command, err := NewCompleteReserveOperationCommand(operations, decisions, capacity, audit, beginner, clock.NewFixedClock(at), ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
	require.NoError(t, err)
	id := testutil.MustDeterministicUUID(89701)
	key := model.ReserveOperationIdentity{IntegrationID: "verified-producer", TransactionID: id}
	gomock.InOrder(
		beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil),
		operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationReleased, at).Return(&model.ReserveOperationState{Status: model.OperationReleased, CompletedAt: &at}, false, nil),
		decisions.EXPECT().GetByOperationWithTx(gomock.Any(), tx, key).Return(nil, nil),
		tx.EXPECT().Commit().Return(nil),
	)
	result, err := command.ExecuteReport(completionAuth(t.Context()), id, model.OperationReleased)
	require.NoError(t, err)
	require.NoError(t, result.Validate())
	require.Nil(t, result.EvaluationID)
	require.Zero(t, result.Flipped)
}

func TestCompleteReserveOperationReportMovementAndReplay(t *testing.T) {
	for _, replay := range []bool{false, true} {
		t.Run(map[bool]string{false: "first completion", true: "replay"}[replay], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			operations := mocks.NewMockReserveOperationCompleter(ctrl)
			decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
			capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			at := testutil.FixedTime()
			cmd, err := NewCompleteReserveOperationCommand(operations, decisions, capacity, audit, beginner, clock.NewFixedClock(at), ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
			require.NoError(t, err)
			decision, reservation := completionDecision()
			key := decision.Key.Identity()
			calls := []any{
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil),
				operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationConfirmed, at).Return(&model.ReserveOperationState{Status: model.OperationConfirmed, CompletedAt: &at}, !replay, nil),
				decisions.EXPECT().GetByOperationWithTx(gomock.Any(), tx, key).Return(decision, nil),
			}
			if !replay {
				calls = append(calls, capacity.EXPECT().SettleDecisionWithTx(gomock.Any(), tx, decision.Result.EvaluationID, model.StatusConfirmed).Return([]*model.Reservation{reservation}, nil), audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).Return(nil))
			}
			calls = append(calls, tx.EXPECT().Commit().Return(nil))
			gomock.InOrder(calls...)
			result, err := cmd.ExecuteReport(completionAuth(t.Context()), key.TransactionID, model.OperationConfirmed)
			require.NoError(t, err)
			require.NotNil(t, result.EvaluationID)
			require.Equal(t, decision.Result.EvaluationID, *result.EvaluationID)
			require.Equal(t, map[bool]int{false: 1, true: 0}[replay], result.Flipped)
			require.NoError(t, result.Validate())
		})
	}
}
