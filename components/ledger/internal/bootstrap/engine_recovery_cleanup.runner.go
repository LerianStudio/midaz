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
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"go.opentelemetry.io/otel/attribute"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// engineRecoveryCleanupRepository is the Redis surface the cleanup runner uses:
// the cleanup schedule itself and the distributed lock that elects one pass.
type engineRecoveryCleanupRepository interface {
	CleanupEngineRecovery(context.Context, time.Time, int) (txRedis.RecoveryCleanupResult, error)
	EngineRecoveryCleanupBacklog(context.Context, time.Time) (txRedis.RecoveryCleanupBacklog, error)
	SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	DeleteIfValue(ctx context.Context, key, value string) (bool, error)
}

// tenantIDSource lists the tenants whose schedules a multi-tenant pass drains.
// Satisfied by *tenantcache.TenantCache.
type tenantIDSource interface {
	TenantIDs() []string
}

const (
	// engineRecoveryCleanupInterval bounds how long a due execution waits for
	// its cleanup after its retention deadline.
	engineRecoveryCleanupInterval = 10 * time.Second
	// engineRecoveryCleanupBudget is how long a pass keeps starting new pages,
	// shared by every tenant. The page in progress always completes.
	engineRecoveryCleanupBudget = 5 * time.Second
	// engineRecoveryCleanupPageSize is the schedule page cleaned per call.
	engineRecoveryCleanupPageSize = 100
	// engineRecoveryCleanupPagesPerRound is each tenant's turn before the pass
	// moves to the next tenant. Rounds repeat while budget remains, so one large
	// tenant cannot starve the others and a single tenant still uses the budget.
	engineRecoveryCleanupPagesPerRound = 10
	// engineRecoveryCleanupLockTTL is in whole seconds, as SetNX expects. It
	// outlives the budget plus the page in progress, and frees the lock if the
	// leader dies mid-pass.
	engineRecoveryCleanupLockTTL time.Duration = 15
)

// EngineRecoveryCleanupRunner releases the Redis artifacts of durable engine
// executions once their retention deadline passes. It runs a pass at startup
// and on every tick, independently of the recovery runner's cycle, and needs
// only Redis: no PostgreSQL connection is opened in either tenancy mode.
type EngineRecoveryCleanupRunner struct {
	logger     libLog.Logger
	repository engineRecoveryCleanupRepository
	clock      func() time.Time
	tenants    tenantIDSource
	interval   time.Duration
	budget     time.Duration
	newTicker  func(time.Duration) (<-chan time.Time, func())
	metrics    *metrics.MetricsFactory
	// rotation advances the first tenant of each multi-tenant pass.
	rotation int
}

// NewEngineRecoveryCleanupRunner builds a single-tenant runner over the
// recovery repository.
func NewEngineRecoveryCleanupRunner(logger libLog.Logger, repository engineRecoveryCleanupRepository) *EngineRecoveryCleanupRunner {
	return &EngineRecoveryCleanupRunner{
		logger:     logger,
		repository: repository,
		clock:      time.Now,
		interval:   engineRecoveryCleanupInterval,
		budget:     engineRecoveryCleanupBudget,
		newTicker: func(interval time.Duration) (<-chan time.Time, func()) {
			ticker := time.NewTicker(interval)
			return ticker.C, ticker.Stop
		},
	}
}

// initEngineRecoveryCleanupRunner builds the runner over the transaction Redis
// repository. Multi-tenant deployments drain every cached tenant's schedule. A
// repository without engine recovery cleanup yields no runner.
func initEngineRecoveryCleanupRunner(
	logger libLog.Logger,
	repository txRedis.RedisRepository,
	multiTenantEnabled bool,
	tenants *tenantcache.TenantCache,
	metricsFactory *metrics.MetricsFactory,
) *EngineRecoveryCleanupRunner {
	cleanup, ok := repository.(engineRecoveryCleanupRepository)
	if !ok {
		logger.Log(context.Background(), libLog.LevelWarn, "Engine recovery cleanup is unavailable for the configured Redis repository")
		return nil
	}

	runner := NewEngineRecoveryCleanupRunner(logger, cleanup).WithMetricsFactory(metricsFactory)

	// A nil cache stays out of the interface so the runner never iterates it.
	if multiTenantEnabled && tenants != nil {
		runner.WithTenants(tenants)
	}

	return runner
}

// WithTenants switches the runner to multi-tenant passes over the tenants the
// source lists. A nil source keeps single-tenant passes.
func (r *EngineRecoveryCleanupRunner) WithTenants(tenants tenantIDSource) *EngineRecoveryCleanupRunner {
	if tenants != nil {
		r.tenants = tenants
	}

	return r
}

// WithMetricsFactory sets the factory for the backlog gauges and the outcome
// counter. A nil factory disables them.
func (r *EngineRecoveryCleanupRunner) WithMetricsFactory(factory *metrics.MetricsFactory) *EngineRecoveryCleanupRunner {
	r.metrics = factory

	return r
}

