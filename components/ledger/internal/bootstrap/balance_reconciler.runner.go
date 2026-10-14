// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libRedis "github.com/LerianStudio/lib-commons/v7/commons/redis"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	libRuntime "github.com/LerianStudio/lib-observability/v4/runtime"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

const (
	defaultBalanceReconcilerIntervalMs   = 900000
	defaultBalanceReconcilerGraceMs      = 5400000
	defaultBalanceReconcilerPageSize     = 500
	defaultBalanceReconcilerPassBudgetMs = 60000

	// balanceReconcilerPageTimeout mirrors the statement timeout of one page read:
	// the page in progress when the budget ends still runs this long.
	balanceReconcilerPageTimeout = 30 * time.Second
	// balanceReconcilerLockMargin covers tenant resolution and clock skew between pods.
	balanceReconcilerLockMargin = 15 * time.Second

	// balanceReconcilerMaxDivergenceWarns caps the per-balance divergence Warn logs
	// of one tenant scan; the rest are counted and reported once at publication.
	balanceReconcilerMaxDivergenceWarns = 100

	// Failure reasons of the pass-failure counter.
	balanceReconcilerReasonPGResolve = "pg_resolve"
	balanceReconcilerReasonQuery     = "query"
	balanceReconcilerReasonLock      = "lock"
	balanceReconcilerReasonPanic     = "panic"
)

// balanceReconcilerDivergentDirections are the directions published as divergence
// series. Every completed scan writes all three, zero included, so a recovered
// divergence clears its alert.
var balanceReconcilerDivergentDirections = []mmodel.BalanceTrailDirection{
	mmodel.BalanceTrailBalanceAhead,
	mmodel.BalanceTrailTrailAhead,
	mmodel.BalanceTrailValueMismatch,
}

// BalanceReconcilerConfig is the effective reconciler configuration, in milliseconds
// and balances per page.
type BalanceReconcilerConfig struct {
	IntervalMs   int
	GraceMs      int
	PageSize     int
	PassBudgetMs int
}

// balanceReconcilerScan is one tenant's table scan in progress. A scan can span
// several passes when the budget ends first; its cursor and tallies stay in memory
// until the table ends, and a restart only delays the next publication.
type balanceReconcilerScan struct {
	cursor    uuid.UUID
	startedAt time.Time
	pages     int
	counts    map[mmodel.BalanceTrailDirection]int64
	// warned and suppressed split the divergences of this scan between logged
	// Warns and those held back by the cap.
	warned     int
	suppressed int
}

// BalanceReconcilerRunner compares, per balance, the live balance row with the head
// of its operation trail. Each pass pages through every tenant's balance table
// under a shared budget, classifies each pair, and publishes the divergence gauges
// of a tenant only once its whole table has been scanned. It only reads.
type BalanceReconcilerRunner struct {
	logger     libLog.Logger
	useCase    *query.UseCase
	lease      reconcilerLease
	pgResolver tenantPGResolver
	tenants    tenantIDSource
	metrics    *metrics.MetricsFactory
	clock      func() time.Time
	newTicker  func(time.Duration) (<-chan time.Time, func())
	cfg        BalanceReconcilerConfig
	// rotation advances the first tenant of each multi-tenant pass.
	rotation int
	// scans holds each tenant's scan in progress, keyed by tenant id ("" in single-tenant).
	scans map[string]*balanceReconcilerScan
}

// NewBalanceReconcilerRunner builds a single-tenant runner. Any non-positive value
// in cfg falls back to its default with a Warn log, never a startup failure.
func NewBalanceReconcilerRunner(logger libLog.Logger, useCase *query.UseCase, lease reconcilerLease, cfg BalanceReconcilerConfig) *BalanceReconcilerRunner {
	cfg = clampBalanceReconcilerConfig(logger, cfg)

	return &BalanceReconcilerRunner{
		logger:  logger,
		useCase: useCase,
		lease:   lease,
		clock:   time.Now,
		newTicker: func(interval time.Duration) (<-chan time.Time, func()) {
			ticker := time.NewTicker(interval)
			return ticker.C, ticker.Stop
		},
		cfg:   cfg,
		scans: map[string]*balanceReconcilerScan{},
	}
}

