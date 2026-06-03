// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/log"
	libRuntime "github.com/LerianStudio/lib-observability/runtime"

	"tracer/internal/services/cache"
	"tracer/internal/services/metrics"
	"tracer/pkg/clock"
	"tracer/pkg/constant"
	"tracer/pkg/resilience"
)

// defaultMaxTenantWorkers mirrors the MULTI_TENANT_MAX_TENANT_POOLS default.
// It acts as a defensive ceiling on in-flight tenant worker sets; the PostgreSQL
// pool manager (tmpostgres.Manager) already applies an LRU eviction policy on
// DB connections, but the supervisor has no back-pressure on its own, so it
// stops spawning workers once this cap is reached.
const defaultMaxTenantWorkers = 100

// TenantLister captures the subset of the Tenant Manager client needed by the
// supervisor for initial tenant discovery. Keeping the surface area small
// (instead of pulling in the whole tmclient.Client) makes the supervisor easy
// to unit test with an in-process fake.
type TenantLister interface {
	GetActiveTenantsByService(ctx context.Context, service string) ([]*tmclient.TenantSummary, error)
}

// CircuitBreakerTemplate is a factory-style description of the circuit breaker
// configuration to apply to each tenant's sync worker. The supervisor clones
// this template once per tenant, qualifying the name so that failures in one
// tenant never trip another tenant's breaker.
type CircuitBreakerTemplate struct {
	NamePrefix    string
	MaxRequests   uint32
	Interval      time.Duration
	Timeout       time.Duration
	FailureThresh uint32
	FailureRatio  float64
	MinRequests   uint32
}

func (t CircuitBreakerTemplate) forTenant(tenantID string) resilience.CircuitBreakerConfig {
	return resilience.CircuitBreakerConfig{
		Name:          fmt.Sprintf("%s:%s", t.NamePrefix, tenantID),
		MaxRequests:   t.MaxRequests,
		Interval:      t.Interval,
		Timeout:       t.Timeout,
		FailureThresh: t.FailureThresh,
		FailureRatio:  t.FailureRatio,
		MinRequests:   t.MinRequests,
	}
}

// WorkerSupervisorDeps bundles the dependencies the supervisor needs. Using a
// struct instead of a long constructor arg list keeps the bootstrap call site
// readable and leaves room to add new fields without churning every test.
type WorkerSupervisorDeps struct {
	RuleCache     *cache.RuleCache
	SyncRepo      RuleSyncRepository
	UsageRepo     UsageCounterCleanupRepository
	Compiler      ExpressionCompiler
	SyncConfig    RuleSyncWorkerConfig
	CleanupConfig UsageCleanupWorkerConfig
	// CleanupWorkerEnabled gates whether per-tenant cleanup workers spawn at
	// all. False ⇒ only the sync worker runs per tenant. Mirrors the
	// single-tenant CLEANUP_WORKER_ENABLED knob so operators get consistent
	// behavior in both modes (H8).
	CleanupWorkerEnabled bool
	CBTemplate           CircuitBreakerTemplate
	TenantList           TenantLister
	Service              string // tenant-manager service name; defaults to "tracer"
	Clock                clock.Clock
	MaxTenants           int
	Logger               libLog.Logger
	// Metrics emits the canonical multi-tenant metrics (tenant_connections_*
	// and tenant_consumers_active). Optional: when nil, the supervisor uses a
	// no-op, so all existing call sites keep working unchanged.
	Metrics metrics.MultiTenantMetrics
	// PoolResolver is handed to every per-tenant worker so each cycle can
	// resolve the tenant-scoped Postgres pool and inject it onto the cycle
	// context via tmcore.ContextWithPG. REQUIRED in MT mode — without it,
	// per-tenant workers poll the root DB. Leave nil for unit tests that do
	// not need the injection (the workers gracefully skip the injection
	// step when this is nil, preserving the pre-fix behavior for the test).
	PoolResolver WorkerPoolResolver
}

