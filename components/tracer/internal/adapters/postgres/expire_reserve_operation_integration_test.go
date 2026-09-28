// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func expireCommand(t *testing.T, db *sql.DB) *command.ExpireReserveOperationCommand {
	t.Helper()
	conn := &testutil.IntegrationDBAdapter{DB: db}
	decisions, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	c, err := command.NewExpireReserveOperationCommand(NewReserveOperationRepository(), decisions, newReservationRepoIntegration(db),
		NewAuditEventRepositoryWithConnection(conn), beginner, command.ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 100})
	require.NoError(t, err)
	return c
}

func operationEvents(t *testing.T, db *sql.DB, transactionID uuid.UUID, eventType model.AuditEventType) []uuid.UUID {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT event_id FROM audit_events WHERE resource_id=$1 AND event_type=$2 ORDER BY id", transactionID.String(), string(eventType))
	require.NoError(t, err)
	defer rows.Close()
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		require.NoError(t, rows.Scan(&id))
		ids = append(ids, id)
	}
	require.NoError(t, rows.Err())
	return ids
}

func readOperationState(t *testing.T, db *sql.DB, key model.ReserveOperationIdentity) (string, sql.NullTime) {
	t.Helper()
	var (
		status string
		at     sql.NullTime
	)
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT status, completed_at FROM reserve_operations WHERE integration_id=$1 AND transaction_id=$2",
		key.IntegrationID, key.TransactionID).Scan(&status, &at))
	return status, at
}

func TestIntegrationExpireReserveOperationReturnsCapacityAndFencesCompletion(t *testing.T) {
	db := completionDatabase(t)
	expire := expireCommand(t, db)
	completion, decisions, audit := completionCommand(t, db, true)
	limitID := createTestLimitNamed(t, db, 77501, "expiry")
	d, res := persistedDecision(), decisionCapacity(limitID, 77502)
	persistDecisionCapacity(t, db, newReservationRepoIntegration(db), decisions, d, res)
	key := d.Key.Identity()
	at := testutil.FixedTime().Add(time.Hour)

	expired, err := expire.Execute(t.Context(), key, at)
	require.NoError(t, err)
	require.Equal(t, 1, expired)
	require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, res.ID))
	cur, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
	require.True(t, cur.IsZero(), "expiry never consumes usage")
	require.True(t, held.IsZero())
	status, completedAt := readOperationState(t, db, key)
	require.Equal(t, string(model.OperationExpired), status)
	require.Equal(t, at, completedAt.Time.UTC())

	ids := operationEvents(t, db, key.TransactionID, model.AuditEventOperationExpired)
	require.Len(t, ids, 1)
	event, err := audit.GetByID(t.Context(), ids[0])
	require.NoError(t, err)
	require.Equal(t, model.AuditActionExpire, event.Action)
	require.Equal(t, model.ResourceTypeReserveOperation, event.ResourceType)
	require.Equal(t, model.ActorTypeSystem, event.Actor.ActorType)
	require.Equal(t, at, event.CreatedAt.UTC())
	body, err := json.Marshal(event.Context)
	require.NoError(t, err)
	var data command.ReserveCompletionAuditContext
	require.NoError(t, json.Unmarshal(body, &data))
	require.Equal(t, key.IntegrationID, data.IntegrationID)
	require.Equal(t, key.TransactionID, data.TransactionID)
	require.Equal(t, d.Result.EvaluationID, *data.EvaluationID)
	require.Equal(t, model.OperationExpired, data.Status)
	require.Len(t, data.Reservations, 1)
	require.Equal(t, res.ID, data.Reservations[0].ID)
	require.Equal(t, model.StatusExpired, data.Reservations[0].After)
	valid, err := audit.VerifyHashChain(t.Context(), ids[0])
	require.NoError(t, err)
	require.True(t, valid.IsValid)

	// A replay changes nothing and appends nothing.
	expired, err = expire.Execute(t.Context(), key, at.Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, expired)
	_, completedAt = readOperationState(t, db, key)
	require.Equal(t, at, completedAt.Time.UTC())

	// A late producer outcome contradicts the recorded expiry.
	ctx := completionContext(t.Context(), key.IntegrationID)
	for _, outcome := range []model.ReserveOperationStatus{model.OperationConfirmed, model.OperationReleased} {
		state, err := completion.Execute(ctx, key.TransactionID, outcome)
		require.ErrorIs(t, err, constant.ErrReserveOperationConflict, string(outcome))
		require.Nil(t, state)
	}
	require.Len(t, operationEvents(t, db, key.TransactionID, model.AuditEventOperationExpired), 1)
	require.Empty(t, operationEvents(t, db, key.TransactionID, model.AuditEventOperationConfirmed))
	require.Empty(t, operationEvents(t, db, key.TransactionID, model.AuditEventOperationReleased))
	cur, held = readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
	require.True(t, cur.IsZero())
	require.True(t, held.IsZero())
}

