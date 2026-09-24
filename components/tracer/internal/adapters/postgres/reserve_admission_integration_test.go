// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func admissionFixture(t *testing.T, db *sql.DB, evaluationTime ...time.Time) (*command.ReserveAdmissionCommand, *ContextPolicyRepository, tracercontract.ReserveRequest) {
	t.Helper()
	now := testutil.FixedTime()
	if len(evaluationTime) > 0 {
		now = evaluationTime[0]
	}
	return admissionFixtureWithConnection(t, db, &testutil.IntegrationDBAdapter{DB: db}, now, true)
}

func admissionFixtureWithConnection(t *testing.T, db *sql.DB, conn pgdb.Connection, now time.Time, singleTenant bool) (*command.ReserveAdmissionCommand, *ContextPolicyRepository, tracercontract.ReserveRequest) {
	t.Helper()
	facts := tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
	engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{Limits: facts, CostLimit: 100000, MaxExpressionBytes: 5000})
	require.NoError(t, err)
	evaluator, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: 10, TotalCost: 100000})
	require.NoError(t, err)
	policies, err := NewContextPolicyRepository(conn, 10)
	require.NoError(t, err)
	resolver, err := query.NewResolveContextPolicyQuery(policies, 10)
	require.NoError(t, err)
	compiled, err := query.NewCompiledContextPolicyQuery(resolver, evaluator, query.CompiledPolicyCacheConfig{MaxEntries: 10, MaxCompilations: 4, SingleTenant: singleTenant})
	require.NoError(t, err)
	decisions, err := NewReserveDecisionRepository(conn, 10, 100)
	require.NoError(t, err)
	beginner := pgdb.NewTxBeginnerAdapter(dbresolver.New(dbresolver.WithPrimaryDBs(db)))
	beginner.SetMultiTenantEnabled(!singleTenant)
	c, err := command.NewReserveAdmissionCommand(command.ReserveAdmissionDependencies{
		Decisions: decisions, Operations: NewReserveOperationRepository(), Capacity: newReservationRepoIntegration(db), Limits: contextLimitRepository(t, 10), Policies: compiled, Evaluator: evaluator, Audit: NewAuditEventRepositoryWithConnection(conn), Transactions: beginner,
	}, clock.NewFixedClock(now), command.ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: facts, MaxLimits: 10, MaxScopesPerLimit: 10, MaxReservations: 100}, MaxRules: 10, SingleTenant: singleTenant, MaxTimestampAge: 24 * time.Hour, ClockSkewTolerance: time.Second, ReservationLifetime: time.Hour})
	require.NoError(t, err)
	account := testutil.MustDeterministicUUID(89001)
	asset := tracercontract.AssetRef{Namespace: "ledger", ID: "official-asset", Code: "USD"}
	blocked, longLived := false, false
	r := tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(89002), RequestID: testutil.MustDeterministicUUID(89003), ContextID: "official", ValidationMode: tracercontract.ValidationRulesAndLimits, TransactionTimestamp: testutil.FixedTime(), LongLived: &longLived, Amount: "10.125", Asset: asset, Context: tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}}
	return c, policies, r
}

func admissionPolicy(t *testing.T, db *sql.DB, repo *ContextPolicyRepository, action model.Decision) {
	t.Helper()
	policy := model.ContextPolicy{ID: testutil.MustDeterministicUUID(89004), Revision: 1, DefaultDecision: model.DecisionDeny, Rules: []model.ContextPolicyRule{{ID: testutil.MustDeterministicUUID(89005), Revision: 1, Expression: "true", Action: action}}}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, repo.PublishWithTx(t.Context(), tx, policy, "operator", testutil.FixedTime()))
	require.NoError(t, repo.BindWithTx(t.Context(), tx, model.PolicyBindingKey{IntegrationID: "producer", ContextID: "official"}, model.PolicyRevision{ID: policy.ID, Revision: 1}, nil, "operator", testutil.FixedTime()))
	require.NoError(t, tx.Commit())
}