// WorkerSupervisor manages per-tenant background workers (rule sync + usage
// cleanup) in multi-tenant mode. One instance of each worker type is spawned
// per active tenant, with their lifecycle tied to tenant lifecycle events.
//
// Not used in single-tenant mode — when MULTI_TENANT_ENABLED=false, workers
// run as singletons via the Launcher directly (unchanged from the pre-MT
// behaviour).
type WorkerSupervisor struct {
	// workers is the tenantID → *tenantWorkerSet map. Read-heavy happy path
	// (EnsureWorkers is called from middleware on every request) benefits from
	// sync.Map's lock-free Load. The slow path — spawning a new tenant's
	// workers — is serialised by spawnMu so the "load-miss, then insert" race
	// between concurrent new-tenant requests degenerates to an idempotent
	// second-check under the spawn lock.
	workers sync.Map
	// spawnMu guards the spawn + Store transition plus the cap check. Held
	// only on the cold path; never on Load.
	spawnMu sync.Mutex
	// activeCount mirrors len(workers) for the cap check without requiring a
	// lock. Written only under spawnMu (Inc) or in StopWorkers after a
	// successful LoadAndDelete (Dec), both of which are serialised per
	// tenantID.
	activeCount atomic.Int64

	// Immutable after construction — safe to read without any synchronisation.
	ruleCache            *cache.RuleCache
	syncRepo             RuleSyncRepository
	usageRepo            UsageCounterCleanupRepository
	compiler             ExpressionCompiler
	syncConfig           RuleSyncWorkerConfig
	cleanupConfig        UsageCleanupWorkerConfig
	cleanupWorkerEnabled bool
	cbTemplate           CircuitBreakerTemplate
	tenantList           TenantLister
	service              string
	clock                clock.Clock
	maxTenants           int
	logger               libLog.Logger
	metrics              metrics.MultiTenantMetrics
	poolResolver         WorkerPoolResolver

	// shutdownCh is closed by Shutdown to unblock a Run loop (if one was
	// registered with the Launcher). Using a channel + sync.Once lets us
	// safely call Shutdown multiple times (the Launcher and a test cleanup
	// might both fire it) without panicking on a double-close.
	shutdownCh   chan struct{}
	shutdownOnce sync.Once

	// shuttingDown is set to true the moment Shutdown begins so EnsureWorkers
	// can fail-fast instead of racing to spawn new tenant workers that would
	// miss the snapshot drain. Read-mostly: written once by Shutdown under
	// spawnMu, read by EnsureWorkers under spawnMu.
	shuttingDown atomic.Bool
}

// ErrSupervisorShutdown is returned by EnsureWorkers when Shutdown has begun.
// Callers (HTTP middleware, tenant event handlers) can match it via errors.Is
// to surface a clean 503 / drop the event without retrying.
//
// Aliased to constant.ErrSupervisorShuttingDown (TRC-0334) so the wire-level
// error carries a stable TRC code while existing callers keep using the
// package-local symbol.
var ErrSupervisorShutdown = constant.ErrSupervisorShuttingDown

// tenantWorkerSet holds the running workers for a single tenant plus the
// cancel function that tears them down. done is closed after both worker
// goroutines have returned — StopWorkers waits on it for deterministic
// shutdown, so callers can rely on "StopWorkers returned" meaning "workers
// exited".
type tenantWorkerSet struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// NewWorkerSupervisor constructs a supervisor.
// Returns an error if any required dependency is missing; the tenant lister
// is optional (InitialTenantSync becomes a no-op when nil).
//
// UsageRepo is only required when CleanupWorkerEnabled is true. Operators
// running with the cleanup worker disabled (CLEANUP_WORKER_ENABLED=false)
// do not need to pass a fake/no-op usage repository — the constructor skips
// the nil-check on that dependency, and the cleanup goroutine never spawns,
// so a nil repo is never dereferenced. This mirrors the single-tenant
// behavior where the cleanup worker simply isn't constructed when disabled.
//
// Constructor failures are TRC-coded via the sentinels in pkg/constant/errors
// so callers (bootstrap, tests) can errors.Is them without grepping strings.
func NewWorkerSupervisor(deps WorkerSupervisorDeps) (*WorkerSupervisor, error) {
	if deps.RuleCache == nil {
		return nil, constant.ErrSupervisorNilRuleCache
	}

	if deps.SyncRepo == nil {
		return nil, constant.ErrSupervisorNilSyncRepo
	}

	// UsageRepo is only consumed by the per-tenant cleanup worker; skipping
	// the nil-check when CleanupWorkerEnabled=false lets operators omit the
	// dependency entirely instead of fabricating a no-op double.
	if deps.CleanupWorkerEnabled && deps.UsageRepo == nil {
		return nil, constant.ErrSupervisorNilUsageRepo
	}

	if deps.Compiler == nil {
		return nil, constant.ErrSupervisorNilCompiler
	}

	if deps.Logger == nil {
		return nil, constant.ErrSupervisorNilLogger
	}

	if deps.Clock == nil {
		deps.Clock = clock.RealClock{}
	}

	if deps.MaxTenants <= 0 {
		deps.MaxTenants = defaultMaxTenantWorkers
	}

	if deps.Service == "" {
		deps.Service = "tracer"
	}

	if deps.Metrics == nil {
		// No-op metrics backend: keeps the supervisor's call sites unconditional
		// even when the operator hasn't wired a real Prometheus meter. Callers
		// that want real metrics must pass a non-nil Metrics in Deps.
		deps.Logger.Log(context.Background(), libLog.LevelDebug,
			"Metrics backend nil — using no-op implementation")
		deps.Metrics = metrics.NewMultiTenantMetrics(false, nil, nil)
	}

	return &WorkerSupervisor{
		ruleCache:            deps.RuleCache,
		syncRepo:             deps.SyncRepo,
		usageRepo:            deps.UsageRepo,
		compiler:             deps.Compiler,
		syncConfig:           deps.SyncConfig,
		cleanupConfig:        deps.CleanupConfig,
		cleanupWorkerEnabled: deps.CleanupWorkerEnabled,
		cbTemplate:           deps.CBTemplate,
		tenantList:           deps.TenantList,
		service:              deps.Service,
		clock:                deps.Clock,
		maxTenants:           deps.MaxTenants,
		logger:               deps.Logger,
		metrics:              deps.Metrics,
		poolResolver:         deps.PoolResolver,
		shutdownCh:           make(chan struct{}),
	}, nil
}

