// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package dashboard

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/dashboard"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

// Compile-time interface implementation check.
var _ Repository = (*DashboardPostgreSQLRepository)(nil)

// The three statements below are written out rather than assembled with
// squirrel, which the repository standard otherwise asks for. Squirrel has no
// vocabulary for GROUPING SETS, for a generate_series LEFT JOIN, or for the
// per-row scale division; expressing them through it would mean handing it raw
// SQL fragments anyway, with the placeholder numbering split across Go and
// text. Placeholders are still positional $N and nothing is concatenated.

// windowPlanMode forces every dashboard read to be planned against the
// parameters it was actually handed.
//
// pgx's default exec mode caches a SERVER-SIDE prepared statement per
// connection; PostgreSQL plans the first five executions against the real
// parameters and then, under the default plan_cache_mode=auto, switches to a
// GENERIC plan built with no knowledge of them. These reads are exactly the
// shape that punishes: a generic plan cannot know that one ledger holds most
// of a table while its neighbour holds a hundred rows, nor that a 90-day
// window covers a quarter of the history rather than the fraction a default
// selectivity estimate assumes. The measured consequence on the tracer's own
// dashboard was 95ms becoming 323ms on the sixth execution of a pooled
// connection, with nothing in the code changing to explain it.
//
// The mode is applied to Assets too, which carries no window. Its parameters
// are the org and ledger ids, whose selectivity against the balance table
// varies by exactly the same orders of magnitude, and one rule that holds for
// every statement in this file is easier to keep true than an exception that
// has to be re-measured whenever a query changes shape.
const windowPlanMode = pgx.QueryExecModeExec

// metricsQuery aggregates the window THREE ways in ONE pass: over every row
// (the grouping set `()`, which yields the headline total), once per status,
// and once per asset. One scan means the total, the status breakdown and the
// per-asset volume cannot disagree with each other — three statements would be
// three instants, and this endpoint publishes all three side by side and
// caches them together for a minute.
//
// The status breakdown comes from a GROUPING SET rather than from a row of
// COUNT(*) FILTER (WHERE status = '...') columns, so no status literal appears
// in this statement at all: a status added to the ledger's lifecycle shows up
// here with no edit, and a status in the data that the ledger's own list does
// not know is reported rather than silently folded into the total.
//
// The settled set DOES appear, as a parameter, because "which statuses moved
// money" is a product decision rather than something the data can be asked.
// constant.SettledTransactionStatuses is its single definition.
//
// Row order is fixed so the scan can read the grand total before anything else
// and never has to look ahead: total first, then the status rows, then the
// asset rows.
const metricsQuery = `
	SELECT
		GROUPING(asset_code) AS no_asset,
		GROUPING(status)     AS no_status,
		asset_code,
		status,
		COUNT(*) AS transactions,
		COUNT(*) FILTER (WHERE status = ANY($5)) AS settled_transactions,
		COALESCE(SUM(amount) FILTER (WHERE status = ANY($5)), 0) AS settled_amount
	FROM "transaction"
	WHERE organization_id = $1
	  AND ledger_id = $2
	  AND created_at >= $3
	  AND created_at < $4
	  AND deleted_at IS NULL
	GROUP BY GROUPING SETS ((), (status), (asset_code))
	ORDER BY no_asset DESC, no_status DESC, asset_code ASC, status ASC`

