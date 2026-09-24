// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestReserveOperationRepositoryRejectsBeforeWrite(t *testing.T) {
	t.Parallel()
	repo := NewReserveOperationRepository()
	identity := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: testutil.MustDeterministicUUID(74001)}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := repo.LockWithTx(ctx, nil, identity)
	require.ErrorIs(t, err, context.Canceled)
	_, _, err = repo.CompleteWithTx(ctx, nil, identity, model.OperationConfirmed, testutil.FixedTime())
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.LockWithTx(t.Context(), nil, identity)
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	tx := mocks.NewMockTx(gomock.NewController(t))
	_, err = repo.LockWithTx(t.Context(), tx, model.ReserveOperationIdentity{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	for _, state := range []model.ReserveOperationStatus{model.OperationOpen, "EXPIRED", ""} {
		_, _, err = repo.CompleteWithTx(t.Context(), tx, identity, state, testutil.FixedTime())
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
	_, _, err = repo.CompleteWithTx(t.Context(), tx, identity, model.OperationConfirmed, time.Time{})
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	// Failure to acquire the operation row must not continue into completion,
	// commit, or an automatic retry. The use case owns rollback.
	tx.EXPECT().ExecContext(gomock.Any(), gomock.Any(), identity.IntegrationID, identity.TransactionID).Return(nil, context.DeadlineExceeded)
	state, changed, err := repo.CompleteWithTx(t.Context(), tx, identity, model.OperationConfirmed, testutil.FixedTime())
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Nil(t, state)
	require.False(t, changed)
}
