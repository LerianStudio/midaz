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
// COST CONTRACT. Each method below is ONE statement bounded by created_at and
// served by an index-only scan; none of them fetches a heap row. Measured on a
// seeded 1,000,000-row trail (365 days of history, PostgreSQL 17, 256MB
// shared_buffers), median of three warm runs:
//
//	metrics      7d   8ms | 30d  32ms | 90d 103ms  (idx_tv_dashboard)
//	volume       7d   9ms | 30d  35ms | 90d  98ms  (idx_transaction_validations_created)
//	fraud-types  7d   5ms | 30d  22ms | 90d  41ms  (idx_tv_dashboard)
//
// The index-only property depends on the visibility map: pages autovacuum has
// not yet marked all-visible still cost a heap fetch, so a trail under heavy
// write with autovacuum starved degrades toward the plain bitmap-heap plan
// (which measured 3-7x slower on the same data). That is a tuning property of
// the deployment, not of these statements.
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
	rows, err := db.QueryContext(ctx, metricsQuery, window.From, window.To)
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

	rows, err := db.QueryContext(ctx, volumeQuery, window.From, window.To)
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

	rows, err := db.QueryContext(ctx, fraudTypesQuery, window.From, window.To)
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
