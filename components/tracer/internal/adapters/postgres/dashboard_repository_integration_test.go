// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// The validation trail is append-only: migration 000003 installs an ON DELETE
// DO INSTEAD NOTHING rule, so a test CANNOT clean up the rows it wrote. Every
// test below therefore owns a disjoint slice of time and asserts only over its
// own window — which is also how the production reads are bounded, so the
// isolation and the contract are the same mechanism.
const dashboardTestEpoch = "2021-01-01T00:00:00Z"

// dashboardWindowAt carves the nth 10-day window out of the test epoch, spaced
// 30 days apart so consecutive windows do NOT touch. The gap is load-bearing:
// the half-open test below deliberately writes a row at its window's exclusive
// upper bound, and with contiguous slots that row landed on day zero of the
// next test's window and inflated its counts. Nothing can clean it up either,
// because the trail is append-only.
func dashboardWindowAt(t *testing.T, slot int) model.DashboardWindow {
	t.Helper()

	base, err := time.Parse(time.RFC3339, dashboardTestEpoch)
	require.NoError(t, err)

	from := base.AddDate(0, 0, slot*30).UTC()

	return model.DashboardWindow{From: from, To: from.AddDate(0, 0, 10)}
}

// seededValidation is one row of the fixture, described in the terms the
// assertions use.
type seededValidation struct {
	dayOffset       int
	decision        string
	transactionType string
	asset           string
	amount          string
	processingMs    float64
	matchedRules    []uuid.UUID
	evaluatedRules  []uuid.UUID
}

// seedValidations inserts the fixture inside the given window.
func seedValidations(t *testing.T, db *sql.DB, window model.DashboardWindow, rows []seededValidation) {
	t.Helper()

	for i, row := range rows {
		createdAt := window.From.AddDate(0, 0, row.dayOffset).Add(time.Duration(i) * time.Minute)
		require.True(t, createdAt.Before(window.To), "fixture row %d falls outside its own window", i)

		matched := row.matchedRules
		if matched == nil {
			matched = []uuid.UUID{}
		}

		evaluated := row.evaluatedRules
		if evaluated == nil {
			evaluated = []uuid.UUID{}
		}

		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			uuid.New(), row.transactionType, row.amount, row.asset, createdAt,
			`{"accountId":"11111111-1111-1111-1111-111111111111","type":"CHECKING"}`,
			row.decision, pqUUIDArray(matched), pqUUIDArray(evaluated),
			row.processingMs, createdAt,
		)
		require.NoError(t, err, "seeding fixture row %d", i)
	}
}

// pqUUIDArray renders a UUID slice as a PostgreSQL array literal. lib/pq's
// array helper is not a dependency here and the fixture is small, so the
// literal is written directly.
func pqUUIDArray(ids []uuid.UUID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}

	return "{" + strings.Join(parts, ",") + "}"
}

func newDashboardTestRepo(t *testing.T, db *sql.DB) *DashboardRepository {
	t.Helper()

	return NewDashboardRepository(&testutil.IntegrationDBAdapter{DB: db}, clock.NewFixedClock(testutil.FixedTime()))
}

