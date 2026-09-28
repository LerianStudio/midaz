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
	"github.com/shopspring/decimal"
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

func admissionFixtureWithConnection(t *testing.T, db *sql.DB, conn pgdb.Connection, now time.Time, singleTenant bool, accountBounds ...int) (*command.ReserveAdmissionCommand, *ContextPolicyRepository, tracercontract.ReserveRequest) {
	t.Helper()
	maxAccounts := 10
	if len(accountBounds) > 0 {
		maxAccounts = accountBounds[0]
	}
	return admissionFixtureWithLifetime(t, db, conn, now, singleTenant, maxAccounts, time.Hour, 720*time.Hour)
}

func admissionFixtureWithLifetime(t *testing.T, db *sql.DB, conn pgdb.Connection, now time.Time, singleTenant bool, maxAccounts int, lifetime, longLivedLifetime time.Duration) (*command.ReserveAdmissionCommand, *ContextPolicyRepository, tracercontract.ReserveRequest) {
	t.Helper()
	facts := tracercontract.Limits{MaxAccounts: maxAccounts, MaxEntries: 2 * maxAccounts, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128}
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
	limits, err := NewContextLimitRepository(ContextLimitRepositoryConfig{MaxAccounts: maxAccounts, MaxLimits: 10, MaxScopes: maxAccounts, MaxScopeBytes: 256 * maxAccounts})
	require.NoError(t, err)
	c, err := command.NewReserveAdmissionCommand(command.ReserveAdmissionDependencies{
		Decisions: decisions, Operations: NewReserveOperationRepository(), Capacity: newReservationRepoIntegration(db), Limits: limits, Policies: compiled, Evaluator: evaluator, Audit: NewAuditEventRepositoryWithConnection(conn), Transactions: beginner,
	}, clock.NewFixedClock(now), command.ReserveAdmissionConfig{Plan: query.ContextReservationConfig{Facts: facts, MaxLimits: 10, MaxScopesPerLimit: maxAccounts, MaxReservations: 100}, MaxRules: 10, SingleTenant: singleTenant, MaxTimestampAge: 24 * time.Hour, ClockSkewTolerance: time.Second, ReservationLifetime: lifetime, LongLivedLifetime: longLivedLifetime})
	require.NoError(t, err)
	account := testutil.MustDeterministicUUID(89001)
	asset := "USD"
	blocked, longLived := false, false
	r := tracercontract.ReserveRequest{ContractRevision: tracercontract.ReserveContractRevision, TransactionID: testutil.MustDeterministicUUID(89002), RequestID: testutil.MustDeterministicUUID(89003), ContextID: "official", ValidationMode: tracercontract.ValidationRulesAndLimits, TransactionTimestamp: testutil.FixedTime(), LongLived: &longLived, Amount: "10.125", Asset: asset, Context: tracercontract.Context{Accounts: []tracercontract.Account{{ID: account, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: asset}}, Entries: []tracercontract.Entry{{AccountID: account, Direction: tracercontract.Debit, Amount: "10.125", Asset: asset}}}}
	return c, policies, r
}

func admissionPolicy(t *testing.T, db *sql.DB, repo *ContextPolicyRepository, action model.Decision, rules ...model.ContextPolicyRule) {
	t.Helper()
	policy := model.ContextPolicy{ID: testutil.MustDeterministicUUID(89004), Revision: 1, DefaultDecision: model.DecisionDeny, Rules: []model.ContextPolicyRule{{ID: testutil.MustDeterministicUUID(89005), Revision: 1, Expression: "true", Action: action}}}
	if len(rules) > 0 {
		policy.Rules = rules
	}
	tx, err := db.BeginTx(t.Context(), nil)
	require.NoError(t, err)
	defer tx.Rollback()
	require.NoError(t, repo.PublishWithTx(t.Context(), tx, policy, "operator", testutil.FixedTime()))
	require.NoError(t, repo.BindWithTx(t.Context(), tx, model.PolicyBindingKey{IntegrationID: "producer", ContextID: "official"}, model.PolicyRevision{ID: policy.ID, Revision: 1}, nil, "operator", testutil.FixedTime()))
	require.NoError(t, tx.Commit())
}

func admissionLimit(t *testing.T, db *sql.DB, r tracercontract.ReserveRequest, seed int64, maxAmount string, scopes ...model.Scope) uuid.UUID {
	t.Helper()
	id := contextLimitRow(t, db, seed, r.Context.Accounts[0].ID)
	_, err := db.ExecContext(t.Context(), "UPDATE limits SET max_amount=$2 WHERE id=$1", id, maxAmount)
	require.NoError(t, err)
	if len(scopes) > 0 {
		raw, err := json.Marshal(scopes)
		require.NoError(t, err)
		_, err = db.ExecContext(t.Context(), "UPDATE limits SET scopes=$2 WHERE id=$1", id, raw)
		require.NoError(t, err)
	}
	setContextLimitAsset(t, db, id, r.Asset)
	return id
}

