// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/reservationlock"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestReserveAdmissionRequiresDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := NewReserveAdmissionCommand(ReserveAdmissionDependencies{Decisions: mocks.NewMockReserveAdmissionDecisions(ctrl)}, clock.NewFixedClock(testutil.FixedTime()), ReserveAdmissionConfig{})
	require.Error(t, err)
}

func admissionUnitRequest() tracercontract.ReserveRequest {
	blocked, longLived := false, false
	account := testutil.MustDeterministicUUID(89901)
	asset := "TOKEN"
	return tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(89902), RequestID: testutil.MustDeterministicUUID(89903), ContextID: "official", ValidationMode: tracercontract.ValidationLimits, TransactionTimestamp: testutil.FixedTime(), LongLived: &longLived, Amount: "10.125", Asset: asset, Context: tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}}
}

func TestReserveAdmissionTransactionFailures(t *testing.T) {
	failure := errors.New("injected admission failure")
	for _, stage := range []string{"success", "read", "begin", "lock", "second read", "account lock", "limits", "create", "schedule expiry", "audit", "commit", "past", "future"} {
		t.Run(stage, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			decisions := mocks.NewMockReserveAdmissionDecisions(ctrl)
			operations := mocks.NewMockReserveAdmissionOperations(ctrl)
			capacity := mocks.NewMockReserveAdmissionCapacity(ctrl)
			limits := mocks.NewMockReserveAdmissionLimits(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			deps := ReserveAdmissionDependencies{Decisions: decisions, Operations: operations, Capacity: capacity, Limits: limits, Policies: mocks.NewMockReserveAdmissionPolicies(ctrl), Evaluator: mocks.NewMockReserveAdmissionEvaluator(ctrl), Audit: audit, Transactions: beginner}
			config := ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: true, MaxTimestampAge: 24 * time.Hour, ReservationLifetime: time.Hour, LongLivedLifetime: 720 * time.Hour}
			c, err := NewReserveAdmissionCommand(deps, clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			request := admissionUnitRequest()
			want := failure
			if stage == "past" {
				request.TransactionTimestamp = request.TransactionTimestamp.Add(-24 * time.Hour)
				want = constant.ErrValidationTimestampPast
			}
			if stage == "future" {
				request.TransactionTimestamp = request.TransactionTimestamp.Add(time.Second)
				want = constant.ErrValidationTimestampFuture
			}
			key := model.ReserveOperationKey{IntegrationID: "verified-producer", TransactionID: request.TransactionID, RequestID: request.RequestID}
			var calls []any
			stepErr := func(name string) error {
				if name == stage {
					return failure
				}
				return nil
			}
			calls = append(calls, decisions.EXPECT().Get(gomock.Any(), key).Return(nil, stepErr("read")))
			if stage != "read" {
				calls = append(calls, beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, stepErr("begin")))
				if stage != "begin" {
					calls = append(calls, operations.EXPECT().LockWithTx(gomock.Any(), tx, key.Identity()).Return(&model.ReserveOperationState{Status: model.OperationOpen}, stepErr("lock")))
					if stage != "lock" {
						calls = append(calls, decisions.EXPECT().GetWithTx(gomock.Any(), tx, key).Return(nil, stepErr("second read")))
						if stage != "second read" && stage != "past" && stage != "future" {
							calls = append(calls, capacity.EXPECT().AcquireReserveScopeLock(gomock.Any(), tx, reservationlock.AccountKey(request.Context.Accounts[0].ID)).Return(stepErr("account lock")))
							if stage != "account lock" {
								calls = append(calls, limits.EXPECT().ListCandidatesWithTx(gomock.Any(), tx, []string{"TOKEN"}, []uuid.UUID{request.Context.Accounts[0].ID}).Return(nil, stepErr("limits")))
								if stage != "limits" {
									calls = append(calls, decisions.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.Tx, d model.ReserveDecision) error {
										require.NoError(t, d.Validate(10, 100))
										require.Equal(t, tracercontract.DecisionAllow, d.Result.Decision)
										require.Empty(t, d.Result.ReservationIDs)
										return stepErr("create")
									}))
									if stage != "create" {
										// A decision that reserved nothing still expires, with the direct TTL.
										calls = append(calls, operations.EXPECT().ScheduleExpiryWithTx(gomock.Any(), tx, key.Identity(), testutil.FixedTime().Add(time.Hour)).Return(stepErr("schedule expiry")))
									}
									if stage != "create" && stage != "schedule expiry" {
										calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, e *model.AuditEvent) error {
											require.Equal(t, model.ResourceTypeReserveOperation, e.ResourceType)
											require.Equal(t, model.AuditResultAllow, e.Result)
											return stepErr("audit")
										}))
										if stage != "audit" {
											calls = append(calls, tx.EXPECT().Commit().Return(stepErr("commit")))
										}
									}
								}
							}
						}
					}
					if stage != "success" {
						calls = append(calls, tx.EXPECT().Rollback().Return(nil))
					}
				}
			}
			gomock.InOrder(calls...)
			got, err := c.Execute(completionAuth(t.Context()), request)
			if stage == "success" {
				require.NoError(t, err)
				require.Equal(t, tracercontract.DecisionAllow, got.Decision)
			} else {
				require.ErrorIs(t, err, want)
				require.Nil(t, got)
			}
		})
	}
}

