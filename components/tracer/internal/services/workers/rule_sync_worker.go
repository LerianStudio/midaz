// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability"
	libLog "github.com/LerianStudio/lib-observability/log"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	libOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/google/uuid"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/logging"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/resilience"
)

// RuleSyncWorker periodically polls the database for rule changes
// and applies deltas to the in-memory cache with CEL recompilation.
// Implements libCommons.App interface for Launcher integration.
//
// In multi-tenant mode, tenantID scopes every poll + cache apply to a single
// tenant (the context is enriched with the tenantID at the top of runLoop
// and runSyncCycle, so downstream repositories and cache calls resolve the
// correct tenant bucket). In single-tenant mode tenantID is "" and behaviour
// is identical to the pre-multi-tenant worker.
type RuleSyncWorker struct {
	tenantID       string
	cache          RuleSyncCache
	repo           RuleSyncRepository
	compiler       ExpressionCompiler
	config         RuleSyncWorkerConfig
	logger         libLog.Logger
	clock          clock.Clock
	lastSync       time.Time
	circuitBreaker *resilience.CircuitBreaker
	// poolResolver is non-nil in multi-tenant mode. When set, each sync
	// cycle resolves the tenant-scoped Postgres pool via the resolver and
	// stashes it onto the cycle context via tmcore.ContextWithPG, so that
	// downstream repository getDB(ctx) calls land on the correct tenant DB.
	// In single-tenant mode (tenantID == "") this is nil and the cycle falls
	// back to the repository's static connection.
	poolResolver WorkerPoolResolver
}

// NewRuleSyncWorker creates a new rule sync worker.
// Returns ErrNilRuleCache if cache is nil.
// Returns ErrNilRepository if repo is nil.
// Returns ErrNilExpressionCompiler if compiler is nil.
// Returns ErrNilLogger if logger is nil.
// Returns ErrInvalidPollInterval if PollInterval <= 0.
// Returns ErrInvalidStalenessThreshold if StalenessThreshold <= 0.
// Returns ErrInvalidOverlapBuffer if OverlapBuffer < 0.
// Returns ErrNilCircuitBreaker if cb is nil.
// The clk parameter is optional; if nil, uses clock.RealClock{}.
func NewRuleSyncWorker(
	ruleCache RuleSyncCache,
	repo RuleSyncRepository,
	compiler ExpressionCompiler,
	config RuleSyncWorkerConfig,
	logger libLog.Logger,
	cb *resilience.CircuitBreaker,
	clk clock.Clock,
	tenantID string,
) (*RuleSyncWorker, error) {
	return NewRuleSyncWorkerWithPoolResolver(ruleCache, repo, compiler, config, logger, cb, clk, tenantID, nil)
}

// NewRuleSyncWorkerWithPoolResolver is the full constructor. In MT mode,
// callers pass a non-nil poolResolver so each sync cycle resolves the
// tenant-scoped Postgres pool and injects it onto the context; single-tenant
// callers can use NewRuleSyncWorker (poolResolver defaults to nil).
//
// Separate constructor instead of a setter keeps the worker immutable after
// construction — a fresh worker is either MT-aware or not, with no
// intermediate state to reason about.
func NewRuleSyncWorkerWithPoolResolver(
	ruleCache RuleSyncCache,
	repo RuleSyncRepository,
	compiler ExpressionCompiler,
	config RuleSyncWorkerConfig,
	logger libLog.Logger,
	cb *resilience.CircuitBreaker,
	clk clock.Clock,
	tenantID string,
	poolResolver WorkerPoolResolver,
) (*RuleSyncWorker, error) {
	if ruleCache == nil {
		return nil, ErrNilRuleCache
	}

	if repo == nil {
		return nil, ErrNilRepository
	}

	if compiler == nil {
		return nil, ErrNilExpressionCompiler
	}

	if logger == nil {
		return nil, ErrNilLogger
	}

	if config.PollInterval <= 0 {
		return nil, ErrInvalidPollInterval
	}

	if config.StalenessThreshold <= 0 {
		return nil, ErrInvalidStalenessThreshold
	}

	if config.OverlapBuffer < 0 {
		return nil, ErrInvalidOverlapBuffer
	}

	if cb == nil {
		return nil, ErrNilCircuitBreaker
	}

	if clk == nil {
		clk = clock.RealClock{}
	}

	return &RuleSyncWorker{
		tenantID:       tenantID,
		cache:          ruleCache,
		repo:           repo,
		compiler:       compiler,
		config:         config,
		logger:         logger,
		circuitBreaker: cb,
		clock:          clk,
		poolResolver:   poolResolver,
	}, nil
}

