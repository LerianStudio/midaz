// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/migrations"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/contextutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func decisionMigration(t *testing.T, direction string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, migrations.WriteTo(dir))
	data, err := os.ReadFile(filepath.Join(dir, "000028_reserve_decisions."+direction+".sql"))
	require.NoError(t, err)
	return string(data)
}

func decisionDatabase(t *testing.T, suffix string) (*sql.DB, *ReserveDecisionRepository) {
	t.Helper()
	db := contextPolicyDatabase(t, suffix)
	_, err := db.ExecContext(t.Context(), decisionMigration(t, "up"))
	require.NoError(t, err)
	repo, err := NewReserveDecisionRepository(&testutil.IntegrationDBAdapter{DB: db}, 10, 100)
	require.NoError(t, err)
	return db, repo
}

func persistedDecision() model.ReserveDecision {
	return model.ReserveDecision{
		Key:       model.ReserveOperationKey{IntegrationID: "producer-a", TransactionID: testutil.MustDeterministicUUID(72001), RequestID: testutil.MustDeterministicUUID(72002)},
		ContextID: "ledger-a", Fingerprint: sha256.Sum256([]byte("frozen request")), ValidationMode: tracercontract.ValidationLimits, CreatedAt: testutil.FixedTime(),
		Result: tracercontract.ReserveResult{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(72001), EvaluationID: testutil.MustDeterministicUUID(72003), Decision: tracercontract.DecisionAllow, Controls: tracercontract.ReserveControls{Rules: tracercontract.RulesNotRequested, Limits: tracercontract.LimitsEvaluated}, ReservationIDs: []uuid.UUID{}, Reasons: []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}},
	}
}

func saveDecision(t *testing.T, db *sql.DB, repo *ReserveDecisionRepository, d model.ReserveDecision) {
	t.Helper()
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	require.NoError(t, repo.CreateWithTx(t.Context(), tx, d))
	require.NoError(t, tx.Commit())
}

func TestIntegrationReserveDecisionOutcomesAndRollback(t *testing.T) {
	db, repo := decisionDatabase(t, "outcomes")
	for i, outcome := range []tracercontract.Decision{tracercontract.DecisionAllow, tracercontract.DecisionDeny, tracercontract.DecisionReview} {
		t.Run(string(outcome), func(t *testing.T) {
			d := persistedDecision()
			d.Key.TransactionID[0] = byte(i + 1)
			d.Key.RequestID[0] = byte(i + 1)
			d.Result.TransactionID = d.Key.TransactionID
			d.Result.EvaluationID[0] = byte(i + 1)
			d.Result.Decision = outcome
			if outcome == tracercontract.DecisionDeny {
				d.Result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}
			}
			if outcome == tracercontract.DecisionReview {
				policyRepo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 10)
				require.NoError(t, err)
				policy := storedContextPolicy()
				tx, err := db.BeginTx(t.Context(), nil)
				require.NoError(t, err)
				t.Cleanup(func() { _ = tx.Rollback() })
				require.NoError(t, policyRepo.PublishWithTx(t.Context(), tx, policy, "admin", testutil.FixedTime()))
				require.NoError(t, tx.Commit())
				d.ValidationMode = tracercontract.ValidationRulesAndLimits
				d.Result.Controls.Rules = tracercontract.RulesEvaluated
				d.Result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied, tracercontract.ReasonRuleReview}
				d.Policy = &model.ReserveDecisionPolicy{ID: policy.ID, Revision: policy.Revision, BindingVersion: 9, EvaluatedRules: []model.RuleRevision{{ID: policy.Rules[1].ID, Revision: 1}, {ID: policy.Rules[0].ID, Revision: 1}}, MatchedRules: []model.RuleRevision{{ID: policy.Rules[0].ID, Revision: 1}}}
			}
			saveDecision(t, db, repo, d)
			got, err := repo.Get(t.Context(), d.Key)
			require.NoError(t, err)
			require.Equal(t, d, *got)
			// A separate transaction sees the complete original outcome, even when
			// it owns no capacity. In-memory mutations cannot rewrite the snapshot.
			got.Result.Reasons[0] = "MUTATED"
			again, err := repo.Get(t.Context(), d.Key)
			require.NoError(t, err)
			require.Equal(t, d, *again)
		})
	}
	d := persistedDecision()
	d.Key.TransactionID[0] = 99
	d.Key.RequestID[0] = 99
	d.Result.TransactionID = d.Key.TransactionID
	d.Result.EvaluationID[0] = 99
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWithTx(t.Context(), tx, d))
	inside, err := repo.GetWithTx(t.Context(), tx, d.Key)
	require.NoError(t, err)
	require.NotNil(t, inside)
	require.NoError(t, tx.Rollback())
	outside, err := repo.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Nil(t, outside)
}