// volumeQuery buckets the window by UTC calendar day and gap-fills it.
//
// The generate_series LEFT JOIN is the whole point: a plain GROUP BY omits a
// day with no transactions, and an absent day is not the same statement as a
// day with none. Every renderer downstream would have to invent the missing
// days itself, and the first one that forgot would draw a chart whose bars
// slide silently onto the wrong dates.
//
// The series ends on the date of the LAST INSTANT the window contains, not on
// the date of its exclusive upper bound. A window is half-open, so one ending
// at exactly midnight does not touch the day that midnight opens, and
// generating up to $4's date would publish a trailing day that is empty by
// construction rather than because nothing happened.
//
// Per day the statement emits one total row (any status, any asset) and one
// row per asset carrying that day's settled volume, from the same grouping-set
// discipline as metricsQuery. A day with no rows survives the join as the
// total row alone, with the aggregate columns NULL, which COALESCE turns into
// an honest zero.
const volumeQuery = `
	WITH windowed AS (
		SELECT (created_at AT TIME ZONE 'UTC')::date AS bucket, asset_code, status, amount
		FROM "transaction"
		WHERE organization_id = $1
		  AND ledger_id = $2
		  AND created_at >= $3
		  AND created_at < $4
		  AND deleted_at IS NULL
	),
	agg AS (
		SELECT
			bucket,
			GROUPING(asset_code) AS no_asset,
			asset_code,
			COUNT(*) AS transactions,
			COUNT(*) FILTER (WHERE status = ANY($5)) AS settled_transactions,
			COALESCE(SUM(amount) FILTER (WHERE status = ANY($5)), 0) AS settled_amount
		FROM windowed
		GROUP BY GROUPING SETS ((bucket), (bucket, asset_code))
	),
	days AS (
		SELECT generate_series(
			($3::timestamptz AT TIME ZONE 'UTC')::date,
			(($4::timestamptz - INTERVAL '1 microsecond') AT TIME ZONE 'UTC')::date,
			INTERVAL '1 day'
		)::date AS bucket
	)
	SELECT
		d.bucket,
		COALESCE(a.no_asset, 1) AS no_asset,
		a.asset_code,
		COALESCE(a.transactions, 0),
		COALESCE(a.settled_transactions, 0),
		COALESCE(a.settled_amount, 0)
	FROM days d
	LEFT JOIN agg a ON a.bucket = d.bucket
	ORDER BY d.bucket ASC, no_asset DESC, a.asset_code ASC`

// assetsQuery reads the ledger's CURRENT position per asset off the balance
// table. It carries no created_at predicate because it aggregates no history:
// balance holds one running total per (account, asset, balance key), so its
// cost is the ledger's account count and stays flat as the transaction table
// grows.
//
// There is no scale arithmetic here, and that is a schema fact worth stating
// because the obvious assumption is the opposite one. balance ORIGINALLY stored
// an integer value beside a per-row scale (migration 000003), which would have
// made a plain SUM add centavos to satoshis; migration 000005 converted
// available and on_hold to DECIMAL — folding each row's scale into its own
// value — and DROPPED the scale column. Summing DECIMALs of differing precision
// is exact in numeric, so the per-row division that schema would have demanded
// is not merely unnecessary now, it would not compile. Should a scale column
// ever return, the division must come back WITH it and must happen per row,
// before the sum.
//
// Accounts counts DISTINCT account_id rather than rows: one account may hold
// several balance rows for one asset, one per balance key (migration 000010,
// unique on account+asset+key in 000032), and counting rows would report more
// accounts than the ledger has.
//
// overdraft_used (migration 000031) is deliberately not folded into either
// figure. It is credit consumed, not a position held, and adding it to
// available would report money the ledger does not have.
const assetsQuery = `
	SELECT
		asset_code,
		COUNT(DISTINCT account_id) AS accounts,
		COALESCE(SUM(available), 0) AS available,
		COALESCE(SUM(on_hold), 0) AS on_hold
	FROM balance
	WHERE organization_id = $1
	  AND ledger_id = $2
	  AND deleted_at IS NULL
	GROUP BY asset_code
	ORDER BY asset_code ASC`

// DashboardPostgreSQLRepository answers the dashboard reads out of the
// tenant's own database.
type DashboardPostgreSQLRepository struct {
	connection    *libPostgres.Client
	requireTenant bool
	now           func() time.Time
}

// NewDashboardPostgreSQLRepository returns a repository over the given
// connection. requireTenant refuses a read that arrives without a tenant
// database in context, matching the transaction repository's posture.
func NewDashboardPostgreSQLRepository(pc *libPostgres.Client, requireTenant ...bool) *DashboardPostgreSQLRepository {
	r := &DashboardPostgreSQLRepository{connection: pc, now: func() time.Time { return time.Now().UTC() }}
	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	return r
}