func TestDashboardRepository_Metrics_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 1)

	// 10 validations: 7 ALLOW, 2 DENY, 1 REVIEW.
	// Blocked volume: USD 100.50 + 200.25 = 300.75, one asset only.
	// Mean processing time: (1+2+3+4+5+6+7+8+9+10)/10 = 5.5ms.
	seedValidations(t, db, window, []seededValidation{
		{dayOffset: 0, decision: "ALLOW", transactionType: "CARD", asset: "USD", amount: "10.00", processingMs: 1},
		{dayOffset: 0, decision: "ALLOW", transactionType: "CARD", asset: "USD", amount: "10.00", processingMs: 2},
		{dayOffset: 1, decision: "ALLOW", transactionType: "WIRE", asset: "USD", amount: "10.00", processingMs: 3},
		{dayOffset: 1, decision: "ALLOW", transactionType: "WIRE", asset: "USD", amount: "10.00", processingMs: 4},
		{dayOffset: 2, decision: "ALLOW", transactionType: "PIX", asset: "USD", amount: "10.00", processingMs: 5},
		{dayOffset: 2, decision: "ALLOW", transactionType: "PIX", asset: "USD", amount: "10.00", processingMs: 6},
		{dayOffset: 3, decision: "ALLOW", transactionType: "CRYPTO", asset: "USD", amount: "10.00", processingMs: 7},
		{dayOffset: 3, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "100.50", processingMs: 8},
		{dayOffset: 4, decision: "DENY", transactionType: "WIRE", asset: "USD", amount: "200.25", processingMs: 9},
		{dayOffset: 4, decision: "REVIEW", transactionType: "PIX", asset: "USD", amount: "50.00", processingMs: 10},
	})

	metrics, err := newDashboardTestRepo(t, db).Metrics(context.Background(), window)
	require.NoError(t, err)

	assert.EqualValues(t, 10, metrics.TransactionsProcessed)
	assert.EqualValues(t, 7, metrics.Allowed)
	assert.EqualValues(t, 2, metrics.FraudsBlocked)
	assert.EqualValues(t, 1, metrics.ManualReviews)

	assert.InDelta(t, 0.7, metrics.ApprovalRate, 1e-9)
	assert.InDelta(t, 0.2, metrics.FraudDetectionRate, 1e-9)
	assert.InDelta(t, 0.1, metrics.ManualReviewRate, 1e-9)
	assert.InDelta(t, 5.5, metrics.AvgProcessingTimeMs, 1e-9)

	require.Len(t, metrics.AmountSavedByAsset, 1)
	assert.Equal(t, "USD", metrics.AmountSavedByAsset[0].Asset)
	assert.Equal(t, "300.75", metrics.AmountSavedByAsset[0].Amount.String(),
		"only DENY amounts count, and they are summed exactly rather than in float")
	assert.Equal(t, "300.75", metrics.AmountSaved, "a single-asset window names its figure")
	assert.Equal(t, "USD", metrics.Asset)

	assert.Equal(t, window.From, metrics.WindowStart)
	assert.Equal(t, window.To, metrics.WindowEnd)
	assert.False(t, metrics.UpdatedAt.IsZero(), "the figures must say when they were computed")
}

// Money never crosses assets, and the split must come from ONE aggregation so
// the parts cannot disagree with the whole.
func TestDashboardRepository_Metrics_MultiAsset_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 2)

	seedValidations(t, db, window, []seededValidation{
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "100.00", processingMs: 1},
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "BRL", amount: "900.00", processingMs: 2},
		{dayOffset: 1, decision: "DENY", transactionType: "PIX", asset: "BRL", amount: "100.00", processingMs: 3},
		{dayOffset: 1, decision: "ALLOW", transactionType: "WIRE", asset: "EUR", amount: "500.00", processingMs: 4},
	})

	metrics, err := newDashboardTestRepo(t, db).Metrics(context.Background(), window)
	require.NoError(t, err)

	assert.EqualValues(t, 4, metrics.TransactionsProcessed)
	assert.EqualValues(t, 3, metrics.FraudsBlocked)

	require.Len(t, metrics.AmountSavedByAsset, 2, "EUR blocked nothing, so it carries no money to report")
	assert.Equal(t, "BRL", metrics.AmountSavedByAsset[0].Asset, "largest exposure first")
	assert.Equal(t, "1000", metrics.AmountSavedByAsset[0].Amount.String())
	assert.Equal(t, "USD", metrics.AmountSavedByAsset[1].Asset)
	assert.Equal(t, "100", metrics.AmountSavedByAsset[1].Amount.String())

	assert.Empty(t, metrics.AmountSaved, "two assets have no single figure")
	assert.Empty(t, metrics.Asset)
}

func TestDashboardRepository_Metrics_EmptyWindow_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)

	metrics, err := newDashboardTestRepo(t, db).Metrics(context.Background(), dashboardWindowAt(t, 3))
	require.NoError(t, err, "an empty window is an answer, not an error")

	assert.Zero(t, metrics.TransactionsProcessed)
	assert.Zero(t, metrics.ApprovalRate, "no traffic divides by nothing")
	assert.Empty(t, metrics.AmountSavedByAsset)
	assert.NotNil(t, metrics.AmountSavedByAsset, "an empty breakdown serializes as [], never null")
}