// Run satisfies the libCommons.App contract so the supervisor can be registered
// with the Launcher in multi-tenant mode. It blocks until Shutdown is called
// (via Launcher-propagated OS signals or direct invocation), then tears down
// every per-tenant worker set and returns.
//
// Returning nil is consistent with the HTTPServer.Run / worker.Run siblings —
// the Launcher treats a nil error as a clean exit.
func (s *WorkerSupervisor) Run(_ *libCommons.Launcher) error {
	<-s.shutdownCh

	s.Shutdown()

	return nil
}

// EnsureWorkers spawns the two workers (sync + cleanup) for a tenant if they
// are not already running. Idempotent — safe to call on every request. Rejects
// empty tenantIDs because those would silently collide with the single-tenant
// bucket in the rule cache.
//
// The ctx argument is used only for logging and for the underlying tenant
// discovery flow; worker goroutines intentionally run on a fresh
// context.Background() so they survive the request/event that triggered them.
//
//nolint:contextcheck // tenant worker lifecycle is deliberately decoupled from ctx.
func (s *WorkerSupervisor) EnsureWorkers(ctx context.Context, tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("supervisor: tenantID is required")
	}

	// M17: Defense-in-depth. tmmiddleware.WithTenantDB already enforces the
	// alphanumeric+`-_` whitelist via tmcore.IsValidTenantID before any tenant
	// ID reaches this point. The supervisor is also invoked from event
	// handlers (OnTenantAdded) and InitialTenantSync — sources that come from
	// the Tenant Manager HTTP API, which SHOULD be trusted but can be
	// misconfigured. Re-validating here guarantees that no arbitrary string
	// (log-injection payload, path traversal) can reach spans/metrics/logs
	// via the supervisor regardless of upstream integrity.
	if !tmcore.IsValidTenantID(tenantID) {
		return fmt.Errorf("supervisor: tenant ID %q failed validation (must match %s)", tenantID, "[a-zA-Z0-9][a-zA-Z0-9_-]*")
	}

	// Fast path: already present. Lock-free Load keeps the middleware hook
	// cheap under steady-state load (every request re-asserts EnsureWorkers).
	if _, ok := s.workers.Load(tenantID); ok {
		return nil
	}

	// Slow path: serialise the "check, spawn, insert" transition. spawnMu also
	// guards activeCount writes so the cap check cannot race with another
	// goroutine that is mid-spawn.
	s.spawnMu.Lock()
	defer s.spawnMu.Unlock()

	// Block new tenant spawns once Shutdown begins. Without this, a request
	// or tenant event racing with termination can store a tenantWorkerSet
	// AFTER Shutdown's snapshot was taken, leaking goroutines and DB pools
	// past process exit. spawnMu coordinates this check with Shutdown's
	// snapshot — see Shutdown.
	if s.shuttingDown.Load() {
		return ErrSupervisorShutdown
	}

	// Double-check after acquiring the spawn lock: another goroutine may have
	// won the race to spawn this tenant between our Load and Lock.
	if _, ok := s.workers.Load(tenantID); ok {
		return nil
	}

	if s.activeCount.Load() >= int64(s.maxTenants) {
		active := int(s.activeCount.Load())
		s.logger.With(
			libLog.String("operation", "supervisor.ensure_workers"),
			libLog.String("tenant_id", tenantID),
			libLog.Int("active_tenants", active),
			libLog.Int("max_tenants", s.maxTenants),
		).Log(ctx, libLog.LevelWarn, "Tenant worker cap reached, declining to spawn workers")

		s.metrics.IncConnectionErrors(ctx, tenantID, constant.ModuleName, "tenant_cap_reached")

		// Wrap the sentinel so the HTTP middleware (M18) can match via
		// errors.Is while still preserving the cap value in the message.
		return fmt.Errorf("%w: cap=%d, active=%d", ErrTenantCapReached, s.maxTenants, active)
	}

	// Per-tenant circuit breaker — template cloned + tenant-qualified so one
	// tenant's failures cannot trip another tenant's breaker. The state-change
	// callback inside NewCircuitBreaker logs with context.Background(); that is
	// by design (the breaker outlives any single request), so the contextcheck
	// warning here is a false positive.
	//
	//nolint:contextcheck // breaker lifecycle is intentionally context-less.
	cb := resilience.NewCircuitBreaker(s.cbTemplate.forTenant(tenantID), s.logger)

	syncWorker, err := NewRuleSyncWorkerWithPoolResolver(
		s.ruleCache, s.syncRepo, s.compiler, s.syncConfig, s.logger, cb, s.clock, tenantID, s.poolResolver,
	)
	if err != nil {
		s.metrics.IncConnectionErrors(ctx, tenantID, constant.ModuleName, "sync_worker_init")

		return fmt.Errorf("supervisor: new sync worker for tenant %q: %w", tenantID, err)
	}

	// H8: cleanup worker is optional. CLEANUP_WORKER_ENABLED=false propagates
	// here as cleanupWorkerEnabled=false, in which case only the sync worker
	// runs. The cleanup worker mirrors the single-tenant semantics of the
	// same flag.
	var cleanupWorker *UsageCleanupWorker

	if s.cleanupWorkerEnabled {
		cleanupWorker, err = NewUsageCleanupWorkerWithPoolResolver(
			s.usageRepo, s.cleanupConfig, s.logger, s.clock, tenantID, s.poolResolver,
		)
		if err != nil {
			s.metrics.IncConnectionErrors(ctx, tenantID, constant.ModuleName, "cleanup_worker_init")

			return fmt.Errorf("supervisor: new cleanup worker for tenant %q: %w", tenantID, err)
		}
	}

	tenantCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	set := &tenantWorkerSet{cancel: cancel, done: done}
	s.workers.Store(tenantID, set)
	s.activeCount.Add(1)
	active := int(s.activeCount.Load())

	// C2: every per-tenant goroutine runs through
	// runtime.SafeGoWithContextAndComponent so a panic is captured by
	// lib-commons' panic-observability trident (recover + structured log
	// + panic_recovered_total counter + span event) instead of tearing
	// down the process. Without this, a rogue rule expression could crash
	// every tenant on the pod. Component label "workers.tenant-supervisor"
	// scopes panic counters in dashboards by subsystem.
	const supervisorComponent = "workers.tenant-supervisor"

	libRuntime.SafeGoWithContextAndComponent(
		tenantCtx,
		s.logger,
		supervisorComponent,
		"tenant-worker-set:"+tenantID,
		libRuntime.KeepRunning,
		func(ctx context.Context) {
			defer close(done)

			var wg sync.WaitGroup

			wg.Add(1)

			libRuntime.SafeGoWithContextAndComponent(
				ctx,
				s.logger,
				supervisorComponent,
				"sync-worker:"+tenantID,
				libRuntime.KeepRunning,
				func(innerCtx context.Context) {
					defer wg.Done()

					_ = syncWorker.RunWithContext(innerCtx)
				},
			)

			if cleanupWorker != nil {
				wg.Add(1)

				libRuntime.SafeGoWithContextAndComponent(
					ctx,
					s.logger,
					supervisorComponent,
					"cleanup-worker:"+tenantID,
					libRuntime.KeepRunning,
					func(innerCtx context.Context) {
						defer wg.Done()

						_ = cleanupWorker.RunWithContext(innerCtx)
					},
				)
			}

			wg.Wait()
		},
	)

	s.logger.With(
		libLog.String("operation", "supervisor.ensure_workers"),
		libLog.String("tenant_id", tenantID),
		libLog.Int("active_tenants", active),
	).Log(ctx, libLog.LevelInfo, "Spawned per-tenant workers")

	// Emit both canonical metrics INSIDE the spawn-lock critical section so
	// the gauge value is tied to the visible map transition. Ordering matters:
	// tenant_connections_total is a cumulative counter (monotonic) while
	// tenant_consumers_active is a gauge that must match the live worker set
	// count. Paired with the Dec in StopWorkers' LoadAndDelete block.
	s.metrics.IncConnectionsTotal(ctx, tenantID, constant.ModuleName)
	s.metrics.IncConsumersActive(ctx, tenantID, constant.ModuleName)

	return nil
}

