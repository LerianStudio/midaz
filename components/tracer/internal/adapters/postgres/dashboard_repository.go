// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOtel "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/trace"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// Compile-time interface implementation check.
var _ query.DashboardRepository = (*DashboardRepository)(nil)

// DashboardRepository answers the dashboard reads out of the tenant's own
// database. Tenant resolution is the pgdb.Connection's job: in multi-tenant
// mode GetDB returns the pool the tenant middleware put in the context and
// refuses to fall back to the root pool, so nothing here needs — or is
// allowed — a tenant parameter.
//
// COST CONTRACT. Each method below is ONE statement bounded by created_at,
// every one of them inside a 200ms budget at 1,000,000 rows. Measured
// end-to-end over HTTP on the seeded trail
// (scripts/seed_dashboard_benchmark.sql, 365 days, PostgreSQL 17, 256MB
// shared_buffers), median of three warm runs:
//
//	metrics      7d   7ms | 30d  23ms | 90d  67ms  index-only, idx_tv_dashboard
//	volume       7d   5ms | 30d  18ms | 90d  53ms  index-only, idx_tv_created
//	fraud-types  7d   5ms | 30d  15ms | 90d  25ms  index-only, idx_tv_dashboard
//	top-rules    7d  29ms | 30d  46ms | 90d 106ms  index scan + HEAP
//
// Three of the four never touch the heap. TopRules does, and cannot be made
// not to: it reads matched_rule_ids and evaluated_rule_ids, whose unbounded
// width keeps them out of any covering index (see topRulesQuery), so it pays
// ~11,800 buffers at 90 days against ~1,800 for metrics. It is the endpoint
// that binds the operating limit, and the one the cache protects most.
//
// Every windowed statement passes windowPlanMode; read that comment before
// changing how these are issued.
//
// The index-only property depends on the visibility map: pages autovacuum has
// not yet marked all-visible still cost a heap fetch, so a trail under heavy
// write with autovacuum starved degrades toward a plain index scan over the
// heap (measured 1.4-1.7x slower on the same data). That is a tuning property
// of the deployment, not of these statements.
type DashboardRepository struct {
	conn  pgdb.Connection
	clock clock.Clock
}

// NewDashboardRepository creates a dashboard repository over the given
// connection. clk supplies UpdatedAt so the figures carry the time they were
// computed rather than the time they were read back out of a cache.
func NewDashboardRepository(conn pgdb.Connection, clk clock.Clock) *DashboardRepository {
	if clk == nil {
		clk = clock.New()
	}

	return &DashboardRepository{conn: conn, clock: clk}
}

// windowPlanMode forces every windowed dashboard read to be planned against
// the window it was actually handed.
//
// Without it these reads get 3.4x slower after five minutes of uptime, and
// nothing in the code changes to explain it. pgx's default exec mode caches a
// SERVER-SIDE prepared statement per connection; PostgreSQL plans the first
// five executions against the real parameters and then, under the default
// plan_cache_mode=auto, switches to a GENERIC plan built with no knowledge of
// them. A generic plan cannot know a window holds 25% of the table rather than
// the 0.3% its default selectivity assumes, so it drops the parallel scan and
// the Memoize node, and /top-rules over 90 days goes from 95 ms to 323 ms —
// past the 200 ms budget these reads are held to.
//
// Measured on 1,000,000 rows, one pooled connection, eight consecutive
// executions of the /top-rules query:
//
//	default (cache statement): 105 95 95 95 94 | 323 323 321  ms
//	QueryExecModeExec:         104 95 94 93 95 |  94  95  96  ms
//
// QueryExecModeExec uses an unnamed prepared statement, which PostgreSQL
// always plans against the given parameters. The re-planning it pays for is
// under a millisecond on these four statements and is not visible in the
// end-to-end numbers.
//
// The cause is specific, and it is NOT simply the created_at predicate all four
// reads share. Measured over eight executions each, /metrics and /volume do not
// regress at all (66/65/66/66/66 | 68/65/65 ms and 67/61/60/62/61 | 60/60/60
// ms): their plans hold up without the window's selectivity. What the generic
// plan discards is the LATERAL fan-out plan — the parallel scan and the Memoize
// over the unnested rule ids — which is why /top-rules loses 3.4x and
// /fraud-types, whose aggregate is also parameter-sensitive, loses 1.7x.
//
// The mode is nevertheless applied to all four. Its cost on the two that do not
// regress is zero within noise (measured: 66/65/65/66/65/65/65/65 ms under this
// mode against 66/64/64/64/64/66/64/64 default), and one rule that holds for
// every windowed read is easier to keep true than two exceptions that have to
// be re-measured whenever a query changes shape. activeCountsQuery takes no
// parameters and is therefore unaffected.
const windowPlanMode = pgx.QueryExecModeExec