func TestIntegrationReserveDecisionUniquenessAndImmutability(t *testing.T) {
	db, repo := decisionDatabase(t, "constraints")
	d := persistedDecision()
	saveDecision(t, db, repo, d)
	for _, change := range []func(*model.ReserveDecision){
		func(d *model.ReserveDecision) { d.Fingerprint[0]++ },
		func(d *model.ReserveDecision) {
			d.Key.TransactionID[0]++
			d.Result.TransactionID = d.Key.TransactionID
			d.Result.EvaluationID[0]++
		},
		func(d *model.ReserveDecision) { d.Key.RequestID[0]++; d.Result.EvaluationID[0]++ },
	} {
		other := d.Clone()
		change(&other)
		tx, err := db.BeginTx(t.Context(), nil)
		require.NoError(t, err)
		require.ErrorIs(t, repo.CreateWithTx(t.Context(), tx, other), constant.ErrReserveDecisionConflict)
		require.NoError(t, tx.Rollback())
		if other.Key != d.Key {
			got, err := repo.Get(t.Context(), other.Key)
			require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
			require.Nil(t, got)
		}
	}
	for _, statement := range []string{"UPDATE reserve_decisions SET context_id='changed'", "DELETE FROM reserve_decisions", "TRUNCATE reserve_decisions"} {
		_, err := db.ExecContext(t.Context(), statement)
		require.Error(t, err)
	}
	_, err := db.ExecContext(t.Context(), decisionMigration(t, "down"))
	require.Error(t, err, "rollback must refuse to erase durable decisions")
	got, err := repo.Get(t.Context(), d.Key)
	require.NoError(t, err)
	require.Equal(t, d, *got)
	other := d.Clone()
	other.Key.IntegrationID = "producer-b"
	other.Result.EvaluationID[0]++
	saveDecision(t, db, repo, other)
	got, err = repo.Get(t.Context(), other.Key)
	require.NoError(t, err)
	require.Equal(t, other, *got)
}

func TestIntegrationReserveDecisionConcurrentWriters(t *testing.T) {
	db, repo := decisionDatabase(t, "concurrent")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Go(func() {
			<-start
			d := persistedDecision()
			d.Result.EvaluationID[0] += byte(i)
			tx, err := db.BeginTx(t.Context(), nil)
			if err != nil {
				results <- err
				return
			}
			defer tx.Rollback()
			if err = repo.CreateWithTx(t.Context(), tx, d); err == nil {
				err = tx.Commit()
			}
			results <- err
		})
	}
	close(start)
	wg.Wait()
	close(results)
	winners, conflicts := 0, 0
	for err := range results {
		if err == nil {
			winners++
		} else if errors.Is(err, constant.ErrReserveDecisionConflict) {
			conflicts++
		} else {
			require.NoError(t, err)
		}
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 1, conflicts)
	var count int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM reserve_decisions").Scan(&count))
	require.Equal(t, 1, count)
}

func TestIntegrationReserveDecisionTenantIsolation(t *testing.T) {
	dbA, repoA := decisionDatabase(t, "tenant-a")
	dbB, _ := decisionDatabase(t, "tenant-b")
	d := persistedDecision()
	saveDecision(t, dbA, repoA, d)
	conn := &pgdb.PostgresConnectionAdapter{}
	conn.SetMultiTenantEnabled(true)
	repo, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	_, err = repo.Get(t.Context(), d.Key)
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext)
	// An intentionally empty replica must not hide a committed decision.
	got, err := repo.Get(tmcore.ContextWithPG(t.Context(), dbresolver.New(dbresolver.WithPrimaryDBs(dbA), dbresolver.WithReplicaDBs(dbB))), d.Key)
	require.NoError(t, err)
	require.Equal(t, d, *got)
	got, err = repo.Get(tmcore.ContextWithPG(t.Context(), dbresolver.New(dbresolver.WithPrimaryDBs(dbB))), d.Key)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestIntegrationReserveDecisionEmptyMigrationRollback(t *testing.T) {
	db, _ := decisionDatabase(t, "rollback")
	_, err := db.ExecContext(context.Background(), decisionMigration(t, "down"))
	require.NoError(t, err)
	_, err = db.ExecContext(context.Background(), decisionMigration(t, "up"))
	require.NoError(t, err)
}