// clampBalanceReconcilerConfig replaces every non-positive value with its default.
func clampBalanceReconcilerConfig(logger libLog.Logger, cfg BalanceReconcilerConfig) BalanceReconcilerConfig {
	cfg.IntervalMs = clampBalanceReconcilerKnob(logger, "BALANCE_RECONCILER_INTERVAL_MS", cfg.IntervalMs, defaultBalanceReconcilerIntervalMs)
	cfg.GraceMs = clampBalanceReconcilerKnob(logger, "BALANCE_RECONCILER_GRACE_MS", cfg.GraceMs, defaultBalanceReconcilerGraceMs)
	cfg.PageSize = clampBalanceReconcilerKnob(logger, "BALANCE_RECONCILER_PAGE_SIZE", cfg.PageSize, defaultBalanceReconcilerPageSize)
	cfg.PassBudgetMs = clampBalanceReconcilerKnob(logger, "BALANCE_RECONCILER_PASS_BUDGET_MS", cfg.PassBudgetMs, defaultBalanceReconcilerPassBudgetMs)

	return cfg
}

func clampBalanceReconcilerKnob(logger libLog.Logger, env string, value, fallback int) int {
	if value > 0 {
		return value
	}

	logger.Log(context.Background(), libLog.LevelWarn, "Invalid balance reconciler setting; using default",
		libLog.String("env", env), libLog.Int("value", value), libLog.Int("default", fallback))

	return fallback
}

// initBalanceReconcilerRunner builds the reconciler from the ledger configuration.
// It yields no runner when the reconciler is disabled, when no lock manager can be
// built over the Redis client, or when a multi-tenant deployment lacks the tenant
// cache or the per-tenant PostgreSQL manager: scanning the default pool there would
// reconcile the wrong database.
func initBalanceReconcilerRunner(
	cfg *Config,
	logger libLog.Logger,
	queryUC *query.UseCase,
	redisClient *libRedis.Client,
	multiTenantEnabled bool,
	tenants *tenantcache.TenantCache,
	pgManager *tmpostgres.Manager,
	metricsFactory *metrics.MetricsFactory,
) *BalanceReconcilerRunner {
	ctx := context.Background()

	if !cfg.BalanceReconcilerEnabled {
		logger.Log(ctx, libLog.LevelInfo, "Balance reconciler disabled")
		return nil
	}

	if multiTenantEnabled && (tenants == nil || pgManager == nil) {
		logger.Log(ctx, libLog.LevelWarn, "Balance reconciler is unavailable: multi-tenant mode without tenant cache or PostgreSQL manager")
		return nil
	}

	lockManager, err := libRedis.NewRedisLockManager(redisClient)
	if err != nil {
		logger.Log(ctx, libLog.LevelWarn, "Balance reconciler is unavailable: failed to build the Redis lock manager", libLog.Err(err))
		return nil
	}

	reconcilerCfg := clampBalanceReconcilerConfig(logger, BalanceReconcilerConfig{
		IntervalMs:   cfg.BalanceReconcilerIntervalMs,
		GraceMs:      cfg.BalanceReconcilerGraceMs,
		PageSize:     cfg.BalanceReconcilerPageSize,
		PassBudgetMs: cfg.BalanceReconcilerPassBudgetMs,
	})
	lease := newRedisReconcilerLease(lockManager, logger, balanceReconcilerLeaseExpiry(reconcilerCfg))

	runner := NewBalanceReconcilerRunner(logger, queryUC, lease, reconcilerCfg).WithMetricsFactory(metricsFactory)

	// Typed nils stay out of the interfaces so a single-tenant runner never iterates them.
	if multiTenantEnabled {
		runner.WithTenants(tenants, pgManager)
	}

	logger.Log(ctx, libLog.LevelInfo, "Balance reconciler configured",
		libLog.Bool("multi_tenant", runner.tenants != nil),
		libLog.Int("interval_ms", runner.cfg.IntervalMs),
		libLog.Int("grace_ms", runner.cfg.GraceMs),
		libLog.Int("page_size", runner.cfg.PageSize),
		libLog.Int("pass_budget_ms", runner.cfg.PassBudgetMs),
		libLog.Any("lease_expiry_ms", balanceReconcilerLeaseExpiry(runner.cfg).Milliseconds()))

	return runner
}

// WithTenants switches the runner to multi-tenant passes over the tenants the
// source lists, each read through its own PostgreSQL handle. A nil source or
// resolver keeps single-tenant passes.
func (r *BalanceReconcilerRunner) WithTenants(tenants tenantIDSource, pgResolver tenantPGResolver) *BalanceReconcilerRunner {
	if tenants != nil && pgResolver != nil {
		r.tenants = tenants
		r.pgResolver = pgResolver
	}

	return r
}