func admissionLimit(t *testing.T, db *sql.DB, r tracercontract.ReserveRequest, seed int64, maxAmount string) uuid.UUID {
	t.Helper()
	id := contextLimitRow(t, db, seed, r.Context.Accounts[0].ID)
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET max_amount=$2 WHERE id=$1", id, maxAmount)
	require.NoError(t, err)
	bindContextLimit(t, db, contextLimitRepository(t, 10), id, r.Asset)
	return id
}

func TestIntegrationReserveAdmissionDecisionsAndReplay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		action  model.Decision
		ceiling string
		want    tracercontract.Decision
	}{
		{"allow", model.DecisionAllow, "100", tracercontract.DecisionAllow},
		{"rule deny", model.DecisionDeny, "100", tracercontract.DecisionDeny},
		{"review", model.DecisionReview, "100", tracercontract.DecisionReview},
		{"limit overrides review", model.DecisionReview, "20", tracercontract.DecisionDeny},
		{"limit overrides allow", model.DecisionAllow, "20", tracercontract.DecisionDeny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := completionDatabase(t)
			c, repo, r := admissionFixture(t, db)
			admissionPolicy(t, db, repo, tc.action)
			limit := admissionLimit(t, db, r, 89101, tc.ceiling)
			// The request itself fits every ceiling; existing usage causes the denial.
			period := testutil.FixedTime().Format("2006-01-02")
			_, err := db.ExecContext(t.Context(), "INSERT INTO usage_counters(limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES($1,$2,$3,10,0)", limit, "acct:"+r.Context.Accounts[0].ID.String(), period)
			require.NoError(t, err)
			// A single available connection proves policy reads reuse the admission Tx.
			db.SetMaxOpenConns(1)
			ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
			defer cancel()
			got, err := c.Execute(ctx, r)
			require.NoError(t, err)
			require.Equal(t, tc.want, got.Decision)
			wantHeld := "0"
			if tc.want == tracercontract.DecisionAllow {
				wantHeld = "10.125"
				require.Len(t, got.ReservationIDs, 1)
			} else {
				require.Empty(t, got.ReservationIDs)
			}
			current, held := readCounterDecimal(t, db, limit, "acct:"+r.Context.Accounts[0].ID.String(), period)
			require.Equal(t, "10", current.String())
			require.Equal(t, wantHeld, held.String())
			events := completionEvents(t, db, r.TransactionID)
			require.Len(t, events, 1)
			audit := NewAuditEventRepositoryWithConnection(&testutil.IntegrationDBAdapter{DB: db})
			verified, err := audit.VerifyHashChain(ctx, events[0])
			require.NoError(t, err)
			require.True(t, verified.IsValid)
			// Replay does not need today's policy or duplicate the event/capacity.
			_, err = db.ExecContext(ctx, "DELETE FROM evaluation_policy_bindings")
			require.NoError(t, err)
			restarted, _, _ := admissionFixture(t, db, testutil.FixedTime().Add(48*time.Hour))
			again, err := restarted.Execute(ctx, r)
			require.NoError(t, err)
			require.Equal(t, got, again)
			require.Len(t, completionEvents(t, db, r.TransactionID), 1)
			if tc.want == tracercontract.DecisionAllow {
				complete, _, _ := completionCommand(t, db, true)
				for range 2 {
					_, err = complete.Execute(ctx, r.TransactionID, model.OperationConfirmed)
					require.NoError(t, err)
				}
				current, held = readCounterDecimal(t, db, limit, "acct:"+r.Context.Accounts[0].ID.String(), period)
				require.Equal(t, "20.125", current.String())
				require.True(t, held.IsZero())
				afterCompletion, err := restarted.Execute(ctx, r)
				require.NoError(t, err)
				require.Equal(t, got, afterCompletion)
				require.Len(t, completionEvents(t, db, r.TransactionID), 2)
			}
			r.Amount = "11"
			result, err := c.Execute(ctx, r)
			require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
			require.Nil(t, result)
		})
	}
}

