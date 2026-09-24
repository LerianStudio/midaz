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
	asset := tracercontract.AssetRef{Namespace: "assets", ID: "official", Code: "TOKEN"}
	return tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(89902), RequestID: testutil.MustDeterministicUUID(89903), ContextID: "official", ValidationMode: tracercontract.ValidationLimits, TransactionTimestamp: testutil.FixedTime(), LongLived: &longLived, Amount: "10.125", Asset: asset, Context: tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}}
}

func TestReserveAdmissionTransactionFailures(t *testing.T) {
	failure := errors.New("injected admission failure")
	for _, stage := range []string{"success", "read", "begin", "lock", "second read", "account lock", "limits", "create", "audit", "commit", "past", "future"} {
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
			config := ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: true, MaxTimestampAge: 24 * time.Hour, ReservationLifetime: time.Hour}
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
								calls = append(calls, limits.EXPECT().ListCandidatesWithTx(gomock.Any(), tx, "assets", []uuid.UUID{request.Context.Accounts[0].ID}).Return(nil, stepErr("limits")))
								if stage != "limits" {
									calls = append(calls, decisions.EXPECT().CreateWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.Tx, d model.ReserveDecision) error {
										require.NoError(t, d.Validate(10, 100))
										require.Equal(t, tracercontract.DecisionAllow, d.Result.Decision)
										require.Empty(t, d.Result.ReservationIDs)
										return stepErr("create")
									}))
									if stage != "create" {
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