func TestIntegrationReserveDecisionMigrationPreservesLegacyCapacity(t *testing.T) {
	db := contextPolicyDatabase(t, "legacy")
	_, err := db.ExecContext(t.Context(), `
		CREATE TABLE usage_counters (LIKE public.usage_counters INCLUDING ALL);
		CREATE TABLE usage_reservations (LIKE public.usage_reservations INCLUDING ALL)`)
	require.NoError(t, err)
	limitID := testutil.MustDeterministicUUID(72501)
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_counters
		(id,limit_id,scope_key,period_key,current_usage,reserved_usage,last_updated_at)
		VALUES ($1,$2,'account:legacy','2026-09','12.125','0.00000001',$3)`, testutil.MustDeterministicUUID(72502), limitID, testutil.FixedTime())
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `INSERT INTO usage_reservations
		(id,limit_id,scope_key,period_key,amount,status,transaction_id,reservation_expires_at,created_at)
		VALUES ($1,$2,'account:legacy','2026-09','0.00000001','RESERVED',$3,$4,$4)`, testutil.MustDeterministicUUID(72503), limitID, testutil.MustDeterministicUUID(72504), testutil.FixedTime())
	require.NoError(t, err)
	const snapshotSQL = `SELECT jsonb_build_array(
		(SELECT to_jsonb(c) FROM usage_counters c),
		(SELECT to_jsonb(r) FROM usage_reservations r),
		(SELECT jsonb_agg(indexdef ORDER BY indexname) FROM pg_indexes
		 WHERE schemaname=current_schema() AND tablename IN ('usage_counters','usage_reservations')))::text`
	var before, after string
	require.NoError(t, db.QueryRowContext(t.Context(), snapshotSQL).Scan(&before))
	_, err = db.ExecContext(t.Context(), decisionMigration(t, "up"))
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), snapshotSQL).Scan(&after))
	require.Equal(t, before, after)
	_, err = db.ExecContext(t.Context(), decisionMigration(t, "down"))
	require.NoError(t, err)
	require.NoError(t, db.QueryRowContext(t.Context(), snapshotSQL).Scan(&after))
	require.Equal(t, before, after)
}

func TestIntegrationReserveDecisionReplayAfterPolicyRebind(t *testing.T) {
	db, repo := decisionDatabase(t, "rebind")
	policyRepo, err := NewContextPolicyRepository(&testutil.IntegrationDBAdapter{DB: db}, 10)
	require.NoError(t, err)
	d := persistedDecision()
	d.ValidationMode = tracercontract.ValidationRulesAndLimits
	d.Result.Decision = tracercontract.DecisionDeny
	d.Result.Controls = tracercontract.ReserveControls{Rules: tracercontract.RulesEvaluated, Limits: tracercontract.LimitsSkippedRuleDeny}
	d.Result.Reasons = []tracercontract.ReserveReason{tracercontract.ReasonPolicyDefaultDeny}
	p := storedContextPolicy()
	p.Rules = []model.ContextPolicyRule{}
	key := model.PolicyBindingKey{IntegrationID: d.Key.IntegrationID, ContextID: d.ContextID}
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := policyRepo.PublishWithTx(t.Context(), tx, p, "admin", testutil.FixedTime()); err != nil {
			return err
		}
		return policyRepo.BindWithTx(t.Context(), tx, key, model.PolicyRevision{ID: p.ID, Revision: 1}, nil, "admin", testutil.FixedTime())
	}))
	d.Policy = &model.ReserveDecisionPolicy{ID: p.ID, Revision: 1, BindingVersion: 1, DefaultUsed: true, EvaluatedRules: []model.RuleRevision{}, MatchedRules: []model.RuleRevision{}}
	asset := tracercontract.AssetRef{Namespace: "producer-assets", ID: "asset-1", Code: "TOKEN"}
	longLived := false
	request := tracercontract.ReserveRequest{
		ContractRevision: tracercontract.ReserveContractRevision, TransactionID: d.Key.TransactionID, RequestID: d.Key.RequestID,
		ContextID: d.ContextID, ValidationMode: d.ValidationMode, TransactionTimestamp: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		LongLived: &longLived, Amount: "1.00", Asset: asset,
		Context: tracercontract.Context{Accounts: []tracercontract.Account{}, Entries: []tracercontract.Entry{{External: true, Direction: tracercontract.Debit, Amount: "1", Asset: asset}}},
	}
	config := query.ReserveReplayConfig{Limits: tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}, MaxRules: 10, MaxReservations: 100}
	scope := tracercontract.ReserveScope{TenantID: "tenant-a", IntegrationID: key.IntegrationID, AssetNamespace: asset.Namespace}
	d.Fingerprint, err = request.Fingerprint(t.Context(), scope, config.Limits)
	require.NoError(t, err)
	saveDecision(t, db, repo, d)

	p.Revision, p.DefaultDecision = 2, model.DecisionAllow
	expectedVersion := int64(1)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if err := policyRepo.PublishWithTx(t.Context(), tx, p, "admin", testutil.FixedTime()); err != nil {
			return err
		}
		return policyRepo.BindWithTx(t.Context(), tx, key, model.PolicyRevision{ID: p.ID, Revision: 2}, &expectedVersion, "admin", testutil.FixedTime())
	}))
	active, err := policyRepo.GetActive(t.Context(), key)
	require.NoError(t, err)
	require.Equal(t, int64(2), active.Revision)
	require.Equal(t, model.DecisionAllow, active.DefaultDecision)

	// Reconstruct the repository/query as a restarted process would, without a
	// cached decision or evaluator. Replay still returns revision 1's DENY.
	restartedRepo, err := NewReserveDecisionRepository(&testutil.IntegrationDBAdapter{DB: db}, 10, 100)
	require.NoError(t, err)
	lookup, err := query.NewLookupReserveDecisionQuery(restartedRepo, config)
	require.NoError(t, err)
	ctx := tmcore.ContextWithTenantID(contextutil.WithIntegrationIdentity(t.Context(), contextutil.IntegrationIdentity{ID: key.IntegrationID, AssetNamespace: asset.Namespace}), "tenant-a")
	request.Amount = "1"
	replayed, err := lookup.Execute(ctx, request)
	require.NoError(t, err)
	require.Equal(t, d, *replayed)
}