func TestReserveAdmissionCandidateCodesFollowTheAssetCodeRule(t *testing.T) {
	t.Parallel()
	blocked := false
	token, legacy, other := testutil.MustDeterministicUUID(89911), testutil.MustDeterministicUUID(89912), testutil.MustDeterministicUUID(89913)
	for _, tc := range []struct {
		name   string
		assets map[uuid.UUID]string
		want   []string
	}{
		{"non-conforming stored code selects nothing", map[uuid.UUID]string{legacy: "usdt"}, []string{}},
		{"conforming codes are sorted", map[uuid.UUID]string{token: "XBT", legacy: "usdt", other: "BTC"}, []string{"BTC", "XBT"}},
		{"repeated conforming codes are distinct", map[uuid.UUID]string{token: "TOKEN", legacy: "usdt", other: "TOKEN"}, []string{"TOKEN"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			decisions := mocks.NewMockReserveAdmissionDecisions(ctrl)
			operations := mocks.NewMockReserveAdmissionOperations(ctrl)
			capacity := mocks.NewMockReserveAdmissionCapacity(ctrl)
			limits := mocks.NewMockReserveAdmissionLimits(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			deps := ReserveAdmissionDependencies{Decisions: decisions, Operations: operations, Capacity: capacity, Limits: limits, Policies: mocks.NewMockReserveAdmissionPolicies(ctrl), Evaluator: mocks.NewMockReserveAdmissionEvaluator(ctrl), Audit: audit, Transactions: beginner}
			config := ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: true, MaxTimestampAge: 24 * time.Hour, ReservationLifetime: time.Hour, LongLivedLifetime: 720 * time.Hour}
			c, err := NewReserveAdmissionCommand(deps, clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)
			request := admissionUnitRequest()
			request.Context = tracercontract.Context{}
			ids := make([]uuid.UUID, 0, len(tc.assets))
			for _, id := range []uuid.UUID{token, legacy, other} {
				asset, ok := tc.assets[id]
				if !ok {
					continue
				}
				ids = append(ids, id)
				request.Context.Accounts = append(request.Context.Accounts, tracercontract.Account{ID: id, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
				request.Context.Entries = append(request.Context.Entries, tracercontract.Entry{AccountID: id, Direction: tracercontract.Debit, Amount: "1", Asset: asset})
			}
			request.Amount = "1"
			decisions.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, nil)
			beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
			operations.EXPECT().LockWithTx(gomock.Any(), tx, gomock.Any()).Return(&model.ReserveOperationState{Status: model.OperationOpen}, nil)
			decisions.EXPECT().GetWithTx(gomock.Any(), tx, gomock.Any()).Return(nil, nil)
			capacity.EXPECT().AcquireReserveScopeLock(gomock.Any(), tx, gomock.Any()).Return(nil).Times(len(ids))
			limits.EXPECT().ListCandidatesWithTx(gomock.Any(), tx, tc.want, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.Tx, _ []string, got []uuid.UUID) ([]model.ContextAccountLimit, error) {
				require.ElementsMatch(t, ids, got)
				return []model.ContextAccountLimit{}, nil
			})
			decisions.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).Return(nil)
			operations.EXPECT().ScheduleExpiryWithTx(gomock.Any(), tx, gomock.Any(), gomock.Any()).Return(nil)
			audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).Return(nil)
			tx.EXPECT().Commit().Return(nil)
			got, err := c.Execute(completionAuth(t.Context()), request)
			require.NoError(t, err)
			require.Equal(t, tracercontract.DecisionAllow, got.Decision)
		})
	}
}