// WithMetricsFactory sets the factory for the reconciler gauges and the failure
// counter. A nil factory disables them.
func (r *BalanceReconcilerRunner) WithMetricsFactory(factory *metrics.MetricsFactory) *BalanceReconcilerRunner {
	r.metrics = factory

	return r
}

// WithClock injects the clock that drives the grace window, the pass budget and
// the scan timestamps. A nil clock leaves the existing clock unchanged.
func (r *BalanceReconcilerRunner) WithClock(clock func() time.Time) *BalanceReconcilerRunner {
	if clock != nil {
		r.clock = clock
	}

	return r
}

// Run executes a pass immediately and then one per tick until SIGTERM/SIGINT.
// The Launcher parameter is unused, as for the other Midaz workers.
func (r *BalanceReconcilerRunner) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r.logger.Log(ctx, libLog.LevelInfo, "Balance reconciler runner started",
		libLog.Bool("multi_tenant", r.tenants != nil),
		libLog.Int("interval_ms", r.cfg.IntervalMs))

	r.run(ctx)

	r.logger.Log(ctx, libLog.LevelInfo, "Balance reconciler runner shutting down")

	return nil
}

// run executes passes until ctx ends, then gives leadership up once. Leadership
// is kept between passes so one pod advances every scan cursor.
func (r *BalanceReconcilerRunner) run(ctx context.Context) {
	ticks, stopTicks := r.newTicker(time.Duration(r.cfg.IntervalMs) * time.Millisecond)
	defer stopTicks()
	defer r.lease.Release(ctx)

	for {
		r.pass(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// balanceReconcilerScope is one tenant's balance table within a pass.
type balanceReconcilerScope struct {
	tenantID string
	ctx      context.Context
}

// pass runs one reconciler pass. A panic is recovered, logged and recorded by the
// runtime package, and counted as a pass failure for the tenant being scanned.
func (r *BalanceReconcilerRunner) pass(ctx context.Context) {
	current := ""
	finished := false

	// Registered before the recovery so it runs after it: a pass that did not
	// finish was cut short by a panic.
	defer func() {
		if !finished {
			r.countFailure(ctx, current, balanceReconcilerReasonPanic)
		}
	}()
	defer libRuntime.RecoverWithPolicyAndContext(ctx, r.logger, "ledger", "balance_reconciler.pass", libRuntime.KeepRunning)

	r.runPass(ctx, &current)

	finished = true
}

// runPass keeps leadership, then scans each tenant under the shared budget.
// current tracks the tenant being scanned for the panic counter.
func (r *BalanceReconcilerRunner) runPass(ctx context.Context, current *string) {
	if ctx.Err() != nil {
		return
	}

	held, fresh, err := r.lease.Hold(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return
		}

		r.logger.Log(ctx, libLog.LevelWarn, "Failed to hold balance reconciler lease", libLog.Err(err))
		r.countFailure(ctx, "", balanceReconcilerReasonLock)

		return
	}

	if !held {
		r.logger.Log(ctx, libLog.LevelDebug, "Another pod leads the balance reconciler; skipping pass")
		return
	}

	if fresh {
		// Another pod may have led meanwhile: cursors from an earlier tenure are stale.
		r.scans = map[string]*balanceReconcilerScan{}
	}

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "balance_reconciler.pass")
	defer span.End()

	budgetEnds := r.clock().Add(time.Duration(r.cfg.PassBudgetMs) * time.Millisecond)
	scopes := r.scopes(ctx)

	listed := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		listed[scope.tenantID] = struct{}{}
	}

	completed := 0

	for _, scope := range scopes {
		if ctx.Err() != nil {
			return
		}

		if !r.clock().Before(budgetEnds) {
			break
		}

		*current = scope.tenantID

		if r.scan(ctx, span, scope, budgetEnds) {
			completed++
		}
	}

	if ctx.Err() != nil {
		return
	}

	// A tenant gone from the cache leaves no publication behind: its series expire.
	for tenantID := range r.scans {
		if _, ok := listed[tenantID]; !ok {
			delete(r.scans, tenantID)
		}
	}

	span.SetAttributes(
		attribute.Int("app.balance_reconciler.scopes", len(scopes)),
		attribute.Int("app.balance_reconciler.completed", completed),
	)
}