// StopWorkers cancels a tenant's workers, waits for them to exit, and evicts
// the tenant's cache entry. Safe to call with a tenantID that was never
// spawned — in that case it is a no-op.
//
// LoadAndDelete is atomic: only one caller (per tenantID) observes ok==true,
// so the activeCount decrement and the consumers_active decrement fire
// exactly once per successful stop even under concurrent invocation.
func (s *WorkerSupervisor) StopWorkers(tenantID string) {
	v, ok := s.workers.LoadAndDelete(tenantID)
	if !ok {
		return
	}

	set := v.(*tenantWorkerSet)
	set.cancel()
	<-set.done

	s.ruleCache.EvictTenant(tenantID)

	// Decrement the gauge only after the worker goroutines have actually
	// exited — dashboards that drive "active consumers" alerts rely on the
	// invariant that the gauge value matches the number of live worker sets.
	s.activeCount.Add(-1)
	s.metrics.DecConsumersActive(context.Background(), tenantID, constant.ModuleName)

	s.logger.With(
		libLog.String("operation", "supervisor.stop_workers"),
		libLog.String("tenant_id", tenantID),
	).Log(context.Background(), libLog.LevelInfo, "Stopped per-tenant workers and evicted cache")
}

// InitialTenantSync lists all active tenants from the Tenant Manager and
// pre-warms workers for each. Called once at boot. Mirrors reporter's
// performInitialTenantSync pattern — individual spawn failures are logged
// but do not abort the sweep. The first spawn error is returned so the
// caller can surface it; the supervisor stays usable either way.
func (s *WorkerSupervisor) InitialTenantSync(ctx context.Context) error {
	if s.tenantList == nil {
		s.logger.With(
			libLog.String("operation", "supervisor.initial_sync"),
		).Log(ctx, libLog.LevelWarn, "InitialTenantSync skipped: tenant lister is not configured")

		return nil
	}

	tenants, err := s.tenantList.GetActiveTenantsByService(ctx, s.service)
	if err != nil {
		s.logger.With(
			libLog.String("operation", "supervisor.initial_sync"),
			libLog.String("error.message", err.Error()),
		).Log(ctx, libLog.LevelError, "Failed to list active tenants")

		return fmt.Errorf("supervisor: list tenants: %w", err)
	}

	var firstErr error

	for _, t := range tenants {
		if t == nil || t.ID == "" {
			continue
		}

		if spawnErr := s.EnsureWorkers(ctx, t.ID); spawnErr != nil {
			s.logger.With(
				libLog.String("operation", "supervisor.initial_sync"),
				libLog.String("tenant_id", t.ID),
				libLog.String("error.message", spawnErr.Error()),
			).Log(ctx, libLog.LevelError, "Failed to pre-warm workers for tenant")

			if firstErr == nil {
				firstErr = spawnErr
			}
		}
	}

	s.logger.With(
		libLog.String("operation", "supervisor.initial_sync"),
		libLog.Int("tenants_listed", len(tenants)),
	).Log(ctx, libLog.LevelInfo, "Initial tenant worker sync complete")

	return firstErr
}

// Shutdown stops all tenant workers. Called on service termination.
// Blocks until every tenant's worker goroutines have exited.
//
// Idempotent — calling Shutdown multiple times is safe. The first call
// closes shutdownCh (which unblocks Run) and drains the worker map;
// subsequent calls observe an empty map and return immediately.
func (s *WorkerSupervisor) Shutdown() {
	// Mark shutdown BEFORE snapshotting the workers map so any EnsureWorkers
	// caller still inside the spawn-lock critical section observes the flag
	// and bails out with ErrSupervisorShutdown. We hold spawnMu across both
	// the flag flip and the snapshot to make the ordering atomic from the
	// supervisor's perspective: any spawn that completes BEFORE this lock
	// section is visible in the snapshot; any spawn racing to acquire spawnMu
	// AFTER will see shuttingDown=true and return without storing.
	s.spawnMu.Lock()
	s.shuttingDown.Store(true)

	s.shutdownOnce.Do(func() {
		close(s.shutdownCh)
	})

	ids := make([]string, 0, s.activeCount.Load())
	s.workers.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	s.spawnMu.Unlock()

	for _, id := range ids {
		s.StopWorkers(id)
	}
}