func TestIntegrationReserveAdmissionRollbackAndTerminalFence(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixture(t, db)
	r.ValidationMode = tracercontract.ValidationLimits
	admissionLimit(t, db, r, 89201, "100")
	ctx := completionContext(t.Context(), "producer")
	_, err := db.ExecContext(ctx, `CREATE FUNCTION suppress_admission_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$; CREATE TRIGGER suppress_admission_audit BEFORE INSERT ON audit_events FOR EACH ROW EXECUTE FUNCTION suppress_admission_audit()`)
	require.NoError(t, err)
	result, err := c.Execute(ctx, r)
	require.Error(t, err)
	require.Nil(t, result)
	for _, table := range []string{"reserve_decisions", "reserve_operations", "usage_reservations", "usage_counters", "audit_events"} {
		var count int
		require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count))
		require.Zero(t, count, table)
	}
	_, err = db.ExecContext(ctx, "DROP TRIGGER suppress_admission_audit ON audit_events")
	require.NoError(t, err)
	completion, _, _ := completionCommand(t, db, true)
	_, err = completion.Execute(ctx, r.TransactionID, model.OperationReleased)
	require.NoError(t, err)
	result, err = c.Execute(ctx, r)
	require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
	require.Nil(t, result)
}

func TestIntegrationReserveAdmissionConcurrentCrossedAccounts(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixture(t, db)
	r.ValidationMode = tracercontract.ValidationLimits
	sharedLimit := admissionLimit(t, db, r, 89301, "100")
	other := r.Context.Accounts[0]
	other.ID = testutil.MustDeterministicUUID(89302)
	scopes, err := json.Marshal([]model.Scope{{AccountID: &r.Context.Accounts[0].ID}, {AccountID: &other.ID}})
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), "UPDATE limits SET scopes=$2 WHERE id=$1", sharedLimit, scopes)
	require.NoError(t, err)
	r.Context.Accounts = append(r.Context.Accounts, other)
	entry := r.Context.Entries[0]
	entry.AccountID = other.ID
	r.Context.Entries = append(r.Context.Entries, entry)
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 15*time.Second)
	defer cancel()
	const calls = 12
	start := make(chan struct{})
	type outcome struct {
		r   *tracercontract.ReserveResult
		err error
	}
	done := make(chan outcome, calls)
	for i := range calls {
		go func() {
			<-start
			request := r
			if i%2 == 1 {
				request.TransactionID = testutil.MustDeterministicUUID(89304)
				request.RequestID = testutil.MustDeterministicUUID(89305)
				request.Context.Entries = slices.Clone(r.Context.Entries)
				slices.Reverse(request.Context.Entries)
			}
			result, err := c.Execute(ctx, request)
			done <- outcome{result, err}
		}()
	}
	close(start)
	ids := map[uuid.UUID]int{}
	for range calls {
		out := <-done
		require.NoError(t, out.err)
		require.Equal(t, tracercontract.DecisionAllow, out.r.Decision)
		require.Len(t, out.r.ReservationIDs, 2)
		ids[out.r.EvaluationID]++
	}
	require.Len(t, ids, 2)
	var decisions, reservations, events int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM reserve_decisions").Scan(&decisions))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_reservations").Scan(&reservations))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events").Scan(&events))
	require.Equal(t, 2, decisions)
	require.Equal(t, 4, reservations)
	require.Equal(t, 2, events)
	for id, count := range ids {
		require.Equal(t, calls/2, count, fmt.Sprint(id))
	}
}