// The window is half-open [From, To): a row exactly at To belongs to the next
// window, or consecutive windows would double-count it.
func TestDashboardRepository_Metrics_WindowIsHalfOpen_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 4)

	insert := func(at time.Time) {
		_, err := db.Exec(`
			INSERT INTO transaction_validations
				(request_id, transaction_type, amount, asset, transaction_timestamp,
				 account, decision, matched_rule_ids, evaluated_rule_ids,
				 processing_time_ms, created_at)
			VALUES ($1,'CARD','10.00','USD',$2,'{}'::jsonb,'ALLOW','{}','{}',1.0,$2)`,
			uuid.New(), at)
		require.NoError(t, err)
	}

	insert(window.From)                   // included: the lower bound is inclusive
	insert(window.To)                     // excluded: the upper bound is exclusive
	insert(window.From.Add(-time.Second)) // excluded: before the window

	metrics, err := newDashboardTestRepo(t, db).Metrics(context.Background(), window)
	require.NoError(t, err)

	assert.EqualValues(t, 1, metrics.TransactionsProcessed,
		"only the row at From belongs to [From, To)")
}

func TestDashboardRepository_Volume_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 5)

	// 3 on day 0, 1 on day 2, 2 on day 4. Day 1 and day 3 carry nothing.
	rows := []seededValidation{
		{dayOffset: 0, decision: "ALLOW", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 0, decision: "ALLOW", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 2, decision: "ALLOW", transactionType: "PIX", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 4, decision: "REVIEW", transactionType: "WIRE", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 4, decision: "ALLOW", transactionType: "WIRE", asset: "USD", amount: "1.00", processingMs: 1},
	}
	seedValidations(t, db, window, rows)

	volume, err := newDashboardTestRepo(t, db).Volume(context.Background(), window)
	require.NoError(t, err)

	require.Len(t, volume.Points, 3, "only days that carried traffic produce a point")

	want := map[string]int64{
		window.From.Format(time.DateOnly):                  3,
		window.From.AddDate(0, 0, 2).Format(time.DateOnly): 1,
		window.From.AddDate(0, 0, 4).Format(time.DateOnly): 2,
	}

	got := map[string]int64{}
	for _, p := range volume.Points {
		got[p.Date] = p.Volume
	}

	assert.Equal(t, want, got)

	for i := 1; i < len(volume.Points); i++ {
		assert.Less(t, volume.Points[i-1].Date, volume.Points[i].Date, "points are chronological")
	}

	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}$`, volume.Points[0].Date, "the console reads YYYY-MM-DD")
}

func TestDashboardRepository_FraudTypes_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 6)

	// Flagged (DENY + REVIEW): CARD 3, PIX 1. WIRE flags nothing but carries
	// traffic, so it proves Total is reported independently of Count.
	seedValidations(t, db, window, []seededValidation{
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 1, decision: "REVIEW", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 1, decision: "ALLOW", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 2, decision: "DENY", transactionType: "PIX", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 2, decision: "ALLOW", transactionType: "WIRE", asset: "USD", amount: "1.00", processingMs: 1},
		{dayOffset: 3, decision: "ALLOW", transactionType: "WIRE", asset: "USD", amount: "1.00", processingMs: 1},
	})

	result, err := newDashboardTestRepo(t, db).FraudTypes(context.Background(), window)
	require.NoError(t, err)

	assert.EqualValues(t, 4, result.TotalFlagged)

	byType := map[string]model.FraudTypeSlice{}
	for _, slice := range result.Types {
		byType[slice.Type] = slice
	}

	require.Contains(t, byType, "CARD")
	assert.EqualValues(t, 3, byType["CARD"].Count)
	assert.EqualValues(t, 4, byType["CARD"].Total, "CARD carried one ALLOW as well")
	assert.InDelta(t, 0.75, byType["CARD"].Percentage, 1e-9)

	require.Contains(t, byType, "PIX")
	assert.EqualValues(t, 1, byType["PIX"].Count)
	assert.InDelta(t, 0.25, byType["PIX"].Percentage, 1e-9)

	require.Contains(t, byType, "WIRE")
	assert.EqualValues(t, 0, byType["WIRE"].Count, "a type with clean traffic still reports")
	assert.EqualValues(t, 2, byType["WIRE"].Total)
	assert.InDelta(t, 0.0, byType["WIRE"].Percentage, 1e-9)

	var shares float64
	for _, slice := range result.Types {
		shares += slice.Percentage
	}

	assert.InDelta(t, 1.0, shares, 1e-9, "shares of the flagged traffic sum to one")
}

// Active counts are point-in-time, not windowed: they describe the rules
// guarding traffic NOW, and a window far in the past must not change them.
func TestDashboardRepository_Metrics_ActiveCounts_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 7)

	ruleID := uuid.New()
	_, err := db.Exec(`
		INSERT INTO rules (id, name, expression, action, scopes, status, activated_at)
		VALUES ($1,$2,'amount > 1','DENY','[]'::jsonb,'ACTIVE',now())`,
		ruleID, "dashboard-active-"+ruleID.String()[:8])
	require.NoError(t, err)

	deletedID := uuid.New()
	_, err = db.Exec(`
		INSERT INTO rules (id, name, expression, action, scopes, status, deleted_at)
		VALUES ($1,$2,'amount > 1','DENY','[]'::jsonb,'ACTIVE',now())`,
		deletedID, "dashboard-deleted-"+deletedID.String()[:8])
	require.NoError(t, err)

	metrics, err := newDashboardTestRepo(t, db).Metrics(context.Background(), window)
	require.NoError(t, err)

	var wantRules int64
	require.NoError(t, db.QueryRow(
		`SELECT COUNT(*) FROM rules WHERE status='ACTIVE' AND deleted_at IS NULL`).Scan(&wantRules))

	assert.Equal(t, wantRules, metrics.ActiveRules)
	assert.GreaterOrEqual(t, metrics.ActiveRules, int64(1))

	var softDeletedCounted bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM rules WHERE id=$1 AND deleted_at IS NOT NULL)`, deletedID).Scan(&softDeletedCounted))
	assert.True(t, softDeletedCounted, "fixture sanity: the soft-deleted rule exists")
	assert.NotEqual(t, wantRules+1, metrics.ActiveRules, "a soft-deleted rule is not active")
}

