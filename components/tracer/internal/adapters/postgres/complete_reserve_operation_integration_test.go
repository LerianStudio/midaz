// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func completionDatabase(t *testing.T, suffix ...string) *sql.DB {
	t.Helper()
	admin := testutil.SetupIntegrationDB(t)
	nameInput := t.Name()
	if len(suffix) > 0 {
		nameInput += suffix[0]
	}
	digest := sha256.Sum256([]byte(nameInput))
	name := fmt.Sprintf("completion_test_%x", digest[:8])
	_, err := admin.ExecContext(t.Context(), "CREATE DATABASE "+name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := admin.ExecContext(context.Background(), "DROP DATABASE "+name+" WITH (FORCE)")
		require.NoError(t, err)
	})
	dsn, err := url.Parse(testutil.GetTestDSN())
	require.NoError(t, err)
	dsn.Path = "/" + name
	db, err := sql.Open("pgx", dsn.String())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	// Use a real tenant database/public schema. Historical migrations inspect
	// public explicitly; a schema-only fixture silently skips their guards.
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	files, err := filepath.Glob(filepath.Join(dir, "*.up.sql"))
	require.NoError(t, err)
	for _, file := range files {
		body, err := os.ReadFile(file)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), string(body))
		require.NoError(t, err, filepath.Base(file))
	}
	return db
}

func completionCommand(t *testing.T, db *sql.DB, singleTenant bool) (*command.CompleteReserveOperationCommand, *ReserveDecisionRepository, *AuditEventRepository) {
	t.Helper()
	conn := &testutil.IntegrationDBAdapter{DB: db}
	decisions, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	audit := NewAuditEventRepositoryWithConnection(conn)
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	beginner.SetMultiTenantEnabled(!singleTenant)
	c, err := command.NewCompleteReserveOperationCommand(NewReserveOperationRepository(), decisions,
		newReservationRepoIntegration(db), audit, beginner, clock.NewFixedClock(testutil.FixedTime()),
		command.ReserveCompletionConfig{SingleTenant: singleTenant, MaxRules: 10, MaxReservations: 100})
	require.NoError(t, err)
	return c, decisions, audit
}

func completionContext(ctx context.Context, integration string) context.Context {
	return contextutil.WithIntegrationIdentity(ctx, contextutil.IntegrationIdentity{ID: integration, AssetNamespace: "ledger"})
}

