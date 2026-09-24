// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"database/sql"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestIntegrationReservationAddressCompletesWholeOperation(t *testing.T) {
	for _, outcome := range []model.ReserveOperationStatus{model.OperationConfirmed, model.OperationReleased} {
		t.Run(string(outcome), func(t *testing.T) {
			db := completionDatabase(t)
			reporter, decisions, audit := completionCommand(t, db, true)
			capacity := newReservationRepoIntegration(db)
			limit := createTestLimitNamed(t, db, 88931, "address completion")
			decision := persistedDecision()
			first, second, legacy := decisionCapacity(limit, 88932), decisionCapacity(limit, 88933), decisionCapacity(limit, 88934)
			second.ScopeKey = "account:second"
			decision.Result.ReservationIDs = []uuid.UUID{first.ID, second.ID}
			require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
				if _, err := NewReserveOperationRepository().LockWithTx(t.Context(), tx, decision.Key.Identity()); err != nil {
					return err
				}
				for _, reservation := range []*model.Reservation{first, second} {
					if err := capacity.ReserveForDecisionWithTx(t.Context(), tx, decision.Result.EvaluationID, reservation, decimal.NewFromInt(100), testutil.FixedTime().Add(time.Hour)); err != nil {
						return err
					}
				}
				return decisions.CreateWithTx(t.Context(), tx, decision)
			}))
			require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
				return capacity.ReserveWithTx(t.Context(), tx, legacy, decimal.NewFromInt(100))
			}))
			cmd, err := command.NewCompleteReserveReservationCommand(decisions, reporter, true)
			require.NoError(t, err)
			_, err = cmd.Execute(completionContext(t.Context(), "foreign-producer"), first.ID, outcome)
			require.ErrorIs(t, err, constant.ErrReservationNotFound)
			ctx := completionContext(t.Context(), decision.Key.IntegrationID)
			_, err = cmd.Execute(ctx, legacy.ID, outcome)
			require.ErrorIs(t, err, constant.ErrReservationNotFound)
			result, err := cmd.Execute(ctx, first.ID, outcome)
			require.NoError(t, err)
			require.NoError(t, result.Validate())
			require.Equal(t, first.ID, result.ReservationID)
			require.Equal(t, decision.Result.EvaluationID, *result.EvaluationID)
			var settled int
			require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_reservations WHERE decision_id = $1 AND status = $2", decision.Result.EvaluationID, outcome).Scan(&settled))
			require.Equal(t, 2, settled, "a reservation address completes the entire coordinated operation")
			replay, err := cmd.Execute(ctx, second.ID, outcome)
			require.NoError(t, err)
			require.Equal(t, second.ID, replay.ReservationID)
			opposite := model.OperationReleased
			if outcome == opposite {
				opposite = model.OperationConfirmed
			}
			_, err = cmd.Execute(ctx, second.ID, opposite)
			require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
			events := completionEvents(t, db, decision.Key.TransactionID)
			require.Len(t, events, 1)
			verified, err := audit.VerifyHashChain(ctx, events[0])
			require.NoError(t, err)
			require.True(t, verified.IsValid)
			var legacyStatus string
			require.NoError(t, db.QueryRowContext(ctx, "SELECT status FROM usage_reservations WHERE id = $1", legacy.ID).Scan(&legacyStatus))
			require.Equal(t, string(model.StatusReserved), legacyStatus)
		})
	}
}

func TestIntegrationReservationOwnerTenantIsolation(t *testing.T) {
	a, b := completionDatabase(t, "a"), completionDatabase(t, "b")
	_, decisions, _ := completionCommand(t, a, true)
	limit := createTestLimitNamed(t, a, 88941, "owner tenant")
	decision, reservation := persistedDecision(), decisionCapacity(limit, 88942)
	persistDecisionCapacity(t, a, newReservationRepoIntegration(a), decisions, decision, reservation)
	conn := &pgdb.PostgresConnectionAdapter{}
	conn.SetMultiTenantEnabled(true)
	repo, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	ctxA := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(a)))
	ctxB := tmcore.ContextWithPG(tmcore.ContextWithTenantID(t.Context(), "tenant-b"), dbresolver.New(dbresolver.WithPrimaryDBs(b)))
	owner, err := repo.GetReservationOwner(ctxA, decision.Key.IntegrationID, reservation.ID)
	require.NoError(t, err)
	require.NotNil(t, owner)
	owner, err = repo.GetReservationOwner(ctxB, decision.Key.IntegrationID, reservation.ID)
	require.NoError(t, err)
	require.Nil(t, owner)
	_, err = repo.GetReservationOwner(t.Context(), decision.Key.IntegrationID, reservation.ID)
	require.Error(t, err)
}