func withReserveAsset(r tracercontract.ReserveRequest, asset string, seed int64) tracercontract.ReserveRequest {
	r.TransactionID = testutil.MustDeterministicUUID(seed)
	r.RequestID = testutil.MustDeterministicUUID(seed + 1)
	r.Asset = asset
	r.Context.Accounts = slices.Clone(r.Context.Accounts)
	r.Context.Entries = slices.Clone(r.Context.Entries)
	for i := range r.Context.Accounts {
		r.Context.Accounts[i].Asset = asset
	}
	for i := range r.Context.Entries {
		r.Context.Entries[i].Asset = asset
	}
	return r
}

func TestIntegrationReserveAdmissionMatchesLimitsByAssetCode(t *testing.T) {
	db := completionDatabase(t)
	c, _, base := admissionFixture(t, db)
	base.ValidationMode = tracercontract.ValidationLimits
	btc := withReserveAsset(base, "BTC", 89701)
	limit := admissionLimit(t, db, btc, 89700, "10")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()

	denied, err := c.Execute(ctx, btc)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionDeny, denied.Decision)
	require.Equal(t, []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}, denied.Reasons)
	require.Empty(t, denied.ReservationIDs)

	xbt := withReserveAsset(base, "XBT", 89703)
	allowed, err := c.Execute(ctx, xbt)
	require.NoError(t, err, "a limit of another asset code is not applicable")
	require.Equal(t, tracercontract.DecisionAllow, allowed.Decision)
	require.Equal(t, []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}, allowed.Reasons)
	require.Empty(t, allowed.ReservationIDs)
	var counters, reservations int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_counters WHERE limit_id=$1", limit).Scan(&counters))
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM usage_reservations").Scan(&reservations))
	require.Zero(t, counters, "neither decision may touch the BTC counter")
	require.Zero(t, reservations)

	for _, tc := range []struct {
		request tracercontract.ReserveRequest
		want    *tracercontract.ReserveResult
	}{{btc, denied}, {xbt, allowed}} {
		again, err := c.Execute(ctx, tc.request)
		require.NoError(t, err)
		require.Equal(t, tc.want, again)
		require.Len(t, completionEvents(t, db, tc.request.TransactionID), 1)
	}
	var decisions int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM reserve_decisions").Scan(&decisions))
	require.Equal(t, 2, decisions)
	// The fingerprint binds the asset code: the same operation with another code conflicts.
	conflicting := withReserveAsset(base, "XBT", 89701)
	result, err := c.Execute(ctx, conflicting)
	require.ErrorIs(t, err, constant.ErrReserveDecisionConflict)
	require.Nil(t, result)
}

// withReserveDebits replaces the request context with one debit per account,
// each in the code its account holds.
func withReserveDebits(r tracercontract.ReserveRequest, seed int64, debits ...tracercontract.Entry) tracercontract.ReserveRequest {
	blocked := false
	r.TransactionID = testutil.MustDeterministicUUID(seed)
	r.RequestID = testutil.MustDeterministicUUID(seed + 1)
	r.Context = tracercontract.Context{}
	for _, debit := range debits {
		debit.Direction = tracercontract.Debit
		r.Context.Accounts = append(r.Context.Accounts, tracercontract.Account{ID: debit.AccountID, Type: "checking", Status: "ACTIVE", Blocked: &blocked, Asset: debit.Asset})
		r.Context.Entries = append(r.Context.Entries, debit)
	}
	return r
}