// WithClock injects the clock that decides which executions are due and when
// a pass's budget ends. A nil clock leaves the existing clock unchanged.
func (r *EngineRecoveryCleanupRunner) WithClock(clock func() time.Time) *EngineRecoveryCleanupRunner {
	if clock != nil {
		r.clock = clock
	}

	return r
}

// Run executes a pass immediately and then one per tick until SIGTERM/SIGINT.
// The Launcher parameter is unused, as for the other Midaz workers.
func (r *EngineRecoveryCleanupRunner) Run(_ *libCommons.Launcher) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r.logger.Log(ctx, libLog.LevelInfo, "Engine recovery cleanup runner started",
		libLog.Bool("multi_tenant", r.tenants != nil),
		libLog.String("interval", r.interval.String()))

	r.run(ctx)

	r.logger.Log(ctx, libLog.LevelInfo, "Engine recovery cleanup runner shutting down")

	return nil
}

func (r *EngineRecoveryCleanupRunner) run(ctx context.Context) {
	ticks, stopTicks := r.newTicker(r.interval)
	defer stopTicks()

	for {
		r.pass(ctx)

		select {
		case <-ctx.Done():
			return
		case <-ticks:
		}
	}
}

// engineRecoveryCleanupScope is one tenant's schedule within a pass.
type engineRecoveryCleanupScope struct {
	tenantID string
	ctx      context.Context
	result   txRedis.RecoveryCleanupResult
	drained  bool
	err      error
}

func (s *engineRecoveryCleanupScope) finished() bool {
	return s.drained || s.err != nil
}

func (s *engineRecoveryCleanupScope) add(page txRedis.RecoveryCleanupResult) {
	s.result.Scanned += page.Scanned
	s.result.Cleaned += page.Cleaned
	s.result.Stale += page.Stale
	s.result.Rescheduled += page.Rescheduled
	s.result.Failed += page.Failed

	if s.result.FirstFailure == nil {
		s.result.FirstFailure = page.FirstFailure
	}
}

func (r *EngineRecoveryCleanupRunner) pass(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			r.logger.Log(ctx, libLog.LevelError, "Engine recovery cleanup pass panicked",
				libLog.String("panic", fmt.Sprint(recovered)))
		}
	}()

	if ctx.Err() != nil {
		return
	}

	release, acquired := r.acquireLock(ctx)
	if !acquired {
		return
	}

	defer release()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "engine_recovery.cleanup.pass")
	defer span.End()

	budgetEnds := r.clock().Add(r.budget)
	scopes := r.scopes(ctx)

	pending := scopes
	for len(pending) > 0 && ctx.Err() == nil && r.clock().Before(budgetEnds) {
		unfinished := make([]*engineRecoveryCleanupScope, 0, len(pending))

		for _, scope := range pending {
			r.drainRound(ctx, scope, budgetEnds)

			if !scope.finished() {
				unfinished = append(unfinished, scope)
			}
		}

		pending = unfinished
	}

	var cleaned, failed int

	for _, scope := range scopes {
		cleaned += scope.result.Cleaned
		failed += scope.result.Failed
		r.report(ctx, scope)
		r.observe(ctx, scope)
	}

	span.SetAttributes(
		attribute.Int("app.engine_recovery_cleanup.scopes", len(scopes)),
		attribute.Int("app.engine_recovery_cleanup.cleaned", cleaned),
		attribute.Int("app.engine_recovery_cleanup.failed", failed),
		attribute.Int("app.engine_recovery_cleanup.undrained", len(pending)),
	)
}

// drainRound cleans up to one round of pages for the scope, stopping early when
// the schedule is drained, the pass budget is spent, or a call fails.
func (r *EngineRecoveryCleanupRunner) drainRound(ctx context.Context, scope *engineRecoveryCleanupScope, budgetEnds time.Time) {
	for range engineRecoveryCleanupPagesPerRound {
		if ctx.Err() != nil {
			return
		}

		now := r.clock()
		if !now.Before(budgetEnds) {
			return
		}

		page, err := r.repository.CleanupEngineRecovery(scope.ctx, now, engineRecoveryCleanupPageSize)
		scope.add(page)

		if err != nil {
			scope.err = err
			return
		}

		if page.Scanned < engineRecoveryCleanupPageSize {
			scope.drained = true
			return
		}
	}
}

// scopes lists the schedules of this pass. Multi-tenant passes start one
// tenant later each time, over a stable order, so no tenant always goes last.
func (r *EngineRecoveryCleanupRunner) scopes(ctx context.Context) []*engineRecoveryCleanupScope {
	if r.tenants == nil {
		return []*engineRecoveryCleanupScope{{ctx: ctx}}
	}

	tenantIDs := r.tenants.TenantIDs()
	if len(tenantIDs) == 0 {
		return nil
	}

	sort.Strings(tenantIDs)

	start := r.rotation % len(tenantIDs)
	r.rotation++

	scopes := make([]*engineRecoveryCleanupScope, 0, len(tenantIDs))
	for offset := range tenantIDs {
		tenantID := tenantIDs[(start+offset)%len(tenantIDs)]
		scopes = append(scopes, &engineRecoveryCleanupScope{tenantID: tenantID, ctx: tmcore.ContextWithTenantID(ctx, tenantID)})
	}

	return scopes
}