// The cost story is a contract, not a note. If someone drops the INCLUDE
// columns from migration 000024, these reads silently fall back to a heap
// fetch that measured 3-7x slower at 1,000,000 rows — this test is what makes
// that regression loud.
func TestDashboardRepository_QueriesUseTheCoveringIndex_Integration(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	window := dashboardWindowAt(t, 8)

	seedValidations(t, db, window, []seededValidation{
		{dayOffset: 0, decision: "DENY", transactionType: "CARD", asset: "USD", amount: "1.00", processingMs: 1},
	})

	// The planner only prefers an index scan when statistics justify it, so the
	// assertion is made against a planner that cannot choose a sequential scan.
	// What is being pinned is that the index CAN serve the query index-only,
	// which is a property of the index definition, not of the row count.
	_, err := db.Exec("SET enable_seqscan = off")
	require.NoError(t, err)

	var indexExists bool
	require.NoError(t, db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM pg_indexes WHERE indexname = 'idx_transaction_validations_dashboard')`).
		Scan(&indexExists))
	require.True(t, indexExists, "migration 000024 must have created the covering index")

	tests := []struct {
		name  string
		query string
	}{
		{name: "metrics", query: metricsQuery},
		{name: "fraud-types", query: fraudTypesQuery},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := explain(t, db, tt.query, window)

			assert.Contains(t, plan, "Index Only Scan",
				"%s must not fetch heap rows; plan was:\n%s", tt.name, plan)
			assert.NotContains(t, plan, "Heap",
				"%s must not fetch heap rows; plan was:\n%s", tt.name, plan)
			assert.Contains(t, plan, "idx_transaction_validations_dashboard",
				"%s reads decision/asset/amount/processing_time_ms, which only the covering index carries; plan was:\n%s",
				tt.name, plan)
		})
	}

	t.Run("volume", func(t *testing.T) {
		plan := explain(t, db, volumeQuery, window)

		// Volume reads created_at alone, so EITHER index can serve it
		// index-only and the planner is free to pick on cost. The contract
		// being pinned is that it never touches the heap — not which of the
		// two btrees wins, which changes with the row count.
		assert.Contains(t, plan, "Index Only Scan", "plan was:\n%s", plan)
		assert.NotContains(t, plan, "Heap", "volume must not fetch heap rows; plan was:\n%s", plan)
	})
}

// explain returns the query plan as text.
func explain(t *testing.T, db *sql.DB, query string, window model.DashboardWindow) string {
	t.Helper()

	rows, err := db.Query("EXPLAIN "+query, window.From, window.To)
	require.NoError(t, err)

	defer func() { _ = rows.Close() }()

	var plan strings.Builder

	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		fmt.Fprintln(&plan, line)
	}

	require.NoError(t, rows.Err())

	return plan.String()
}