func completionEvents(t *testing.T, db *sql.DB, transactionID uuid.UUID) []uuid.UUID {
	t.Helper()
	rows, err := db.QueryContext(t.Context(), "SELECT event_id FROM audit_events WHERE resource_id=$1 ORDER BY id", transactionID.String())
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

func TestIntegrationCompleteReserveOperationAuditAndReplay(t *testing.T) {
	for _, status := range []model.ReserveOperationStatus{model.OperationConfirmed, model.OperationReleased} {
		t.Run(string(status), func(t *testing.T) {
			db := completionDatabase(t)
			c, decisions, audit := completionCommand(t, db, true)
			limitID := createTestLimitNamed(t, db, 77001, "completion")
			d, res := persistedDecision(), decisionCapacity(limitID, 77002)
			persistDecisionCapacity(t, db, newReservationRepoIntegration(db), decisions, d, res)
			ctx := completionContext(t.Context(), d.Key.IntegrationID)
			state, err := c.Execute(ctx, d.Key.TransactionID, status)
			require.NoError(t, err)
			require.Equal(t, status, state.Status)
			require.Equal(t, testutil.FixedTime(), *state.CompletedAt)
			// A reconstructed command returns the first result without another event.
			restarted, _, _ := completionCommand(t, db, true)
			replay, err := restarted.Execute(ctx, d.Key.TransactionID, status)
			require.NoError(t, err)
			require.Equal(t, state, replay)
			opposite := model.OperationReleased
			if status == opposite {
				opposite = model.OperationConfirmed
			}
			result, err := c.Execute(ctx, d.Key.TransactionID, opposite)
			require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
			require.Nil(t, result)
			ids := completionEvents(t, db, d.Key.TransactionID)
			require.Len(t, ids, 1)
			event, err := audit.GetByID(ctx, ids[0])
			require.NoError(t, err)
			require.Equal(t, model.ActorTypeSystem, event.Actor.ActorType)
			require.Equal(t, d.Key.IntegrationID, event.Actor.ID)
			require.Equal(t, model.AuditResultSuccess, event.Result)
			require.Equal(t, model.ResourceTypeReserveOperation, event.ResourceType)
			require.Equal(t, testutil.FixedTime(), event.CreatedAt.UTC())
			body, err := json.Marshal(event.Context)
			require.NoError(t, err)
			var data command.ReserveCompletionAuditContext
			require.NoError(t, json.Unmarshal(body, &data))
			require.Equal(t, d.Key.IntegrationID, data.IntegrationID)
			require.Equal(t, d.Result.EvaluationID, *data.EvaluationID)
			require.Len(t, data.Reservations, 1)
			require.Equal(t, res.ID, data.Reservations[0].ID)
			require.Equal(t, "10.125", data.Reservations[0].Amount.String())
			valid, err := audit.VerifyHashChain(ctx, ids[0])
			require.NoError(t, err)
			require.True(t, valid.IsValid)
			cur, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
			require.True(t, held.IsZero())
			if status == model.OperationConfirmed {
				require.Equal(t, "10.125", cur.String())
			} else {
				require.True(t, cur.IsZero())
			}
			_, err = db.ExecContext(ctx, capacityMigration(t, "000031_operation_completion_audit.down.sql"))
			require.NoError(t, err)
			valid, err = audit.VerifyHashChain(ctx, ids[0])
			require.NoError(t, err)
			require.True(t, valid.IsValid)
		})
	}
}

func TestIntegrationCompleteReserveOperationBeforeDecisionAndTenantIsolation(t *testing.T) {
	dbA, dbB := completionDatabase(t, "a"), completionDatabase(t, "b")
	c, decisions, audit := completionCommand(t, dbA, false)
	d := persistedDecision()
	ctx := completionContext(t.Context(), d.Key.IntegrationID)
	_, err := c.Execute(ctx, d.Key.TransactionID, model.OperationReleased)
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	ctx = tmcore.ContextWithTenantID(ctx, "tenant-a")
	_, err = c.Execute(ctx, d.Key.TransactionID, model.OperationReleased)
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext)
	ctx = tmcore.ContextWithPG(ctx, dbresolver.New(dbresolver.WithPrimaryDBs(dbA)))
	_, err = c.Execute(ctx, d.Key.TransactionID, model.OperationReleased)
	require.NoError(t, err)
	require.ErrorIs(t, inRealTx(t, dbA, func(tx *sql.Tx) error { return decisions.CreateWithTx(ctx, tx, d) }), constant.ErrReserveOperationConflict)
	ids := completionEvents(t, dbA, d.Key.TransactionID)
	require.Len(t, ids, 1)
	event, err := audit.GetByID(ctx, ids[0])
	require.NoError(t, err)
	require.Nil(t, event.Context["evaluationId"])
	require.Equal(t, model.AuditEventOperationReleased, event.EventType)
	ctxB := tmcore.ContextWithPG(tmcore.ContextWithTenantID(completionContext(t.Context(), d.Key.IntegrationID), "tenant-b"), dbresolver.New(dbresolver.WithPrimaryDBs(dbB)))
	_, err = c.Execute(ctxB, d.Key.TransactionID, model.OperationConfirmed)
	require.NoError(t, err)
	require.Len(t, completionEvents(t, dbB, d.Key.TransactionID), 1)
}

