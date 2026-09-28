// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	dbmocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestExpireReserveOperationTransactionAndFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("injected expiry failure")
	for _, stage := range []string{"success", "replay", "completed first", "begin", "operation", "read", "no decision", "capacity", "missing capacity", "audit", "commit"} {
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			operations := mocks.NewMockReserveOperationCompleter(ctrl)
			decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
			capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
			audit := mocks.NewMockAuditEventRepository(ctrl)
			beginner := dbmocks.NewMockTxBeginner(ctrl)
			tx := dbmocks.NewMockTx(ctrl)
			c, err := NewExpireReserveOperationCommand(operations, decisions, capacity, audit, beginner, ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
			require.NoError(t, err)
			d, res := completionDecision()
			key := d.Key.Identity()
			at := testutil.FixedTime().Add(time.Hour)
			state := &model.ReserveOperationState{Status: model.OperationExpired, CompletedAt: &at}
			want := failure
			if stage == "no decision" || stage == "missing capacity" {
				want = constant.ErrInternalServer
			}
			calls := []any{}
			if stage == "begin" {
				calls = append(calls, beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(nil, failure))
			} else {
				calls = append(calls, beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil))
				switch stage {
				case "operation":
					calls = append(calls, operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationExpired, at).Return(nil, false, failure))
				case "completed first":
					calls = append(calls, operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationExpired, at).Return(nil, false, constant.ErrReserveOperationConflict))
				default:
					calls = append(calls, operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationExpired, at).Return(state, stage != "replay", nil))
				}
				if stage != "operation" && stage != "completed first" && stage != "replay" {
					readErr, decision := error(nil), d
					if stage == "read" {
						readErr = failure
					}
					if stage == "no decision" {
						decision = nil
					}
					calls = append(calls, decisions.EXPECT().GetByOperationWithTx(gomock.Any(), tx, key).Return(decision, readErr))
					if stage != "read" && stage != "no decision" {
						capacityErr, rows := error(nil), []*model.Reservation{res}
						if stage == "capacity" {
							capacityErr = failure
						}
						if stage == "missing capacity" {
							rows = nil
						}
						calls = append(calls, capacity.EXPECT().SettleDecisionWithTx(gomock.Any(), tx, d.Result.EvaluationID, model.StatusExpired).Return(rows, capacityErr))
						if stage != "capacity" && stage != "missing capacity" {
							auditErr := error(nil)
							if stage == "audit" {
								auditErr = failure
							}
							calls = append(calls, audit.EXPECT().InsertWithTx(gomock.Any(), tx, gomock.Any()).DoAndReturn(func(_ context.Context, _ pgdb.DB, e *model.AuditEvent) error {
								require.Equal(t, model.AuditEventOperationExpired, e.EventType)
								require.Equal(t, model.AuditActionExpire, e.Action)
								require.Equal(t, model.ResourceTypeReserveOperation, e.ResourceType)
								require.Equal(t, key.TransactionID.String(), e.ResourceID)
								require.Equal(t, model.ActorTypeSystem, e.Actor.ActorType)
								require.Equal(t, systemActorID, e.Actor.ID, "the tracer, not the producer, expires the operation")
								require.Equal(t, at, e.CreatedAt)
								require.Equal(t, key.IntegrationID, e.Context["integrationId"])
								require.Equal(t, key.TransactionID, e.Context["transactionId"])
								require.Equal(t, d.Result.EvaluationID, e.Context["evaluationId"])
								require.Equal(t, model.OperationExpired, e.Context["status"])
								details, ok := e.Context["reservations"].([]ReserveCompletionReservation)
								require.True(t, ok)
								require.Len(t, details, 1)
								require.Equal(t, res.ID, details[0].ID)
								require.Equal(t, model.StatusReserved, details[0].Before)
								require.Equal(t, model.StatusExpired, details[0].After)
								return auditErr
							}))
						}
					}
				}
				success := stage == "success" || stage == "replay" || stage == "completed first"
				if success || stage == "commit" {
					commitErr := error(nil)
					if stage == "commit" {
						commitErr = failure
					}
					calls = append(calls, tx.EXPECT().Commit().Return(commitErr))
				}
				if !success {
					calls = append(calls, tx.EXPECT().Rollback().Return(nil))
				}
			}
			gomock.InOrder(calls...)
			expired, err := c.Execute(t.Context(), key, at)
			switch stage {
			case "success":
				require.NoError(t, err)
				require.Equal(t, 1, expired, "the count is the reservations moved")
			case "replay", "completed first":
				require.NoError(t, err, "a terminal operation is left untouched")
				require.Zero(t, expired)
			default:
				require.ErrorIs(t, err, want)
				require.Zero(t, expired)
			}
		})
	}
}

func TestExpireReserveOperationRejectsBeforeTransaction(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	operations := mocks.NewMockReserveOperationCompleter(ctrl)
	decisions := mocks.NewMockReserveOperationDecisionReader(ctrl)
	capacity := mocks.NewMockDecisionCapacitySettler(ctrl)
	audit := mocks.NewMockAuditEventRepository(ctrl)
	beginner := dbmocks.NewMockTxBeginner(ctrl)
	config := ReserveCompletionConfig{MaxRules: 10, MaxReservations: 100}

	_, err := NewExpireReserveOperationCommand(nil, decisions, capacity, audit, beginner, config)
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	_, err = NewExpireReserveOperationCommand(operations, decisions, capacity, audit, beginner, ReserveCompletionConfig{MaxRules: 10})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)

	c, err := NewExpireReserveOperationCommand(operations, decisions, capacity, audit, beginner, config)
	require.NoError(t, err)
	d, _ := completionDecision()
	key, at := d.Key.Identity(), testutil.FixedTime()

	_, err = c.Execute(t.Context(), model.ReserveOperationIdentity{IntegrationID: key.IntegrationID, TransactionID: uuid.Nil}, at)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	_, err = c.Execute(t.Context(), key, time.Time{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = c.Execute(canceled, key, at)
	require.ErrorIs(t, err, context.Canceled)
	// Multi-tenant expiry must run for the reaper's tenant on the tenant
	// database it resolved.
	_, err = c.Execute(tmcore.ContextWithPG(t.Context(), dbresolver.New(dbresolver.WithPrimaryDBs(&sql.DB{}))), key, at)
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	_, err = c.Execute(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), key, at)
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext)

	tx := dbmocks.NewMockTx(ctrl)
	beginner.EXPECT().BeginTx(gomock.Any(), nil).Return(tx, nil)
	operations.EXPECT().CompleteWithTx(gomock.Any(), tx, key, model.OperationExpired, at).Return(nil, false, constant.ErrReserveOperationConflict)
	tx.EXPECT().Commit().Return(nil)
	tenant := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(&sql.DB{})))
	expired, err := c.Execute(tenant, key, at)
	require.NoError(t, err)
	require.Zero(t, expired)
}