func TestReserveAdmissionReservationLifetimeFollowsLongLived(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	deps := ReserveAdmissionDependencies{
		Decisions: mocks.NewMockReserveAdmissionDecisions(ctrl), Operations: mocks.NewMockReserveAdmissionOperations(ctrl), Capacity: mocks.NewMockReserveAdmissionCapacity(ctrl),
		Limits: mocks.NewMockReserveAdmissionLimits(ctrl), Policies: mocks.NewMockReserveAdmissionPolicies(ctrl), Evaluator: mocks.NewMockReserveAdmissionEvaluator(ctrl),
		Audit: mocks.NewMockAuditEventRepository(ctrl), Transactions: dbmocks.NewMockTxBeginner(ctrl),
	}
	config := ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: true, MaxTimestampAge: 24 * time.Hour, ReservationLifetime: 5 * time.Minute}
	_, err := NewReserveAdmissionCommand(deps, clock.NewFixedClock(testutil.FixedTime()), config)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody, "a long-lived lifetime is required")
	config.LongLivedLifetime = 720 * time.Hour
	c, err := NewReserveAdmissionCommand(deps, clock.NewFixedClock(testutil.FixedTime()), config)
	require.NoError(t, err)
	direct, pending := false, true
	request := admissionUnitRequest()
	request.LongLived = &direct
	require.Equal(t, 5*time.Minute, c.lifetime(request))
	request.LongLived = &pending
	require.Equal(t, 720*time.Hour, c.lifetime(request))
}