func TestIntegrationExpireReserveOperationLeavesCompletedOperation(t *testing.T) {
	db := completionDatabase(t)
	expire := expireCommand(t, db)
	completion, decisions, _ := completionCommand(t, db, true)
	limitID := createTestLimitNamed(t, db, 77601, "expiry-after-confirm")
	d, res := persistedDecision(), decisionCapacity(limitID, 77602)
	persistDecisionCapacity(t, db, newReservationRepoIntegration(db), decisions, d, res)
	key := d.Key.Identity()
	_, err := completion.Execute(completionContext(t.Context(), key.IntegrationID), key.TransactionID, model.OperationConfirmed)
	require.NoError(t, err)

	expired, err := expire.Execute(t.Context(), key, testutil.FixedTime().Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, expired, "a confirm that committed first wins")
	status, _ := readOperationState(t, db, key)
	require.Equal(t, string(model.OperationConfirmed), status)
	require.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))
	cur, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
	require.Equal(t, "10.125", cur.String())
	require.True(t, held.IsZero())
	require.Empty(t, operationEvents(t, db, key.TransactionID, model.AuditEventOperationExpired))
}

// TestIntegrationExpireReserveOperationRacesConfirm runs a producer confirm and
// the reaper's expiry of the same operation concurrently. The operation row
// lock serializes them: exactly one wins, the held amount leaves reserved_usage
// once, and exactly one operation audit event is written.
func TestIntegrationExpireReserveOperationRacesConfirm(t *testing.T) {
	for round := range 5 {
		t.Run(fmt.Sprintf("round-%d", round), func(t *testing.T) {
			db := completionDatabase(t)
			expire := expireCommand(t, db)
			completion, decisions, _ := completionCommand(t, db, true)
			limitID := createTestLimitNamed(t, db, 77701, "expiry-race")
			d, res := persistedDecision(), decisionCapacity(limitID, 77702)
			persistDecisionCapacity(t, db, newReservationRepoIntegration(db), decisions, d, res)
			key := d.Key.Identity()
			_, heldBefore := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
			require.True(t, heldBefore.Equal(res.Amount))

			var (
				wg                    sync.WaitGroup
				start                 = make(chan struct{})
				confirmErr, expireErr error
				expired               int
			)
			wg.Add(2)
			go func() {
				defer wg.Done()
				<-start
				_, confirmErr = completion.Execute(completionContext(t.Context(), key.IntegrationID), key.TransactionID, model.OperationConfirmed)
			}()
			go func() {
				defer wg.Done()
				<-start
				expired, expireErr = expire.Execute(t.Context(), key, testutil.FixedTime().Add(time.Hour))
			}()
			close(start)
			wg.Wait()

			require.NoError(t, expireErr)
			status, _ := readOperationState(t, db, key)
			cur, heldAfter := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
			require.True(t, heldBefore.Sub(heldAfter).Equal(res.Amount), "the held amount leaves reserved_usage exactly once")
			confirmed := operationEvents(t, db, key.TransactionID, model.AuditEventOperationConfirmed)
			expiredEvents := operationEvents(t, db, key.TransactionID, model.AuditEventOperationExpired)
			require.Len(t, append(confirmed, expiredEvents...), 1, "exactly one operation audit event")

			switch status {
			case string(model.OperationConfirmed):
				require.NoError(t, confirmErr)
				require.Zero(t, expired)
				require.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))
				require.True(t, cur.Equal(res.Amount))
				require.Len(t, confirmed, 1)
			case string(model.OperationExpired):
				require.ErrorIs(t, confirmErr, constant.ErrReserveOperationConflict)
				require.Equal(t, 1, expired)
				require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, res.ID))
				require.True(t, cur.IsZero())
				require.Len(t, expiredEvents, 1)
			default:
				t.Fatalf("operation left in non-terminal status %q", status)
			}
		})
	}
}