// scan pages through one tenant's balance table until it ends, the budget is
// spent, or a step fails. It reports whether the table ended in this pass. The
// page in progress always completes; a failure keeps the cursor for the next pass.
func (r *BalanceReconcilerRunner) scan(ctx context.Context, span oteltrace.Span, scope balanceReconcilerScope, budgetEnds time.Time) bool {
	scopeCtx := scope.ctx

	if r.pgResolver != nil {
		resolved, err := r.resolveTenantPG(scope.ctx, scope.tenantID)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}

			libOpentelemetry.HandleSpanError(span, "Failed to resolve tenant PostgreSQL for balance reconciler", err)
			r.logger.Log(ctx, libLog.LevelWarn, "Balance reconciler failed to resolve tenant PostgreSQL",
				libLog.String("tenant_id", scope.tenantID), libLog.Err(err))
			r.countFailure(ctx, scope.tenantID, balanceReconcilerReasonPGResolve)

			return false
		}

		scopeCtx = resolved
	}

	state, ok := r.scans[scope.tenantID]
	if !ok {
		state = &balanceReconcilerScan{
			startedAt: r.clock(),
			counts:    map[mmodel.BalanceTrailDirection]int64{},
		}
		r.scans[scope.tenantID] = state
	}

	grace := time.Duration(r.cfg.GraceMs) * time.Millisecond

	for r.clock().Before(budgetEnds) {
		if ctx.Err() != nil {
			return false
		}

		pairs, next, err := r.useCase.ReconcileBalancesPage(scopeCtx, state.cursor, r.cfg.PageSize)
		if err != nil {
			if ctx.Err() != nil {
				return false
			}

			libOpentelemetry.HandleSpanError(span, "Failed to read balance reconciler page", err)
			r.logger.Log(ctx, libLog.LevelWarn, "Balance reconciler failed to read a page",
				libLog.String("tenant_id", scope.tenantID), libLog.Err(err))
			r.countFailure(ctx, scope.tenantID, balanceReconcilerReasonQuery)

			return false
		}

		for _, pair := range pairs {
			direction := query.ClassifyBalanceTrailPair(pair, r.clock(), grace)
			state.counts[direction]++

			if !balanceReconcilerIsDivergence(direction) {
				continue
			}

			if state.warned >= balanceReconcilerMaxDivergenceWarns {
				state.suppressed++
				continue
			}

			state.warned++

			r.reportDivergence(ctx, scope.tenantID, pair, direction)
		}

		state.pages++
		state.cursor = next

		if len(pairs) < r.cfg.PageSize {
			if ctx.Err() != nil {
				return false
			}

			r.publish(ctx, scope.tenantID, state)
			delete(r.scans, scope.tenantID)

			return true
		}
	}

	r.logger.Log(ctx, libLog.LevelDebug, "Balance reconciler budget spent before the table ended",
		libLog.String("tenant_id", scope.tenantID), libLog.Int("pages", state.pages))

	return false
}

// balanceReconcilerIsDivergence reports whether direction is a divergence: neither
// a match, a balance inside the grace window, nor one that cannot be compared.
func balanceReconcilerIsDivergence(direction mmodel.BalanceTrailDirection) bool {
	switch direction {
	case mmodel.BalanceTrailOK, mmodel.BalanceTrailDeferred, mmodel.BalanceTrailUnverifiable:
		return false
	default:
		return true
	}
}

// reportDivergence logs one divergent balance with its identifiers and both
// versions. Monetary values never reach the log.
func (r *BalanceReconcilerRunner) reportDivergence(ctx context.Context, tenantID string, pair mmodel.BalanceTrailPair, direction mmodel.BalanceTrailDirection) {
	trailVersion := int64(-1)
	if pair.Trail != nil {
		trailVersion = pair.Trail.Version
	}

	r.logger.Log(ctx, libLog.LevelWarn, "Balance diverges from its operation trail",
		libLog.String("tenant_id", tenantID),
		libLog.String("balance_id", pair.BalanceID.String()),
		libLog.String("organization_id", pair.OrganizationID.String()),
		libLog.String("ledger_id", pair.LedgerID.String()),
		libLog.String("direction", string(direction)),
		libLog.Any("balance_version", pair.Version),
		libLog.Any("trail_version", trailVersion))
}