func TestIntegrationReserveAdmissionMultiAssetScopeMatchesByCode(t *testing.T) {
	db := completionDatabase(t)
	c, _, base := admissionFixture(t, db)
	base.ValidationMode = tracercontract.ValidationLimits
	base.Asset = "BTC"
	x, y := testutil.MustDeterministicUUID(89801), testutil.MustDeterministicUUID(89802)
	scoped := withReserveDebits(base, 89803, tracercontract.Entry{AccountID: y, Amount: "1", Asset: "BTC"})
	limit := admissionLimit(t, db, scoped, 89800, "100", model.Scope{AccountID: &x}, model.Scope{AccountID: &y})
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()

	xbtOnX := tracercontract.Entry{AccountID: x, Amount: "500", Asset: "XBT"}
	for i, tc := range []struct {
		name     string
		debits   []tracercontract.Entry
		decision tracercontract.Decision
		reserved int
	}{
		{"XBT on X and BTC over cap on Y", []tracercontract.Entry{xbtOnX, {AccountID: y, Amount: "150", Asset: "BTC"}}, tracercontract.DecisionDeny, 0},
		{"BTC over cap on Y alone", []tracercontract.Entry{{AccountID: y, Amount: "150", Asset: "BTC"}}, tracercontract.DecisionDeny, 0},
		{"XBT on X alone", []tracercontract.Entry{xbtOnX}, tracercontract.DecisionAllow, 0},
		{"XBT on X and BTC within cap on Y", []tracercontract.Entry{xbtOnX, {AccountID: y, Amount: "50", Asset: "BTC"}}, tracercontract.DecisionAllow, 1},
	} {
		result, err := c.Execute(ctx, withReserveDebits(base, 89810+int64(2*i), tc.debits...))
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.decision, result.Decision, tc.name)
		require.Len(t, result.ReservationIDs, tc.reserved, tc.name)
		if tc.decision == tracercontract.DecisionDeny {
			require.Equal(t, []tracercontract.ReserveReason{tracercontract.ReasonLimitExceeded}, result.Reasons, tc.name)
		}
	}
	var scopes []string
	rows, err := db.QueryContext(ctx, "SELECT scope_key FROM usage_reservations WHERE limit_id=$1", limit)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var key string
		require.NoError(t, rows.Scan(&key))
		scopes = append(scopes, key)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []string{"acct:" + y.String()}, scopes, "only the BTC debit reserves against the BTC limit")
}

func TestIntegrationReserveAdmissionNonConformingStoredCodeSelectsNoLimit(t *testing.T) {
	db := completionDatabase(t)
	c, _, base := admissionFixture(t, db)
	base.ValidationMode = tracercontract.ValidationLimits
	account := testutil.MustDeterministicUUID(89851)
	request := withReserveDebits(base, 89852, tracercontract.Entry{AccountID: account, Amount: "10", Asset: "usdt"})
	// An ACTIVE limit scoped to the account, in the conforming code nearest the
	// stored one, with a cap below the debit: exact matching must not apply it.
	limitRequest := request
	limitRequest.Asset = "USDT"
	limit := admissionLimit(t, db, limitRequest, 89850, "1")
	var status, asset string
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT status, asset FROM limits WHERE id=$1", limit).Scan(&status, &asset))
	require.Equal(t, "ACTIVE", status)
	require.Equal(t, "USDT", asset)
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()
	result, err := c.Execute(ctx, request)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, result.Decision)
	require.Equal(t, []tracercontract.ReserveReason{tracercontract.ReasonLimitsSatisfied}, result.Reasons)
	require.Empty(t, result.ReservationIDs)
	var counters, reservations int
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_counters WHERE limit_id=$1", limit).Scan(&counters))
	require.NoError(t, db.QueryRowContext(t.Context(), "SELECT count(*) FROM usage_reservations WHERE limit_id=$1", limit).Scan(&reservations))
	require.Zero(t, counters, "the USDT limit accrues no usage from a usdt debit")
	require.Zero(t, reservations, "the USDT limit holds no reservation for a usdt debit")
}

