// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestDecisionCapacityRejectsBeforeDatabase(t *testing.T) {
	t.Parallel()
	repo := NewUsageReservationRepositoryWithConnection(NewUsageCounterRepositoryWithConnection(nil))
	tx := mocks.NewMockTx(gomock.NewController(t))
	at := testutil.FixedTime()
	id := testutil.MustDeterministicUUID(76001)
	res := &model.Reservation{
		ID: id, TransactionID: id, LimitID: id, Amount: decimal.NewFromInt(1),
		Status: model.StatusReserved, ScopeKey: "account", PeriodKey: "2026-09", CreatedAt: at, ReservationExpiresAt: at,
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(canceled, tx, id, res, decimal.NewFromInt(100), at), context.Canceled)
	_, err := repo.SettleDecisionWithTx(canceled, tx, id, model.StatusConfirmed)
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), nil, id, res, decimal.NewFromInt(100), at), pgdb.ErrNilConnection)
	for _, mutate := range []func(*model.Reservation){
		func(r *model.Reservation) { r.ID = uuid.Nil },
		func(r *model.Reservation) { r.Amount = decimal.Zero },
		func(r *model.Reservation) { r.Amount = decimal.NewFromInt(-1) },
		func(r *model.Reservation) { r.Status = model.StatusConfirmed },
		func(r *model.Reservation) { r.TransactionID = uuid.Nil },
		func(r *model.Reservation) { r.CreatedAt = time.Time{} },
		func(r *model.Reservation) { r.ConfirmedAt = &at },
	} {
		bad := *res
		mutate(&bad)
		require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), tx, id, &bad, decimal.NewFromInt(100), at), constant.ErrInvalidRequestBody)
	}
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), tx, id, res, decimal.Zero, at), constant.ErrUsageCounterExceedsLimit)
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), tx, uuid.Nil, res, decimal.NewFromInt(100), at), constant.ErrInvalidRequestBody)
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), tx, id, res, decimal.NewFromInt(100), time.Time{}), constant.ErrInvalidRequestBody)
	_, err = repo.SettleDecisionWithTx(t.Context(), nil, id, model.StatusConfirmed)
	require.ErrorIs(t, err, pgdb.ErrNilConnection)
	for _, status := range []model.ReservationStatus{model.StatusExpired, model.StatusReserved, ""} {
		_, err := repo.SettleDecisionWithTx(t.Context(), tx, id, status)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
	}
	// A duplicate is a conflict, not a second reserve or an implicit replay.
	tx.EXPECT().ExecContext(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, &pgconn.PgError{Code: "23505"})
	require.ErrorIs(t, repo.ReserveForDecisionWithTx(t.Context(), tx, id, res, decimal.NewFromInt(100), at), constant.ErrReserveDecisionConflict)
}