func TestIntegrationCompleteReserveOperationAuditFailureRollsBackEverything(t *testing.T) {
	for _, withCapacity := range []bool{false, true} {
		t.Run(map[bool]string{false: "before decision", true: "with capacity"}[withCapacity], func(t *testing.T) {
			db := completionDatabase(t)
			c, decisions, _ := completionCommand(t, db, true)
			d := persistedDecision()
			limitID := createTestLimitNamed(t, db, 77101, "audit-failure")
			res := decisionCapacity(limitID, 77102)
			if withCapacity {
				persistDecisionCapacity(t, db, newReservationRepoIntegration(db), decisions, d, res)
			}
			_, err := db.ExecContext(t.Context(), `CREATE FUNCTION reject_completion_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$
                BEGIN RAISE EXCEPTION 'injected audit failure'; END $$;
                CREATE TRIGGER reject_completion_audit_test BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION reject_completion_audit_test()`)
			require.NoError(t, err)
			ctx := completionContext(t.Context(), d.Key.IntegrationID)
			state, err := c.Execute(ctx, d.Key.TransactionID, model.OperationConfirmed)
			require.Error(t, err)
			require.Nil(t, state)
			require.Empty(t, completionEvents(t, db, d.Key.TransactionID))
			var count int
			require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM reserve_operations WHERE status<>'OPEN'").Scan(&count))
			require.Zero(t, count)
			if withCapacity {
				cur, held := readCounterDecimal(t, db, limitID, res.ScopeKey, res.PeriodKey)
				require.True(t, cur.IsZero())
				require.Equal(t, "10.125", held.String())
				require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, res.ID))
			}
			_, err = db.ExecContext(t.Context(), "DROP TRIGGER reject_completion_audit_test ON audit_events")
			require.NoError(t, err)
			_, err = c.Execute(ctx, d.Key.TransactionID, model.OperationConfirmed)
			require.NoError(t, err)
			require.Len(t, completionEvents(t, db, d.Key.TransactionID), 1)
		})
	}
}

func TestIntegrationCompleteReserveOperationConcurrentAudit(t *testing.T) {
	for _, scenario := range []string{"same outcome", "contradictory outcome", "different integration"} {
		t.Run(scenario, func(t *testing.T) {
			db := completionDatabase(t)
			c, _, audit := completionCommand(t, db, true)
			transactionID := testutil.MustDeterministicUUID(77201)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			start := make(chan struct{})
			result := make(chan error, 2)
			for i := range 2 {
				go func() {
					<-start
					identity, status := "producer-a", model.OperationConfirmed
					if i == 1 && scenario == "different integration" {
						identity = "producer-b"
					}
					if i == 1 && scenario == "contradictory outcome" {
						status = model.OperationReleased
					}
					_, err := c.Execute(completionContext(ctx, identity), transactionID, status)
					result <- err
				}()
			}
			close(start)
			conflicts := 0
			for range 2 {
				err := <-result
				if errors.Is(err, constant.ErrReserveOperationConflict) {
					conflicts++
				} else {
					require.NoError(t, err)
				}
			}
			if scenario == "contradictory outcome" {
				require.Equal(t, 1, conflicts)
			} else {
				require.Zero(t, conflicts)
			}
			expected := 1
			if scenario == "different integration" {
				expected = 2
			}
			ids := completionEvents(t, db, transactionID)
			require.Len(t, ids, expected, "legacy transaction dedup must not hide a different producer's event")
			for _, id := range ids {
				verification, err := audit.VerifyHashChain(ctx, id)
				require.NoError(t, err)
				require.True(t, verification.IsValid)
			}
		})
	}
}

func TestIntegrationCompleteReserveOperationMissingCapacityFailsClosed(t *testing.T) {
	db := completionDatabase(t)
	c, decisions, _ := completionCommand(t, db, true)
	d := persistedDecision()
	d.Result.ReservationIDs = []uuid.UUID{testutil.MustDeterministicUUID(77301)}
	saveDecision(t, db, decisions, d)
	state, err := c.Execute(completionContext(t.Context(), d.Key.IntegrationID), d.Key.TransactionID, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrInternalServer)
	require.Nil(t, state)
	require.Empty(t, completionEvents(t, db, d.Key.TransactionID))
	var status string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT status FROM reserve_operations").Scan(&status))
	require.Equal(t, "OPEN", status)
}

func TestIntegrationCompleteReserveOperationSuppressedAuditFailsClosed(t *testing.T) {
	db := completionDatabase(t)
	c, _, _ := completionCommand(t, db, true)
	_, err := db.ExecContext(t.Context(), `CREATE FUNCTION suppress_completion_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$
        BEGIN RETURN NULL; END $$;
        CREATE TRIGGER suppress_completion_audit_test BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION suppress_completion_audit_test()`)
	require.NoError(t, err)
	id := testutil.MustDeterministicUUID(77401)
	state, err := c.Execute(completionContext(t.Context(), "producer"), id, model.OperationConfirmed)
	require.ErrorIs(t, err, constant.ErrInternalServer)
	require.Nil(t, state)
	require.Empty(t, completionEvents(t, db, id))
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM reserve_operations").Scan(&count))
	require.Zero(t, count, "a zero-row audit insert must roll back completion")
}