// metricsQuery aggregates the window twice in one pass: once over every row
// (the grouping set `()`, which yields the headline counters) and once per
// asset (which yields the blocked volume split). Both aggregations read the
// same rows in the same statement, so the per-asset blocked amounts sum to the
// headline blocked count exactly. Two statements could not promise that: they
// are two instants, and the dashboard publishes total and breakdown side by
// side and caches the pair for a minute.
//
// is_total is GROUPING(): 1 on the grand-total row, 0 on the per-asset rows.
// The total sorts first so the scan can rely on reading it before any asset
// row, and assets follow in descending blocked volume with a name tiebreak, so
// two assets with equal exposure keep a stable order between reads.
//
// Every column the statement touches — created_at, decision, asset, amount,
// processing_time_ms — lives in idx_tv_dashboard, which is what keeps this an
// index-only scan.
const metricsQuery = `
	SELECT
		GROUPING(asset) AS is_total,
		asset,
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE decision = 'ALLOW')  AS allowed,
		COUNT(*) FILTER (WHERE decision = 'DENY')   AS denied,
		COUNT(*) FILTER (WHERE decision = 'REVIEW') AS review,
		COALESCE(SUM(amount) FILTER (WHERE decision = 'DENY'), 0) AS denied_amount,
		COALESCE(AVG(processing_time_ms), 0) AS avg_ms
	FROM transaction_validations
	WHERE created_at >= $1 AND created_at < $2
	GROUP BY GROUPING SETS ((), (asset))
	ORDER BY is_total DESC, denied_amount DESC, asset ASC`

// activeCountsQuery counts the rules and limits that are live right now. It is
// point-in-time rather than windowed on purpose: "how many rules are active"
// is a question about the configuration as it stands, not about the window,
// and answering it for a past window would report rules that have since been
// retired as if they were still guarding traffic.
//
// Both counts are over small configuration tables (tens to low thousands of
// rows), so they cost a sequential scan measured in microseconds.
const activeCountsQuery = `
	SELECT
		(SELECT COUNT(*) FROM rules  WHERE status = 'ACTIVE' AND deleted_at IS NULL),
		(SELECT COUNT(*) FROM limits WHERE status = 'ACTIVE' AND deleted_at IS NULL)`

// volumeQuery buckets the window by UTC calendar day. It reads created_at and
// nothing else, so the pre-existing idx_transaction_validations_created serves
// it index-only with no help from the covering index.
const volumeQuery = `
	SELECT (created_at AT TIME ZONE 'UTC')::date AS bucket, COUNT(*) AS volume
	FROM transaction_validations
	WHERE created_at >= $1 AND created_at < $2
	GROUP BY 1
	ORDER BY 1`

// fraudTypesQuery counts flagged (DENY + REVIEW) decisions per transaction
// type, alongside that type's total traffic so a reader can tell a rare type
// from a clean one. Four enum values bound the group count by construction.
const fraudTypesQuery = `
	SELECT transaction_type,
		COUNT(*) FILTER (WHERE decision IN ('DENY','REVIEW')) AS flagged,
		COUNT(*) AS total
	FROM transaction_validations
	WHERE created_at >= $1 AND created_at < $2
	GROUP BY transaction_type
	ORDER BY flagged DESC, transaction_type ASC`