// Metrics returns the headline dashboard panel for the window.
func (r *DashboardPostgreSQLRepository) Metrics(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardMetrics, error) {
	ctx, span, logger := r.startSpan(ctx, "postgres.dashboard.metrics", organizationID, ledgerID)
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, metricsQuery, windowPlanMode,
		organizationID, ledgerID, window.From, window.To, pq.Array(constant.SettledTransactionStatuses))
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics: querying", err)
	}
	defer func() { _ = rows.Close() }()

	metrics := &mmodel.DashboardMetrics{
		ByStatus:      newStatusCounts(),
		VolumeByAsset: make([]mmodel.DashboardAssetVolume, 0),
		WindowStart:   window.From,
		WindowEnd:     window.To,
		UpdatedAt:     r.now(),
	}

	for rows.Next() {
		var row metricsRow
		if err := rows.Scan(&row.noAsset, &row.noStatus, &row.asset, &row.status,
			&row.transactions, &row.settledTransactions, &row.settledAmount); err != nil {
			return nil, fail(ctx, span, logger, "dashboard metrics: scanning row", err)
		}

		row.applyTo(metrics)
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard metrics: iterating rows", err)
	}

	return metrics, nil
}

// metricsRow is one row of the grouping-set result. Which of the three sets it
// belongs to is read off the two GROUPING flags rather than off which columns
// happen to be NULL — asset_code and status are both NOT NULL in the schema,
// but reading a NULL as "this is the total row" would silently turn a schema
// change that relaxed either column into a corrupted headline.
type metricsRow struct {
	noAsset             int
	noStatus            int
	asset               sql.NullString
	status              sql.NullString
	transactions        int64
	settledTransactions int64
	settledAmount       decimal.Decimal
}

// applyTo folds one row into the metrics entity.
func (row metricsRow) applyTo(metrics *mmodel.DashboardMetrics) {
	switch {
	case row.noAsset == 1 && row.noStatus == 1:
		metrics.Total = row.transactions

	case row.noAsset == 1 && row.noStatus == 0:
		metrics.ByStatus[row.status.String] = row.transactions

	case row.noAsset == 0 && row.noStatus == 1:
		// An asset that settled nothing carried no volume, so it is left out
		// rather than published as a zero the reader has to dismiss. The count
		// is the test, not the amount: a settled transaction of zero is still a
		// transaction, and dropping its asset would lose it from the breakdown
		// while Total still counted it.
		if row.settledTransactions == 0 {
			return
		}

		metrics.VolumeByAsset = append(metrics.VolumeByAsset, mmodel.DashboardAssetVolume{
			Asset:        row.asset.String,
			Amount:       row.settledAmount,
			Transactions: row.settledTransactions,
		})
	}
}