func TestReserveAdmissionReplayConflictsOnceTheOperationExpired(t *testing.T) {
	t.Parallel()
	config := ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: true, MaxTimestampAge: 24 * time.Hour, ReservationLifetime: time.Hour, LongLivedLifetime: 720 * time.Hour}
	request := admissionUnitRequest()
	key := model.ReserveOperationKey{IntegrationID: "verified-producer", TransactionID: request.TransactionID, RequestID: request.RequestID}

	stored := func(t *testing.T) model.ReserveDecision {
		t.Helper()
		ctrl := gomock.NewController(t)
		decisions := mocks.NewMockReserveAdmissionDecisions(ctrl)
		operations := mocks.NewMockReserveAdmissionOperations(ctrl)
		capacity := mocks.NewMockReserveAdmissionCapacity(ctrl)
		limits := mocks.NewMockReserveAdmissionLimits(ctrl)
		audit := mocks.NewMockAuditEventRepository(ctrl)
		beginner := dbmocks.NewMockTxBeginner(ctrl)
		tx := dbmocks.NewMockTx(ctrl)
		c, err := NewReserveAdmissionCommand(ReserveAdmissionDependencies{Decisions: decisions, Operations: operations, Capacity: capacity, Limits: limits, Policies: mocks.NewMockReserveAdmissionPolicies(ctrl), Evaluator: mocks.NewMockReserveAdmissionEvaluator(ctrl), Audit: audit, Transactions: beginner}, clock.NewFixedClock(testutil.FixedTime()), config)
		require.NoError(t, err)

		var created model.ReserveDecision
		decisions.EXPECT().Get(gomock.Any(), key).Return(nil, nil)
		beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
		operations.EXPECT().LockWithTx(gomock.Any(), tx, key.Identity()).Return(&model.ReserveOperationState{Status: model.OperationOpen}, nil)
		decisions.EXPECT().GetWithTx(gomock.Any(), tx, key).Return(nil, nil)
		capacity.EXPECT().AcquireReserveScopeLock(gomock.Any(), tx, gomock.Any()).Return(nil)
		limits.EXPECT().ListCandidatesWithTx(gomock.Any(), tx, gomock.Any(), gomock.Any()).Return([]model.ContextAccountLimit{}, nil)
		decisions.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.Tx, d model.ReserveDecision) error {
			created = d
			return nil
		})
		operations.EXPECT().ScheduleExpiryWithTx(gomock.Any(), tx, key.Identity(), gomock.Any()).Return(nil)
		audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).Return(nil)
		tx.EXPECT().Commit().Return(nil)
		_, err = c.Execute(completionAuth(t.Context()), request)
		require.NoError(t, err)

		return created
	}

	for _, tc := range []struct {
		name     string
		preLock  bool
		status   model.ReserveOperationStatus
		wantErr  error
		mismatch bool
	}{
		{name: "pre-lock replay of an open operation returns the decision", preLock: true, status: model.OperationOpen},
		{name: "pre-lock replay of a confirmed operation returns the decision", preLock: true, status: model.OperationConfirmed},
		{name: "pre-lock replay of an expired operation conflicts", preLock: true, status: model.OperationExpired, wantErr: constant.ErrReserveOperationConflict},
		{name: "locked replay of an expired operation conflicts", status: model.OperationExpired, wantErr: constant.ErrReserveOperationConflict},
		{name: "mismatched pre-lock replay is rejected without the lock", preLock: true, mismatch: true, wantErr: constant.ErrReserveDecisionConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decision := stored(t)
			ctrl := gomock.NewController(t)
			decisions := mocks.NewMockReserveAdmissionDecisions(ctrl)
			operations := mocks.NewMockReserveAdmissionOperations(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			c, err := NewReserveAdmissionCommand(ReserveAdmissionDependencies{Decisions: decisions, Operations: operations, Capacity: mocks.NewMockReserveAdmissionCapacity(ctrl), Limits: mocks.NewMockReserveAdmissionLimits(ctrl), Policies: mocks.NewMockReserveAdmissionPolicies(ctrl), Evaluator: mocks.NewMockReserveAdmissionEvaluator(ctrl), Audit: mocks.NewMockAuditEventRepository(ctrl), Transactions: beginner}, clock.NewFixedClock(testutil.FixedTime()), config)
			require.NoError(t, err)

			replayed := request
			if tc.mismatch {
				replayed.ContextID = "other"
			}

			if tc.preLock {
				decisions.EXPECT().Get(gomock.Any(), key).Return(&decision, nil)
			} else {
				decisions.EXPECT().Get(gomock.Any(), key).Return(nil, nil)
			}

			if !tc.mismatch {
				completedAt := testutil.FixedTime()
				state := &model.ReserveOperationState{Status: tc.status}
				if tc.status != model.OperationOpen {
					state.CompletedAt = &completedAt
				}
				beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
				operations.EXPECT().LockWithTx(gomock.Any(), tx, key.Identity()).Return(state, nil)
				decisions.EXPECT().GetWithTx(gomock.Any(), tx, key).Return(&decision, nil)
				if tc.wantErr == nil {
					tx.EXPECT().Commit().Return(nil)
				} else {
					tx.EXPECT().Rollback().Return(nil)
				}
			}

			got, err := c.Execute(completionAuth(t.Context()), replayed)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, got)

				return
			}

			require.NoError(t, err)
			require.Equal(t, decision.Result.EvaluationID, got.EvaluationID)
			require.Equal(t, tracercontract.DecisionAllow, got.Decision)
		})
	}
}