// publish writes a completed scan's gauges and logs its summary.
func (r *BalanceReconcilerRunner) publish(ctx context.Context, tenantID string, state *balanceReconcilerScan) {
	now := r.clock()
	durationMs := now.Sub(state.startedAt).Milliseconds()

	if r.metrics != nil {
		for _, direction := range balanceReconcilerDivergentDirections {
			r.setGauge(ctx, utils.BalanceReconcilerDivergences,
				map[string]string{"tenant_id": tenantID, "direction": string(direction)}, state.counts[direction])
		}

		labels := map[string]string{"tenant_id": tenantID}
		r.setGauge(ctx, utils.BalanceReconcilerDeferred, labels, state.counts[mmodel.BalanceTrailDeferred])
		r.setGauge(ctx, utils.BalanceReconcilerUnverifiable, labels, state.counts[mmodel.BalanceTrailUnverifiable])
		r.setGauge(ctx, utils.BalanceReconcilerLastScanTimestamp, labels, now.Unix())
		r.setGauge(ctx, utils.BalanceReconcilerLastScanDuration, labels, durationMs)
	}

	if state.suppressed > 0 {
		r.logger.Log(ctx, libLog.LevelWarn, "Balance reconciler suppressed divergence logs beyond the per-scan cap",
			libLog.String("tenant_id", tenantID),
			libLog.Int("logged", state.warned),
			libLog.Int("suppressed", state.suppressed))
	}

	r.logger.Log(ctx, libLog.LevelInfo, "Balance reconciler scan completed",
		libLog.String("tenant_id", tenantID),
		libLog.Int("pages", state.pages),
		libLog.Any("duration_ms", durationMs),
		libLog.Any("ok_count", state.counts[mmodel.BalanceTrailOK]),
		libLog.Any("deferred_count", state.counts[mmodel.BalanceTrailDeferred]),
		libLog.Any("unverifiable_count", state.counts[mmodel.BalanceTrailUnverifiable]),
		libLog.Any("balance_ahead_count", state.counts[mmodel.BalanceTrailBalanceAhead]),
		libLog.Any("trail_ahead_count", state.counts[mmodel.BalanceTrailTrailAhead]),
		libLog.Any("value_mismatch_count", state.counts[mmodel.BalanceTrailValueMismatch]))
}

// scopes lists the balance tables of this pass. Multi-tenant passes start one
// tenant later each time, over a stable order, so no tenant always goes last.
func (r *BalanceReconcilerRunner) scopes(ctx context.Context) []balanceReconcilerScope {
	if r.tenants == nil {
		return []balanceReconcilerScope{{ctx: ctx}}
	}

	tenantIDs := r.tenants.TenantIDs()
	if len(tenantIDs) == 0 {
		return nil
	}

	sort.Strings(tenantIDs)

	start := r.rotation % len(tenantIDs)
	r.rotation++

	scopes := make([]balanceReconcilerScope, 0, len(tenantIDs))
	for offset := range tenantIDs {
		tenantID := tenantIDs[(start+offset)%len(tenantIDs)]
		scopes = append(scopes, balanceReconcilerScope{tenantID: tenantID, ctx: tmcore.ContextWithTenantID(ctx, tenantID)})
	}

	return scopes
}

// resolveTenantPG returns ctx carrying a freshly resolved transaction-module PG
// handle for tenantID. Resolution is per pass: the tenant manager may close or
// swap a tenant's pool at any time, so a handle captured once would dangle.
func (r *BalanceReconcilerRunner) resolveTenantPG(ctx context.Context, tenantID string) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	db, err := r.pgResolver.GetDB(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("resolve tenant postgres: %w", err)
	}

	if db == nil {
		return nil, errors.New("resolve tenant postgres: nil handle")
	}

	return tmcore.ContextWithPG(ctx, db, constant.ModuleTransaction), nil
}

// countFailure adds one to the pass-failure counter. Best-effort: a nil factory
// or a failed emit never affects the pass.
func (r *BalanceReconcilerRunner) countFailure(ctx context.Context, tenantID, reason string) {
	if r.metrics == nil {
		return
	}

	counter, err := r.metrics.Counter(utils.BalanceReconcilerPassFailures)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to create balance reconciler failure counter", libLog.Err(err))
		return
	}

	if err := counter.WithLabels(map[string]string{"tenant_id": tenantID, "reason": reason}).AddOne(ctx); err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to emit balance reconciler failure counter", libLog.Err(err))
	}
}

func (r *BalanceReconcilerRunner) setGauge(ctx context.Context, metric metrics.Metric, labels map[string]string, value int64) {
	gauge, err := r.metrics.Gauge(metric)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to create balance reconciler gauge", libLog.String("metric", metric.Name), libLog.Err(err))
		return
	}

	if err := gauge.WithLabels(labels).Set(ctx, value); err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to emit balance reconciler gauge", libLog.String("metric", metric.Name), libLog.Err(err))
	}
}