// report logs the scope outcome once per pass: rejected proofs need operator
// attention, an interrupted scope is retried on the next tick.
func (r *EngineRecoveryCleanupRunner) report(ctx context.Context, scope *engineRecoveryCleanupScope) {
	var tenant []any
	if scope.tenantID != "" {
		tenant = []any{libLog.String("tenant_id", scope.tenantID)}
	}

	if scope.err != nil && !errors.Is(scope.err, context.Canceled) {
		r.logger.Log(ctx, libLog.LevelWarn, "Engine recovery cleanup stopped before draining the schedule",
			append(tenant, libLog.Err(scope.err))...)
	}

	if scope.result.Failed > 0 {
		r.logger.Log(ctx, libLog.LevelError, "Engine recovery cleanup rejected execution proofs",
			append(tenant, libLog.Int("failed_count", scope.result.Failed), libLog.Err(scope.result.FirstFailure))...)
	}

	if scope.result.Scanned > 0 {
		r.logger.Log(ctx, libLog.LevelDebug, "Engine recovery cleanup pass finished",
			append(tenant,
				libLog.Int("scanned_count", scope.result.Scanned),
				libLog.Int("cleaned_count", scope.result.Cleaned),
				libLog.Int("stale_count", scope.result.Stale),
				libLog.Int("rescheduled_count", scope.result.Rescheduled),
				libLog.Bool("drained", scope.drained))...)
	}
}

// observe emits the scope's outcome counts and the backlog its schedule still
// holds after the pass. Metrics are best-effort: a missing factory skips the
// backlog read, and a failed read or emit is logged at Debug only.
func (r *EngineRecoveryCleanupRunner) observe(ctx context.Context, scope *engineRecoveryCleanupScope) {
	if r.metrics == nil {
		return
	}

	outcomes := []struct {
		name  string
		count int
	}{
		{"cleaned", scope.result.Cleaned},
		{"stale", scope.result.Stale},
		{"rescheduled", scope.result.Rescheduled},
		{"failed", scope.result.Failed},
	}

	if counter, err := r.metrics.Counter(utils.EngineRecoveryCleanupEntries); err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to create engine recovery cleanup counter", libLog.Err(err))
	} else {
		for _, outcome := range outcomes {
			if outcome.count == 0 {
				continue
			}

			labels := map[string]string{"tenant_id": scope.tenantID, "outcome": outcome.name}
			if err := counter.WithLabels(labels).Add(ctx, int64(outcome.count)); err != nil {
				r.logger.Log(ctx, libLog.LevelDebug, "Failed to emit engine recovery cleanup counter", libLog.Err(err))
			}
		}
	}

	now := r.clock()

	backlog, err := r.repository.EngineRecoveryCleanupBacklog(scope.ctx, now)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to read engine recovery cleanup backlog", libLog.Err(err))
		return
	}

	overdue := int64(0)
	if backlog.Due > 0 && backlog.OldestDueMs > 0 {
		overdue = max(int64(now.Sub(time.UnixMilli(backlog.OldestDueMs)).Seconds()), 0)
	}

	labels := map[string]string{"tenant_id": scope.tenantID}
	r.setGauge(ctx, utils.EngineRecoveryCleanupDue, labels, backlog.Due)
	r.setGauge(ctx, utils.EngineRecoveryCleanupOldestOverdue, labels, overdue)
}

func (r *EngineRecoveryCleanupRunner) setGauge(ctx context.Context, metric metrics.Metric, labels map[string]string, value int64) {
	gauge, err := r.metrics.Gauge(metric)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to create engine recovery cleanup gauge", libLog.String("metric", metric.Name), libLog.Err(err))
		return
	}

	if err := gauge.WithLabels(labels).Set(ctx, value); err != nil {
		r.logger.Log(ctx, libLog.LevelDebug, "Failed to emit engine recovery cleanup gauge", libLog.String("metric", metric.Name), libLog.Err(err))
	}
}

// acquireLock elects one pass per tick across pods. Correctness does not depend
// on it: the cleanup script deletes only an execution whose scheduled score is
// unchanged, so concurrent cleaners at most repeat a no-op.
func (r *EngineRecoveryCleanupRunner) acquireLock(ctx context.Context) (func(), bool) {
	key := utils.EngineRecoveryCleanupLockKey()
	owner := podIdentifier()

	acquired, err := r.repository.SetNX(ctx, key, owner, engineRecoveryCleanupLockTTL)
	if err != nil {
		r.logger.Log(ctx, libLog.LevelWarn, "Failed to acquire engine recovery cleanup lock", libLog.Err(err))
		return nil, false
	}

	if !acquired {
		r.logger.Log(ctx, libLog.LevelDebug, "Another pod holds the engine recovery cleanup lock; skipping pass")
		return nil, false
	}

	release := func() {
		releaseCtx := context.WithoutCancel(ctx)
		if _, err := r.repository.DeleteIfValue(releaseCtx, key, owner); err != nil {
			r.logger.Log(releaseCtx, libLog.LevelWarn, "Failed to release engine recovery cleanup lock", libLog.Err(err))
		}
	}

	return release, true
}