// Run implements the libCommons.App interface for Launcher integration.
// Handles OS signals (SIGINT, SIGTERM) for graceful shutdown.
func (w *RuleSyncWorker) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return w.runLoop(ctx)
}

// RunWithContext runs the worker with a provided context.
// Useful for testing or external orchestration.
func (w *RuleSyncWorker) RunWithContext(ctx context.Context) error {
	return w.runLoop(ctx)
}

// runLoop is the internal loop that handles sync cycles.
func (w *RuleSyncWorker) runLoop(ctx context.Context) error {
	if w.tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, w.tenantID)
	}

	// Initialize lastSync from cache's warm-up timestamp
	w.lastSync = w.cache.LastSyncTime(ctx)

	w.logger.With(
		libLog.String("operation", "worker.rule_sync.run"),
		libLog.String("poll_interval", w.config.PollInterval.String()),
		libLog.String("overlap_buffer", w.config.OverlapBuffer.String()),
		libLog.String("last_sync", w.lastSync.Format(time.RFC3339)),
	).Log(ctx, libLog.LevelInfo, "Starting rule sync worker")

	tickerChan, stopTicker := w.clock.NewTicker(w.config.PollInterval)
	defer stopTicker()

	for {
		select {
		case <-ctx.Done():
			w.logger.With(
				libLog.String("operation", "worker.rule_sync.run"),
			).Log(ctx, libLog.LevelInfo, "Rule sync worker stopped")

			return nil

		case <-tickerChan:
			w.runSyncCycle(ctx)
		}
	}
}

// injectTenantPool resolves the tenant's Postgres pool and stashes it onto
// the cycle context via tmcore.ContextWithPG, returning the new ctx and true.
// Returns (ctx, true) unchanged in single-tenant mode (empty tenantID or nil
// resolver) so the caller can keep using it with its existing repo path.
//
// Returns (_, false) when pool resolution errors out; the caller MUST skip
// this cycle rather than continue on the root default pool. Pool resolution
// is done PER-CYCLE (not per-spawn) because the pool Manager's LRU eviction
// can invalidate a reference captured once.
//
// On failure, the active span is marked as failed and the failure surfaces
// via the worker's standard skip-metrics path (label reason="pool_resolve").
// Without this, a broken tenant DB would look like the worker simply stopped
// changing and skip/error counters would never move.
func (w *RuleSyncWorker) injectTenantPool(
	ctx context.Context,
	logger libLog.Logger,
	span trace.Span,
	metricsFactory *libMetrics.MetricsFactory,
) (context.Context, bool) {
	if w.tenantID == "" || w.poolResolver == nil {
		return ctx, true
	}

	tenantDB, err := w.poolResolver.GetTenantDB(ctx, w.tenantID)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to resolve tenant pool", err)

		logger.With(
			libLog.String("operation", "worker.rule_sync.resolve_pool"),
			libLog.String("tenant_id", w.tenantID),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to resolve tenant pool; skipping sync cycle")

		w.emitSkipMetrics(ctx, metricsFactory, "error", "pool_resolve")

		return ctx, false
	}

	return tmcore.ContextWithPG(ctx, tenantDB), true
}