func TestIntegrationReserveAdmissionConcurrentLimitExhaustion(t *testing.T) {
	db := completionDatabase(t)
	admission, policies, request := admissionFixture(t, db)
	admissionPolicy(t, db, policies, model.DecisionAllow)
	limit := admissionLimit(t, db, request, 89901, "100")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 15*time.Second)
	defer cancel()
	const calls = 20
	start := make(chan struct{})
	type outcome struct {
		result *tracercontract.ReserveResult
		err    error
	}
	results := make(chan outcome, calls)
	for i := range calls {
		go func() {
			<-start
			input := request
			input.TransactionID = testutil.MustDeterministicUUID(89910 + int64(i))
			input.RequestID = testutil.MustDeterministicUUID(89940 + int64(i))
			result, err := admission.Execute(ctx, input)
			results <- outcome{result, err}
		}()
	}
	close(start)
	allowed, denied := 0, 0
	for range calls {
		result := <-results
		require.NoError(t, result.err)
		switch result.result.Decision {
		case tracercontract.DecisionAllow:
			allowed++
			require.Len(t, result.result.ReservationIDs, 1)
		case tracercontract.DecisionDeny:
			denied++
			require.Empty(t, result.result.ReservationIDs)
		default:
			t.Fatalf("unexpected decision %s", result.result.Decision)
		}
	}
	require.Equal(t, 9, allowed, "ten exact 10.125 debits would exceed 100")
	require.Equal(t, 11, denied)
	used, held := readCounterDecimal(t, db, limit, "acct:"+request.Context.Accounts[0].ID.String(), request.TransactionTimestamp.Format("2006-01-02"))
	require.True(t, used.IsZero())
	require.True(t, held.Equal(decimal.RequireFromString("91.125")), held.String())
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
				for attempt := range 2 {
					report, err := complete.ExecuteReport(ctx, r.TransactionID, model.OperationConfirmed)
					require.NoError(t, err)
					require.NoError(t, report.Validate())
					require.NotNil(t, report.EvaluationID)
					require.Equal(t, got.EvaluationID, *report.EvaluationID)
					require.Equal(t, 1-attempt, report.Flipped)
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
	other := r.Context.Accounts[0]
	other.ID = testutil.MustDeterministicUUID(89302)
	admissionLimit(t, db, r, 89301, "100", model.Scope{AccountID: &r.Context.Accounts[0].ID}, model.Scope{AccountID: &other.ID})
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
	report, err := complete.ExecuteReport(ctx, r.TransactionID, model.OperationConfirmed)
	require.NoError(t, err)
	require.Zero(t, report.Flipped)
	require.NotNil(t, report.EvaluationID)
	require.Equal(t, got.EvaluationID, *report.EvaluationID)
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

func TestIntegrationReserveAdmissionReservationLifetimeFollowsLongLived(t *testing.T) {
	for _, longLived := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "long-lived"}[longLived], func(t *testing.T) {
			db := completionDatabase(t)
			c, _, r := admissionFixtureWithLifetime(t, db, &testutil.IntegrationDBAdapter{DB: db}, testutil.FixedTime(), true, 10, 5*time.Minute, 720*time.Hour)
			r.ValidationMode = tracercontract.ValidationLimits
			r.LongLived = &longLived
			admissionLimit(t, db, r, 89801, "100")
			ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
			defer cancel()
			result, err := c.Execute(ctx, r)
			require.NoError(t, err)
			require.Equal(t, tracercontract.DecisionAllow, result.Decision)
			require.Len(t, result.ReservationIDs, 1)
			var expiresAt time.Time
			require.NoError(t, db.QueryRowContext(ctx, "SELECT reservation_expires_at FROM usage_reservations WHERE id=$1", result.ReservationIDs[0]).Scan(&expiresAt))
			want := 5 * time.Minute
			if longLived {
				want = 720 * time.Hour
			}
			require.Equal(t, testutil.FixedTime().Add(want), expiresAt.UTC())
		})
	}
}

// TestIntegrationReserveAdmissionReplayAfterExpiryConflicts proves an expired
// operation's stored ALLOW is never replayed: its capacity went back to the
// counter, so the producer must not treat the decision as still holding it.
func TestIntegrationReserveAdmissionReplayAfterExpiryConflicts(t *testing.T) {
	db := completionDatabase(t)
	c, _, r := admissionFixtureWithLifetime(t, db, &testutil.IntegrationDBAdapter{DB: db}, testutil.FixedTime(), true, 10, 5*time.Minute, 720*time.Hour)
	r.ValidationMode = tracercontract.ValidationLimits
	limitID := admissionLimit(t, db, r, 89811, "100")
	ctx, cancel := context.WithTimeout(completionContext(t.Context(), "producer"), 10*time.Second)
	defer cancel()

	first, err := c.Execute(ctx, r)
	require.NoError(t, err)
	require.Equal(t, tracercontract.DecisionAllow, first.Decision)
	require.Len(t, first.ReservationIDs, 1)
	replayed, err := c.Execute(ctx, r)
	require.NoError(t, err, "a replay before expiry returns the stored decision")
	require.Equal(t, first.EvaluationID, replayed.EvaluationID)

	key := model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: r.TransactionID}
	expired, err := expireCommand(t, db).Execute(ctx, key, testutil.FixedTime().Add(10*time.Minute))
	require.NoError(t, err)
	require.Equal(t, 1, expired)

	late, err := c.Execute(ctx, r)
	require.ErrorIs(t, err, constant.ErrReserveOperationConflict)
	require.Nil(t, late)
	var reserved string
	require.NoError(t, db.QueryRowContext(ctx, "SELECT reserved_usage::text FROM usage_counters WHERE limit_id=$1", limitID).Scan(&reserved))
	require.True(t, decimal.RequireFromString(reserved).IsZero(), "the replay reserves nothing again")
}