func TestIntegrationReserveAdmissionRollsBackEarlierCapacity(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixture(t, db)
	r.ValidationMode = tracercontract.ValidationLimits
	limits := []uuid.UUID{admissionLimit(t, db, r, 89401, "100"), admissionLimit(t, db, r, 89402, "100")}
	slices.SortFunc(limits, func(a, b uuid.UUID) int { return bytes.Compare(a[:], b[:]) })
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET max_amount=20 WHERE id=$1", limits[1])
	require.NoError(t, err)
	period := testutil.FixedTime().Format("2006-01-02")
	scope := "acct:" + r.Context.Accounts[0].ID.String()
	for _, id := range limits {
		_, err = db.ExecContext(t.Context(), "INSERT INTO usage_counters(limit_id,scope_key,period_key,current_usage,reserved_usage) VALUES($1,$2,$3,10,0)", id, scope, period)
		require.NoError(t, err)
	}
	got, err := c.Execute(completionContext(t.Context(), "producer"), r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionDeny, got.Decision)
	require.Empty(t, got.ReservationIDs)
	for _, id := range limits {
		current, held := readCounterDecimal(t, db, id, scope, period)
		require.Equal(t, "10", current.String())
		require.True(t, held.IsZero())
	}
	var rows int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations").Scan(&rows))
	require.Zero(t, rows)
	require.Len(t, completionEvents(t, db, r.TransactionID), 1)
}

func TestIntegrationReserveAdmissionTenantIsolation(t *testing.T) {
	a, b := completionDatabase(t, "a"), completionDatabase(t, "b")
	connection := &pgdb.PostgresConnectionAdapter{}
	connection.SetMultiTenantEnabled(true)
	c, repoA, r := admissionFixtureWithConnection(t, a, connection, testutil.FixedTime(), false)
	_, repoB, _ := admissionFixture(t, b)
	admissionPolicy(t, a, repoA, model.DecisionAllow)
	admissionPolicy(t, b, repoB, model.DecisionDeny)
	for _, db := range []*sql.DB{a, b} {
		admissionLimit(t, db, r, 89501, "100")
	}
	ctxA := tmcore.ContextWithPG(tmcore.ContextWithTenantID(completionContext(t.Context(), "producer"), "tenant-a"), dbresolver.New(dbresolver.WithPrimaryDBs(a)))
	ctxB := tmcore.ContextWithPG(tmcore.ContextWithTenantID(completionContext(t.Context(), "producer"), "tenant-b"), dbresolver.New(dbresolver.WithPrimaryDBs(b)))
	_, err := c.Execute(completionContext(t.Context(), "producer"), r)
	require.ErrorIs(t, err, constant.ErrReservationTenantRequired)
	_, err = c.Execute(tmcore.ContextWithTenantID(completionContext(t.Context(), "producer"), "tenant-a"), r)
	require.ErrorIs(t, err, pgdb.ErrNoTenantInContext)
	first, err := c.Execute(ctxA, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, first.Decision)
	second, err := c.Execute(ctxB, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionDeny, second.Decision)
	require.NotEqual(t, first.EvaluationID, second.EvaluationID)
	for _, db := range []*sql.DB{a, b} {
		require.Len(t, completionEvents(t, db, r.TransactionID), 1)
	}
}

func TestIntegrationReserveAdmissionExternalWithoutLimits(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixture(t, db)
	r.ValidationMode = tracercontract.ValidationLimits
	r.Context.Accounts = []tracercontract.Account{}
	r.Context.Entries[0].AccountID = uuid.Nil
	r.Context.Entries[0].External = true
	ctx := completionContext(t.Context(), "producer")
	got, err := c.Execute(ctx, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, got.Decision)
	require.Empty(t, got.ReservationIDs)
	complete, _, _ := completionCommand(t, db, true)
	_, err = complete.Execute(ctx, r.TransactionID, model.OperationConfirmed)
	require.NoError(t, err)
	again, err := c.Execute(ctx, r)
	require.NoError(t, err)
	require.Equal(t, got, again)
	require.Len(t, completionEvents(t, db, r.TransactionID), 2)
}

func TestIntegrationReserveAdmissionConcurrentContentConflict(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixture(t, db)
	r.ValidationMode = tracercontract.ValidationLimits
	admissionLimit(t, db, r, 89601, "100")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, amount := range []tracercontract.Amount{"10.125", "11"} {
		go func() {
			request := r
			request.Amount = amount
			<-start
			_, err := c.Execute(ctx, request)
			results <- err
		}()
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
			conflicts++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	require.Len(t, completionEvents(t, db, r.TransactionID), 1)
	var reservations int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations").Scan(&reservations))
	require.Equal(t, 1, reservations)
}