// runSyncCycle executes a single poll-classify-compile-apply cycle.
// Errors are logged but not returned — the worker continues running.
func (w *RuleSyncWorker) runSyncCycle(ctx context.Context) {
	_, tracer, _, metricsFactory := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "worker.rule_sync.sync_cycle")
	defer span.End()

	start := w.clock.Now()
	logger := logging.WithTrace(ctx, w.logger)

	// Multi-tenant: resolve the tenant's Postgres pool for this cycle and
	// stash it onto ctx. Extracted into a helper to keep runSyncCycle under
	// the package gocyclo budget. On failure the helper marks the span as
	// failed and emits skip metrics (reason="pool_resolve") so a broken
	// tenant DB shows up in dashboards rather than looking like a stalled
	// worker.
	injectedCtx, ok := w.injectTenantPool(ctx, logger, span, metricsFactory)
	if !ok {
		return
	}

	ctx = injectedCtx

	logger.With(
		libLog.String("operation", "worker.rule_sync.sync_cycle"),
		libLog.String("last_sync", w.lastSync.Format(time.RFC3339)),
	).Log(ctx, libLog.LevelDebug, "Running rule sync cycle")

	// 1. Query delta with overlap buffer (circuit breaker protected)
	since := w.lastSync.Add(-w.config.OverlapBuffer)

	fetched, err := w.queryDelta(ctx, since)
	if err != nil {
		// Error classification (circuit-open / context-cancelled / db-error) is
		// extracted to keep runSyncCycle under the package gocyclo budget. The
		// helper logs, marks the span, and emits skip metrics as appropriate.
		w.handleQueryDeltaError(ctx, span, logger, metricsFactory, err)

		return // lastSync NOT updated on error
	}

	// 2. If no results, touch cache staleness and update lastSync
	if len(fetched) == 0 {
		w.cache.ApplyChanges(ctx, nil, nil)
		// Mark the per-tenant bucket ready even on empty fetches. A brand-new
		// tenant with zero rules still needs the readiness gate to open so
		// /v1/validations can return ALLOW via the default-decision path
		// instead of TRC-0281. Idempotent.
		w.cache.MarkReady(ctx)
		w.lastSync = w.clock.Now()

		logger.With(
			libLog.String("operation", "worker.rule_sync.sync_cycle"),
		).Log(ctx, libLog.LevelDebug, "No rule changes detected")

		w.emitSuccessMetrics(ctx, metricsFactory, start, 0)

		return
	}

	// 3. Get current cache state for classification
	allCached := w.cache.GetActiveRules(ctx, nil)
	cachedMap := make(map[uuid.UUID]*cache.CachedRule, len(allCached))

	for _, r := range allCached {
		if r == nil || r.Rule == nil {
			continue
		}

		cachedMap[r.Rule.ID] = r
	}

	// 4. Classify changes
	changes := ClassifyChanges(cachedMap, fetched)

	if changes.IsEmpty() {
		w.cache.ApplyChanges(ctx, nil, nil)
		w.cache.MarkReady(ctx)
		w.updateLastSync(fetched)

		logger.With(
			libLog.String("operation", "worker.rule_sync.sync_cycle"),
			libLog.Int("fetched_count", len(fetched)),
		).Log(ctx, libLog.LevelDebug, "Overlap buffer: all changes already applied")

		w.emitSuccessMetrics(ctx, metricsFactory, start, 0)

		return
	}

	// 5. Compile CEL for new + updated rules
	toCompile := make([]*model.Rule, 0, len(changes.New)+len(changes.Updated))
	toCompile = append(toCompile, changes.New...)
	toCompile = append(toCompile, changes.Updated...)

	upserts := make([]*cache.CachedRule, 0, len(toCompile))

	var compileErrors int

	for _, rule := range toCompile {
		var program any

		compiled, compileErr := w.compiler.Compile(ctx, rule.Expression)
		if compileErr != nil {
			compileErrors++

			logger.With(
				libLog.String("operation", "worker.rule_sync.compile"),
				libLog.String("rule_id", rule.ID.String()),
				libLog.String("error.message", compileErr.Error()),
			).Log(ctx, libLog.LevelWarn, "CEL compilation failed for rule, using nil program")
		} else {
			program = compiled
		}

		upserts = append(upserts, &cache.CachedRule{
			Rule:    rule,
			Program: program,
		})
	}

	// 6. Apply changes to cache
	w.cache.ApplyChanges(ctx, upserts, changes.Deleted)
	// The per-tenant bucket is populated — flip the readiness gate so
	// subsequent /v1/validations calls pass IsReady. Idempotent; calling on
	// an already-ready bucket is a no-op.
	w.cache.MarkReady(ctx)

	// 7. Update lastSync
	w.updateLastSync(fetched)

	logger.With(
		libLog.String("operation", "worker.rule_sync.sync_cycle"),
		libLog.Int("new_count", len(changes.New)),
		libLog.Int("updated_count", len(changes.Updated)),
		libLog.Int("deleted_count", len(changes.Deleted)),
	).Log(ctx, libLog.LevelDebug, "Rule sync cycle completed")

	// 8. Emit metrics and span attributes
	changedCount := len(changes.New) + len(changes.Updated) + len(changes.Deleted)
	w.emitSuccessMetrics(ctx, metricsFactory, start, changedCount)

	if metricsFactory != nil && compileErrors > 0 {
		if counter, err := metricsFactory.Counter(MetricCacheSyncErrorsTotal); err == nil && counter != nil {
			_ = counter.WithLabels(map[string]string{"reason": "compile_error"}).
				Add(ctx, int64(compileErrors))
		}
	}

	span.SetAttributes(
		attribute.Int("app.response.new_count", len(changes.New)),
		attribute.Int("app.response.updated_count", len(changes.Updated)),
		attribute.Int("app.response.deleted_count", len(changes.Deleted)),
		attribute.Int("app.response.compile_errors", compileErrors),
		attribute.Int("app.response.cache_size", w.cache.Size(ctx)),
	)
}