// topRulesQuery ranks the rules by how often they fired in the window.
//
// ONE pass over the window, and the single most expensive statement of the
// four. The fan-out is the reason: CROSS JOIN LATERAL unnest turns each
// validation into one row per rule it evaluated (three in the benchmark
// fixture), so the aggregate sees ~3x the rows the other endpoints do and the
// arrays force a heap read the covering index cannot serve.
//
// matches counts the same unnested rows FILTERed to those the rule actually
// matched. That is only correct because matched_rule_ids is a SUBSET of
// evaluated_rule_ids by construction: the engine appends every rule id to
// EvaluatedRuleIDs BEFORE testing whether it matched
// (internal/services/query/complete_evaluator.go, "// c. Track in
// EvaluatedRuleIDs"), and the decision maker assembles matched ids from the
// deny, review and allow sets, each of which the same loop produced. Were that
// ever to change, a matched-but-not-evaluated rule would silently vanish from
// this panel rather than error, so the invariant is asserted by an integration
// test rather than left to this comment.
//
// productType is read from the RULE's own scopes, not from the traffic. The
// traffic answer needs mode() WITHIN GROUP over the unnested rows, which forces
// a full sort per group and measured 48 -> 200 ms at 30 days; it is also the
// wrong answer, describing what the rule happened to see rather than what it
// is configured to guard.
//
// The rule arrays are NOT indexed and must not be. A btree over them would
// carry one entry per array, and an array of ~167 active rule ids exceeds the
// btree tuple limit ("index row size 2712 exceeds btree version 4 maximum
// 2704") — which fails the INSERT, losing a validation that cannot be
// retried against an append-only trail. A GIN index answers containment, not
// the per-rule aggregation this needs.
const topRulesQuery = `
	SELECT r.name,
		(SELECT sc->>'transactionType'
		   FROM jsonb_array_elements(r.scopes) sc
		  WHERE sc ? 'transactionType'
		  LIMIT 1) AS product_type,
		s.matches,
		s.executions,
		COALESCE(s.matches::float8 / NULLIF(s.executions, 0), 0) AS detection_rate,
		COALESCE(s.avg_ms, 0) AS avg_ms
	FROM (
		SELECT u.rule_id,
			COUNT(*) AS executions,
			COUNT(*) FILTER (WHERE u.rule_id = ANY (v.matched_rule_ids)) AS matches,
			AVG(v.processing_time_ms) FILTER (WHERE u.rule_id = ANY (v.matched_rule_ids)) AS avg_ms
		FROM transaction_validations v
		CROSS JOIN LATERAL unnest(v.evaluated_rule_ids) AS u(rule_id)
		WHERE v.created_at >= $1 AND v.created_at < $2
		GROUP BY u.rule_id
	) s
	JOIN rules r ON r.id = s.rule_id
	ORDER BY s.matches DESC, r.name ASC
	LIMIT $3`

// Metrics returns the headline dashboard panel for the window.
func (r *DashboardRepository) Metrics(ctx context.Context, window model.DashboardWindow) (*model.DashboardMetrics, error) {
	ctx, span, logger := r.startSpan(ctx, "repository.dashboard.metrics")
	defer span.End()

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics: get database connection", err)
	}

	metrics, err := scanMetrics(ctx, db, window)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics", err)
	}

	if err := db.QueryRowContext(ctx, activeCountsQuery).Scan(&metrics.ActiveRules, &metrics.ActiveLimits); err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics: scanning active counts", err)
	}

	metrics.WindowStart = window.From
	metrics.WindowEnd = window.To
	metrics.UpdatedAt = r.clock.Now().UTC()
	metrics.ApplyRates()

	return metrics, nil
}