// Volume returns the gap-filled per-day series across the window.
func (r *DashboardPostgreSQLRepository) Volume(ctx context.Context, organizationID, ledgerID uuid.UUID, window dashboard.Window) (*mmodel.DashboardVolume, error) {
	ctx, span, logger := r.startSpan(ctx, "postgres.dashboard.volume", organizationID, ledgerID)
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, volumeQuery, windowPlanMode,
		organizationID, ledgerID, window.From, window.To, pq.Array(constant.SettledTransactionStatuses))
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: querying", err)
	}
	defer func() { _ = rows.Close() }()

	volume := &mmodel.DashboardVolume{
		Points:      make([]mmodel.DashboardVolumePoint, 0),
		WindowStart: window.From,
		WindowEnd:   window.To,
		UpdatedAt:   r.now(),
	}

	for rows.Next() {
		var (
			bucket              time.Time
			noAsset             int
			asset               sql.NullString
			transactions        int64
			settledTransactions int64
			settledAmount       decimal.Decimal
		)

		if err := rows.Scan(&bucket, &noAsset, &asset, &transactions, &settledTransactions, &settledAmount); err != nil {
			return nil, fail(ctx, span, logger, "dashboard volume: scanning row", err)
		}

		date := bucket.Format(time.DateOnly)

		// The statement orders by day, and within a day emits the total row
		// before its asset rows, so the point a row belongs to is always the
		// one most recently appended.
		if noAsset == 1 {
			volume.Points = append(volume.Points, mmodel.DashboardVolumePoint{
				Date:         date,
				Transactions: transactions,
				ByAsset:      make([]mmodel.DashboardAssetVolume, 0),
			})

			continue
		}

		if len(volume.Points) == 0 || volume.Points[len(volume.Points)-1].Date != date {
			return nil, fail(ctx, span, logger, "dashboard volume: row order",
				fmt.Errorf("asset row for %s arrived before its day total", date))
		}

		if settledTransactions == 0 {
			continue
		}

		point := &volume.Points[len(volume.Points)-1]
		point.ByAsset = append(point.ByAsset, mmodel.DashboardAssetVolume{
			Asset:        asset.String,
			Amount:       settledAmount,
			Transactions: settledTransactions,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard volume: iterating rows", err)
	}

	return volume, nil
}

// Assets returns the ledger's current position per asset.
func (r *DashboardPostgreSQLRepository) Assets(ctx context.Context, organizationID, ledgerID uuid.UUID) (*mmodel.DashboardAssets, error) {
	ctx, span, logger := r.startSpan(ctx, "postgres.dashboard.assets", organizationID, ledgerID)
	defer span.End()

	db, err := r.getDB(ctx)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard assets: get database connection", err)
	}

	rows, err := db.QueryContext(ctx, assetsQuery, windowPlanMode, organizationID, ledgerID)
	if err != nil {
		return nil, fail(ctx, span, logger, "dashboard assets: querying", err)
	}
	defer func() { _ = rows.Close() }()

	assets := &mmodel.DashboardAssets{
		Assets:    make([]mmodel.DashboardAssetPosition, 0),
		UpdatedAt: r.now(),
	}

	for rows.Next() {
		var position mmodel.DashboardAssetPosition

		if err := rows.Scan(&position.Asset, &position.Accounts, &position.Available, &position.OnHold); err != nil {
			return nil, fail(ctx, span, logger, "dashboard assets: scanning row", err)
		}

		assets.Assets = append(assets.Assets, position)
	}

	if err := rows.Err(); err != nil {
		return nil, fail(ctx, span, logger, "dashboard assets: iterating rows", err)
	}

	return assets, nil
}

// newStatusCounts seeds the breakdown with every status the ledger knows, at
// zero. A status the window contains overwrites its entry; one it does not
// keeps the zero, which is the difference between "none happened" and "this
// ledger has no such state".
func newStatusCounts() map[string]int64 {
	counts := make(map[string]int64, len(constant.TransactionStatuses))
	for _, status := range constant.TransactionStatuses {
		counts[status] = 0
	}

	return counts
}

// getDB resolves the PostgreSQL connection for the current request. In
// multi-tenant mode the middleware injects a tenant-specific resolver into
// context; in single-tenant mode this falls back to the static connection.
func (r *DashboardPostgreSQLRepository) getDB(ctx context.Context) (dbresolver.DB, error) {
	if db := tmcore.GetPGContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	if db := tmcore.GetPGContext(ctx); db != nil {
		return db, nil
	}

	if r.requireTenant {
		return nil, fmt.Errorf("tenant postgres connection missing from context")
	}

	if r.connection == nil {
		return nil, fmt.Errorf("postgres connection not available")
	}

	return r.connection.Resolver(ctx)
}

// startSpan opens the repository span with the request scope on it and returns
// the logger alongside.
func (r *DashboardPostgreSQLRepository) startSpan(ctx context.Context, name string, organizationID, ledgerID uuid.UUID) (context.Context, trace.Span, libLog.Logger) {
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, name)
	span.SetAttributes(
		attribute.String("app.request.organization_id", organizationID.String()),
		attribute.String("app.request.ledger_id", ledgerID.String()),
	)

	return ctx, span, logger
}

// fail records an error on the span and in the log, then wraps it. Every
// failure path in this file goes through it, so none of them can report to one
// destination and not the other.
func fail(ctx context.Context, span trace.Span, logger libLog.Logger, message string, err error) error {
	wrapped := fmt.Errorf("%s: %w", message, err)

	libOpentelemetry.HandleSpanError(span, message, wrapped)

	if logger != nil {
		logger.Log(ctx, libLog.LevelError, message, libLog.Err(err))
	}

	return wrapped
}