// handleQueryDeltaError classifies a queryDelta failure and performs the
// matching side effects (logging, span marking, skip metrics). Extracted from
// runSyncCycle to keep it under the package gocyclo budget; the control flow
// and every side effect are identical to the inlined version.
func (w *RuleSyncWorker) handleQueryDeltaError(
	ctx context.Context,
	span trace.Span,
	logger libLog.Logger,
	metricsFactory *libMetrics.MetricsFactory,
	err error,
) {
	// Circuit breaker open or half-open rejecting: skip cycle, serve stale cache
	if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
		logger.With(
			libLog.String("operation", "worker.rule_sync.sync_cycle"),
			libLog.String("circuit_breaker.state", "open_or_half_open"),
		).Log(ctx, libLog.LevelWarn, "Circuit breaker rejecting request, skipping sync cycle - serving stale cache")

		w.emitSkipMetrics(ctx, metricsFactory, "skipped", "circuit_open")

		return
	}

	// Context cancellation: normal during shutdown, not a real failure
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		logger.With(
			libLog.String("operation", "worker.rule_sync.sync_cycle"),
		).Log(ctx, libLog.LevelDebug, "Sync cycle interrupted by context cancellation")

		return
	}

	libOtel.HandleSpanError(span, "Delta query failed", err)
	logger.With(
		libLog.String("operation", "worker.rule_sync.sync_cycle"),
		libLog.String("error.message", err.Error()),
	).Log(ctx, libLog.LevelError, "Failed to query rule changes")

	w.emitSkipMetrics(ctx, metricsFactory, "error", "db_error")
}

