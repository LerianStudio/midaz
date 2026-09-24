// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestReserveDecisionRepositoryRejectsBeforeDatabase(t *testing.T) {
	t.Parallel()
	conn := mocks.NewMockConnection(gomock.NewController(t))
	_, err := NewReserveDecisionRepository(nil, 10, 100)
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	_, err = NewReserveDecisionRepository(conn, 0, 100)
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	repo, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	key := model.ReserveOperationKey{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(73001), RequestID: testutil.MustDeterministicUUID(73002)}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = repo.Get(ctx, key)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.GetWithTx(ctx, nil, key)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, repo.CreateWithTx(ctx, nil, model.ReserveDecision{}), context.Canceled)
	_, err = repo.Get(t.Context(), model.ReserveOperationKey{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	_, err = repo.GetWithTx(t.Context(), nil, key)
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	require.ErrorIs(t, repo.CreateWithTx(t.Context(), nil, model.ReserveDecision{}), pgdb.ErrNilConnection)
	tx := mocks.NewMockTx(gomock.NewController(t))
	require.ErrorIs(t, repo.CreateWithTx(t.Context(), tx, model.ReserveDecision{}), constant.ErrInvalidRequestBody)
	_, err = repo.GetWithTx(t.Context(), tx, model.ReserveOperationKey{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	for _, cause := range []error{nil, context.DeadlineExceeded} {
		conn.EXPECT().GetDB(gomock.Any()).Return(nil, cause)
		_, err = repo.Get(t.Context(), key)
		if cause == nil {
			require.ErrorIs(t, err, pgdb.ErrNilConnection)
		} else {
			require.ErrorIs(t, err, cause)
		}
	}
}

func TestReserveDecisionRepositoryLeavesTransactionToCaller(t *testing.T) {
	t.Parallel()
	failure := errors.New("database write failed")
	for _, tt := range []struct {
		name    string
		result  sql.Result
		failure error
		want    error
	}{
		{"inserted", sqlmock.NewResult(0, 1), nil, nil},
		{"duplicate", sqlmock.NewResult(0, 0), nil, constant.ErrReserveDecisionConflict},
		{"write failure", nil, failure, failure},
		{"rows affected failure", sqlmock.NewErrorResult(failure), nil, failure},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo, err := NewReserveDecisionRepository(mocks.NewMockConnection(ctrl), 10, 100)
			require.NoError(t, err)
			tx := mocks.NewMockTx(ctrl)
			d := model.ReserveDecision{Key: model.ReserveOperationKey{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(73001), RequestID: testutil.MustDeterministicUUID(73002)}, ContextID: "ledger", ValidationMode: tracercontract.ValidationLimits, CreatedAt: testutil.FixedTime(), Result: tracercontract.ReserveResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(73001), EvaluationID: testutil.MustDeterministicUUID(73003), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}}}
			tx.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(tt.result, tt.failure)
			require.ErrorIs(t, repo.CreateWithTx(t.Context(), tx, d), tt.want)
			// No Commit or Rollback expectation: the repository must leave audit and
			// capacity changes in the same transaction owned by the use case.
		})
	}
}
