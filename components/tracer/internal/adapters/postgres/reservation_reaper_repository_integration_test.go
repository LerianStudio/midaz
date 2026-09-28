// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/bxcodec/dbresolver/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/workers"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func TestIntegrationReaperExpiresDecisionReservationWithItsOperation(t *testing.T) {
	db := completionDatabase(t)
	conn := &testutil.IntegrationDBAdapter{DB: db}
	admittedAt := testutil.FixedTime()
	admission, _, r := admissionFixtureWithLifetime(t, db, conn, admittedAt, true, 10, time.Second, 720*time.Hour)
	r.ValidationMode = tracercontract.ValidationLimits
	limitID := admissionLimit(t, db, r, 77701, "100")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()
	admitted, err := admission.Execute(ctx, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, admitted.Decision)
	require.Len(t, admitted.ReservationIDs, 1)
	reservationID := admitted.ReservationIDs[0]
	var scopeKey, periodKey string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT scope_key, period_key FROM usage_reservations WHERE id=$1", reservationID).Scan(&scopeKey, &periodKey))
	_, held := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	require.Equal(t, "10.125", held.String())

	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	reservations := newReservationRepoIntegration(db)
	audit := NewAuditEventRepositoryWithConnection(conn)
	expire := expireCommand(t, db)
	sweptAt := admittedAt.Add(2 * time.Second)
	reaper, err := workers.NewReservationReaperWorkerWithPoolResolver(NewReservationReaperRepository(conn, beginner, reservations), command.NewRecordAuditEventCommand(audit), expire,
		workers.ReservationReaperWorkerConfig{ReapInterval: time.Second, BatchSize: workers.DefaultReservationReaperBatchSize}, testutil.NewMockLogger(), clock.NewFixedClock(sweptAt), "", nil)
	require.NoError(t, err)

	// Before the TTL elapses the reservation stays held.
	early, err := workers.NewReservationReaperWorkerWithPoolResolver(NewReservationReaperRepository(conn, beginner, reservations), command.NewRecordAuditEventCommand(audit), expire,
		workers.ReservationReaperWorkerConfig{ReapInterval: time.Second, BatchSize: workers.DefaultReservationReaperBatchSize}, testutil.NewMockLogger(), clock.NewFixedClock(admittedAt), "", nil)
	require.NoError(t, err)
	released, err := early.RunOnce(ctx)
	require.NoError(t, err)
	require.Zero(t, released)
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, reservationID))

	released, err = reaper.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, released)

	require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, reservationID))
	cur, held := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	require.True(t, cur.IsZero())
	require.True(t, held.IsZero(), "expired decision capacity no longer counts against the limit")
	key := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: r.TransactionID}
	status, completedAt := readOperationState(t, db, key)
	require.Equal(t, string(model.OperationExpired), status)
	require.Equal(t, sweptAt, completedAt.Time.UTC())

	ids := operationEvents(t, db, r.TransactionID, model.AuditEventOperationExpired)
	require.Len(t, ids, 1)
	event, err := audit.GetByID(ctx, ids[0])
	require.NoError(t, err)
	require.Equal(t, admitted.EvaluationID.String(), event.Context["evaluationId"])
	valid, err := audit.VerifyHashChain(ctx, ids[0])
	require.NoError(t, err)
	require.True(t, valid.IsValid)
	var batches int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events WHERE event_type=$1", string(model.AuditEventReservationExpired)).Scan(&batches))
	require.Zero(t, batches, "the legacy batch row never counts decision capacity")

	// The swept row is gone from the sweep, and a late confirm conflicts.
	released, err = reaper.RunOnce(ctx)
	require.NoError(t, err)
	require.Zero(t, released)
	completion, _, _ := completionCommand(t, db, true)
	state, err := completion.Execute(ctx, r.TransactionID, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
	require.Nil(t, state)
	require.Len(t, operationEvents(t, db, r.TransactionID, model.AuditEventOperationExpired), 1)
	_, held = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	require.True(t, held.IsZero())
}