// emitSuccessMetrics records metrics for a successful sync cycle.
// Called on all success paths: no-results, overlap-only, and full sync.
func (w *RuleSyncWorker) emitSuccessMetrics(
	ctx context.Context,
	mf *libMetrics.MetricsFactory,
	start time.Time,
	rulesChanged int,
) {
	if mf == nil {
		return
	}

	if counter, err := mf.Counter(MetricCacheSyncPollsTotal); err == nil && counter != nil {
		_ = counter.WithLabels(map[string]string{"status": "success"}).AddOne(ctx)
	}

	elapsed := w.clock.Now().Sub(start).Milliseconds()

	if hist, err := mf.Histogram(MetricCacheSyncDuration); err == nil && hist != nil {
		_ = hist.Record(ctx, elapsed)
	}

	if rulesChanged > 0 {
		if counter, err := mf.Counter(MetricCacheSyncRulesChanged); err == nil && counter != nil {
			_ = counter.Add(ctx, int64(rulesChanged))
		}
	}

	if gauge, err := mf.Gauge(MetricCacheSyncRuleCacheSize); err == nil && gauge != nil {
		_ = gauge.Set(ctx, int64(w.cache.Size(ctx)))
	}

	if gauge, err := mf.Gauge(MetricCacheSyncStalenessSeconds); err == nil && gauge != nil {
		_ = gauge.Set(ctx, 0)
	}
}

// emitSkipMetrics records metrics when a sync cycle is skipped or fails.
// Called on circuit-breaker-open and DB-error paths.
func (w *RuleSyncWorker) emitSkipMetrics(
	ctx context.Context,
	mf *libMetrics.MetricsFactory,
	pollStatus string,
	errorReason string,
) {
	if mf == nil {
		return
	}

	if counter, err := mf.Counter(MetricCacheSyncPollsTotal); err == nil && counter != nil {
		_ = counter.WithLabels(map[string]string{"status": pollStatus}).AddOne(ctx)
	}

	if counter, err := mf.Counter(MetricCacheSyncErrorsTotal); err == nil && counter != nil {
		_ = counter.WithLabels(map[string]string{"reason": errorReason}).AddOne(ctx)
	}

	staleness := int64(w.clock.Now().Sub(w.lastSync).Seconds())

	if gauge, err := mf.Gauge(MetricCacheSyncStalenessSeconds); err == nil && gauge != nil {
		_ = gauge.Set(ctx, staleness)
	}
}

// updateLastSync sets lastSync to the maximum UpdatedAt from fetched results.
// If no UpdatedAt exceeds current lastSync (overlap-only), advances to clock.Now()
// to prevent stagnation.
//
// NOTE: The empty guard below is defense-in-depth. Currently unreachable because
// callers (runSyncCycle) only invoke updateLastSync when len(fetched) > 0.
// Kept as a safety net against future refactoring.
func (w *RuleSyncWorker) updateLastSync(fetched []*model.Rule) {
	if len(fetched) == 0 {
		w.lastSync = w.clock.Now()
		return
	}

	maxTime := w.lastSync

	for _, r := range fetched {
		if r == nil {
			continue
		}

		if r.UpdatedAt.After(maxTime) {
			maxTime = r.UpdatedAt
		}
	}

	// If no rule advanced past lastSync (overlap-only re-fetch),
	// advance to clock.Now() to break stagnation.
	if maxTime.Equal(w.lastSync) {
		w.lastSync = w.clock.Now()
		return
	}

	w.lastSync = maxTime
}

// queryDelta executes the delta query wrapped in the circuit breaker.
func (w *RuleSyncWorker) queryDelta(ctx context.Context, since time.Time) ([]*model.Rule, error) {
	result, err := w.circuitBreaker.Execute(ctx, func() (any, error) {
		return w.repo.GetRulesUpdatedSince(ctx, since)
	})
	if err != nil {
		return nil, err
	}

	// Use comma-ok pattern to safely handle nil result from Execute.
	// When repo returns (nil, nil), Execute returns (nil, nil) and
	// a bare type assertion on nil interface would panic.
	rules, ok := result.([]*model.Rule)
	if !ok {
		if result != nil {
			return nil, fmt.Errorf(
				"unexpected result type %T from circuit breaker Execute",
				result,
			)
		}

		return nil, nil
	}

	return rules, nil
}