// scanMetrics folds the grouping-set rows into the metrics entity. The grand
// total arrives first (is_total DESC), so the asset rows that follow can be
// appended without a second pass.
func scanMetrics(ctx context.Context, db pgdb.DB, window model.DashboardWindow) (*model.DashboardMetrics, error) {
	rows, err := db.QueryContext(ctx, metricsQuery, windowPlanMode, window.From, window.To)
	if err != nil {
		return nil, fmt.Errorf("dashboard metrics: querying: %w", err)
	}
	defer func() { _ = rows.Close() }()

	metrics := &model.DashboardMetrics{AmountSavedByAsset: make([]model.AssetAmount, 0)}

	for rows.Next() {
		var (
			isTotal        int
			asset          sql.NullString
			total, allowed int64
			denied, review int64
			deniedAmount   decimal.Decimal
			avgMs          float64
		)

		if err := rows.Scan(&isTotal, &asset, &total, &allowed, &denied, &review, &deniedAmount, &avgMs); err != nil {
			return nil, fmt.Errorf("dashboard metrics: scanning row: %w", err)
		}

		if isTotal == 1 {
			metrics.TransactionsProcessed = total
			metrics.Allowed = allowed
			metrics.FraudsBlocked = denied
			metrics.ManualReviews = review
			metrics.AvgProcessingTimeMs = avgMs

			continue
		}

		// An asset row with nothing blocked carries no money to report, so it
		// is left out rather than published as a zero the reader must dismiss.
		if deniedAmount.IsZero() {
			continue
		}

		metrics.AmountSavedByAsset = append(metrics.AmountSavedByAsset, model.AssetAmount{
			Asset:  asset.String,
			Amount: deniedAmount,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard metrics: iterating rows: %w", err)
	}

	return metrics, nil
}

// Volume returns the per-day validation counts across the window.
func (r *DashboardRepository) Volume(ctx context.Context, window model.DashboardWindow) (*model.DashboardVolume, error) {
	ctx, span, logger := r.startSpan(ctx, "repository.dashboard.volume")
	defer span.End()

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, volumeQuery, windowPlanMode, window.From, window.To)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: querying", err)
	}
	defer func() { _ = rows.Close() }()

	volume := &model.DashboardVolume{
		Points:      make([]model.VolumePoint, 0),
		WindowStart: window.From,
		WindowEnd:   window.To,
		UpdatedAt:   r.clock.Now().UTC(),
	}

	for rows.Next() {
		var (
			bucket time.Time
			count  int64
		)

		if err := rows.Scan(&bucket, &count); err != nil {
			return nil, fail(ctx, span, logger, "dashboard volume: scanning row", err)
		}

		volume.Points = append(volume.Points, model.VolumePoint{
			Date:   bucket.Format(time.DateOnly),
			Volume: count,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: iterating rows", err)
	}

	return volume, nil
}

// FraudTypes returns the flagged breakdown by transaction type.
func (r *DashboardRepository) FraudTypes(ctx context.Context, window model.DashboardWindow) (*model.DashboardFraudTypes, error) {
	ctx, span, logger := r.startSpan(ctx, "repository.dashboard.fraud_types")
	defer span.End()

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard fraud types: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, fraudTypesQuery, windowPlanMode, window.From, window.To)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard fraud types: querying", err)
	}
	defer func() { _ = rows.Close() }()

	result := &model.DashboardFraudTypes{
		Types:       make([]model.FraudTypeSlice, 0),
		WindowStart: window.From,
		WindowEnd:   window.To,
		UpdatedAt:   r.clock.Now().UTC(),
	}

	for rows.Next() {
		var slice model.FraudTypeSlice

		if err := rows.Scan(&slice.Type, &slice.Count, &slice.Total); err != nil {
			return nil, fail(ctx, span, logger, "dashboard fraud types: scanning row", err)
		}

		result.TotalFlagged += slice.Count
		result.Types = append(result.Types, slice)
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard fraud types: iterating rows", err)
	}

	// Shares are computed here rather than in SQL: a window with no flagged
	// traffic has no denominator, and a window-wide percentage in SQL costs a
	// second aggregation over the same rows to produce the same number.
	if result.TotalFlagged > 0 {
		denominator := float64(result.TotalFlagged)
		for i := range result.Types {
			result.Types[i].Percentage = float64(result.Types[i].Count) / denominator
		}
	}

	return result, nil
}

// TopRules returns the busiest rules in the window.
func (r *DashboardRepository) TopRules(ctx context.Context, window model.DashboardWindow) (*model.DashboardTopRules, error) {
	ctx, span, logger := r.startSpan(ctx, "repository.dashboard.top_rules")
	defer span.End()

	db, err := r.conn.GetDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard top rules: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, topRulesQuery, windowPlanMode, window.From, window.To, model.DashboardTopRulesLimit)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard top rules: querying", err)
	}
	defer func() { _ = rows.Close() }()

	result := &model.DashboardTopRules{
		Rules:       make([]model.TopRule, 0, model.DashboardTopRulesLimit),
		WindowStart: window.From,
		WindowEnd:   window.To,
		UpdatedAt:   r.clock.Now().UTC(),
	}

	for rows.Next() {
		var (
			rule        model.TopRule
			productType sql.NullString
		)

		if err := rows.Scan(&rule.Name, &productType, &rule.Matches, &rule.Executions,
			&rule.DetectionRate, &rule.AvgProcessingMs); err != nil {
			return nil, fail(ctx, span, logger, "dashboard top rules: scanning row", err)
		}

		// A rule scoped to no transaction type applies to all of them, which
		// the wire renders as an absent productType rather than a made-up one.
		rule.ProductType = productType.String

		result.Rules = append(result.Rules, rule)
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard top rules: iterating rows", err)
	}

	return result, nil
}

// startSpan opens the repository span and returns the trace-enriched logger
// alongside it, so a failure is both recorded on the span and visible in the
// logs with its trace id attached.
func (r *DashboardRepository) startSpan(ctx context.Context, name string) (context.Context, trace.Span, libLog.Logger) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, name)

	return ctx, span, logging.WithTrace(ctx, logger)
}

// fail records an error on the span and in the log, then wraps it. Every
// failure path in this file goes through it, so none of them can report to one
// destination and not the other.
func fail(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error) error {
	wrapped := fmt.Errorf("%s: %w", message, err)

	libOtel.HandleSpanError(span, message, wrapped)

	if logger != nil {
		logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))
	}

	return wrapped
}