// TestIntegrationReaperBatchCapSplitsAnOperation caps a sweep below the rows one
// decision holds. The sweep reads one row, the operation's expiry settles both,
// and the count reflects the two rows that moved; the next sweep finds nothing,
// so no row is counted twice.
func TestIntegrationReaperBatchCapSplitsAnOperation(t *testing.T) {
	db := completionDatabase(t)
	conn := &testutil.IntegrationDBAdapter{DB: db}
	admittedAt := testutil.FixedTime()
	admission, _, r := admissionFixtureWithLifetime(t, db, conn, admittedAt, true, 10, time.Second, 720*time.Hour)
	r.ValidationMode = tracercontract.ValidationLimits
	admissionLimit(t, db, r, 77801, "100")
	admissionLimit(t, db, r, 77802, "200")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()
	admitted, err := admission.Execute(ctx, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, admitted.Decision)
	require.Len(t, admitted.ReservationIDs, 2)

	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	repo := NewReservationReaperRepository(conn, beginner, newReservationRepoIntegration(db))
	sweptAt := admittedAt.Add(2 * time.Second)
	read, err := repo.FindExpiredReservations(ctx, sweptAt, nil, 1)
	require.NoError(t, err)
	require.Len(t, read, 1, "the cap bounds what one sweep reads")

	reaper, err := workers.NewReservationReaperWorkerWithPoolResolver(repo, command.NewRecordAuditEventCommand(NewAuditEventRepositoryWithConnection(conn)), expireCommand(t, db),
		workers.ReservationReaperWorkerConfig{ReapInterval: time.Second, BatchSize: 1}, testutil.NewMockLogger(), clock.NewFixedClock(sweptAt), "", nil)
	require.NoError(t, err)

	released, err := reaper.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, released, "the operation's expiry moved both rows")
	for _, id := range admitted.ReservationIDs {
		require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, id))
	}
	require.Len(t, operationEvents(t, db, r.TransactionID, model.AuditEventOperationExpired), 1)

	released, err = reaper.RunOnce(ctx)
	require.NoError(t, err)
	require.Zero(t, released)
	require.Len(t, operationEvents(t, db, r.TransactionID, model.AuditEventOperationExpired), 1)
}

// TestIntegrationReaperFailingOperationDoesNotStarveNewerExpiries lowers the
// reservation bound below an outstanding decision, so that operation fails to
// expire on every sweep while its rows hold the head of the expiry order and
// fill a whole page. The next sweep resumes past them: a newer decision and a
// newer legacy row expire, and the failing operation stays RESERVED.
func TestIntegrationReaperFailingOperationDoesNotStarveNewerExpiries(t *testing.T) {
	db := completionDatabase(t)
	conn := &testutil.IntegrationDBAdapter{DB: db}
	admittedAt := testutil.FixedTime()
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()

	oldest, _, failing := admissionFixtureWithLifetime(t, db, conn, admittedAt, true, 10, time.Second, 720*time.Hour)
	failing.ValidationMode = tracercontract.ValidationLimits
	admissionLimit(t, db, failing, 77901, "100")
	admissionLimit(t, db, failing, 77902, "200")
	stuck, err := oldest.Execute(ctx, failing)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, stuck.Decision)
	require.Len(t, stuck.ReservationIDs, 2)

	newer, _, base := admissionFixtureWithLifetime(t, db, conn, admittedAt.Add(time.Second), true, 10, time.Second, 720*time.Hour)
	healthy := withReserveAsset(base, "EUR", 77911)
	healthy.ValidationMode = tracercontract.ValidationLimits
	admissionLimit(t, db, healthy, 77910, "100")
	admitted, err := newer.Execute(ctx, healthy)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, admitted.Decision)
	require.Len(t, admitted.ReservationIDs, 1)

	reservations := newReservationRepoIntegration(db)
	legacyLimit := createTestLimitNamed(t, db, 77920, "starvation")
	legacy, err := model.NewReservation(legacyLimit, testutil.MustDeterministicUUID(77921), "acct:77921", "2026-09", decimal.NewFromInt(400),
		admittedAt.Add(3*time.Second), admittedAt)
	require.NoError(t, err)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return reservations.ReserveWithTx(ctx, tx, legacy, decimal.NewFromInt(10000))
	}))

	// The operator lowered the bound below the stuck decision's two reservations.
	decisions, err := NewReserveDecisionRepository(conn, 10, 1)
	require.NoError(t, err)
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	expire, err := command.NewExpireReserveOperationCommand(NewReserveOperationRepository(), decisions, reservations,
		NewAuditEventRepositoryWithConnection(conn), beginner, command.ReserveCompletionConfig{SingleTenant: true, MaxRules: 10, MaxReservations: 1})
	require.NoError(t, err)

	sweptAt := admittedAt.Add(time.Minute)
	reaper, err := workers.NewReservationReaperWorkerWithPoolResolver(NewReservationReaperRepository(conn, beginner, reservations),
		command.NewRecordAuditEventCommand(NewAuditEventRepositoryWithConnection(conn)), expire,
		workers.ReservationReaperWorkerConfig{ReapInterval: time.Second, BatchSize: 2}, testutil.NewMockLogger(), clock.NewFixedClock(sweptAt), "", nil)
	require.NoError(t, err)

	released, err := reaper.RunOnce(ctx)
	require.Error(t, err, "the stuck operation fills the first page and fails")
	require.Zero(t, released)
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, admitted.ReservationIDs[0]))
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, legacy.ID))

	released, err = reaper.RunOnce(ctx)
	require.NoError(t, err)
	require.Equal(t, 2, released, "the newer decision and the newer legacy row expire behind the stuck operation")
	require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, admitted.ReservationIDs[0]))
	require.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, legacy.ID))
	status, _ := readOperationState(t, db, model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: healthy.TransactionID})
	require.Equal(t, string(model.OperationExpired), status)

	for _, id := range stuck.ReservationIDs {
		require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, id))
	}
	status, _ = readOperationState(t, db, model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: failing.TransactionID})
	require.NotEqual(t, string(model.OperationExpired), status)
}
